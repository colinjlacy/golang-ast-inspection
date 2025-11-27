.PHONY: help build-profiler podman-build podman-up podman-down docker-build docker-up docker-down test clean check-podman

# Detect if we're using podman or docker
CONTAINER_CMD := $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)
COMPOSE_CMD := $(shell command -v podman-compose 2>/dev/null || command -v docker-compose 2>/dev/null)

help:
	@echo "eBPF Container Profiler - Make targets:"
	@echo ""
	@echo "  podman-build     - Build container images with Podman"
	@echo "  podman-up        - Start containers with Podman"
	@echo "  podman-down      - Stop containers with Podman"
	@echo "  docker-build     - Build container images with Docker"
	@echo "  docker-up        - Start containers with Docker"
	@echo "  docker-down      - Stop containers with Docker"
	@echo "  test             - Run test HTTP requests"
	@echo "  clean            - Clean build artifacts"
	@echo ""
	@echo "Note: eBPF compilation happens inside the container build"
	@echo "      (eBPF can only be compiled on Linux)"

check-podman:
	@command -v podman >/dev/null 2>&1 || { echo "Error: podman not found. Install podman or use 'make docker-*' targets"; exit 1; }

# Podman targets (recommended for macOS)
podman-build: check-podman
	@echo "Building with Podman..."
	@mkdir -p container/traces
	podman-compose -f container/docker-compose.yml build

podman-up: check-podman
	@echo "Starting containers with Podman..."
	@mkdir -p container/traces
	podman-compose -f container/docker-compose.yml up -d
	@echo ""
	@echo "✅ Containers started with Podman"
	@echo ""
	@echo "View logs:"
	@echo "  podman-compose -f container/docker-compose.yml logs -f"
	@echo ""
	@echo "Make test requests:"
	@echo "  make test"
	@echo ""
	@echo "View traces:"
	@echo "  cat container/traces/http-trace.txt"

podman-down: check-podman
	podman-compose -f container/docker-compose.yml down

# Docker targets (for Linux systems with Docker)
docker-build:
	@echo "Building with Docker..."
	@mkdir -p container/traces
	docker compose -f container/docker-compose.yml build

docker-up:
	@echo "Starting containers with Docker..."
	@mkdir -p container/traces
	docker compose -f container/docker-compose.yml up -d
	@echo ""
	@echo "✅ Containers started with Docker"
	@echo ""
	@echo "View logs:"
	@echo "  docker compose -f container/docker-compose.yml logs -f"
	@echo ""
	@echo "Make test requests:"
	@echo "  make test"
	@echo ""
	@echo "View traces:"
	@echo "  cat container/traces/http-trace.txt"

docker-down:
	docker compose -f container/docker-compose.yml down

# Test target (works with either)
test:
	@echo "Making test HTTP requests..."
	@echo ""
	@curl -s http://localhost:8080/ && echo "" || echo "❌ Failed to connect to http://localhost:8080/"
	@sleep 1
	@curl -s http://localhost:8080/users && echo "" || true
	@sleep 1
	@curl -s http://localhost:8080/user/42 && echo "" || true
	@sleep 1
	@curl -s "http://localhost:8080/message?text=hello" && echo "" || true
	@sleep 1
	@curl -s http://localhost:8080/health && echo "" || true
	@echo ""
	@echo "✅ Test requests completed"
	@echo ""
	@echo "Check traces:"
	@echo "  cat container/traces/http-trace.txt"

clean:
	rm -f profiler
	rm -rf container/traces/*
	@echo "✅ Cleaned build artifacts"

