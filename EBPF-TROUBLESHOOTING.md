# eBPF Troubleshooting Guide

## Error: "function not implemented"

```
failed to attach to sys_enter_write: opening tracepoint perf event: function not implemented
```

### Cause

This error means the kernel doesn't support the required eBPF features or they're not accessible in the container.

### Common Reasons

1. **Podman VM kernel is too old** (< 5.8)
2. **Tracepoints not enabled** in kernel config
3. **Debug filesystem not mounted**
4. **Insufficient capabilities**

## Diagnostic Steps

### 1. Check Podman VM Kernel Version

```bash
podman machine ssh
uname -r
# Need 5.8 or newer
```

### 2. Check If Tracepoints Are Available

```bash
podman exec -it profiler sh
ls /sys/kernel/debug/tracing/events/syscalls/
# Should show sys_enter_write and sys_exit_read
```

### 3. Check BPF Filesystem

```bash
podman machine ssh
mount | grep bpf
# Should show bpffs mounted
```

### 4. Check Container Capabilities

```bash
podman exec -it profiler sh
cat /proc/self/status | grep Cap
```

## Solutions

### Solution 1: Update Podman Machine

```bash
# Stop current machine
podman machine stop

# Remove old machine
podman machine rm

# Create new machine with latest Fedora
podman machine init --disk-size 50 --memory 4096

# Start it
podman machine start
```

### Solution 2: Enable Privileged Mode

Already added to docker-compose.yml:
```yaml
privileged: true
```

### Solution 3: Mount Required Filesystems

Already added:
```yaml
volumes:
  - /sys/kernel/debug:/sys/kernel/debug:rw
  - /sys/kernel/tracing:/sys/kernel/tracing:rw
  - /sys/fs/bpf:/sys/fs/bpf:rw
```

### Solution 4: Check Podman Machine Settings

```bash
# SSH into Podman machine
podman machine ssh

# Check if debugfs is mounted
mount | grep debugfs

# If not, mount it
sudo mount -t debugfs debugfs /sys/kernel/debug

# Check kernel config
zcat /proc/config.gz | grep CONFIG_DEBUG_FS
# Should be =y

zcat /proc/config.gz | grep CONFIG_TRACEPOINTS
# Should be =y
```

## Alternative: Skip eBPF for MVP

If eBPF continues to be problematic, you can:

### Option A: Use Simplified Approach

Create a version that just logs HTTP requests without eBPF:

```bash
# Modify test app to log requests
# Much simpler but requires app modification
```

### Option B: Use tcpdump in Container

```bash
# Run tcpdump in sidecar container
podman run --net=container:test-app \
  nicolaka/netshoot \
  tcpdump -i any -A 'tcp port 8080'
```

### Option C: HTTP Proxy Approach

Run a transparent proxy that logs all traffic:
- No eBPF required
- Works everywhere
- Requires app to use proxy

## Testing eBPF Availability

Run this diagnostic container:

```bash
podman run --rm --privileged \
  -v /sys/kernel/debug:/sys/kernel/debug:rw \
  -v /sys/kernel/tracing:/sys/kernel/tracing:rw \
  ubuntu:22.04 \
  bash -c "
    apt-get update && apt-get install -y bpftool &&
    bpftool feature probe kernel &&
    ls -la /sys/kernel/debug/tracing/events/syscalls/ | head -20
  "
```

This will show:
- Available BPF features
- Available tracepoints
- What's accessible

## Expected Working Environment

For eBPF to work fully, you need:

✅ Linux kernel 5.8+  
✅ CONFIG_DEBUG_FS=y  
✅ CONFIG_TRACEPOINTS=y  
✅ CONFIG_BPF=y  
✅ CONFIG_BPF_EVENTS=y  
✅ debugfs mounted  
✅ bpffs mounted  
✅ Container running as privileged or with CAP_BPF  

## Next Steps

1. **Check kernel version** in Podman VM
2. **Try the diagnostic container** above
3. **Update docker-compose** (already done)
4. **Rebuild and retry**: `make podman-down && make podman-up`

If eBPF still doesn't work, we can implement a simpler non-eBPF version that's more compatible with containerized environments.

