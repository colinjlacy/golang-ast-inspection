package ebpf

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/colinjlacy/golang-ast-inspection/pkg/capture"
)

const (
	maxDataSize  = 16384
	taskCommLen  = 16
	eventTypeSend = 1
	eventTypeRecv = 2
)

// HTTPProbe represents the loaded eBPF program and its attachments
type HTTPProbe struct {
	objs  *ebpf.Collection
	links []link.Link
	rb    *ringbuf.Reader
}

// LoadHTTPProbe loads the eBPF program and attaches it to kernel hooks
func LoadHTTPProbe() (*HTTPProbe, error) {
	// For now, we'll create a simple stub since we need the actual compiled eBPF object
	// In production, this would load from the compiled .o file
	
	// This is a placeholder - actual implementation would:
	// 1. Load eBPF object file
	// 2. Create maps
	// 3. Attach programs to hooks
	
	return nil, errors.New("eBPF loading not yet implemented - requires compiled eBPF program")
}

// LoadHTTPProbeFromFile loads an eBPF program from a compiled object file
func LoadHTTPProbeFromFile(objPath string) (*HTTPProbe, error) {
	// Load pre-compiled eBPF object
	spec, err := ebpf.LoadCollectionSpec(objPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load eBPF spec: %w", err)
	}

	// Load the collection
	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to create eBPF collection: %w", err)
	}

	probe := &HTTPProbe{
		objs:  coll,
		links: make([]link.Link, 0),
	}

	// Attach to write syscall tracepoint
	if prog := coll.Programs["trace_write_enter"]; prog != nil {
		l, err := link.Tracepoint("syscalls", "sys_enter_write", prog, nil)
		if err != nil {
			probe.Close()
			return nil, fmt.Errorf("failed to attach to sys_enter_write: %w", err)
		}
		probe.links = append(probe.links, l)
	}

	// Attach to read syscall tracepoint
	if prog := coll.Programs["trace_read_exit"]; prog != nil {
		l, err := link.Tracepoint("syscalls", "sys_exit_read", prog, nil)
		if err != nil {
			probe.Close()
			return nil, fmt.Errorf("failed to attach to sys_exit_read: %w", err)
		}
		probe.links = append(probe.links, l)
	}

	// Open ring buffer
	if eventsMap := coll.Maps["events"]; eventsMap != nil {
		rb, err := ringbuf.NewReader(eventsMap)
		if err != nil {
			probe.Close()
			return nil, fmt.Errorf("failed to create ring buffer reader: %w", err)
		}
		probe.rb = rb
	} else {
		probe.Close()
		return nil, fmt.Errorf("events map not found in eBPF program")
	}

	return probe, nil
}

// ReadEvents reads events from the eBPF ring buffer
func (p *HTTPProbe) ReadEvents() (<-chan *capture.RawEvent, <-chan error) {
	events := make(chan *capture.RawEvent, 100)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		for {
			record, err := p.rb.Read()
			if err != nil {
				if errors.Is(err, ringbuf.ErrClosed) {
					return
				}
				errs <- fmt.Errorf("reading from ring buffer: %w", err)
				continue
			}

			// Parse the event
			event, err := parseEvent(record.RawSample)
			if err != nil {
				errs <- fmt.Errorf("parsing event: %w", err)
				continue
			}

			events <- event
		}
	}()

	return events, errs
}

// parseEvent parses a raw event from the ring buffer
func parseEvent(data []byte) (*capture.RawEvent, error) {
	if len(data) < 8+4+4+4+1+4+taskCommLen {
		return nil, fmt.Errorf("event data too short")
	}

	buf := bytes.NewReader(data)

	event := &capture.RawEvent{}

	// Parse timestamp (u64)
	var timestamp uint64
	if err := binary.Read(buf, binary.LittleEndian, &timestamp); err != nil {
		return nil, err
	}
	event.Timestamp = time.Unix(0, int64(timestamp))

	// Parse PID (u32)
	if err := binary.Read(buf, binary.LittleEndian, &event.PID); err != nil {
		return nil, err
	}

	// Parse TID (u32)
	if err := binary.Read(buf, binary.LittleEndian, &event.TID); err != nil {
		return nil, err
	}

	// Parse FD (u32)
	if err := binary.Read(buf, binary.LittleEndian, &event.Fd); err != nil {
		return nil, err
	}

	// Parse type (u8)
	var eventType uint8
	if err := binary.Read(buf, binary.LittleEndian, &eventType); err != nil {
		return nil, err
	}
	if eventType == eventTypeSend {
		event.Direction = capture.DirSend
	} else {
		event.Direction = capture.DirRecv
	}

	// Parse data length (u32)
	var dataLen uint32
	if err := binary.Read(buf, binary.LittleEndian, &dataLen); err != nil {
		return nil, err
	}

	// Parse comm (task command name)
	comm := make([]byte, taskCommLen)
	if err := binary.Read(buf, binary.LittleEndian, &comm); err != nil {
		return nil, err
	}
	// Convert to string, stopping at first null byte
	for i, b := range comm {
		if b == 0 {
			event.Comm = string(comm[:i])
			break
		}
	}
	if event.Comm == "" {
		event.Comm = string(comm)
	}

	// Parse data
	if dataLen > 0 && dataLen <= maxDataSize {
		event.Data = make([]byte, dataLen)
		n, err := buf.Read(event.Data)
		if err != nil && n == 0 {
			return nil, err
		}
		event.Data = event.Data[:n]
	}

	return event, nil
}

// Close closes the eBPF probe and cleans up resources
func (p *HTTPProbe) Close() error {
	var errs []error

	// Close ring buffer
	if p.rb != nil {
		if err := p.rb.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	// Detach links
	for _, l := range p.links {
		if err := l.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	// Close collection
	if p.objs != nil {
		p.objs.Close()
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing probe: %v", errs)
	}

	return nil
}

