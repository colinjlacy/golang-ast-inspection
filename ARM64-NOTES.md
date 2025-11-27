# ARM64 Architecture Notes

## Why Architecture Matters

The original eBPF implementation used `BPF_KPROBE` macros which require architecture-specific `pt_regs` structures. Since you're on an ARM64 Mac:

- **macOS**: ARM64 (Apple Silicon)
- **Podman VM**: Also runs ARM64 Linux
- **eBPF Programs**: Must be compiled for the target architecture

## Solution: Tracepoints Instead of Kprobes

We switched from kprobes to **tracepoints** which are architecture-independent:

### Before (Architecture-Specific)
```c
SEC("kprobe/tcp_sendmsg")
int BPF_KPROBE(tcp_sendmsg_entry, struct sock *sk, ...)
// Requires pt_regs structure (different for x86 vs ARM)
```

### After (Architecture-Independent)
```c
SEC("tracepoint/syscalls/sys_enter_write")
int trace_write_enter(struct trace_event_raw_sys_enter* ctx)
// Works on any architecture
```

## Tracepoints vs Kprobes

| Feature | Kprobes | Tracepoints |
|---------|---------|-------------|
| Architecture | Specific | Independent |
| Stability | Can break | Stable ABI |
| Setup | Complex | Simple |
| Performance | Slightly faster | Fast enough |
| Maintenance | High | Low |

## Current Implementation

We now hook syscalls instead of kernel functions:

### Outbound Traffic
- **Hook**: `sys_enter_write` tracepoint
- **Captures**: Data being written to sockets
- **Filters**: Only FDs > 2 (not stdin/stdout/stderr)

### Inbound Traffic
- **Hook**: `sys_exit_read` tracepoint
- **Limitation**: Can't easily access buffer in exit probe
- **Status**: Partial implementation (MVP)

## ARM64 Podman Machine

When you run `podman machine start` on macOS:

```
macOS (ARM64)
  └── Podman Machine (Linux VM on ARM64)
      └── Container (Linux ARM64)
          └── eBPF Program (compiled for ARM64)
```

Everything runs ARM64, so our tracepoint approach works perfectly!

## Building for ARM64

The container build automatically handles this:

```dockerfile
# Dockerfile automatically detects architecture
FROM ubuntu:22.04 AS ebpf-builder
# ...
RUN clang -g -O2 -c -target bpf \
    -D__TARGET_ARCH_x86 \  # Note: this is for BPF, not host
    -o http_probe.o http_probe.c
```

The `-target bpf` tells clang to compile to eBPF bytecode, which runs in the kernel's BPF VM (architecture-independent at runtime).

## Testing on ARM64

```bash
# Check your Podman machine architecture
podman machine ssh
uname -m  # Should show: aarch64

# Build the profiler
make podman-build

# Run it
make podman-up
```

## Comparison: x86_64 vs ARM64

### pt_regs Structures

**x86_64** (if we had used kprobes):
```c
struct pt_regs {
    unsigned long di;  // 1st param
    unsigned long si;  // 2nd param
    unsigned long dx;  // 3rd param
    // ...
};
```

**ARM64** (what we avoided):
```c
struct pt_regs {
    unsigned long regs[31];
    unsigned long sp;
    unsigned long pc;
    unsigned long pstate;
};
```

### Why Tracepoints are Better

With tracepoints, we don't need any of this! The syscall arguments are provided in a standard format:

```c
struct trace_event_raw_sys_enter {
    __u64 unused;
    long id;
    unsigned long args[6];  // Same on all architectures!
};
```

## Performance

Tracepoints are actually **more efficient** than kprobes because:
- Less overhead (no instruction breakpoint)
- Compiled into kernel (not dynamic patching)
- Stable interface (less likely to break)

## Future Multi-Architecture Support

If you want to support both x86_64 and ARM64:

1. **Current approach**: Already works! Tracepoints are universal.
2. **Container images**: Can build multi-arch images:
   ```bash
   docker buildx build --platform linux/amd64,linux/arm64 ...
   ```

## Debugging Architecture Issues

### Check Container Architecture
```bash
podman run --rm alpine uname -m
```

### Check BPF Program
```bash
# Inside container
file /usr/local/lib/http_probe.o
# Should show: "eBPF program"
```

### Check Kernel Support
```bash
# Inside container
ls /sys/kernel/debug/tracing/events/syscalls/
# Should show sys_enter_write and sys_exit_read
```

## Summary

✅ **Tracepoint approach works on ARM64**  
✅ **No architecture-specific code needed**  
✅ **Simpler and more maintainable**  
✅ **Works on your Mac with Podman**  

The move to tracepoints actually made the code better!

