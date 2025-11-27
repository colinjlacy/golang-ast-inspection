package output

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/colinjlacy/golang-ast-inspection/pkg/http"
)

// Writer handles output of profiling data to a file
type Writer struct {
	file     *os.File
	filename string
	mu       sync.Mutex
}

// NewWriter creates a new output writer
func NewWriter(filename string) (*Writer, error) {
	file, err := os.Create(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}

	w := &Writer{
		file:     file,
		filename: filename,
	}

	// Write header
	w.writeHeader()

	return w, nil
}

// Close closes the output file
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file != nil {
		w.writeFooter()
		err := w.file.Close()
		w.file = nil
		return err
	}
	return nil
}

func (w *Writer) writeHeader() {
	fmt.Fprintf(w.file, "Container HTTP Profiler Output\n")
	fmt.Fprintf(w.file, "==============================\n\n")
}

func (w *Writer) writeFooter() {
	fmt.Fprintf(w.file, "\n==============================\n")
	fmt.Fprintf(w.file, "End of trace\n")
}

// WriteHTTPTransaction writes an HTTP transaction to the output file
func (w *Writer) WriteHTTPTransaction(tx *http.HTTPTransaction) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return fmt.Errorf("writer is closed")
	}

	// Timestamp and process info
	fmt.Fprintf(w.file, "[%s] PID %d\n",
		tx.Timestamp.Format("2006-01-02 15:04:05.000"),
		tx.PID)

	// Request info
	if tx.Request != nil {
		fmt.Fprintf(w.file, "  → HTTP %s %s\n",
			tx.Request.Method,
			tx.Request.URL)

		// Show key headers
		if host := tx.Request.Headers["Host"]; host != "" {
			fmt.Fprintf(w.file, "     Host: %s\n", host)
		}
		if contentType := tx.Request.Headers["Content-Type"]; contentType != "" {
			fmt.Fprintf(w.file, "     Content-Type: %s\n", contentType)
		}

		// Show body if present
		if len(tx.Request.Body) > 0 {
			bodyPreview := string(tx.Request.Body)
			if len(bodyPreview) > 200 {
				bodyPreview = bodyPreview[:200] + "..."
			}
			bodyPreview = strings.ReplaceAll(bodyPreview, "\n", "\\n")
			fmt.Fprintf(w.file, "     Body: %s\n", bodyPreview)
		}
	}

	// Response info
	if tx.Response != nil {
		duration := ""
		if tx.DurationMs > 0 {
			duration = fmt.Sprintf(" (%.1fms)", tx.DurationMs)
		}

		fmt.Fprintf(w.file, "  ← Response %d %s%s\n",
			tx.Response.StatusCode,
			tx.Response.StatusText,
			duration)

		// Show key headers
		if contentType := tx.Response.Headers["Content-Type"]; contentType != "" {
			fmt.Fprintf(w.file, "     Content-Type: %s\n", contentType)
		}
		if contentLength := tx.Response.Headers["Content-Length"]; contentLength != "" {
			fmt.Fprintf(w.file, "     Content-Length: %s\n", contentLength)
		}

		// Show body if present
		if len(tx.Response.Body) > 0 {
			bodyPreview := string(tx.Response.Body)
			if len(bodyPreview) > 500 {
				bodyPreview = bodyPreview[:500] + "..."
			}
			// Clean up for display
			bodyPreview = strings.ReplaceAll(bodyPreview, "\n", " ")
			bodyPreview = strings.ReplaceAll(bodyPreview, "\r", "")
			fmt.Fprintf(w.file, "     Body: %s\n", bodyPreview)
		}
	}

	fmt.Fprintf(w.file, "\n")

	return nil
}

// WriteMessage writes a raw message to the output file
func (w *Writer) WriteMessage(format string, args ...interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return fmt.Errorf("writer is closed")
	}

	_, err := fmt.Fprintf(w.file, format, args...)
	return err
}

