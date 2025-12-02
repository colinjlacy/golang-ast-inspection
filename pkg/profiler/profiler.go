//go:build linux

package profiler

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"golang.org/x/sys/unix"
)

const (
	dirSend = 0
	dirRecv = 1
)

// Event mirrors the struct emitted from the BPF program. Keep layout/alignment identical.
type Event struct {
	Ts        uint64
	Pid       uint32
	Tid       uint32
	DataLen   uint32
	Sport     uint16
	Dport     uint16
	Family    uint16
	Direction uint8
	// _         [1]byte // padding to align to 8 bytes for the following arrays
	Comm  [16]byte
	Saddr [16]byte
	Daddr [16]byte
	Data  [256]byte
}

type Parsed struct {
	Method     string
	URL        string
	Body       string
	StatusCode string
	Headers    map[string]string
}

type ContainerInfo struct {
	Service       string `json:"service,omitempty"`
	Image         string `json:"image,omitempty"`
	ContainerID   string `json:"container_id,omitempty"`
	ContainerName string `json:"container_name,omitempty"`
}

type HTTPEvent struct {
	Timestamp            string            `json:"timestamp"`
	PID                  uint32            `json:"pid"`
	Comm                 string            `json:"comm"`
	Cmdline              string            `json:"cmdline"`
	Direction            string            `json:"direction"`
	SourceIP             string            `json:"source_ip"`
	SourcePort           uint16            `json:"source_port"`
	DestIP               string            `json:"dest_ip"`
	DestPort             uint16            `json:"dest_port"`
	Bytes                uint32            `json:"bytes"`
	Method               string            `json:"method,omitempty"`
	URL                  string            `json:"url,omitempty"`
	StatusCode           string            `json:"status_code,omitempty"`
	Body                 string            `json:"body,omitempty"`
	Headers              map[string]string `json:"headers,omitempty"`
	RawPayload           string            `json:"raw_payload,omitempty"`
	SourceContainer      *ContainerInfo    `json:"source_container,omitempty"`
	DestinationContainer *ContainerInfo    `json:"destination_container,omitempty"`
	DestinationType      string            `json:"destination_type,omitempty"` // "container" or "external"
}

type Runner struct {
	targetPort          uint16
	outputPath          string
	envOutputPath       string
	envPrefixes         []string
	adiProfileAllowed   []string
	seenPIDs            map[uint32]string // stores cmdline for each PID
	pidAdiProfiles      map[uint32]string // stores ADI_PROFILE value for each PID
	pidNames            map[uint32]string // stores ADI_PROFILE_NAME value for each PID
	writtenPIDs         map[uint32]bool   // tracks which PIDs have been written to YAML
	containerResolver   *ContainerResolver
	containerdSocket    string
	containerdNamespace string
}

func NewRunner(port uint16, outputPath string, envOutputPath string, envPrefixes []string, adiProfileAllowed []string, containerdSocket string, containerdNamespace string) *Runner {
	return &Runner{
		targetPort:          port,
		outputPath:          outputPath,
		envOutputPath:       envOutputPath,
		envPrefixes:         envPrefixes,
		adiProfileAllowed:   adiProfileAllowed,
		seenPIDs:            make(map[uint32]string),
		pidAdiProfiles:      make(map[uint32]string),
		pidNames:            make(map[uint32]string),
		writtenPIDs:         make(map[uint32]bool),
		containerdSocket:    containerdSocket,
		containerdNamespace: containerdNamespace,
	}
}

func (r *Runner) shouldProfilePID(pid uint32) (bool, string, string) {
	// Read environment variables from /proc/<pid>/environ
	environPath := fmt.Sprintf("/proc/%d/environ", pid)
	data, err := os.ReadFile(environPath)
	if err != nil {
		// Process may have exited, skip silently
		return false, "", ""
	}

	// Parse environ file to find ADI_PROFILE, ADI_PROFILE_DISABLED, and ADI_PROFILE_NAME
	var adiProfile string
	var adiProfileName string
	var adiProfileDisabled bool

	parts := strings.Split(string(data), "\x00")
	for _, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(part, "=")
		if idx > 0 {
			key := part[:idx]
			value := part[idx+1:]

			if key == "ADI_PROFILE" {
				adiProfile = value
			} else if key == "ADI_PROFILE_DISABLED" && value == "1" {
				adiProfileDisabled = true
			} else if key == "ADI_PROFILE_NAME" {
				adiProfileName = value
			}
		}
	}

	// Check opt-in criteria
	// 1. ADI_PROFILE_DISABLED must not be set
	if adiProfileDisabled {
		return false, "", ""
	}

	// 2. ADI_PROFILE must be present
	if adiProfile == "" {
		return false, "", ""
	}

	// 3. If ADI_PROFILE_ALLOWED is set, value must be in the list
	if len(r.adiProfileAllowed) > 0 {
		allowed := false
		for _, allowedValue := range r.adiProfileAllowed {
			if adiProfile == allowedValue {
				allowed = true
				break
			}
		}
		if !allowed {
			return false, "", ""
		}
	}

	// All checks passed
	return true, adiProfile, adiProfileName
}

