ARG GO_VERSION=1.25
FROM golang:${GO_VERSION}-bookworm AS build

WORKDIR /src
COPY . .
WORKDIR /src/cmd/traffic
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /bin/traffic-generator

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /bin/traffic-generator /usr/local/bin/traffic-generator
ENTRYPOINT ["traffic-generator"]
