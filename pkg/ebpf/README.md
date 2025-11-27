# eBPF HTTP Probe

## Current Status

⚠️ **MVP/Work in Progress**: The eBPF program is a simplified implementation that demonstrates the concept. Full production implementation requires more work.

## Known Limitations

1. **Data Capture**: The current iovec reading is simplified and may not capture all data correctly
2. **Hook Points**: May need adjustment based on kernel version
3. **Type Definitions**: Using minimal vmlinux.h (in production, generate from your kernel)

## Building

The eBPF program must be built on Linux (or inside a Linux container):

```bash
# Inside container (automatic during docker build)
make http_probe.o

# This compiles http_probe.c to http_probe.o
```

## Generating Full vmlinux.h

For production use, generate a complete vmlinux.h from your kernel:

```bash
# On your Linux system
sudo bpftool btf dump file /sys/kernel/btf/vmlinux format c > vmlinux.h
```

This provides all kernel type definitions for your specific kernel version.

## Alternative: Use libbpf-bootstrap

For a more robust implementation, consider using [libbpf-bootstrap](https://github.com/libbpf/libbpf-bootstrap) templates which provide:
- Complete vmlinux.h generation
- Proper BPF program structure
- CO-RE (Compile Once, Run Everywhere) support
- Better error handling

## Debugging

### Check eBPF Program Loaded

```bash
# Inside container
bpftool prog list

# Should show your programs
```

### Check Maps

```bash
bpftool map list
```

### View Ring Buffer

```bash
bpftool map event MAPID
```

## Future Improvements

- [ ] Proper iovec iteration for data capture
- [ ] Better hook point selection (maybe tcp_sendpage, etc.)
- [ ] CO-RE support for kernel compatibility
- [ ] More robust HTTP detection
- [ ] Support for chunked encoding
- [ ] Better error handling

## Resources

- [eBPF Tutorial](https://ebpf.io/what-is-ebpf/)
- [libbpf Documentation](https://libbpf.readthedocs.io/)
- [BPF Helpers](https://man7.org/linux/man-pages/man7/bpf-helpers.7.html)
- [Cilium eBPF Library](https://github.com/cilium/ebpf)