func (r *Runner) collectAndWriteEnv(pid uint32, envFile *os.File) {
	// Check if we've already written this PID
	if r.writtenPIDs[pid] {
		return
	}
	r.writtenPIDs[pid] = true

	// Read environment variables from /proc/<pid>/environ
	environPath := fmt.Sprintf("/proc/%d/environ", pid)
	data, err := os.ReadFile(environPath)

	// Write YAML document separator
	fmt.Fprintf(envFile, "---\n")

	fmt.Fprintf(envFile, "adi_profile_pid: %d\n", pid)

	// Add ADI_PROFILE value if we have it
	if adiProfileValue, ok := r.pidAdiProfiles[pid]; ok {
		fmt.Fprintf(envFile, "adi_profile_match: \"%s\"\n", adiProfileValue)
	}

	// Add ADI_PROFILE_NAME as "adi_profile_name" if we have it
	if name, ok := r.pidNames[pid]; ok {
		fmt.Fprintf(envFile, "adi_profile_name: \"%s\"\n", name)
	}

	// Add adi_profile_cmdline from the stored value
	if cmdline, ok := r.seenPIDs[pid]; ok {
		// Escape quotes and newlines in cmdline
		escapedCmdline := strings.ReplaceAll(cmdline, "\\", "\\\\")
		escapedCmdline = strings.ReplaceAll(escapedCmdline, "\"", "\\\"")
		escapedCmdline = strings.ReplaceAll(escapedCmdline, "\n", "\\n")
		fmt.Fprintf(envFile, "adi_profile_cmdline: \"%s\"\n", escapedCmdline)
	}

	if err != nil {
		// Handle error case
		fmt.Fprintf(envFile, "error: \"%v\"\n", err)
		envFile.Sync()
		return
	}

	if len(data) == 0 {
		fmt.Fprintf(envFile, "adi_profile_env: {}\n")
		envFile.Sync()
		return
	}

	// Parse environ file (null-separated key=value pairs)
	envVars := make(map[string]string)
	parts := strings.Split(string(data), "\x00")
	for _, part := range parts {
		if part == "" {
			continue
		}
		// Split on first '=' to get key and value
		idx := strings.Index(part, "=")
		if idx > 0 {
			key := part[:idx]
			value := part[idx+1:]

			// Filter by prefix if prefixes are specified
			if len(r.envPrefixes) > 0 {
				hasPrefix := false
				for _, prefix := range r.envPrefixes {
					if strings.HasPrefix(key, prefix) {
						hasPrefix = true
						break
					}
				}
				if !hasPrefix {
					continue
				}
			}

			envVars[key] = value
		}
	}

	// Write env vars as YAML
	fmt.Fprintf(envFile, "env:\n")
	if len(envVars) == 0 {
		fmt.Fprintf(envFile, "  {}\n")
	} else {
		for key, value := range envVars {
			// Escape quotes and newlines in value
			escapedValue := strings.ReplaceAll(value, "\\", "\\\\")
			escapedValue = strings.ReplaceAll(escapedValue, "\"", "\\\"")
			escapedValue = strings.ReplaceAll(escapedValue, "\n", "\\n")
			fmt.Fprintf(envFile, "  %s: \"%s\"\n", key, escapedValue)
		}
	}
	envFile.Sync()
}

