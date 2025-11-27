# Multi-stage build for eBPF HTTP Profiler

# Stage 1: Build eBPF program
FROM ubuntu:22.04 AS ebpf-builder

RUN apt-get update && apt-get install -y \
    clang \
    llvm \
    libbpf-dev \
    linux-headers-generic \
    make \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Copy eBPF source
COPY pkg/ebpf/http_probe.c pkg/ebpf/vmlinux.h pkg/ebpf/Makefile ./

# Build eBPF program
RUN make http_probe.o

# Stage 2: Build Go application
FROM golang:1.21-alpine AS go-builder

RUN apk add --no-cache git

WORKDIR /app

# Copy go mod files
COPY go.mod ./
COPY go.sum* ./

# Download dependencies
RUN go mod download

# Copy source code
COPY cmd/ cmd/
COPY pkg/ pkg/

# Build profiler
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o profiler ./cmd/container-profiler

# Stage 3: Final image
FROM alpine:latest

RUN apk add --no-cache ca-certificates

# Copy eBPF program
COPY --from=ebpf-builder /build/http_probe.o /usr/local/lib/http_probe.o

# Copy Go binary
COPY --from=go-builder /app/profiler /usr/local/bin/profiler

# Create traces directory
RUN mkdir -p /traces

# Set environment variables
ENV OUTPUT_FILE=/traces/http-trace.txt

ENTRYPOINT ["/usr/local/bin/profiler"]

