# golang-ast-inspection

Minimal eBPF-backed HTTP syscall profiler plus a tiny test service and traffic generator. Everything is wired for x86_64 Ubuntu (20/22/23) and Go 1.25.

## What’s here
- `cmd/server`: basic HTTP service on port 8080 with `/`, `/healthz`, `/echo`, `/slow`.
- `cmd/traffic`: small Go script that repeatedly hits the service.
- `cmd/profiler`: eBPF-powered profiler that attaches to socket syscalls and writes request/response metadata to a local file.
- `bpf/profiler.bpf.c`: BPF program (compiled via `bpf2go` during build).

## Setup (Ubuntu 20/22/23/25)
- Go toolchain 1.25+ 
- Install C libraries (I had to sudo on a lima VM):
```sh
sudo apt-get update
sudo apt-get install -y --no-install-recommends \
    clang llvm make pkg-config libelf-dev zlib1g-dev linux-libc-dev libbpf-dev
sudo rm -rf /var/lib/apt/lists/*
```
- set up necessary symlink:
```sh
arch="$(uname -m)" && \
case "${arch}" in \
    x86_64) multiarch="x86_64-linux-gnu" ;; \
    aarch64|arm64) multiarch="aarch64-linux-gnu" ;; \
    *) echo "Unsupported architecture: ${arch}" >&2; exit 1 ;; \
esac && \
ln -sf /usr/include/${multiarch}/asm /usr/include/asm
```
- Set environment variables:
```sh
export GOOS=linux
export GOARCH=arm64 # or whatever
export CGO_ENABLED=1
```
- Build all the things:
```sh
# Linux only; requires clang/llvm and kernel headers
go mod download
go generate ./pkg/profiler            # builds the BPF object via bpf2go (emits files under pkg/profiler with tag ebpf_build)
go build ./cmd/server                 # HTTP service
go build ./cmd/traffic                # traffic generator
go build -tags ebpf_build ./cmd/profiler  # profiler binary (uses generated bindings)
```

## Run 'dis mofo:
- You can run the profiler first, and it'll hang out waiting for any HTTP traffic to arrive via `syscall`:
```sh
sudo OUTPUT_PATH="/some/path/ebpf_http_profiler.log" ./profiler # sudo because eBPF? I guess?
```
- Then stand up the demo HTTP server and traffic generator in OCI containers:
```sh
docker compose up -d
# podman compose up -d
# nerdctl compose up -d
```
- The profiler captures all HTTP traffic (both client and server side) from any process on the system. The traffic generator issues GET/POST traffic in a loop so you can see request/response bodies, methods, URLs, and status codes captured from syscall payloads.

## Go Big(-ish)

I've got [another repo](https://github.com/colinjlacy/bookinfo-docker-compose) that put the [Istio Bookinfo](https://github.com/colinjlacy/bookinfo-docker-compose) demo into a docker-compose file. 

You can:
- run the profiler in this repo
- stand up the containers in that repo
- and then run that repo's `run-traffic-gen.sh` to profile traffic happening between all of the different services

## Output format
JSON lines with syscall-derived metadata and parsed HTTP fields:
```json
{
  "timestamp": "2024-04-08T18:24:10.123456789Z",
  "pid": 1234,
  "comm": "traffic-generat",
  "cmdline": "/bin/traffic-generator",
  "direction": "send",
  "source_ip": "127.0.0.1",
  "source_port": 54321,
  "dest_ip": "127.0.0.1",
  "dest_port": 8080,
  "bytes": 89,
  "method": "GET",
  "url": "/echo",
  "body": "{\"message\":\"hello\"}",
  "headers": {
    "Host": "127.0.0.1:8080",
    "User-Agent": "Go-http-client/1.1",
    "Content-Type": "application/json"
  },
  "raw_payload": "GET /echo HTTP/1.1\r\nHost: ..."
}
```
Fields include parsed HTTP method, URL, status code (for responses), headers, request/response bodies, plus the complete raw payload from the syscalls. Only HTTP traffic is logged; UDP and non-HTTP TCP traffic is filtered out.