func (r *Runner) Run(ctx context.Context) error {
	if err := ensureMemlock(); err != nil {
		return err
	}

	// Initialize container resolver if containerd socket is provided
	if r.containerdSocket != "" {
		resolver, err := NewContainerResolver(r.containerdSocket, r.containerdNamespace)
		if err != nil {
			log.Printf("warning: failed to initialize container resolver: %v", err)
			log.Printf("continuing without container metadata enrichment")
		} else {
			r.containerResolver = resolver
			defer r.containerResolver.Close()
			log.Printf("container resolver initialized for namespace '%s'", r.containerdNamespace)
		}
	}

	var objs profilerObjects
	if err := loadProfilerObjects(&objs, nil); err != nil {
		return fmt.Errorf("loading bpf objects: %w", err)
	}
	defer objs.Close()

	links := []link.Link{}
	attachTracepoint := func(category, name string, prog *ebpf.Program) error {
		l, err := link.Tracepoint(category, name, prog, nil)
		if err != nil {
			return err
		}
		links = append(links, l)
		return nil
	}

	if err := attachTracepoint("syscalls", "sys_enter_bind", objs.TraceSysEnterBind); err != nil {
		return fmt.Errorf("attach sys_enter_bind: %w", err)
	}
	if err := attachTracepoint("syscalls", "sys_enter_connect", objs.TraceSysEnterConnect); err != nil {
		return fmt.Errorf("attach sys_enter_connect: %w", err)
	}
	if err := attachTracepoint("syscalls", "sys_enter_accept4", objs.TraceSysEnterAccept4); err != nil {
		return fmt.Errorf("attach sys_enter_accept4: %w", err)
	}
	if err := attachTracepoint("syscalls", "sys_exit_accept4", objs.TraceSysExitAccept4); err != nil {
		return fmt.Errorf("attach sys_exit_accept4: %w", err)
	}
	if err := attachTracepoint("syscalls", "sys_enter_sendto", objs.TraceSysEnterSendto); err != nil {
		return fmt.Errorf("attach sys_enter_sendto: %w", err)
	}
	if err := attachTracepoint("syscalls", "sys_enter_recvfrom", objs.TraceSysEnterRecvfrom); err != nil {
		return fmt.Errorf("attach sys_enter_recvfrom: %w", err)
	}
	if err := attachTracepoint("syscalls", "sys_exit_recvfrom", objs.TraceSysExitRecvfrom); err != nil {
		return fmt.Errorf("attach sys_exit_recvfrom: %w", err)
	}
	if err := attachTracepoint("syscalls", "sys_enter_write", objs.TraceSysEnterWrite); err != nil {
		return fmt.Errorf("attach sys_enter_write: %w", err)
	}
	if err := attachTracepoint("syscalls", "sys_enter_read", objs.TraceSysEnterRead); err != nil {
		return fmt.Errorf("attach sys_enter_read: %w", err)
	}
	if err := attachTracepoint("syscalls", "sys_exit_read", objs.TraceSysExitRead); err != nil {
		return fmt.Errorf("attach sys_exit_read: %w", err)
	}
	if err := attachTracepoint("syscalls", "sys_enter_sendmsg", objs.TraceSysEnterSendmsg); err != nil {
		return fmt.Errorf("attach sys_enter_sendmsg: %w", err)
	}
	if err := attachTracepoint("syscalls", "sys_enter_recvmsg", objs.TraceSysEnterRecvmsg); err != nil {
		return fmt.Errorf("attach sys_enter_recvmsg: %w", err)
	}
	if err := attachTracepoint("syscalls", "sys_exit_recvmsg", objs.TraceSysExitRecvmsg); err != nil {
		return fmt.Errorf("attach sys_exit_recvmsg: %w", err)
	}

	defer func() {
		for _, l := range links {
			_ = l.Close()
		}
	}()

	rd, err := ringbuf.NewReader(objs.Events)
	if err != nil {
		return fmt.Errorf("open ringbuf: %w", err)
	}
	defer rd.Close()

	outFile, err := os.OpenFile(r.outputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open output file: %w", err)
	}
	defer outFile.Close()
	writer := bufio.NewWriter(outFile)
	defer writer.Flush()

	envFile, err := os.OpenFile(r.envOutputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open env output file: %w", err)
	}
	defer envFile.Close()

	log.Printf("profiler attached, filtering for port %d, writing to %s", r.targetPort, r.outputPath)
	log.Printf("environment variables will be written to %s", r.envOutputPath)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)

	shutdown := make(chan struct{})
	go func() {
		defer close(shutdown)
		select {
		case <-ctx.Done():
			log.Printf("context done, closing ringbuf")
		case <-signals:
			log.Printf("signal received, closing ringbuf")
		}
		log.Printf("closing ringbuf")
		rd.Close()
	}()

	for {
		record, err := rd.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				log.Printf("ringbuf closed, exiting")
				return nil
			}
			return fmt.Errorf("read ringbuf: %w", err)
		}

		ev := (*Event)(unsafe.Pointer(&record.RawSample[0]))

		parsed := parseHTTP(ev)

		// Filter: only output if we successfully parsed HTTP data
		if !isHTTPTraffic(ev, parsed) {
			continue
		}

		// Check opt-in criteria for new PIDs
		if _, seen := r.seenPIDs[ev.Pid]; !seen {
			shouldProfile, adiProfileValue, adiProfileName := r.shouldProfilePID(ev.Pid)

			// Read cmdline once for this PID
			cmdline := ""
			cmdlinePath := fmt.Sprintf("/proc/%d/cmdline", ev.Pid)
			if data, err := os.ReadFile(cmdlinePath); err == nil && len(data) > 0 {
				// /proc/<pid>/cmdline is null-separated. Replace with spaces and trim
				cmdline = strings.ReplaceAll(string(data), "\x00", " ")
				cmdline = strings.TrimSpace(cmdline)
			} else if err != nil {
				cmdline = fmt.Sprintf("[cmdline error: %v]", err)
			} else {
				cmdline = "[cmdline empty]"
			}

			if !shouldProfile {
				// Silently skip this PID - it doesn't meet opt-in criteria
				r.seenPIDs[ev.Pid] = cmdline // Mark as seen with cmdline so we don't check again
				continue
			}

			// Store the cmdline, ADI_PROFILE value, and ADI_PROFILE_NAME for this PID
			r.seenPIDs[ev.Pid] = cmdline
			r.pidAdiProfiles[ev.Pid] = adiProfileValue
			if adiProfileName != "" {
				r.pidNames[ev.Pid] = adiProfileName
			}
		} else {
			// For already-seen PIDs, check if they passed opt-in
			if _, hasProfile := r.pidAdiProfiles[ev.Pid]; !hasProfile {
				// This PID was previously rejected
				continue
			}
		}

		// Collect environment variables for new PIDs
		r.collectAndWriteEnv(ev.Pid, envFile)

		jsonLine, err := r.formatEventJSON(ev, parsed)
		if err != nil {
			log.Printf("error formatting event: %v", err)
			continue
		}

		if _, err := writer.WriteString(jsonLine + "\n"); err != nil {
			log.Printf("error writing line: %v", err)
			return fmt.Errorf("write log: %w", err)
		}
		writer.Flush()

		select {
		case <-ctx.Done():
			log.Printf("context done, exiting")
			return nil
		case <-signals:
			log.Printf("signal received, exiting")
			return nil
		default:
		}
	}
}

