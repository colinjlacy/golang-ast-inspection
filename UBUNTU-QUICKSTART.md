# Ubuntu Quick Start - 5 Minutes

The fastest way to get the HTTP profiler running on Ubuntu.

## Prerequisites Check (30 seconds)

```bash
# Check Ubuntu version (need 22.04+)
lsb_release -a

# Check kernel version (need 5.8+)
uname -r

# Check Docker is installed
docker --version

# Check Docker Compose
docker compose version
```

All good? Continue! ✅

## Installation (3 minutes)

### Step 1: Clone Project

```bash
# If you haven't already
cd /your/workspace
git clone <repo-url>
cd golang-ast-inspection
```

### Step 2: Build Everything

```bash
make docker-build
```

This builds:
- eBPF program (inside Linux container)
- Go profiler application
- Test HTTP server

**Wait 2-3 minutes** for build to complete.

### Step 3: Start Profiling

```bash
make docker-up
```

Containers are now running!

## Testing (1 minute)

### Make Test Requests

```bash
make test
```

You should see HTTP responses from the test server.

### View Captured Traffic

```bash
cat container/traces/http-trace.txt
```

**Expected output**:
```
Container HTTP Profiler Output
==============================

Profiler started at 2025-11-27 18:45:12

[2025-11-27 18:45:15.123] PID 42
  → HTTP GET /
     Host: localhost:8080
  ← Response 200 OK
     Content-Type: text/plain
     Body: Hello from test server!

[2025-11-27 18:45:16.234] PID 42
  → HTTP GET /users
     Host: localhost:8080
  ← Response 200 OK
     Content-Type: application/json
     Body: [{"id":1,"name":"Alice"},...]
```

## Success Indicators

✅ You should see:
- 5 HTTP transactions in trace file
- Each with request method, URL
- Each with response status code
- Timestamps on each transaction
- No errors in logs

## Cleanup

```bash
make docker-down
```

## What's Next?

### Use with Your Own App

Replace the test app with your actual HTTP service:

```yaml
# In docker-compose.yml, replace test-app with:
your-app:
  image: your-app-image
  # ... your app config
```

Then restart:
```bash
make docker-down
make docker-up
```

### View Live Logs

```bash
docker compose -f container/docker-compose.yml logs -f profiler
```

### Continuous Monitoring

```bash
# Watch traces in real-time
tail -f container/traces/http-trace.txt
```

## Troubleshooting

### No trace file?

```bash
# Check logs
docker compose logs profiler

# Look for "eBPF program loaded" message
```

### Empty trace file?

```bash
# Make sure app is receiving traffic
curl http://localhost:8080/

# Check profiler is running
docker ps
```

### Build failed?

```bash
# Check kernel version
uname -r  # Need 5.8+

# Try cleaning and rebuilding
make clean
make docker-build
```

## Full Documentation

- **[UBUNTU-SETUP.md](UBUNTU-SETUP.md)** - Complete Ubuntu guide
- **[DEPLOYMENT-CHECKLIST.md](DEPLOYMENT-CHECKLIST.md)** - Pre-deployment checklist
- **[EBPF-TROUBLESHOOTING.md](EBPF-TROUBLESHOOTING.md)** - eBPF-specific issues
- **[README.md](README.md)** - Full project documentation

## One-Command Test

```bash
# Start, test, and view results
make docker-up && sleep 5 && make test && cat container/traces/http-trace.txt
```

That's it! You're profiling HTTP traffic with eBPF! 🎉

