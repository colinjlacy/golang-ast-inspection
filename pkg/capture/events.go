package capture

import "time"

// Direction indicates whether data is being sent or received
type Direction int

const (
	DirSend Direction = 1 // Outbound
	DirRecv Direction = 2 // Inbound
)

func (d Direction) String() string {
	if d == DirSend {
		return "SEND"
	}
	return "RECV"
}

// RawEvent represents a captured network event from eBPF
type RawEvent struct {
	Timestamp time.Time
	PID       uint32
	TID       uint32
	Fd        uint32
	Direction Direction
	Data      []byte
	Comm      string // Process command name
}

// HTTPEvent represents a parsed HTTP event (for future use with structured output)
type HTTPEvent struct {
	Timestamp time.Time
	PID       uint32
	Method    string
	URL       string
	Headers   map[string]string
	Body      []byte
	IsRequest bool
}