// ntohs converts a 16-bit integer from network byte order (big endian) to host byte order.
// In network protocols, port numbers and similar fields are transmitted in big endian order.
// This function reverses the byte order, assuming the host is little endian (which is true for common platforms).
func ntohs(v uint16) uint16 {
	return (v >> 8) | (v << 8)
}

func (r *Runner) portMatches(ev *Event) bool {
	return ntohs(ev.Sport) == r.targetPort || ntohs(ev.Dport) == r.targetPort
}

func (r *Runner) formatEventJSON(ev *Event, parsed Parsed) (string, error) {
	dir := "send"
	if ev.Direction == dirRecv {
		dir = "recv"
	}
	sport := ntohs(ev.Sport)
	dport := ntohs(ev.Dport)
	saddr := ipFromBytes(ev.Family, ev.Saddr[:])
	daddr := ipFromBytes(ev.Family, ev.Daddr[:])
	payload := string(ev.Data[:ev.DataLen])

	// Get the stored cmdline for this PID
	cmdline := r.seenPIDs[ev.Pid]

	event := HTTPEvent{
		Timestamp:  time.Unix(0, int64(ev.Ts)).Format(time.RFC3339Nano),
		PID:        ev.Pid,
		Comm:       strings.Trim(string(ev.Comm[:]), "\x00"),
		Cmdline:    cmdline,
		Direction:  dir,
		SourceIP:   saddr.String(),
		SourcePort: sport,
		DestIP:     daddr.String(),
		DestPort:   dport,
		Bytes:      ev.DataLen,
		Method:     parsed.Method,
		URL:        parsed.URL,
		StatusCode: parsed.StatusCode,
		Body:       parsed.Body,
		Headers:    parsed.Headers,
		RawPayload: payload,
	}

	// Enrich with container metadata if resolver is available
	if r.containerResolver != nil {
		// Resolve source container from PID
		if srcMeta := r.containerResolver.ResolvePIDToContainer(ev.Pid); srcMeta != nil {
			event.SourceContainer = &ContainerInfo{
				Service:       srcMeta.Service,
				Image:         fmt.Sprintf("%s:%s", srcMeta.Image, srcMeta.ImageTag),
				ContainerID:   srcMeta.ContainerID,
				ContainerName: srcMeta.ContainerName,
			}
		}

		// Resolve destination container from IP:port
		if dstMeta := r.containerResolver.ResolveDestination(daddr, dport); dstMeta != nil {
			event.DestinationContainer = &ContainerInfo{
				Service:       dstMeta.Service,
				Image:         fmt.Sprintf("%s:%s", dstMeta.Image, dstMeta.ImageTag),
				ContainerID:   dstMeta.ContainerID,
				ContainerName: dstMeta.ContainerName,
			}
			event.DestinationType = "container"
		} else {
			// External destination
			event.DestinationType = "external"
		}
	}

	jsonBytes, err := json.Marshal(event)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

func parseHTTP(ev *Event) Parsed {
	var out Parsed
	data := ev.Data[:ev.DataLen]
	text := string(data)

	// Try to parse as HTTP response first (starts with "HTTP/")
	if strings.HasPrefix(text, "HTTP/1.") || strings.HasPrefix(text, "HTTP/2") {
		parts := strings.SplitN(text, " ", 3)
		if len(parts) >= 2 {
			out.StatusCode = strings.TrimSpace(parts[1])
		}
		out.Body = extractBody(text)
		out.Headers = extractHeaders(text)
	} else {
		// Try to parse as HTTP request (starts with method)
		fields := strings.Fields(text)
		if len(fields) >= 2 && isHTTPMethod(fields[0]) {
			out.Method = fields[0]
			out.URL = fields[1]
			out.Body = extractBody(text)
			out.Headers = extractHeaders(text)
		}
	}
	return out
}

func extractHeaders(payload string) map[string]string {
	headers := make(map[string]string)
	parts := strings.SplitN(payload, "\r\n\r\n", 2)
	if len(parts) < 1 {
		return headers
	}

	lines := strings.Split(parts[0], "\r\n")
	// Skip the first line (request/response line)
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		// Parse header: "Key: Value"
		colonIdx := strings.Index(line, ":")
		if colonIdx > 0 {
			key := strings.TrimSpace(line[:colonIdx])
			value := strings.TrimSpace(line[colonIdx+1:])
			headers[key] = value
		}
	}
	return headers
}

