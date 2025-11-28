ARG GO_VERSION=1.25
FROM golang:${GO_VERSION}-bookworm AS build

RUN go env

RUN apt-get update && apt-get install -y --no-install-recommends \
    clang llvm make pkg-config libelf-dev zlib1g-dev linux-libc-dev libbpf-dev \
    && rm -rf /var/lib/apt/lists/*

# Provide /usr/include/asm so linux/bpf.h can locate asm/types.h on all arches.
RUN arch="$(uname -m)" && \
    case "${arch}" in \
        x86_64) multiarch="x86_64-linux-gnu" ;; \
        aarch64|arm64) multiarch="aarch64-linux-gnu" ;; \
        *) echo "Unsupported architecture: ${arch}" >&2; exit 1 ;; \
    esac && \
    ln -sf /usr/include/${multiarch}/asm /usr/include/asm


WORKDIR /src
COPY . .
RUN go mod download
RUN CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go generate ./pkg/profiler
WORKDIR /src/cmd/profiler
RUN CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -tags ebpf_build -o /bin/ebpf-profiler

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates iproute2 && rm -rf /var/lib/apt/lists/*
COPY --from=build /bin/ebpf-profiler /usr/local/bin/ebpf-profiler
VOLUME ["/output"]
ENTRYPOINT ["ebpf-profiler"]
