//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>

char __license[] SEC("license") = "Dual MIT/GPL";

#define MAX_DATA_SIZE 16384
#define TASK_COMM_LEN 16

// Event types
#define EVENT_TYPE_SEND 1
#define EVENT_TYPE_RECV 2

// HTTP event structure
struct http_event {
    u64 timestamp;
    u32 pid;
    u32 tid;
    u32 fd;
    u8 type;           // EVENT_TYPE_SEND or EVENT_TYPE_RECV
    u32 data_len;
    char comm[TASK_COMM_LEN];
    char data[MAX_DATA_SIZE];
};

// Ring buffer for sending events to userspace
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} events SEC(".maps");

// Helper to check if data looks like HTTP
static __always_inline bool is_http_traffic(const char *data, size_t len) {
    if (len < 4) return false;
    
    // Check for HTTP request methods
    if (data[0] == 'G' && data[1] == 'E' && data[2] == 'T' && data[3] == ' ')
        return true;
    if (data[0] == 'P' && data[1] == 'O' && data[2] == 'S' && data[3] == 'T')
        return true;
    if (data[0] == 'P' && data[1] == 'U' && data[2] == 'T' && data[3] == ' ')
        return true;
    if (data[0] == 'D' && data[1] == 'E' && data[2] == 'L' && data[3] == 'E')
        return true;
    if (data[0] == 'H' && data[1] == 'E' && data[2] == 'A' && data[3] == 'D')
        return true;
    if (data[0] == 'P' && data[1] == 'A' && data[2] == 'T' && data[3] == 'C')
        return true;
    if (data[0] == 'O' && data[1] == 'P' && data[2] == 'T' && data[3] == 'I')
        return true;
    
    // Check for HTTP response
    if (len >= 8) {
        if (data[0] == 'H' && data[1] == 'T' && data[2] == 'T' && data[3] == 'P' &&
            data[4] == '/' && data[6] == '.')
            return true;
    }
    
    return false;
}

// Simplified tracepoint-based approach (architecture-independent)
// Hook write syscall to capture outbound data
SEC("tracepoint/syscalls/sys_enter_write")
int trace_write_enter(struct trace_event_raw_sys_enter* ctx)
{
    // Get current process info
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    u32 tid = (u32)pid_tgid;
    
    // Skip kernel threads
    if (pid == 0)
        return 0;
    
    // Get file descriptor (first arg)
    int fd = (int)ctx->args[0];
    
    // Only interested in socket FDs (typically > 2)
    if (fd <= 2)
        return 0;
    
    // Get buffer pointer and size
    void *buf = (void *)ctx->args[1];
    size_t count = (size_t)ctx->args[2];
    
    // Limit data size
    if (count == 0 || count > MAX_DATA_SIZE)
        return 0;
    
    // Reserve space in ring buffer
    struct http_event *event = bpf_ringbuf_reserve(&events, sizeof(*event), 0);
    if (!event)
        return 0;
    
    // Fill event metadata
    event->timestamp = bpf_ktime_get_ns();
    event->pid = pid;
    event->tid = tid;
    event->type = EVENT_TYPE_SEND;
    event->fd = fd;
    event->data_len = count;
    
    // Get process name
    bpf_get_current_comm(&event->comm, sizeof(event->comm));
    
    // Read data from user buffer
    long ret = bpf_probe_read_user(&event->data, count, buf);
    if (ret < 0) {
        bpf_ringbuf_discard(event, 0);
        return 0;
    }
    
    // Check if this is HTTP traffic
    if (!is_http_traffic(event->data, count)) {
        bpf_ringbuf_discard(event, 0);
        return 0;
    }
    
    // Submit event
    bpf_ringbuf_submit(event, 0);
    return 0;
}

// Hook read syscall to capture inbound data  
SEC("tracepoint/syscalls/sys_exit_read")
int trace_read_exit(struct trace_event_raw_sys_exit* ctx)
{
    // Get return value (bytes read)
    long ret = ctx->ret;
    
    // Skip if no data was read
    if (ret <= 0)
        return 0;
    
    // Get current process info
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    u32 tid = (u32)pid_tgid;
    
    // Skip kernel threads
    if (pid == 0)
        return 0;
    
    // Note: We can't easily get the buffer here in the exit probe
    // This is a limitation of the simplified approach
    // For a complete solution, you'd store context in a map from entry probe
    
    return 0;
}

