# macOS Limitations and Solutions

## The Challenge

eBPF is a **Linux kernel feature** that doesn't exist on macOS. This creates several limitations:

### ❌ What Doesn't Work on macOS

1. **Local eBPF compilation** - Can't compile `.c` to `.o` on macOS
2. **Local eBPF execution** - Can't run eBPF programs on macOS kernel
3. **Direct profiling** - Can't profile macOS applications directly

### ✅ What DOES Work

The solution is to use **Podman** (or Docker Desktop), which runs a Linux VM:

```
macOS Host
  └── Podman Machine (Linux VM)
      └── Container (Linux)
          └── eBPF Program ✅ Works!
```

## How It Works

1. **Podman Machine**: Creates a lightweight Linux VM on macOS
2. **Containers Run in VM**: Your containers run in that Linux VM
3. **eBPF in Linux**: eBPF programs run in the Linux kernel inside the VM
4. **Profiling Works**: You can profile applications running in containers

## Setup Steps

### 1. Install Podman

```bash
brew install podman
```

### 2. Initialize Podman Machine

```bash
# Create the VM
podman machine init

# Start the VM
podman machine start

# Verify it's running
podman machine list
# Should show: Running
```

### 3. Install podman-compose

```bash
# Using pip
pip3 install podman-compose

# Or using brew
brew install podman-compose
```

### 4. Build Containers (NOT Local)

```bash
# This builds INSIDE the Linux VM
make podman-build
```

**DO NOT run**: `make build-ebpf` (will fail on macOS)

### 5. Run and Test

```bash
# Start containers
make podman-up

# Make test requests
make test

# View traces
cat container/traces/http-trace.txt

# Stop
make podman-down
```

## Why Previous Approach Failed

The original macOS approach (DTrace) failed because:
- ❌ System Integrity Protection (SIP) blocks DTrace
- ❌ Corporate profile prevents disabling SIP
- ❌ tcpdump parsing was unreliable

## Why Container Approach Works

The eBPF container approach works because:
- ✅ Runs in Linux VM (no SIP restrictions)
- ✅ eBPF is designed for production use
- ✅ Kernel-level capture is reliable
- ✅ Can profile any containerized app

## Architecture

### Local Development on macOS

```
┌─────────────────────────────────┐
│        macOS Host               │
│                                 │
│  ┌───────────────────────────┐ │
│  │  Podman Machine (Linux)   │ │
│  │                           │ │
│  │  ┌─────────────────────┐ │ │
│  │  │   Container         │ │ │
│  │  │   - Application     │ │ │
│  │  │   - eBPF Profiler   │ │ │
│  │  │   - Trace Output    │ │ │
│  │  └─────────────────────┘ │ │
│  │                           │ │
│  └───────────────────────────┘ │
│                                 │
│  Volume Mount:                  │
│  ./container/traces ←─────────┐ │
└─────────────────────────────────┘
```

### Production on Linux

```
┌─────────────────────────────────┐
│      Linux Host                 │
│                                 │
│  ┌─────────────────────┐       │
│  │   Container         │       │
│  │   - Application     │       │
│  │   - eBPF Profiler   │       │
│  │   - Trace Output    │       │
│  └─────────────────────┘       │
│                                 │
│  Volume Mount:                  │
│  /traces ←────────────────────┐ │
└─────────────────────────────────┘
```

## Limitations on macOS

### Performance

- **VM Overhead**: Slight overhead from running in VM
- **Not as Fast**: Not as fast as native Linux
- **Still Good**: Still much better than no profiling at all

### Profiling Scope

- **Container Only**: Can only profile apps running in containers
- **Not Native Apps**: Can't profile native macOS applications
- **That's OK**: Most modern apps run in containers anyway

### Development Workflow

- **No Local Build**: Can't build eBPF locally
- **Container Build**: Must build in container
- **Slower Iteration**: Slightly slower than native builds

## Best Practices for macOS Development

### 1. Use Podman Machine

```bash
# Always check machine is running first
podman machine list

# Start if needed
podman machine start
```

### 2. Build in Containers

```bash
# Use container build targets
make podman-build  # ✅ Good

# Don't try local builds
make build-ebpf    # ❌ Will fail on macOS
```

### 3. Develop Iteratively

```bash
# Make changes to Go code
vim pkg/http/parser.go

# Rebuild just the container
make podman-build

# Restart to test
make podman-down
make podman-up
```

### 4. View Logs

```bash
# Watch profiler logs in real-time
podman-compose -f container/docker-compose.yml logs -f profiler
```

### 5. Debug Issues

```bash
# Check containers are running
podman ps

# Exec into profiler container
podman exec -it profiler-profiler-1 sh

# Check eBPF program exists
ls -l /usr/local/lib/http_probe.o

# Check traces directory
ls -l /traces/
```

## Common Errors and Solutions

### Error: "bpf/bpf_helpers.h not found"

**Cause**: Trying to build eBPF on macOS

**Solution**: Use `make podman-build` instead of `make build-ebpf`

### Error: "Cannot connect to Docker daemon"

**Cause**: Podman machine not running

**Solution**:
```bash
podman machine start
```

### Error: "podman-compose: command not found"

**Cause**: podman-compose not installed

**Solution**:
```bash
pip3 install podman-compose
```

### Error: No traces generated

**Possible Causes**:
1. Containers not running in VM
2. eBPF program failed to load
3. No HTTP traffic being made

**Debug**:
```bash
# Check containers
podman ps

# Check logs
podman-compose logs profiler

# Make test request
curl http://localhost:8080/users
```

## Comparison: macOS vs Linux

| Feature | macOS (Podman) | Linux (Native) |
|---------|----------------|----------------|
| Setup | Complex (VM) | Simple |
| Performance | Good | Excellent |
| eBPF Support | Via VM | Native |
| Build Speed | Slower | Faster |
| Profiling Scope | Containers | Everything |
| Production Use | No | Yes |

## When to Use What

### Use macOS + Podman When:
- ✅ Developing the profiler
- ✅ Testing container setups
- ✅ Learning eBPF
- ✅ Profiling containerized apps

### Use Linux When:
- ✅ Production deployment
- ✅ High-performance needs
- ✅ Profiling native apps
- ✅ Advanced eBPF features

## Alternatives for macOS

If you need to profile native macOS apps (not in containers):

1. **Instruments** - Apple's profiling tool
2. **dtrace** (if SIP disabled) - Kernel tracing
3. **Network.framework** - API-level tracing
4. **Proxy approach** - HTTP proxy for traffic capture
5. **Application instrumentation** - Modify your app

## Summary

✅ **You CAN**: Profile containers on macOS using Podman + eBPF
❌ **You CANNOT**: Profile native macOS apps with eBPF
✅ **Best for**: Container-based development and testing
🚀 **Production**: Deploy to Linux for best results

The container approach works great for development and is identical to how it runs in production!

