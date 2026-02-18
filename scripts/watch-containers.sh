#!/bin/sh
# Watch Docker container exit events and log exit code and OOM status for crash diagnosis.
# Run on the host: ./scripts/watch-containers.sh, or as a container (see docker-compose watcher service).
# Logs to stdout and optionally to a file (set WATCH_LOG=/path/to/log).
#
# Exit code 137 = SIGKILL, typically OOM kill. Docker sets "OOMKilled: true" in container state.

set -e

CONTAINER_NAME="${WATCH_CONTAINER:-cric-go-api}"
WATCH_LOG="${WATCH_LOG:-}"

log() {
  msg="[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $*"
  echo "$msg"
  if [ -n "$WATCH_LOG" ]; then
    echo "$msg" >> "$WATCH_LOG"
  fi
}

log "Watching container: $CONTAINER_NAME (set WATCH_CONTAINER to override)"
log "Exit code 137 = likely OOM kill. OOMKilled=true from inspect = confirmed OOM."

docker events --filter "container=$CONTAINER_NAME" --filter "event=die" --format '{{.Actor.ID}}' 2>/dev/null | while read -r cid; do
  exitCode=$(docker inspect -f '{{.State.ExitCode}}' "$cid" 2>/dev/null || echo "?")
  oomKilled=$(docker inspect -f '{{.State.OOMKilled}}' "$cid" 2>/dev/null || echo "?")
  log "CONTAINER_DIE container=$CONTAINER_NAME id=$cid exit_code=$exitCode OOMKilled=$oomKilled"
  if [ "$exitCode" = "137" ] || [ "$oomKilled" = "true" ]; then
    log "  -> Likely OOM or SIGKILL. Check container logs and set MEM_STATS_INTERVAL=5m for memory stats."
  fi
done
