// profiler.bpf.c

// If you later want full CO-RE with kernel structs, generate vmlinux.h
// and uncomment this:
// #include "vmlinux.h"

#include <linux/types.h>
#include <linux/bpf.h>

#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

#include <stdbool.h>
#include <stddef.h>

#ifndef AF_INET
#define AF_INET  2
#endif
#ifndef AF_INET6
#define AF_INET6 10
#endif



char LICENSE[] SEC("license") = "Dual BSD/GPL";

#define MAX_DATA_SIZE 256

enum direction {
    DIR_SEND = 0,
    DIR_RECV = 1,
};

struct conn_info {
    __u16 family;
    __u16 sport;
    __u16 dport;
    __u8 saddr[16];
    __u8 daddr[16];
};

struct http_event {
    __u64 ts;
    __u32 pid;
    __u32 tid;
    __u32 data_len;
    __u16 sport;
    __u16 dport;
    __u16 family;
    __u8 direction;
    char comm[16];
    __u8 saddr[16];
    __u8 daddr[16];
    __u8 data[MAX_DATA_SIZE];
};

struct recv_args {
    int fd;
    __u64 buf;
    size_t len;
};

struct accept_args {
    int fd;
    __u64 addr;        // userspace sockaddr pointer
    __u64 addrlen_ptr; // pointer to addrlen int
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 20);
} events SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, __u64);
    __type(value, struct recv_args);
    __uint(max_entries, 512);
} recv_args_map SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, __u64);
    __type(value, struct accept_args);
    __uint(max_entries, 512);
} accept_args_map SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, __u64);
    __type(value, struct conn_info);
    __uint(max_entries, 512);
} conn_map SEC(".maps");

// Generic tracepoint context for sys_enter_*
// Matches the format described in
// /sys/kernel/tracing/events/syscalls/sys_enter_* /format
struct sys_enter_args {
    unsigned long long unused; // struct trace_entry, we don't need fields
    long id;
    unsigned long args[6];
};

// Generic tracepoint context for sys_exit_*
// Matches /sys/kernel/tracing/events/syscalls/sys_exit_* /format
struct sys_exit_args {
    unsigned long long unused; // struct trace_entry
    long id;
    long ret;
};

static __always_inline __u64 make_key(__u32 pid, __u32 fd) {
    return ((__u64)pid << 32) | fd;
}

static __always_inline void copy_bytes(__u8 *dst, const __u8 *src, __u32 len) {
#pragma clang loop unroll(full)
    for (int i = 0; i < 16; i++) {
        if (i < (int)len) {
            dst[i] = src[i];
        } else {
            dst[i] = 0;
        }
    }
}

static __always_inline int read_sockaddr(void *addr,
    struct conn_info *info,
    bool is_remote)
{
if (!addr || !info) {
return -1;
}

// just read the family first
struct {
__u16 family;
} hdr = {};

if (bpf_probe_read_user(&hdr, sizeof(hdr), addr) != 0) {
return -1;
}

__u16 family = hdr.family;
info->family = family;

if (family == AF_INET) {
struct ipv4_sock {
__u16 family;
__u16 port;
__u32 addr;
} saddr = {};

if (bpf_probe_read_user(&saddr, sizeof(saddr), addr) != 0) {
return -1;
}

if (is_remote) {
info->dport = saddr.port;
copy_bytes(info->daddr, (const __u8 *)&saddr.addr, 4);
} else {
info->sport = saddr.port;
copy_bytes(info->saddr, (const __u8 *)&saddr.addr, 4);
}
} else if (family == AF_INET6) {
struct ipv6_sock {
__u16 family;
__u16 port;
__u32 flowinfo;
__u8  addr[16];
__u32 scope_id;
} saddr6 = {};

if (bpf_probe_read_user(&saddr6, sizeof(saddr6), addr) != 0) {
return -1;
}

if (is_remote) {
info->dport = saddr6.port;
copy_bytes(info->daddr, saddr6.addr, 16);
} else {
info->sport = saddr6.port;
copy_bytes(info->saddr, saddr6.addr, 16);
}
} else {
return -1;
}

return 0;
}



