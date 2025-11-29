//go:build linux

package profiler

import (
	"bufio"
	"context"
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
}

type Runner struct {
	targetPort uint16
	outputPath string
}

func NewRunner(port uint16, outputPath string) *Runner {
	return &Runner{targetPort: port, outputPath: outputPath}
}

func (r *Runner) Run(ctx context.Context) error {
	if err := ensureMemlock(); err != nil {
		return err
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

	log.Printf("profiler attached, filtering for port %d, writing to %s", r.targetPort, r.outputPath)

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
		// if !r.portMatches(ev) {
		// 	log.Printf("event not for target port, skipping")
		// 	continue
		// }

		parsed := parseHTTP(ev)
		line := r.formatEvent(ev, parsed)
		log.Printf("writing line: %s", line)
		if _, err := writer.WriteString(line + "\n"); err != nil {
			log.Printf("error writing line: %v", err)
			return fmt.Errorf("write log: %w", err)
		}
		log.Printf("flushed writer")
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

func (r *Runner) formatEvent(ev *Event, parsed Parsed) string {
	dir := "send"
	if ev.Direction == dirRecv {
		dir = "recv"
	}
	sport := ntohs(ev.Sport)
	dport := ntohs(ev.Dport)
	saddr := ipFromBytes(ev.Family, ev.Saddr[:])
	daddr := ipFromBytes(ev.Family, ev.Daddr[:])
	payload := strings.TrimSpace(string(ev.Data[:ev.DataLen]))

	// Lookup the command line for the process using /proc/<pid>/cmdline
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

	var parts []string
	parts = append(parts, fmt.Sprintf("ts=%s", time.Unix(0, int64(ev.Ts)).Format(time.RFC3339Nano)))
	parts = append(parts, fmt.Sprintf("pid=%d", ev.Pid))
	parts = append(parts, fmt.Sprintf("comm=%s", strings.Trim(string(ev.Comm[:]), "\x00")))
	parts = append(parts, fmt.Sprintf("cmdline=%q", cmdline))
	parts = append(parts, fmt.Sprintf("dir=%s", dir))
	parts = append(parts, fmt.Sprintf("src=%s:%d", saddr, sport))
	parts = append(parts, fmt.Sprintf("dst=%s:%d", daddr, dport))
	parts = append(parts, fmt.Sprintf("bytes=%d", ev.DataLen))

	if parsed.Method != "" {
		parts = append(parts, fmt.Sprintf("method=%s", parsed.Method))
	}
	if parsed.URL != "" {
		parts = append(parts, fmt.Sprintf("url=%s", parsed.URL))
	}
	if parsed.StatusCode != "" {
		parts = append(parts, fmt.Sprintf("status=%s", parsed.StatusCode))
	}
	if parsed.Body != "" {
		parts = append(parts, fmt.Sprintf("body=%s", parsed.Body))
	}

	parts = append(parts, fmt.Sprintf("raw=%q", payload))
	return strings.Join(parts, " ")
}

func parseHTTP(ev *Event) Parsed {
	var out Parsed
	data := ev.Data[:ev.DataLen]
	text := string(data)

	if ev.Direction == dirSend {
		fields := strings.Fields(text)
		if len(fields) >= 2 && isHTTPMethod(fields[0]) {
			out.Method = fields[0]
			out.URL = fields[1]
			out.Body = extractBody(text)
		}
	} else {
		if strings.HasPrefix(text, "HTTP/1.") || strings.HasPrefix(text, "HTTP/2") {
			parts := strings.SplitN(text, " ", 3)
			if len(parts) >= 2 {
				out.StatusCode = strings.TrimSpace(parts[1])
			}
			out.Body = extractBody(text)
		}
	}
	return out
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
