ARG GO_VERSION=1.25
FROM golang:${GO_VERSION}-bookworm AS build

WORKDIR /src
COPY . .
WORKDIR /src/cmd/server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /bin/http-service

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /bin/http-service /usr/local/bin/http-service
EXPOSE 8080
ENTRYPOINT ["http-service"]
