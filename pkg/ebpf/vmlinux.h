// Minimal vmlinux.h for eBPF program
// In production, generate this with: bpftool btf dump file /sys/kernel/btf/vmlinux format c > vmlinux.h

#ifndef __VMLINUX_H__
#define __VMLINUX_H__

// Basic types
typedef unsigned char __u8;
typedef unsigned short __u16;
typedef unsigned int __u32;
typedef unsigned long long __u64;

typedef signed char __s8;
typedef short __s16;
typedef int __s32;
typedef long long __s64;

typedef __u8 u8;
typedef __u16 u16;
typedef __u32 u32;
typedef __u64 u64;

typedef __s8 s8;
typedef __s16 s16;
typedef __s32 s32;
typedef __s64 s64;

// Network byte order types
typedef __u16 __be16;
typedef __u32 __be32;
typedef __u64 __be64;

// Other kernel types
typedef __u16 __sum16;
typedef __u32 __wsum;

typedef long __kernel_long_t;
typedef unsigned long __kernel_ulong_t;

// size_t for userspace compatibility
#ifndef __SIZE_TYPE__
#define __SIZE_TYPE__ unsigned long
#endif
typedef __SIZE_TYPE__ size_t;

// bool type
typedef _Bool bool;
#define true 1
#define false 0

// BPF map types
#define BPF_MAP_TYPE_HASH 1
#define BPF_MAP_TYPE_ARRAY 2
#define BPF_MAP_TYPE_PROG_ARRAY 3
#define BPF_MAP_TYPE_PERF_EVENT_ARRAY 4
#define BPF_MAP_TYPE_PERCPU_HASH 5
#define BPF_MAP_TYPE_PERCPU_ARRAY 6
#define BPF_MAP_TYPE_STACK_TRACE 7
#define BPF_MAP_TYPE_CGROUP_ARRAY 8
#define BPF_MAP_TYPE_LRU_HASH 9
#define BPF_MAP_TYPE_LRU_PERCPU_HASH 10
#define BPF_MAP_TYPE_LPM_TRIE 11
#define BPF_MAP_TYPE_ARRAY_OF_MAPS 12
#define BPF_MAP_TYPE_HASH_OF_MAPS 13
#define BPF_MAP_TYPE_DEVMAP 14
#define BPF_MAP_TYPE_SOCKMAP 15
#define BPF_MAP_TYPE_CPUMAP 16
#define BPF_MAP_TYPE_XSKMAP 17
#define BPF_MAP_TYPE_SOCKHASH 18
#define BPF_MAP_TYPE_CGROUP_STORAGE 19
#define BPF_MAP_TYPE_REUSEPORT_SOCKARRAY 20
#define BPF_MAP_TYPE_PERCPU_CGROUP_STORAGE 21
#define BPF_MAP_TYPE_QUEUE 22
#define BPF_MAP_TYPE_STACK 23
#define BPF_MAP_TYPE_SK_STORAGE 24
#define BPF_MAP_TYPE_DEVMAP_HASH 25
#define BPF_MAP_TYPE_STRUCT_OPS 26
#define BPF_MAP_TYPE_RINGBUF 27

// Kernel structures (minimal definitions)
struct sock {};
struct msghdr {};
struct task_struct {};
struct tcphdr {};
struct __sk_buff {};

struct iovec {
    void *iov_base;
    __kernel_ulong_t iov_len;
};

// Tracepoint structures for syscall tracing
struct trace_event_raw_sys_enter {
    __u64 unused;
    long id;
    unsigned long args[6];
};

struct trace_event_raw_sys_exit {
    __u64 unused;
    long id;
    long ret;
};

#endif /* __VMLINUX_H__ */

