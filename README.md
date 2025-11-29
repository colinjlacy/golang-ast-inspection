# Golang HTTP Profiler

Minimal eBPF-backed HTTP syscall and environment variable profiler written in Golang, plus a tiny test service and traffic generator. 

It's a PoC in the service of [this initiative](https://github.com/cncf/toc/issues/1797).

## What’s here
- `cmd/server`: basic HTTP service on port 8080 with `/`, `/healthz`, `/echo`, `/slow`.
- `cmd/traffic`: small Go script that repeatedly hits the service.
- `cmd/profiler`: eBPF-powered profiler that attaches to socket syscalls and writes request/response metadata to a local file.
- `bpf/profiler.bpf.c`: BPF program (compiled via `bpf2go` during build).

## What it does:

- Sets up an eBPF profiler to listen for HTTP events and logs the origin PID, to IP and port, from IPand port, method, data, response code, etc.
- For each PID found, pulls the environment variables assigned to the process
- Writes the output of each to an output file

I'll probably keep adding stuff. Please feel free to contribute or make suggestions.

## System compatibility 

**This project currently only runs on Linux.** If you want to run it on a Mac, you'll need a VM. I could not get it working in a Linux container, although that could have something to do with the corporate security profile installed on my machine. 

Since it leverages eBPF, I have strong doubts about it working on Windows.

## Setup (Ubuntu 20/22/23/25)

There are probably better/smarter/faster/cooler ways to run this, but the way I pulled it off was to run a [Lima VM](https://lima-vm.io/) on my Mac. Note that I'm running on an ARM64 Mac, and I have not tested this on an x86 machine of any sort. Which means I also haven't tested it on a real Linux box.

That said, if you'd like to run this:
- If you're using a VM, SSH into that and clone this repo
  - Lima will mount your host machine's home directory as read-only
  - But! you need to generate the Go/C bindings for the eBPF functionality.
  - So! don't rely on the mounted home directory if you've cloned this to your host machine
- Install the Go toolchain 1.25+ 
- Make sure you've got an OCI container runtime installed
  - Most people would say "make sure you've got Docker installed"
  - I used [nerdctl](https://github.com/containerd/nerdctl)
  - [Podman](https://podman.io/) would also work.
- Install C libraries (I had to sudo on a lima VM):
```sh
sudo apt-get update
# have not tested on x86, 
# but I'd imagine you'll have less problems than I did
sudo apt-get install -y --no-install-recommends \
    clang llvm make pkg-config libelf-dev zlib1g-dev linux-libc-dev libbpf-dev
sudo rm -rf /var/lib/apt/lists/*
```
- set up necessary symlink:
```sh
# me and Claude trying to be arch-agnostic
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
export GOARCH=arm64 # or, ya know, whatever
export CGO_ENABLED=1
```
- Build the profiler:
```sh
# Linux only; requires clang/llvm and kernel headers
go mod download
go generate ./pkg/profiler            # builds the BPF object via bpf2go (emits files under pkg/profiler with tag ebpf_build)
go build -tags ebpf_build ./cmd/profiler  # profiler binary (uses generated bindings)
```

## Environment Variables:

There are currently three envirnment variables that you can optionally set, listed below with their default values:

- `OUTPUT_PATH=/var/log/ebpf_http_profiler.log` indicates the file path to which logged HTTP events will be written
- `ENV_OUTPUT_PATH=/var/log/ebpf_http_env.yaml` indicates where the 
- `ENV_PREFIX_LIST=""` is a comma-separated list (see below for an example) of environment variable prefixes to filter the env var firehose.

## Run 'dis mofo:

- You can run the profiler first, and it'll hang out waiting for any HTTP traffic to arrive via `syscall`:
```sh
# sudo because eBPF? I guess?
sudo OUTPUT_PATH="/some/path/ebpf_http_profiler.log" \
  ENV_OUTPUT_PATH="/some/path/ebpf_env_profiler.yaml" \
  ENV_PREFIX_LIST="HTTP_,TARGET_,TOTAL_,REQUEST_" \
 ./profiler 
```
- Then stand up the demo HTTP server and traffic generator in OCI containers:
```sh
docker compose up -d
# podman compose up -d
# nerdctl compose up -d
```

The profiler captures all HTTP traffic (both client and server side) from any process on the system. The traffic generator issues GET/POST traffic in a loop so you can see request/response bodies, methods, URLs, and status codes captured from syscall payloads.

### Environment Variable Collection

The profiler also collects environment variables from each process making HTTP calls. As soon as a new PID is observed, the profiler reads `/proc/<pid>/environ` and writes the results to a separate YAML file.

**Configuration:**
- `ENV_OUTPUT_PATH`: Path to the YAML output file (default: `/var/log/ebpf_http_env.yaml`)
- `ENV_PREFIX_LIST`: Comma-separated list of prefixes to filter environment variables (optional)

**Examples:**

Collect all environment variables:
```sh
sudo OUTPUT_PATH="/some/path/ebpf_http_profiler.log" \
     ENV_OUTPUT_PATH="/some/path/ebpf_http_env.yaml" \
     ./profiler
```

Collect only environment variables with specific prefixes (case-sensitive):
```sh
sudo OUTPUT_PATH="/some/path/ebpf_http_profiler.log" \
     ENV_OUTPUT_PATH="/some/path/ebpf_http_env.yaml" \
     ENV_PREFIX_LIST="REVIEWS_,RATINGS_,MONGO_" \
     ./profiler
```

## Go Big(-ish)

I've got [another repo](https://github.com/colinjlacy/bookinfo-docker-compose) that put the [Istio Bookinfo](https://github.com/colinjlacy/bookinfo-docker-compose) demo into a docker-compose file. 

You can:
- run the profiler in this repo:
```sh
sudo OUTPUT_PATH="/home/lima.linux/http-profiler/output/ebpf_http_profiler.log" \
  ENV_OUTPUT_PATH="/home/lima.linux/http-profiler/output/ebpf_env_profiler.yaml" \
  ENV_PREFIX_LIST="REVIEWS_,RATINGS_,MONGO_,DETAILS_" 
  ./profiler 
```
- stand up the containers in that repo
- and then run that repo's `run-traffic-gen.sh` to profile traffic happening between all of the different services

You'll notice that because you added env var prefix filters you don't get all of the env vars that the OCI runtime sets. Try it again without setting `ENV_PREFIX_LIST` to see the full OCI env var firehose.

## Output format

### HTTP Events (JSON)
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

### Environment Variables (YAML)
Multi-document YAML with one document per PID:
```yaml
pid: 12345
env:
  PATH: "/usr/local/bin:/usr/bin:/bin"
  HOME: "/home/user"
  REVIEWS_SERVICE_URL: "http://reviews:9080"
  RATINGS_HOSTNAME: "ratings"
---
pid: 67890
env:
  MONGO_HOST: "mongodb://db:27017"
  MONGO_DATABASE: "bookinfo"
---
pid: 99999
error: "open /proc/99999/environ: no such file or directory"
```

Each document includes the PID and its environment variables as key-value pairs. If a process exits before the profiler can read its environment, an error message is recorded instead. When `ENV_PREFIX_LIST` is used, only matching environment variables are included (PIDs may have empty `env: {}` if no variables match).
