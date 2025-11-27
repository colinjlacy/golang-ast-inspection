# Deployment Checklist - Ubuntu with Docker

Use this checklist to ensure smooth deployment on Ubuntu.

## Pre-Deployment

### System Requirements

- [ ] Ubuntu 22.04 LTS or newer installed
- [ ] Kernel 5.8+ running (`uname -r`)
- [ ] At least 4 GB RAM available
- [ ] At least 10 GB disk space free
- [ ] Docker installed (`docker --version`)
- [ ] Docker Compose installed (`docker compose version`)
- [ ] User added to docker group (no sudo needed)

### Verify Kernel Features

```bash
# Run this command:
cat /boot/config-$(uname -r) | grep -E 'CONFIG_BPF=|CONFIG_TRACEPOINTS=|CONFIG_DEBUG_FS='

# All three should show =y
```

- [ ] CONFIG_BPF=y
- [ ] CONFIG_TRACEPOINTS=y
- [ ] CONFIG_DEBUG_FS=y

### Verify Filesystem Mounts

```bash
# Check debugfs
mount | grep debugfs

# Check bpffs
mount | grep bpf
```

- [ ] /sys/kernel/debug mounted (debugfs)
- [ ] /sys/fs/bpf mounted (bpffs)

## Deployment Steps

### 1. Get the Code

```bash
git clone <your-repo>
cd golang-ast-inspection
```

- [ ] Code cloned
- [ ] In correct directory

### 2. Build

```bash
make docker-build
```

**Expected**: Successful build of both images

- [ ] eBPF program compiled
- [ ] Go profiler built
- [ ] Test app built
- [ ] No build errors

### 3. Start Services

```bash
make docker-up
```

**Expected**: Two containers running

- [ ] test-app container running
- [ ] profiler container running
- [ ] No error messages in logs

### 4. Verify Services

```bash
# Check containers
docker ps

# Should show 2 containers:
# - container_test-app
# - container_profiler
```

- [ ] Both containers in "Up" state
- [ ] Port 8080 exposed

### 5. Test Application

```bash
# Test the HTTP server
curl http://localhost:8080/

# Should return: "Hello from test server!"
```

- [ ] HTTP server responds
- [ ] Port 8080 accessible

### 6. Verify Profiling

```bash
# Make test requests
make test

# Check profiler logs
docker compose -f container/docker-compose.yml logs profiler | tail -20

# Check trace file
cat container/traces/http-trace.txt
```

- [ ] Test requests completed
- [ ] Profiler logs show "eBPF program loaded"
- [ ] Trace file created
- [ ] HTTP transactions captured in trace file

### 7. Verify Trace Contents

Check `container/traces/http-trace.txt` contains:

- [ ] Header: "Container HTTP Profiler Output"
- [ ] At least 5 HTTP transactions
- [ ] Request method and URL shown
- [ ] Response status codes shown
- [ ] Timestamps present

## Post-Deployment

### Performance Checks

```bash
# Check CPU usage
docker stats --no-stream

# Check memory usage
docker stats --no-stream | grep profiler
```

- [ ] CPU usage < 50%
- [ ] Memory usage < 500MB
- [ ] No memory leaks over time

### Continuous Testing

```bash
# Run continuous requests
while true; do curl -s http://localhost:8080/users > /dev/null; sleep 1; done
```

After 1 minute:

- [ ] Profiler still running
- [ ] Trace file growing
- [ ] No errors in logs
- [ ] Container not restarting

### Load Testing

```bash
# Install apache bench
sudo apt-get install apache2-utils

# Run load test
ab -n 1000 -c 10 http://localhost:8080/
```

- [ ] All requests succeeded
- [ ] Profiler captured traffic
- [ ] No container crashes
- [ ] Acceptable response times

## Troubleshooting

If any checks fail, see:

- **Build issues**: Check Docker and kernel version
- **eBPF loading**: See `EBPF-TROUBLESHOOTING.md`
- **No traces**: Check profiler logs for errors
- **Performance**: Reduce captured data or filter

## Production Readiness

Before production deployment:

- [ ] Reviewed security implications of privileged container
- [ ] Set up log rotation for trace files
- [ ] Configured monitoring/alerting
- [ ] Tested with actual application (not just test-app)
- [ ] Performance tested under expected load
- [ ] Backup plan if profiler fails
- [ ] Documentation for ops team

## Clean Up

When done testing:

```bash
make docker-down
make clean
```

- [ ] Containers stopped
- [ ] Images removed (if desired)
- [ ] Trace files cleaned up

## Success Criteria

✅ **Ready for production when**:

1. All pre-deployment checks pass
2. All deployment steps complete without errors
3. HTTP transactions are captured correctly
4. Performance is acceptable
5. Runs stable for at least 30 minutes
6. Clean shutdown works properly

## Quick Command Reference

```bash
# Start
make docker-up

# Test
make test

# View traces
cat container/traces/http-trace.txt

# View logs
docker compose -f container/docker-compose.yml logs -f profiler

# Stop
make docker-down

# Restart
make docker-down && make docker-up
```

## Timeline

Estimated deployment time: **10-15 minutes**

- System verification: 2-3 min
- Build: 3-5 min  
- Deploy: 1 min
- Testing: 5 min
- Verification: 2-3 min

## Support

If deployment fails:
1. Check kernel version first
2. Review profiler logs
3. See `UBUNTU-SETUP.md` for detailed Ubuntu guide
4. See `EBPF-TROUBLESHOOTING.md` for eBPF issues

Good luck! 🚀

