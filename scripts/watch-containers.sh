#!/bin/sh
# Watch Docker container exit events and log exit code, OOM status, and diagnostics for crash diagnosis.
# Run on the host: ./scripts/watch-containers.sh, or as a container (see docker-compose watcher service).
# Logs to stdout and optionally to a file (set WATCH_LOG=/path/to/log).
#
# Monitors cric-go-api, cric-ml-service and cricket-postgres by default. Use WATCH_CONTAINERS=c1,c2 to override.
# Legacy: WATCH_CONTAINER=cric-go-api for a single container.
#
# Exit code 137 = SIGKILL, typically OOM kill. Docker sets "OOMKilled: true" in container state.

set -e

# Default: both go-api and ml-service
WATCH_CONTAINERS="${WATCH_CONTAINERS:-cric-go-api,cric-ml-service,cricket-postgres}"
if [ -n "${WATCH_CONTAINER}" ]; then
  WATCH_CONTAINERS="${WATCH_CONTAINER}"
fi
WATCH_LOG="${WATCH_LOG:-}"
TAIL_LOGS="${WATCH_TAIL_LOGS:-50}"

log() {
  msg="[$(date -u +%Y-%m-%dT%H:%M:%SZ)] $*"
  echo "$msg"
  if [ -n "$WATCH_LOG" ]; then
    echo "$msg" >> "$WATCH_LOG"
  fi
}

# Normalize watch list to " name1 name2 " for substring matching
WATCH_LIST=" $(echo "$WATCH_CONTAINERS" | tr ',' ' ') "

in_watch_list() {
  name="$1"
  case "$WATCH_LIST" in
    *" $name "*) return 0 ;;
    *) return 1 ;;
  esac
}

report_die() {
  cid="$1"
  # Container may already be removed; capture what we can
  cname=$(docker inspect -f '{{.Name}}' "$cid" 2>/dev/null | sed 's/^\///') || cname="unknown"
  if ! in_watch_list "$cname"; then
    return 0
  fi

  exitCode=$(docker inspect -f '{{.State.ExitCode}}' "$cid" 2>/dev/null || echo "?")
  oomKilled=$(docker inspect -f '{{.State.OOMKilled}}' "$cid" 2>/dev/null || echo "?")
  image=$(docker inspect -f '{{.Config.Image}}' "$cid" 2>/dev/null || echo "?")
  memLimit=$(docker inspect -f '{{.HostConfig.Memory}}' "$cid" 2>/dev/null || echo "?")
  stateErr=$(docker inspect -f '{{.State.Error}}' "$cid" 2>/dev/null || echo "")
  startedAt=$(docker inspect -f '{{.State.StartedAt}}' "$cid" 2>/dev/null || echo "?")
  finishedAt=$(docker inspect -f '{{.State.FinishedAt}}' "$cid" 2>/dev/null || echo "?")

  log "=============================================="
  log "CRASH DETECTED: $cname"
  log "=============================================="
  log "  container=$cname"
  log "  id=$cid"
  log "  image=$image"
  log "  exit_code=$exitCode"
  log "  OOMKilled=$oomKilled"
  log "  memory_limit_bytes=$memLimit"
  log "  started_at=$startedAt"
  log "  finished_at=$finishedAt"
  if [ -n "$stateErr" ]; then
    log "  state_error=$stateErr"
  fi

  if [ "$exitCode" = "137" ] || [ "$oomKilled" = "true" ]; then
    log "  *** LIKELY OOM or SIGKILL ***"
    case "$cname" in
      cric-go-api)
        log "  hint=Check go-app logs; set GOMEMLIMIT, MEM_STATS_INTERVAL=5m, PRECOMPUTE_CONCURRENCY"
        ;;
      cric-ml-service)
        log "  hint=Check ml-service logs; increase mem_limit for ml-service; set n_jobs=1 for fielding/extras/win training"
        ;;
      cricket-postgres)
        # Postgres rarely causes the exhaustion it dies from; suspect a concurrent pipeline or training run.
        log "  hint=Postgres is usually the victim, not the cause; check cric-go-api/cric-ml-service memory at the same timestamp, then raise POSTGRES_MEM_LIMIT or the Docker VM memory"
        ;;
      *)
        log "  hint=Check container logs and memory limit"
        ;;
    esac
  fi

  log "--- Last $TAIL_LOGS log lines ---"
  if docker logs --tail "$TAIL_LOGS" "$cid" 2>/dev/null; then
    :
  else
    log "(unable to fetch logs - container may have been removed)"
  fi
  log "--- End log tail ---"
}

log "Watching containers: $WATCH_CONTAINERS (WATCH_CONTAINERS or WATCH_CONTAINER to override)"
log "Exit code 137 = likely OOM kill. OOMKilled=true from inspect = confirmed OOM."
log "Tail last $TAIL_LOGS log lines per crash (WATCH_TAIL_LOGS to override)."

# Listen for die events; filter applies to type=container, but we still get all containers
# so we filter by name in report_die
docker events --filter "type=container" --filter "event=die" --format '{{.Actor.ID}}' 2>/dev/null | while read -r cid; do
  report_die "$cid"
done
