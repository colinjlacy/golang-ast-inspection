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

Please feel free to make suggestions, either here or in the initiative's [Slack discussion](https://cloud-native.slack.com/archives/C09S7A5T3GF).

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

## Environment Variables

### Profiler Configuration

These environment variables configure the profiler itself:

- `OUTPUT_PATH=/var/log/ebpf_http_profiler.log` - File path for HTTP event logs (JSON format)
- `ENV_OUTPUT_PATH=/var/log/ebpf_http_env.yaml` - File path for environment variable logs (YAML format)
- `ENV_PREFIX_LIST=""` - Comma-separated list of environment variable key prefixes to include (case-sensitive). If not set, all environment variables are collected.
- `ADI_PROFILE_ALLOWED=""` - Comma-separated list of ADI_PROFILE values to profile (see Opt-In Profiling below). If not set, all processes with `ADI_PROFILE` set (any value) will be profiled.

### Opt-In Profiling

The profiler uses an opt-in system to control which processes are profiled. Target processes (the ones being profiled) must set specific environment variables:

**Target Process Environment Variables:**
- `ADI_PROFILE=<environment>` - **Required for profiling.** Indicates the process opts into profiling. The value should indicate the environment (e.g., `local`, `dev`, `staging`, `prod`).
- `ADI_PROFILE_DISABLED=1` - **Override to disable profiling.** If set, the process will not be profiled even if `ADI_PROFILE` is present.

**Profiling Logic:**

A process is profiled only if:
1. `ADI_PROFILE_DISABLED` is **not** set to `1`
2. `ADI_PROFILE` **is** set
3. If `ADI_PROFILE_ALLOWED` is set on the profiler, the `ADI_PROFILE` value must be in the allowed list

**Default Behavior:**
- If `ADI_PROFILE_ALLOWED` is **not set**: All processes with `ADI_PROFILE` set (any value) are profiled
- If `ADI_PROFILE_ALLOWED` **is set**: Only processes whose `ADI_PROFILE` value matches one in the list are profiled
- Processes without `ADI_PROFILE` are **never** profiled

**Example Scenarios:**

| Target Process Has | Profiler Config | Result |
|-------------------|-----------------|---------|
| `ADI_PROFILE=local` | `ADI_PROFILE_ALLOWED=""` (not set) | ✅ Profiled |
| `ADI_PROFILE=local` | `ADI_PROFILE_ALLOWED="local,dev"` | ✅ Profiled |
| `ADI_PROFILE=prod` | `ADI_PROFILE_ALLOWED="local,dev"` | ❌ Not profiled |
| `ADI_PROFILE=local`<br>`ADI_PROFILE_DISABLED=1` | `ADI_PROFILE_ALLOWED="local"` | ❌ Not profiled (override) |
| (no ADI_PROFILE) | `ADI_PROFILE_ALLOWED="local"` | ❌ Not profiled |

## Run 'dis mofo

### Basic Usage

**Step 1: Start the profiler**

Profile all processes with `ADI_PROFILE` set (any value), get the full environment variable map for each process:
```sh
sudo OUTPUT_PATH="/some/path/ebpf_http_profiler.log" \
     ENV_OUTPUT_PATH="/some/path/ebpf_env_profiler.yaml" \
     ./profiler
```

Profile only specific processes that have `ADI_PROFILE=local` or `ADI_PROFILE=dev` set, still get the full environment variable map for each process:
```sh
sudo OUTPUT_PATH="/some/path/ebpf_http_profiler.log" \
     ENV_OUTPUT_PATH="/some/path/ebpf_env_profiler.yaml" \
     ADI_PROFILE_ALLOWED="local,dev" \
     ./profiler
```

Profile with environment variable filtering:
```sh
sudo OUTPUT_PATH="/some/path/ebpf_http_profiler.log" \
     ENV_OUTPUT_PATH="/some/path/ebpf_env_profiler.yaml" \
     ENV_PREFIX_LIST="REVIEWS_,RATINGS_,MONGO_" \
     ADI_PROFILE_ALLOWED="local,dev,staging" \
     ./profiler
```

**Step 2: Start services with opt-in flag**

For the demo HTTP server and traffic generator, `ADI_PROFILE=local` has already been set in `docker-compose.yml`. Run them both with:
```sh
docker compose up -d
# podman compose up -d
# nerdctl compose up -d
```

The profiler captures all HTTP traffic (both client and server side) from processes that meet the opt-in criteria. The traffic generator issues GET/POST traffic in a loop so you can see request/response bodies, methods, URLs, and status codes captured from syscall payloads.

The profiler also collects environment variables from each process making HTTP calls. As soon as a new PID is observed, the profiler reads `/proc/<pid>/environ` and writes the results to a separate YAML file.

You can run the profiler first, and it'll hang out waiting for 
any HTTP traffic to arrive via `syscall`. Or, if you start it while processes are sending traffic, it will profile for as long as it's running.

## Go Big(-ish)

I've got [another repo](https://github.com/colinjlacy/bookinfo-docker-compose) that puts the [Istio Bookinfo](https://github.com/colinjlacy/bookinfo-docker-compose) demo into a docker-compose file. 

To profile the Bookinfo services:

**Step 1: Start the profiler with environment-specific filtering**

```sh
sudo OUTPUT_PATH="/home/lima.linux/http-profiler/output/ebpf_http_profiler.log" \
     ENV_OUTPUT_PATH="/home/lima.linux/http-profiler/output/ebpf_env_profiler.yaml" \
     ENV_PREFIX_LIST="REVIEWS_,RATINGS_,MONGO_,DETAILS_" \
     ADI_PROFILE_ALLOWED="local,dev" \
     ./profiler
```

**Step 2: Start the services and run traffic**

From the other repo's project root:
```sh
docker compose up -d
# podman compose up -d
# nerdctl compose up -d
./scripts/run-traffic-gen.sh
```

The profiler will capture HTTP traffic and environment variables from all services that have `ADI_PROFILE=local` or `ADI_PROFILE=dev` set. Because you specified `ENV_PREFIX_LIST`, you'll only see the filtered environment variables (not the full OCI runtime environment). Try it without `ENV_PREFIX_LIST` to see the full environment variable firehose.

Note that the NodeJS traffic generator app was not profiled at all, due to the fact that it does not have `ADI_PROFILE` set.

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
adi_profile: "local"
env:
  PATH: "/usr/local/bin:/usr/bin:/bin"
  HOME: "/home/user"
  REVIEWS_SERVICE_URL: "http://reviews:9080"
  RATINGS_HOSTNAME: "ratings"
---
pid: 67890
adi_profile: "staging"
env:
  MONGO_HOST: "mongodb://db:27017"
  MONGO_DATABASE: "bookinfo"
---
pid: 99999
adi_profile: "local"
error: "open /proc/99999/environ: no such file or directory"
```

Each document includes:
- `pid`: The process ID
- `adi_profile_match`: The value of the `ADI_PROFILE` environment variable that qualified this process for profiling
- `env`: Key-value pairs of environment variables (filtered by `ENV_PREFIX_LIST` if specified)
- `error`: Error message if the process exits before the profiler can read its environment

When `ENV_PREFIX_LIST` is used, only matching environment variables are included (PIDs may have empty `env: {}` if no variables match).
