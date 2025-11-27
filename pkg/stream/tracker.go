package stream

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/colinjlacy/golang-ast-inspection/pkg/capture"
)

// StreamID uniquely identifies a TCP stream
type StreamID struct {
	PID uint32
	Fd  uint32
}

func (s StreamID) String() string {
	return fmt.Sprintf("pid=%d,fd=%d", s.PID, s.Fd)
}

// TCPStream represents a bidirectional TCP connection
type TCPStream struct {
	ID         StreamID
	SendBuffer *bytes.Buffer // data being sent
	RecvBuffer *bytes.Buffer // data being received
	Closed     bool
}

// NewTCPStream creates a new TCP stream
func NewTCPStream(id StreamID) *TCPStream {
	return &TCPStream{
		ID:         id,
		SendBuffer: &bytes.Buffer{},
		RecvBuffer: &bytes.Buffer{},
	}
}

// Tracker manages all active TCP streams
type Tracker struct {
	streams map[StreamID]*TCPStream
	mu      sync.RWMutex
}

// NewTracker creates a new stream tracker
func NewTracker() *Tracker {
	return &Tracker{
		streams: make(map[StreamID]*TCPStream),
	}
}

// ProcessEvent processes a raw event and updates stream state
func (t *Tracker) ProcessEvent(event *capture.RawEvent) *TCPStream {
	t.mu.Lock()
	defer t.mu.Unlock()

	id := StreamID{PID: event.PID, Fd: event.Fd}

	// Get or create stream
	stream := t.streams[id]
	if stream == nil {
		stream = NewTCPStream(id)
		t.streams[id] = stream
	}

	// Append data to appropriate buffer
	if len(event.Data) > 0 {
		if event.Direction == capture.DirSend {
			stream.SendBuffer.Write(event.Data)
		} else {
			stream.RecvBuffer.Write(event.Data)
		}
	}

	return stream
}

// GetStream returns the stream for a given ID
func (t *Tracker) GetStream(id StreamID) *TCPStream {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.streams[id]
}

// GetAllStreams returns all active streams
func (t *Tracker) GetAllStreams() []*TCPStream {
	t.mu.RLock()
	defer t.mu.RUnlock()

	streams := make([]*TCPStream, 0, len(t.streams))
	for _, stream := range t.streams {
		streams = append(streams, stream)
	}
	return streams
}

// RemoveStream removes a stream from tracking
func (t *Tracker) RemoveStream(id StreamID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.streams, id)
}

// CleanupOldStreams removes streams that appear inactive
// This is a simple heuristic - in production you'd want better lifecycle management
func (t *Tracker) CleanupOldStreams() {
	t.mu.Lock()
	defer t.mu.Unlock()

	for id, stream := range t.streams {
		// Remove if both buffers are empty (data has been consumed)
		if stream.SendBuffer.Len() == 0 && stream.RecvBuffer.Len() == 0 && stream.Closed {
			delete(t.streams, id)
		}
	}
}