func isHTTPTraffic(ev *Event, parsed Parsed) bool {
	// Check if we parsed HTTP data successfully (either request or response)
	return parsed.Method != "" || parsed.StatusCode != ""
}

func extractBody(payload string) string {
	parts := strings.SplitN(payload, "\r\n\r\n", 2)
	if len(parts) < 2 {
		return ""
	}
	body := strings.TrimSpace(parts[1])
	if len(body) > 120 {
		body = body[:120] + "..."
	}
	return body
}

func isHTTPMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

func ipFromBytes(family uint16, raw []byte) netip.Addr {
	switch family {
	case syscall.AF_INET:
		return netip.AddrFrom4([4]byte{raw[0], raw[1], raw[2], raw[3]})
	case syscall.AF_INET6:
		var b [16]byte
		copy(b[:], raw)
		return netip.AddrFrom16(b)
	default:
		return netip.Addr{}
	}
}

func ensureMemlock() error {
	const fallbackLimit = 256 << 20 // 256 MiB
	if err := rlimit.RemoveMemlock(); err == nil {
		return nil
	}
	lim := unix.Rlimit{
		Cur: fallbackLimit,
		Max: fallbackLimit,
	}
	if err := unix.Setrlimit(unix.RLIMIT_MEMLOCK, &lim); err != nil {
		return fmt.Errorf("set memlock limit: %w", err)
	}
	return nil
}
