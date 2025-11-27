package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/colinjlacy/golang-ast-inspection/pkg/ebpf"
	"github.com/colinjlacy/golang-ast-inspection/pkg/http"
	"github.com/colinjlacy/golang-ast-inspection/pkg/output"
	"github.com/colinjlacy/golang-ast-inspection/pkg/stream"
)

const (
	defaultOutputFile = "/traces/http-trace.txt"
	ebpfObjectPath    = "/usr/local/lib/http_probe.o"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run() error {
	log.Println("Container HTTP Profiler starting...")

	// Parse command line flags
	outputFile := os.Getenv("OUTPUT_FILE")
	if outputFile == "" {
		outputFile = defaultOutputFile
	}

	log.Printf("Output file: %s", outputFile)

	// Ensure output directory exists
	if err := os.MkdirAll("/traces", 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create output writer
	writer, err := output.NewWriter(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output writer: %w", err)
	}
	defer writer.Close()

	// Check for eBPF capabilities
	if !hasEBPFCapabilities() {
		return fmt.Errorf("container does not have required capabilities (CAP_SYS_ADMIN or CAP_BPF)")
	}

	// Load eBPF program
	log.Println("Loading eBPF program...")
	probe, err := ebpf.LoadHTTPProbeFromFile(ebpfObjectPath)
	if err != nil {
		// For MVP, provide helpful error message
		return fmt.Errorf("failed to load eBPF program from %s: %w\nMake sure the eBPF program is compiled and available", ebpfObjectPath, err)
	}
	defer probe.Close()

	log.Println("eBPF program loaded and attached")

	// Create processing components
	tracker := stream.NewTracker()
	httpParser := http.NewParser()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start reading events
	events, errs := probe.ReadEvents()

	log.Println("Profiler running. Press Ctrl+C to stop.")
	writer.WriteMessage("Profiler started at %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	eventCount := 0
	txCount := 0

	// Event processing loop
	for {
		select {
		case event := <-events:
			if event == nil {
				log.Println("Event channel closed")
				return nil
			}

			eventCount++

			// Process event through stream tracker
			tcpStream := tracker.ProcessEvent(event)
			if tcpStream == nil {
				continue
			}

			// Try to parse HTTP transactions
			transactions := httpParser.ProcessStream(tcpStream, event.Timestamp)
			for _, tx := range transactions {
				txCount++

				// Write to file
				if err := writer.WriteHTTPTransaction(tx); err != nil {
					log.Printf("Error writing transaction: %v", err)
				}

				// Also log to console
				log.Printf("[%s] PID %d (%s): %s",
					tx.Timestamp.Format("15:04:05.000"),
					tx.PID,
					event.Comm,
					tx.String())
			}

			// Periodic cleanup of old streams
			if eventCount%1000 == 0 {
				tracker.CleanupOldStreams()
			}

		case err := <-errs:
			if err != nil {
				log.Printf("Error reading events: %v", err)
			}

		case sig := <-sigChan:
			log.Printf("Received signal %v, shutting down...", sig)
			writer.WriteMessage("\nProfiler stopped at %s\n", time.Now().Format("2006-01-02 15:04:05"))
			writer.WriteMessage("Total events processed: %d\n", eventCount)
			writer.WriteMessage("Total HTTP transactions: %d\n", txCount)
			return nil
		}
	}
}

// hasEBPFCapabilities checks if the process has the necessary capabilities for eBPF
func hasEBPFCapabilities() bool {
	// Simple check - in production, you'd check specific capabilities
	// For now, check if we're running as root or in a privileged container
	return os.Geteuid() == 0
}

