# Container crash and OOM diagnosis

This document describes how to confirm whether the go-app (go-api) container is crashing due to **out of memory (OOM)** or another reason.

## 1. In-container mechanisms

### Exit code logging (entrypoint)

The go-api image runs an entrypoint script that runs the API binary. When the process exits (normally or killed), the script logs the **exit code** to stderr before the container stops. View with:

```bash
docker logs cric-go-api 2>&1 | tail -20
```

- **Exit code 137** = process received SIGKILL. The Linux kernel often sends this when the **OOM killer** terminates the process. So 137 in logs (or from `docker inspect`) strongly suggests OOM.

### Memory stats in logs

The API can log **memory stats** at startup and periodically so you can see heap usage before a crash.

- **Startup:** One line is always logged at startup: `memory stats` with `heap_alloc_mb`, `heap_sys_mb`, `heap_inuse_mb`, `sys_mb`, `num_gc`.
- **Periodic:** Set `MEM_STATS_INTERVAL` to a duration (e.g. `5m` or `10m`) so the process logs the same stats on an interval. If the container is OOM-killed, the last log line before the crash shows how high memory was.

Example (docker-compose):

```yaml
environment:
  MEM_STATS_INTERVAL: "5m"
```

Or when running the container:

```bash
docker run -e MEM_STATS_INTERVAL=5m ...
```

Use `docker logs cric-go-api` to see these lines. If the last "memory stats" line shows very high `heap_inuse_mb` or `sys_mb` right before the process disappears, that supports an OOM diagnosis.

## 2. Watcher container (recommended)

The stack includes a **watcher** service that runs alongside go-api and logs container exit code and OOM status. It uses the Docker socket to subscribe to events.

Start the full stack (including the watcher):

```bash
docker compose up -d
```

The watcher container (`cric-watcher`) starts with the rest. View its logs:

```bash
docker logs -f cric-watcher
```

When `cric-go-api` (or the container set in `WATCH_CONTAINER`) exits, the watcher logs one line with `exit_code` and `OOMKilled`. Exit code **137** or **OOMKilled=true** indicates a likely OOM kill.

Optional:

- **Watch a different container:** `WATCH_CONTAINER=cric-ml-service docker compose up -d`
- **Persist watcher output to a volume:** `WATCH_LOG=/logs/watcher.log docker compose up -d`, then inspect the `watcher-logs` volume or copy from the container.

The watcher is built from `Dockerfile.watcher` and uses `scripts/watch-containers.sh`; it requires the Docker socket mount (already configured in `docker-compose.yml`).

## 3. Watcher script on the host

You can also run the same script on the host (where `docker` is available) instead of as a container:

```bash
./scripts/watch-containers.sh
```

Options: `WATCH_CONTAINER=cric-go-api` (default), `WATCH_LOG=/path/to/file`. Useful when you are not using the watcher container.

## 4. After a crash

1. **Exit code:**  
   `docker inspect cric-go-api --format '{{.State.ExitCode}}'`  
   If the container was restarted, inspect the **last** run (e.g. from `docker events` or your watcher log). Exit code **137** → likely OOM.

2. **OOMKilled:**  
   `docker inspect cric-go-api --format '{{.State.OOMKilled}}'`  
   Only valid for the **current** (exited) container instance. If the compose/ orchestrator has already recreated the container, this may be for the new one. Use the watcher to capture this at die time.

3. **Last logs:**  
   `docker logs cric-go-api 2>&1 | tail -100`  
   Check for the entrypoint “exited with code …” line and the last “memory stats” line to see usage before exit.

4. **Memory limit:**  
   If you set a memory limit (e.g. `mem_limit` in compose), the kernel can kill the process when usage exceeds it. Increase the limit or reduce workload/memory use.

## 5. Summary

| Signal                    | Meaning                          |
|---------------------------|----------------------------------|
| Exit code 137             | SIGKILL; often OOM               |
| OOMKilled=true (inspect)  | Docker recorded OOM kill         |
| Last “memory stats” high  | Supports OOM (high usage)        |
| Entrypoint “exited 137”   | Visible in logs when using image with entrypoint script |

Use **MEM_STATS_INTERVAL=5m** and the **entrypoint** in the go-api image for in-container visibility. Run the **watcher container** with `docker compose up` so exit code and OOMKilled are recorded in `docker logs cric-watcher` when go-api (or the watched container) dies.
