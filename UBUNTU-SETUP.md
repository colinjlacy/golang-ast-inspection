# Ubuntu Setup Guide

Running the eBPF HTTP Profiler on Ubuntu with Docker is the **recommended approach** - it will work much better than macOS with Podman.

## Why Ubuntu is Better

| Feature | macOS + Podman | Ubuntu + Docker |
|---------|----------------|-----------------|
| eBPF Support | ⚠️ Limited (VM) | ✅ Native |
| Kernel Access | ⚠️ Restricted | ✅ Direct |
| Performance | Good | ✅ Excellent |
| Tracepoints | ⚠️ May not work | ✅ Work perfectly |
| Setup Complexity | High | ✅ Simple |

## Prerequisites

### 1. Ubuntu Version

Recommended: **Ubuntu 22.04 LTS** or newer

```bash
lsb_release -a
# Should show Ubuntu 22.04 or higher
```

### 2. Kernel Version

Need: **5.8 or newer** (Ubuntu 22.04 has 5.15+)

```bash
uname -r
# Should show 5.8 or higher
```

### 3. Docker Installed

```bash
# Check if Docker is installed
docker --version

# If not installed, install Docker:
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# Add your user to docker group (to avoid sudo)
sudo usermod -aG docker $USER
newgrp docker  # Or logout and login
```

### 4. Docker Compose

```bash
# Check version
docker compose version

# Docker Compose is usually included with Docker Desktop
# If you need to install it separately:
sudo apt-get update
sudo apt-get install docker-compose-plugin
```

## Quick Start

### 1. Clone the Repository

```bash
cd /path/to/your/workspace
# Assuming you already have the code
cd golang-ast-inspection
```

### 2. Build the Containers

```bash
make docker-build
```

This will:
- Compile the eBPF program (in Ubuntu container)
- Build the Go profiler
- Build the test HTTP server
- Create Docker images

### 3. Start the Containers

```bash
make docker-up
```

This starts:
- Test HTTP server on port 8080
- eBPF profiler capturing traffic

### 4. Make Test Requests

```bash
make test
```

### 5. View Captured Traffic

```bash
cat container/traces/http-trace.txt
```

### 6. View Live Logs

```bash
docker compose -f container/docker-compose.yml logs -f profiler
```

### 7. Stop Everything

```bash
make docker-down
```

## Expected Output

### Successful Startup

```
Container HTTP Profiler starting...
Output file: /traces/http-trace.txt
Loading eBPF program...
eBPF program loaded and attached
Profiler running. Press Ctrl+C to stop.
```

### Captured HTTP Traffic

```
Container HTTP Profiler Output
==============================

Profiler started at 2025-11-27 18:45:12

[2025-11-27 18:45:15.123] PID 42
  → HTTP GET /
     Host: localhost:8080
  ← Response 200 OK
     Content-Type: text/plain
     Body: Hello from test server!

[2025-11-27 18:45:16.234] PID 42
  → HTTP GET /users
     Host: localhost:8080
  ← Response 200 OK
     Content-Type: application/json
     Content-Length: 89
     Body: [{"id":1,"name":"Alice"},{"id":2,"name":"Bob"},{"id":3,"name":"Charlie"}]
```

## System Requirements

### Minimum

- Ubuntu 20.04+
- Kernel 5.8+
- 2 CPU cores
- 4 GB RAM
- 10 GB disk space

### Recommended

- Ubuntu 22.04 LTS
- Kernel 5.15+
- 4 CPU cores
- 8 GB RAM
- 20 GB disk space

## Required Kernel Features

Check if your kernel has the required features:

```bash
# Check BPF support
cat /boot/config-$(uname -r) | grep CONFIG_BPF
# Should show CONFIG_BPF=y

# Check tracepoints
cat /boot/config-$(uname -r) | grep CONFIG_TRACEPOINTS
# Should show CONFIG_TRACEPOINTS=y

# Check debug filesystem
cat /boot/config-$(uname -r) | grep CONFIG_DEBUG_FS
# Should show CONFIG_DEBUG_FS=y
```

If any are missing, you need a different kernel. Ubuntu 22.04 has all of these by default.

## Debugging

### Check Docker is Running

```bash
docker ps
# Should show running containers or empty list
```

