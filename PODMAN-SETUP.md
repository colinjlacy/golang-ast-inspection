# Podman Setup Guide

Since you're using Podman on macOS, here's how to get the profiler working.

## Prerequisites

1. **Podman installed** ✅ (you have this)
2. **Podman machine running** (needed on macOS)
3. **podman-compose installed** (for docker-compose compatibility)

## Step 1: Check Podman Status

```bash
# Check if podman is installed
podman --version

# Check if podman machine is running
podman machine list

# If no machine exists, create one
podman machine init

# Start the machine
podman machine start
```

## Step 2: Install podman-compose

```bash
# Using pip
pip3 install podman-compose

# Or using brew
brew install podman-compose
```

## Step 3: Verify Setup

```bash
# Test podman
podman ps

# Test podman-compose
podman-compose --version
```

## Step 4: Build and Run

```bash
# Build containers (eBPF compilation happens inside Linux container)
make podman-build

# Start containers
make podman-up

# Check status
podman-compose -f container/docker-compose.yml ps

# View logs
podman-compose -f container/docker-compose.yml logs -f
```

## Step 5: Test

```bash
# Make HTTP requests
make test

# View captured traffic
cat container/traces/http-trace.txt
```

## Step 6: Stop

```bash
make podman-down
```

## Troubleshooting

### Podman Machine Not Running

**Error**: Cannot connect to Podman socket

**Fix**:
```bash
podman machine start
podman machine list  # Should show "Running"
```

### podman-compose Not Found

**Error**: `command not found: podman-compose`

**Fix**:
```bash
# Install with pip
pip3 install podman-compose

# Or with brew
brew install podman-compose
```

### Build Fails - BPF Headers Missing

**Error**: `'bpf/bpf_helpers.h' file not found`

**Explanation**: This is expected on macOS. The eBPF program **must** be compiled inside the Linux container, not on your Mac.

**Fix**: Use `make podman-build` which builds everything inside containers. Do NOT try to run `make build-ebpf` on macOS.

### Port Already in Use

**Error**: Port 8080 already allocated

**Fix**:
```bash
# Find what's using port 8080
lsof -i :8080

# Stop the conflicting service or change the port in docker-compose.yml
```

### No Traces Generated

**Possible causes**:
1. Containers not actually running
2. Test app not receiving requests
3. eBPF program not loading (check container logs)

**Debug**:
```bash
# Check containers are running
podman ps

# Check test-app logs
podman-compose -f container/docker-compose.yml logs test-app

# Check profiler logs
podman-compose -f container/docker-compose.yml logs profiler

# Manually test the app
curl http://localhost:8080/

# Check traces directory
ls -la container/traces/
```

## Alternative: Use Podman Directly (Without Compose)

If podman-compose is problematic, you can use podman directly:

```bash
# Create network
podman network create profiler-net

# Build images
podman build -f test/Dockerfile.test -t test-app .
podman build -f container/Dockerfile -t profiler .

# Run test app
podman run -d --name test-app \
  --network profiler-net \
  -p 8080:8080 \
  test-app

# Run profiler (with capabilities)
podman run -d --name profiler \
  --network profiler-net \
  --cap-add=SYS_ADMIN \
  --cap-add=NET_ADMIN \
  --cap-add=BPF \
  --security-opt apparmor=unconfined \
  --pid=host \
  -v ./container/traces:/traces:Z \
  profiler

# Check logs
podman logs -f profiler

# Make requests
curl http://localhost:8080/users

# View traces
cat container/traces/http-trace.txt

# Stop and remove
podman stop test-app profiler
podman rm test-app profiler
```

## Key Differences: Podman vs Docker

| Feature | Docker | Podman |
|---------|--------|--------|
| Daemon | Required | Daemonless |
| Root | Usually runs as root | Rootless by default |
| macOS | Docker Desktop | Podman machine (VM) |
| Compose | docker-compose | podman-compose |
| Compatibility | Native | Mostly compatible |

## Why eBPF on macOS is Special

**The Challenge**: 
- eBPF is a Linux kernel feature
- macOS doesn't have eBPF
- Can't compile eBPF programs on macOS

**The Solution**:
- Podman runs a Linux VM on macOS
- eBPF programs run inside that Linux VM
- Build everything inside containers (which run in the VM)

**Workflow**:
```
macOS → Podman → Linux VM → Container → eBPF → Profiling
```

## Next Steps

Once everything is working:

1. View real-time logs:
   ```bash
   podman-compose -f container/docker-compose.yml logs -f profiler
   ```

2. Make continuous requests:
   ```bash
   while true; do curl -s http://localhost:8080/users; sleep 1; done
   ```

3. Watch traces grow:
   ```bash
   tail -f container/traces/http-trace.txt
   ```

4. Experiment with your own apps:
   - Replace test-app with your container
   - Ensure it has the required capabilities
   - Mount the traces volume

## Quick Reference

```bash
# Start everything
make podman-up

# Test
make test

# View traces
cat container/traces/http-trace.txt

# View logs
podman-compose -f container/docker-compose.yml logs -f

# Stop everything
make podman-down

# Clean up
make clean
```

Ready to profile! 🚀

