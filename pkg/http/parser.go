package http

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/colinjlacy/golang-ast-inspection/pkg/stream"
)

// HTTPRequest represents a parsed HTTP request
type HTTPRequest struct {
	Method  string
	URL     string
	Version string
	Headers map[string]string
	Body    []byte
}

// HTTPResponse represents a parsed HTTP response
type HTTPResponse struct {
	Version    string
	StatusCode int
	StatusText string
	Headers    map[string]string
	Body       []byte
}

// HTTPTransaction represents a complete HTTP request/response pair
type HTTPTransaction struct {
	Timestamp    time.Time
	PID          uint32
	Request      *HTTPRequest
	Response     *HTTPResponse
	DurationMs   float64
	RequestTime  time.Time
	ResponseTime time.Time
}

// Parser parses HTTP traffic from TCP streams
type Parser struct {
	pendingRequests map[stream.StreamID][]*HTTPRequest
}

// NewParser creates a new HTTP parser
func NewParser() *Parser {
	return &Parser{
		pendingRequests: make(map[stream.StreamID][]*HTTPRequest),
	}
}

// TryParseRequest attempts to parse an HTTP request from the send buffer
func (p *Parser) TryParseRequest(s *stream.TCPStream) (*HTTPRequest, bool) {
	data := s.SendBuffer.Bytes()
	if len(data) == 0 {
		return nil, false
	}

	// Check if it looks like an HTTP request
	if !bytes.HasPrefix(data, []byte("GET ")) &&
		!bytes.HasPrefix(data, []byte("POST ")) &&
		!bytes.HasPrefix(data, []byte("PUT ")) &&
		!bytes.HasPrefix(data, []byte("DELETE ")) &&
		!bytes.HasPrefix(data, []byte("PATCH ")) &&
		!bytes.HasPrefix(data, []byte("HEAD ")) &&
		!bytes.HasPrefix(data, []byte("OPTIONS ")) {
		return nil, false
	}

	reader := bufio.NewReader(bytes.NewReader(data))

	// Parse request line
	requestLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, false
	}

	parts := strings.Fields(requestLine)
	if len(parts) < 3 {
		return nil, false
	}

	req := &HTTPRequest{
		Method:  parts[0],
		URL:     parts[1],
		Version: parts[2],
		Headers: make(map[string]string),
	}

	// Parse headers
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, false
		}

		line = strings.TrimSpace(line)
		if line == "" {
			// End of headers
			break
		}

		// Parse header
		colonIdx := strings.Index(line, ":")
		if colonIdx > 0 {
			key := strings.TrimSpace(line[:colonIdx])
			value := strings.TrimSpace(line[colonIdx+1:])
			req.Headers[key] = value
		}
	}

	// Check for body based on Content-Length
	if contentLengthStr, ok := req.Headers["Content-Length"]; ok {
		contentLength, err := strconv.Atoi(contentLengthStr)
		if err == nil && contentLength > 0 {
			body := make([]byte, contentLength)
			n, err := reader.Read(body)
			if err != nil && err != io.EOF {
				return nil, false
			}
			if n < contentLength {
				// Not enough data yet
				return nil, false
			}
			req.Body = body[:n]
		}
	}

	// Successfully parsed - consume the data from the buffer
	consumed := len(data) - reader.Buffered()
	s.SendBuffer.Next(consumed)

	return req, true
}

// TryParseResponse attempts to parse an HTTP response from the recv buffer
func (p *Parser) TryParseResponse(s *stream.TCPStream) (*HTTPResponse, bool) {
	data := s.RecvBuffer.Bytes()
	if len(data) == 0 {
		return nil, false
	}

	// Check if it looks like an HTTP response
	if !bytes.HasPrefix(data, []byte("HTTP/")) {
		return nil, false
	}

	reader := bufio.NewReader(bytes.NewReader(data))

	// Parse status line
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, false
	}

	parts := strings.SplitN(statusLine, " ", 3)
	if len(parts) < 3 {
		return nil, false
	}

	statusCode, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return nil, false
	}

	resp := &HTTPResponse{
		Version:    parts[0],
		StatusCode: statusCode,
		StatusText: strings.TrimSpace(parts[2]),
		Headers:    make(map[string]string),
	}

	// Parse headers
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, false
		}

		line = strings.TrimSpace(line)
		if line == "" {
			// End of headers
			break
		}

		// Parse header
		colonIdx := strings.Index(line, ":")
		if colonIdx > 0 {
			key := strings.TrimSpace(line[:colonIdx])
			value := strings.TrimSpace(line[colonIdx+1:])
			resp.Headers[key] = value
		}
	}

	// Check for body based on Content-Length
	if contentLengthStr, ok := resp.Headers["Content-Length"]; ok {
		contentLength, err := strconv.Atoi(contentLengthStr)
		if err == nil && contentLength > 0 {
			body := make([]byte, contentLength)
			n, err := reader.Read(body)
			if err != nil && err != io.EOF {
				return nil, false
			}
			if n < contentLength {
				// Not enough data yet
				return nil, false
			}
			resp.Body = body[:n]
		}
	}

	// Successfully parsed - consume the data from the buffer
	consumed := len(data) - reader.Buffered()
	s.RecvBuffer.Next(consumed)

	return resp, true
}

// ProcessStream checks a stream for complete HTTP transactions
func (p *Parser) ProcessStream(s *stream.TCPStream, eventTime time.Time) []*HTTPTransaction {
	var transactions []*HTTPTransaction

	// Try to parse requests from send buffer
	for {
		req, ok := p.TryParseRequest(s)
		if !ok {
			break
		}

		// Store pending request
		p.pendingRequests[s.ID] = append(p.pendingRequests[s.ID], req)
	}

	// Try to parse responses from recv buffer
	for {
		resp, ok := p.TryParseResponse(s)
		if !ok {
			break
		}

		// Match with pending request (FIFO)
		pending := p.pendingRequests[s.ID]
		if len(pending) > 0 {
			req := pending[0]
			p.pendingRequests[s.ID] = pending[1:]

			transaction := &HTTPTransaction{
				Timestamp: eventTime,
				PID:       s.ID.PID,
				Request:   req,
				Response:  resp,
			}

			transactions = append(transactions, transaction)
		}
	}

	return transactions
}

// String returns a human-readable representation of the transaction
func (t *HTTPTransaction) String() string {
	var buf bytes.Buffer

	if t.Request != nil {
		fmt.Fprintf(&buf, "%s %s", t.Request.Method, t.Request.URL)
	}

	if t.Response != nil {
		fmt.Fprintf(&buf, " → %d %s", t.Response.StatusCode, t.Response.StatusText)
	}

	return buf.String()
}