static __always_inline void
maybe_update_conn_from_addr(__u64 key, void *addr, bool is_remote)
{
    struct conn_info info = {};
    int res = read_sockaddr(addr, &info, is_remote);
    if (res == 0) {
        struct conn_info *existing = bpf_map_lookup_elem(&conn_map, &key);
        if (existing) {
            if (info.family != 0) {
                existing->family = info.family;
            }
            if (is_remote) {
                existing->dport = info.dport;
                copy_bytes(existing->daddr, info.daddr, 16);
            } else {
                existing->sport = info.sport;
                copy_bytes(existing->saddr, info.saddr, 16);
            }
            bpf_map_update_elem(&conn_map, &key, existing, BPF_ANY);
        } else {
            bpf_map_update_elem(&conn_map, &key, &info, BPF_ANY);
        }
    }
}


static __always_inline struct conn_info *lookup_conn(__u32 pid, int fd) {
    __u64 key = make_key(pid, fd);
    return bpf_map_lookup_elem(&conn_map, &key);
}

static __always_inline void
fill_event_conn(struct http_event *e, struct conn_info *info)
{
    if (!e || !info) {
        return;
    }
    e->family = info->family;
    e->sport = info->sport;
    e->dport = info->dport;
    copy_bytes(e->saddr, info->saddr, 16);
    copy_bytes(e->daddr, info->daddr, 16);
}

/* -------------------- bind -------------------- */

SEC("tracepoint/syscalls/sys_enter_bind")
int trace_sys_enter_bind(struct sys_enter_args *ctx)
{
    __u32 pid = bpf_get_current_pid_tgid() >> 32;
    int fd = (int)ctx->args[0];
    struct sockaddr *umyaddr = (struct sockaddr *)ctx->args[1];

    struct conn_info info = {};
    if (read_sockaddr(umyaddr, &info, false) == 0) {
        __u64 key = make_key(pid, fd);
        bpf_map_update_elem(&conn_map, &key, &info, BPF_ANY);
    }
    return 0;
}

/* -------------------- connect -------------------- */

SEC("tracepoint/syscalls/sys_enter_connect")
int trace_sys_enter_connect(struct sys_enter_args *ctx)
{
    __u32 pid = bpf_get_current_pid_tgid() >> 32;
    int fd = (int)ctx->args[0];
    struct sockaddr *uservaddr = (struct sockaddr *)ctx->args[1];

    __u64 key = make_key(pid, fd);
    maybe_update_conn_from_addr(key, uservaddr, true);
    return 0;
}

/* -------------------- accept4 -------------------- */

SEC("tracepoint/syscalls/sys_enter_accept4")
int trace_sys_enter_accept4(struct sys_enter_args *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    int fd = (int)ctx->args[0];
    void *upeer_sockaddr = (void *)ctx->args[1];
    int *upeer_addrlen = (int *)ctx->args[2];

    struct accept_args args = {};
    args.fd = fd;
    args.addr = (__u64)upeer_sockaddr;
    args.addrlen_ptr = (__u64)upeer_addrlen;
    bpf_map_update_elem(&accept_args_map, &pid_tgid, &args, BPF_ANY);
    return 0;
}



SEC("tracepoint/syscalls/sys_exit_accept4")
int trace_sys_exit_accept4(struct sys_exit_args *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __s64 ret = ctx->ret;

    struct accept_args *args = bpf_map_lookup_elem(&accept_args_map,
                                                   &pid_tgid);
    if (!args) {
        return 0;
    }
    bpf_map_delete_elem(&accept_args_map, &pid_tgid);

    if (ret < 0) {
        return 0;
    }

    __u32 pid = pid_tgid >> 32;
    __u64 listen_key = make_key(pid, args->fd);
    __u64 new_key = make_key(pid, (__u32)ret);

    struct conn_info info = {};
    struct conn_info *listen_info = bpf_map_lookup_elem(&conn_map,
                                                        &listen_key);
    if (listen_info) {
        info.family = listen_info->family;
        info.sport = listen_info->sport;
        copy_bytes(info.saddr, listen_info->saddr, 16);
    }

    if (args->addr) {
        void *peer = (void *)(unsigned long)args->addr;
        read_sockaddr(peer, &info, true);
    }

    bpf_map_update_elem(&conn_map, &new_key, &info, BPF_ANY);
    return 0;
}



