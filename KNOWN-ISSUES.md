# Known Issues and Limitations

## eBPF Implementation Status

### ⚠️ Current MVP Status

The eBPF program is a **proof-of-concept implementation** with several limitations:

1. **Data Capture Not Production-Ready**
   - Simplified iovec reading may miss data
   - May not capture all HTTP traffic correctly
   - Limited to 16KB per event

2. **Hook Points May Need Tuning**
   - `tcp_sendmsg` and `tcp_cleanup_rbuf` hooks are basic
   - May need kernel-version-specific adjustments
   - Not all kernels expose these functions

3. **Type Definitions Minimal**
   - Using simplified `vmlinux.h`
   - May have type mismatches on some kernels
   - Production needs kernel-specific vmlinux.h

## Build Issues

### eBPF Compilation Errors

**Symptom**: Build fails with type definition errors

**Cause**: Minimal vmlinux.h doesn't match all kernel versions

**Solutions**:

**Option 1**: Generate proper vmlinux.h (Linux only)
```bash
# On Linux system with your kernel
sudo bpftool btf dump file /sys/kernel/btf/vmlinux format c > pkg/ebpf/vmlinux.h
```

**Option 2**: Use pre-generated headers
```bash
# Install linux-headers for your kernel
sudo apt-get install linux-headers-$(uname -r)  # Ubuntu/Debian
sudo dnf install kernel-devel  # Fedora
```

**Option 3**: Simplify the eBPF program further (remove problematic hooks)

### Container Build Fails

**Symptom**: `make podman-build` fails during eBPF compilation

**Common Causes**:
1. Missing BPF headers in container
2. Clang version incompatibility
3. Kernel version mismatch

**Fix**: Update Dockerfile to use Ubuntu 22.04+ with:
```dockerfile
RUN apt-get update && apt-get install -y \
    clang-14 \
    llvm-14 \
    libbpf-dev \
    linux-headers-generic
```

## Runtime Issues

### eBPF Program Won't Load

**Symptom**: Profiler starts but reports "failed to load eBPF program"

**Possible Causes**:
1. Kernel version too old (< 5.8)
2. BTF not enabled
3. Missing capabilities
4. Program verification failed

**Debug Steps**:

```bash
# Check kernel version
uname -r  # Need 5.8+

# Check BTF support
ls /sys/kernel/btf/vmlinux  # Should exist

# Check BPF features
cat /proc/sys/kernel/unprivileged_bpf_disabled  # Should be 0 or 1

# Try loading manually
sudo bpftool prog load http_probe.o /sys/fs/bpf/http_probe
```

### No Events Captured

**Symptom**: Profiler runs but no HTTP transactions in output

**Possible Causes**:
1. eBPF hooks not triggering
2. HTTP detection failing
3. Data not being read correctly
4. Ring buffer not working

**Debug Steps**:

```bash
# Check if BPF programs are loaded
sudo bpftool prog list | grep http

# Check ring buffer map
sudo bpftool map list | grep events

# Check profiler logs
podman logs profiler-container-name

# Try simpler test
curl http://localhost:8080/
```

## Workarounds

### Use Userspace Capture Instead

If eBPF is too problematic, you can use a simpler userspace approach:

**Option A**: Use tcpdump in container
```bash
tcpdump -i any -A 'tcp port 8080' | grep -E '(GET|POST|HTTP)'
```

**Option B**: Use a Go-based packet capture (gopacket)
- Add `github.com/google/gopacket` dependency
- Capture at userspace level
- More reliable but less efficient

**Option C**: HTTP proxy approach
- Run local proxy in container
- Intercept all HTTP traffic
- No eBPF required
- Requires app configuration

### Simplified eBPF Version

Consider using a simpler tracepoint instead of kprobes:

```c
// Instead of kprobe on tcp_sendmsg, use:
SEC("tracepoint/syscalls/sys_enter_write")
int trace_write_entry(struct trace_event_raw_sys_enter* ctx) {
    // Simpler but less specific
}
```

## Production Recommendations

For production use, consider:

1. **Use Established Tools**:
   - [Pixie](https://px.dev/) - Full observability platform
   - [Falco](https://falco.org/) - Security monitoring with eBPF
   - [Cilium Hubble](https://github.com/cilium/hubble) - Network observability

2. **Use libbpf-bootstrap Template**:
   - Proper CO-RE support
   - Better kernel compatibility
   - Production-tested patterns

3. **Professional eBPF Development**:
   - Hire eBPF specialists
   - Use [bpftrace](https://github.com/iovisor/bpftrace) for prototyping
   - Test on multiple kernel versions

## Getting Help

### eBPF Verification Errors

If you see "verifier" errors:
```bash
# Get detailed verifier log
sudo bpftool prog load http_probe.o /sys/fs/bpf/test 2>&1 | less
```

Common issues:
- Unbounded loops (add loop limit)
- Invalid memory access (check bounds)
- Stack size exceeded (reduce local variables)

### Kernel Compatibility

Check kernel BPF features:
```bash
# List available BPF features
sudo bpftool feature

# Check specific helpers
sudo bpftool feature probe kernel helper
```

## Contributing

If you fix any of these issues, please contribute back:
1. Test on multiple kernel versions
2. Document your changes
3. Add error handling
4. Submit PR with test results

## Status Summary

| Component | Status | Production Ready |
|-----------|--------|------------------|
| Go Code | ✅ Complete | ✅ Yes |
| HTTP Parser | ✅ Complete | ✅ Yes |
| Stream Tracker | ✅ Complete | ✅ Yes |
| Output Writer | ✅ Complete | ✅ Yes |
| eBPF Program | ⚠️ MVP | ❌ No |
| Container Build | ⚠️ Works | ⚠️ Needs Testing |
| Documentation | ✅ Complete | ✅ Yes |

**Overall Assessment**: Good for learning and prototyping, needs work for production use.

