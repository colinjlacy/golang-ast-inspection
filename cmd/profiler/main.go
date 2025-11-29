//go:build linux

package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/colinjlacy/golang-ast-inspection/pkg/profiler"
)

func main() {
	port := uint16(envAsInt("HTTP_PORT", 8080))
	output := envOrDefault("OUTPUT_PATH", "/var/log/ebpf_http_profiler.log")
	envOutput := envOrDefault("ENV_OUTPUT_PATH", "/var/log/ebpf_http_env.yaml")

	if err := profiler.NewRunner(port, output, envOutput).Run(context.Background()); err != nil {
		log.Fatalf("profiler failed: %v", err)
	}
}

func envAsInt(key string, def int) int {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return def
}

func envOrDefault(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}