/* -------------------- sendto -------------------- */

SEC("tracepoint/syscalls/sys_enter_sendto")
int trace_sys_enter_sendto(struct sys_enter_args *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 pid = pid_tgid >> 32;
    __u32 tid = (__u32)pid_tgid;

    int fd = (int)ctx->args[0];
    void *buf = (void *)ctx->args[1];
    size_t len = (size_t)ctx->args[2];
    struct sockaddr *dest_addr = (struct sockaddr *)ctx->args[4];

    __u64 key = make_key(pid, fd);
    if (dest_addr) {
        maybe_update_conn_from_addr(key, dest_addr, true);
    }

    struct conn_info *info = lookup_conn(pid, fd);
    if (!info) {
        return 0;
    }

    __u32 copy_len = len > MAX_DATA_SIZE ? MAX_DATA_SIZE : (__u32)len;

    struct http_event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) {
        return 0;
    }

    e->ts = bpf_ktime_get_ns();
    e->pid = pid;
    e->tid = tid;
    e->direction = DIR_SEND;
    e->data_len = copy_len;
    bpf_get_current_comm(&e->comm, sizeof(e->comm));
    fill_event_conn(e, info);

    bpf_probe_read_user(&e->data, copy_len, buf);
    bpf_ringbuf_submit(e, 0);
    return 0;
}

/* -------------------- recvfrom -------------------- */

SEC("tracepoint/syscalls/sys_enter_recvfrom")
int trace_sys_enter_recvfrom(struct sys_enter_args *ctx)
{
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    int fd = (int)ctx->args[0];
    void *buf = (void *)ctx->args[1];
    size_t len = (size_t)ctx->args[2];
    struct sockaddr *src_addr = (struct sockaddr *)ctx->args[4];

    __u32 pid = pid_tgid >> 32;
    __u64 key = make_key(pid, fd);
    if (src_addr) {
        maybe_update_conn_from_addr(key, src_addr, true);
    }

    struct recv_args args = {
        .fd = fd,
        .buf = (__u64)buf,
        .len = len,
    };
    bpf_map_update_elem(&recv_args_map, &pid_tgid, &args, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_recvfrom")
int trace_sys_exit_recvfrom(struct sys_exit_args *ctx)
{
    __s64 ret = ctx->ret;
    if (ret <= 0) {
        return 0;
    }

    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 pid = pid_tgid >> 32;
    __u32 tid = (__u32)pid_tgid;

    struct recv_args *args = bpf_map_lookup_elem(&recv_args_map, &pid_tgid);
    if (!args) {
        return 0;
    }
    bpf_map_delete_elem(&recv_args_map, &pid_tgid);

    struct conn_info *info = lookup_conn(pid, args->fd);
    if (!info) {
        return 0;
    }

    __u32 copy_len = ret > MAX_DATA_SIZE ? MAX_DATA_SIZE : (__u32)ret;

    struct http_event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) {
        return 0;
    }

    e->ts = bpf_ktime_get_ns();
    e->pid = pid;
    e->tid = tid;
    e->direction = DIR_RECV;
    e->data_len = copy_len;
    bpf_get_current_comm(&e->comm, sizeof(e->comm));
    fill_event_conn(e, info);

    void *src_buf = (void *)(unsigned long)args->buf;
    bpf_probe_read_user(&e->data, copy_len, src_buf);
    bpf_ringbuf_submit(e, 0);
    return 0;
}