### Check Kernel Capabilities

```bash
# Check if debugfs is mounted
mount | grep debugfs
# Should show: debugfs on /sys/kernel/debug

# If not mounted:
sudo mount -t debugfs debugfs /sys/kernel/debug
```

### Check BPF Filesystem

```bash
# Check if bpffs is mounted
mount | grep bpf
# Should show: bpffs on /sys/fs/bpf

# If not mounted:
sudo mount -t bpf bpf /sys/fs/bpf
```

### Verify eBPF Works

Run a simple BPF test:

```bash
# Install bpftool
sudo apt-get install linux-tools-common linux-tools-$(uname -r)

# List BPF programs (should work)
sudo bpftool prog list

# Check available features
sudo bpftool feature probe kernel
```

### Check Container Logs

```bash
# View profiler logs
docker compose -f container/docker-compose.yml logs profiler

# View test app logs
docker compose -f container/docker-compose.yml logs test-app

# Follow logs in real-time
docker compose -f container/docker-compose.yml logs -f
```

## Common Issues

### Issue: Permission Denied

```bash
# Add your user to docker group
sudo usermod -aG docker $USER
newgrp docker
```

### Issue: Port 8080 Already in Use

```bash
# Find what's using the port
sudo lsof -i :8080

# Kill it or change the port in docker-compose.yml
```

### Issue: eBPF Program Won't Load

```bash
# Check kernel version
uname -r  # Need 5.8+

# Check if running as privileged
docker compose -f container/docker-compose.yml config | grep privileged
# Should show: privileged: true
```

### Issue: No Traces Captured

```bash
# Check if test app is running
curl http://localhost:8080/

# Check traces directory permissions
ls -la container/traces/

# Try making requests manually
curl http://localhost:8080/users
```

## Performance Tuning

### For Production Use

1. **Remove privileged mode**: Use specific capabilities instead
   ```yaml
   cap_add:
     - BPF
     - PERFMON
     - SYS_RESOURCE
   ```

2. **Limit resources**:
   ```yaml
   deploy:
     resources:
       limits:
         cpus: '2'
         memory: 1G
   ```

3. **Use volume for traces** (already configured):
   ```yaml
   volumes:
     - ./traces:/traces
   ```

## Testing the Setup

### 1. Basic Test

```bash
# Build
make docker-build

# Start
make docker-up

# Test
make test

# Check output
cat container/traces/http-trace.txt
```

### 2. Continuous Test

```bash
# In one terminal, watch logs
docker compose -f container/docker-compose.yml logs -f profiler

# In another terminal, make requests
while true; do
  curl -s http://localhost:8080/users > /dev/null
  sleep 1
done
```

### 3. Performance Test

```bash
# Install apache bench
sudo apt-get install apache2-utils

# Run load test
ab -n 1000 -c 10 http://localhost:8080/

# Check how many transactions were captured
grep "HTTP GET" container/traces/http-trace.txt | wc -l
```

## Architecture Support

The code works on both architectures:

- ✅ **x86_64** (Intel/AMD)
- ✅ **ARM64** (aarch64)

Docker will automatically use the correct architecture.

## Next Steps

Once basic profiling works:

1. **Profile your own apps**: Replace test-app with your container
2. **Add more protocols**: Extend the HTTP parser
3. **Export to OpenTelemetry**: Add tracing integration
4. **Add metrics**: Export to Prometheus
5. **Scale up**: Profile multiple containers

## Security Considerations

Running privileged containers is a security risk. For production:

1. Use minimal capabilities (not `privileged: true`)
2. Run in isolated environment
3. Limit network access
4. Use read-only root filesystem where possible
5. Monitor the profiler itself

## Support

If you encounter issues:

1. Check kernel version: `uname -r`
2. Check Docker version: `docker --version`
3. Check logs: `docker compose logs profiler`
4. Run diagnostics: `sudo bpftool feature probe kernel`
5. See `EBPF-TROUBLESHOOTING.md` for detailed help

## Summary

✅ Ubuntu with Docker is the **ideal environment**  
✅ eBPF works **natively** (no VM overhead)  
✅ All features **fully supported**  
✅ **Production-ready** setup  

This should work out of the box on Ubuntu 22.04+!

