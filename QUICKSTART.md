# Quick Start Guide

Get the eBPF HTTP profiler running in containers in under 10 minutes.

## Prerequisites

- **Podman** (recommended for macOS) or Docker
- **podman-compose** (if using Podman)
- Git and Make

### macOS Users (Podman)

```bash
# Check Podman is running
podman machine list

# If not running, start it
podman machine start

# Install podman-compose
pip3 install podman-compose
# or
brew install podman-compose
```

See [PODMAN-SETUP.md](PODMAN-SETUP.md) for detailed Podman setup.

## Step 1: Build Container Images

**For Podman (macOS)**:
```bash
make podman-build
```

**For Docker (Linux)**:
```bash
make docker-build
```

**Important**: Do NOT try to build eBPF locally on macOS. The build happens inside the container on Linux.

## Step 2: Start Containers

**For Podman**:
```bash
make podman-up
```

**For Docker**:
```bash
make docker-up
```

This starts:
- `test-app` - HTTP server on port 8080
- `profiler` - eBPF profiler capturing traffic

## Step 4: Make Test Requests

```bash
make test
```

This sends HTTP requests to the test server:
- GET /
- GET /users
- GET /user/42
- GET /message?text=hello
- GET /health

## Step 5: View Captured Traffic

```bash
cat container/traces/http-trace.txt
```

You should see captured HTTP transactions with:
- Timestamps
- PIDs
- Request methods and URLs
- Response status codes
- Headers and body previews

## Step 6: View Live Logs

```bash
docker-compose -f container/docker-compose.yml logs -f profiler
```

## Step 7: Stop Everything

**For Podman**:
```bash
make podman-down
```

**For Docker**:
```bash
make docker-down
```

## Expected Output

The trace file should contain entries like:

```
Container HTTP Profiler Output
==============================

Profiler started at 2025-11-27 17:31:45

[2025-11-27 17:31:50.123] PID 15
  → HTTP GET /
     Host: localhost:8080
  ← Response 200 OK
     Content-Type: text/plain
     Body: Hello from test server!

[2025-11-27 17:31:51.234] PID 15
  → HTTP GET /users
     Host: localhost:8080
  ← Response 200 OK
     Content-Type: application/json
     Content-Length: 89
     Body: [{"id":1,"name":"Alice"},{"id":2,"name":"Bob"},{"id":3,"name":"Charlie"}]
```

## Troubleshooting

### Build Fails

**Error**: `clang: command not found`

**Fix**: Install build dependencies:
```bash
# Ubuntu/Debian
sudo apt-get install clang llvm libbpf-dev linux-headers-$(uname -r)

# Fedora
sudo dnf install clang llvm libbpf-devel kernel-devel
```

### eBPF Program Won't Load

**Error**: `failed to load eBPF program`

**Possible causes**:
1. eBPF not compiled: run `make build-ebpf`
2. Kernel too old: requires Linux 5.8+
3. BTF not available: check `/sys/kernel/btf/vmlinux`

**Fix**:
```bash
# Check kernel version
uname -r

# Check BTF
ls -l /sys/kernel/btf/vmlinux

# Rebuild eBPF
make clean
make build-ebpf
```

### No Traffic Captured

**Issue**: Trace file is empty or has no transactions

**Debug**:
```bash
# Check if test app is running
docker ps

# Check if test app is accessible
curl http://localhost:8080/

# Check profiler logs
docker-compose -f container/docker-compose.yml logs profiler

# Manually send requests
curl http://localhost:8080/users
```

### Permission Denied

**Error**: `Operation not permitted` or capability errors

**Fix**: Ensure containers have required capabilities:
```yaml
# In docker-compose.yml
cap_add:
  - SYS_ADMIN
  - NET_ADMIN
  - BPF
```

## Next Steps

Once the basic setup works:

1. **Test with your own app**: Replace test-app with your container
2. **Adjust eBPF hooks**: Modify `pkg/ebpf/http_probe.c` for different protocols
3. **Customize output**: Modify `pkg/output/writer.go` for different formats
4. **Add more protocols**: Extend HTTP parser to handle gRPC, DB protocols

## Using with Podman

Replace `docker` with `podman` or use the `docker` alias:

```bash
# Start with podman-compose
podman-compose -f container/docker-compose.yml up -d

# Or build and run manually
podman build -f container/Dockerfile -t profiler .
podman run --cap-add=SYS_ADMIN --cap-add=NET_ADMIN -v ./traces:/traces profiler
```

## Manual Build (Without Make)

```bash
# Build eBPF
cd pkg/ebpf
clang -g -O2 -c -target bpf -D__TARGET_ARCH_x86 -o http_probe.o http_probe.c
cd ../..

# Build Go
CGO_ENABLED=0 go build -o profiler ./cmd/container-profiler

# Build Docker
docker build -f container/Dockerfile -t profiler .
docker build -f test/Dockerfile.test -t test-app .
```

## Testing Individual Components

### Test eBPF Compilation

```bash
cd pkg/ebpf
make http_probe.o
file http_probe.o  # Should show "eBPF object file"
```

### Test Go Build

```bash
go build -o profiler ./cmd/container-profiler
./profiler  # Will fail without eBPF program, but should compile
```

### Test HTTP Parser

```bash
cd pkg/http
go test -v
```

### Test Stream Tracker

```bash
cd pkg/stream
go test -v
```

## Clean Slate

To start over completely:

```bash
# Stop containers
make docker-down

# Clean everything
make clean

# Remove Docker images
docker-compose -f container/docker-compose.yml down --rmi all

# Rebuild from scratch
make build-all
make docker-build
make docker-up
```

## Getting Help

If you encounter issues:

1. Check kernel version: `uname -r` (need 5.8+)
2. Check eBPF support: `ls /sys/kernel/btf/`
3. Check Docker/Podman version
4. Review container logs
5. Check file permissions in `container/traces/`

Ready to profile! 🚀

