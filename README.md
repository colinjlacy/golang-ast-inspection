# eBPF Container HTTP Profiler

An eBPF-based HTTP profiler that runs inside OCI containers, capturing HTTP/1.x traffic and writing human-readable traces to a file.

## Features

- ✅ Zero-instrumentation profiling using eBPF
- ✅ Captures HTTP/1.x requests and responses  
- ✅ Works inside Docker/Podman containers
- ✅ Minimal performance overhead
- ✅ Human-readable trace output

## Requirements

- **Container Runtime**: Docker or Podman
- **Linux Kernel**: 5.8+ with BTF support
- **Capabilities**: `CAP_SYS_ADMIN`, `CAP_NET_ADMIN`, or `CAP_BPF`
- **Build Tools**:
  - Go 1.21+
  - clang/LLVM (for eBPF compilation)
  - libbpf-dev
  - linux-headers

## Quick Start

### ⭐ Recommended: Ubuntu with Docker

**This is the best environment** - eBPF works natively without any restrictions.

```bash
# Build
make docker-build

# Start
make docker-up

# Test
make test

# View traces
cat container/traces/http-trace.txt

# Stop
make docker-down
```

See **[UBUNTU-SETUP.md](UBUNTU-SETUP.md)** for complete Ubuntu setup guide.

### Alternative: macOS with Podman

⚠️ **Limited eBPF support** - works but has restrictions due to running in a VM.

```bash
# Prerequisites
podman machine start
pip3 install podman-compose

# Build (eBPF compilation happens in container)
make podman-build

# Start
make podman-up

# Test
make test

# View traces
cat container/traces/http-trace.txt

# Stop
make podman-down
```

See [PODMAN-SETUP.md](PODMAN-SETUP.md) and [MACOS-LIMITATIONS.md](MACOS-LIMITATIONS.md) for details.

**Important**: eBPF must be built inside a Linux container, never locally on macOS.

## Project Structure

```
.
├── cmd/
│   └── container-profiler/    # Main profiler application
│       └── main.go
├── pkg/
│   ├── ebpf/                  # eBPF program and loader
│   │   ├── http_probe.c       # eBPF C program
│   │   ├── vmlinux.h          # Kernel type definitions
│   │   ├── loader.go          # Go eBPF loader
│   │   └── Makefile           # eBPF build
│   ├── capture/               # Event types
│   │   └── events.go
│   ├── stream/                # TCP stream reassembly
│   │   └── tracker.go
│   ├── http/                  # HTTP parser
│   │   └── parser.go
│   └── output/                # Output writer
│       └── writer.go
├── container/
│   ├── Dockerfile             # Profiler container image
│   └── docker-compose.yml     # Test setup
├── test/
│   ├── app/
│   │   └── simple-http-app.go # Test HTTP server
│   └── Dockerfile.test        # Test app container
├── Makefile                   # Build automation
├── go.mod                     # Go dependencies
└── README.md                  # This file
```

## How It Works

```
Container Process → Socket I/O → eBPF Hooks → Ring Buffer → Go App → HTTP Parser → File Output
```

1. **eBPF Program** hooks into `tcp_sendmsg` and `tcp_recvmsg` kernel functions
2. **Ring Buffer** efficiently transfers data from kernel to userspace
3. **Go Application** reads events and reassembles TCP streams
4. **HTTP Parser** extracts HTTP requests and responses
5. **File Writer** outputs human-readable traces

## Usage

### With Docker

```bash
docker run --cap-add=SYS_ADMIN --cap-add=NET_ADMIN \
  -v $(pwd)/traces:/traces \
  your-profiler-image
```

### With Podman

```bash
podman run --cap-add=SYS_ADMIN --cap-add=NET_ADMIN \
  -v $(pwd)/traces:/traces \
  your-profiler-image
```

### Environment Variables

- `OUTPUT_FILE` - Output file path (default: `/traces/http-trace.txt`)

## Output Format

```
Container HTTP Profiler Output
==============================

[2025-11-27 17:31:45.123] PID 4213
  → HTTP GET /users
     Host: localhost:8080
  ← Response 200 OK
     Content-Type: application/json
     Body: [{"id":1,"name":"Alice"},{"id":2,"name":"Bob"}]

[2025-11-27 17:31:45.456] PID 4213
  → HTTP POST /user
     Host: localhost:8080
     Content-Type: application/json
     Body: {"name":"New User"}
  ← Response 201 Created
     Content-Type: application/json
     Body: {"id":4,"name":"New User"}
```

## Development

### Build eBPF Program Only

```bash
make build-ebpf
```

### Build Go Profiler Only

```bash
make build-profiler
```

### Clean Build Artifacts

```bash
make clean
```

### View Logs

```bash
docker-compose -f container/docker-compose.yml logs -f profiler
```

## Architecture

### eBPF Program (`pkg/ebpf/http_probe.c`)

- Hooks `tcp_sendmsg` for outbound HTTP requests
- Hooks `tcp_cleanup_rbuf` for inbound HTTP responses
- Filters for HTTP traffic (checks for HTTP methods/responses)
- Sends events via ring buffer

### Go Profiler (`cmd/container-profiler/main.go`)

- Loads and attaches eBPF program
- Reads events from ring buffer
- Tracks TCP streams by (PID, FD)
- Parses HTTP/1.x protocol
- Writes formatted output

## Limitations (MVP)

- ❌ HTTP/1.x only (no HTTP/2 or HTTP/3)
- ❌ Plaintext only (no HTTPS/TLS decryption)
- ❌ Single container profiling
- ❌ No request body capture for large payloads
- ❌ Basic stream reassembly (may miss fragmented requests)

## Troubleshooting

### "failed to load eBPF program"

**Cause**: eBPF program not compiled or not found

**Fix**:
```bash
make build-ebpf
# Ensure http_probe.o exists in pkg/ebpf/
```

### "container does not have required capabilities"

**Cause**: Missing eBPF capabilities

**Fix**: Add capabilities when running:
```bash
docker run --cap-add=SYS_ADMIN --cap-add=NET_ADMIN ...
```

### "No HTTP traffic captured"

**Possible causes**:
1. Test app not making HTTP requests
2. eBPF hooks not attached
3. Traffic not matching HTTP patterns

**Debug**:
```bash
# Check profiler logs
docker-compose logs profiler

# Verify test app is running
curl http://localhost:8080/

# Check traces directory
ls -la container/traces/
```

### BTF Not Available

**Cause**: Kernel doesn't have BTF enabled

**Fix**: Use a kernel with BTF support (5.8+) or generate vmlinux.h:
```bash
bpftool btf dump file /sys/kernel/btf/vmlinux format c > pkg/ebpf/vmlinux.h
```

## Future Enhancements

- [ ] HTTP/2 and gRPC support
- [ ] Multiple protocol detection
- [ ] JSON/structured output
- [ ] OpenTelemetry integration
- [ ] TLS/HTTPS support (with SSL key logging)
- [ ] Multi-container profiling
- [ ] Real-time streaming output
- [ ] Web UI dashboard
- [ ] Database protocol support (Postgres, MySQL, Redis)

## Contributing

This is an MVP implementation. Contributions welcome!

## License

See LICENSE file.

## References

- [eBPF Documentation](https://ebpf.io/)
- [cilium/ebpf Library](https://github.com/cilium/ebpf)
- [BPF Kernel Documentation](https://www.kernel.org/doc/html/latest/bpf/)
- [libbpf](https://github.com/libbpf/libbpf)
