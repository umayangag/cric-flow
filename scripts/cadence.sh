#!/usr/bin/env bash
#
# The data cadence (A-5): one command a scheduler can run unattended.
#
# It starts the `refresh` run plan — fetch → extract → import → retrain → reload —
# waits for it, and reports the freshness verdict the pipeline exists to keep true.
#
# The work is the server's, not this script's. go-app already owns step ordering, the
# lanes, run history and the rule that a plan stops at its first failure; a scheduler
# entry that re-implemented any of that would be a second pipeline to keep in step with
# the first. What is here is what a cron job needs and a browser does not: start it,
# wait without a human watching, and turn the outcome into an exit code.
#
# Nothing published unless everything before it succeeded. `reload` is the last step, so
# a retrain that fails — including on the data-quality gate, which the unattended path
# never accepts on an operator's behalf — leaves `current` pointing where it pointed.
#
# Environment:
#   API_URL          go-app base URL (default http://localhost:8080)
#   API_KEY          X-API-Key for the admin surface (required; no default -- OPS-02)
#   PLAN             run plan to execute (default refresh)
#   POLL_SECONDS     how often to re-read plan state (default 30)
#   TIMEOUT_MINUTES  give up waiting after this long (default 90)
#   DRY_RUN          1 to check preconditions and print the request, changing nothing
#
# Exit codes (distinct so a scheduler can alert on the ones that matter):
#   0  the plan completed and the ratings are fresh
#   1  a precondition failed: missing tool, unreachable API, unknown plan
#   2  a step failed; nothing after it ran
#   3  a plan was already in flight, so this run did not start one
#   4  the plan was cancelled, or the wait timed out
#   5  the plan completed but the ratings are still stale (H-11 would refuse a prediction)

set -euo pipefail

API_URL="${API_URL:-http://localhost:8080}"
# No default (OPS-02). A scheduler entry that fell back to a key every checkout of this
# repository knows would authenticate against any stack started with the old default, so
# an unset key is a precondition failure (exit 1) rather than a silent one.
API_KEY="${API_KEY:-}"
PLAN="${PLAN:-refresh}"
POLL_SECONDS="${POLL_SECONDS:-30}"
TIMEOUT_MINUTES="${TIMEOUT_MINUTES:-90}"
DRY_RUN="${DRY_RUN:-0}"

log() { printf '[cadence %s] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }
die() { log "ERROR: $1"; exit "${2:-1}"; }

require_tool() {
  command -v "$1" >/dev/null 2>&1 || die "$1 is required but not installed"
}

# api METHOD PATH [BODY] — one authenticated call, failing on a non-2xx status.
api() {
  local method="$1" path="$2" body="${3:-}"
  local args=(-fsS --max-time 60 -X "$method" -H "X-API-Key: ${API_KEY}")
  if [ -n "$body" ]; then
    args+=(-H 'Content-Type: application/json' -d "$body")
  fi
  curl "${args[@]}" "${API_URL}${path}"
}

# print_steps STATE — one line per step, so a journal entry says where a run got to.
print_steps() {
  jq -r '.steps[]? | "  \(.status | (. + "        ")[0:9]) \(.label)\(if .error then " — " + .error elif .note then " — " + .note else "" end)"' <<<"$1"
}

require_tool curl
require_tool jq

[ -n "$API_KEY" ] ||
  die "API_KEY is not set and has no default; use the key the stack was started with"

log "target ${API_URL}, plan ${PLAN}, poll ${POLL_SECONDS}s, timeout ${TIMEOUT_MINUTES}m"

curl -fsS --max-time 15 "${API_URL}/health" >/dev/null 2>&1 ||
  die "go-app is not reachable at ${API_URL} (is the stack up?)"

state="$(api GET /ops/pipeline/plan)" || die "could not read plan state (is API_KEY right?)"

known="$(jq -r '.plans[]?' <<<"$state" | tr '\n' ' ')"
grep -qw -- "$PLAN" <<<"$known" || die "unknown plan ${PLAN}; this server offers: ${known}"

if [ "$(jq -r '.running' <<<"$state")" = "true" ]; then
  log "a plan is already in flight:"
  print_steps "$state"
  die "not starting a second one" 3
fi

freshness() { api GET /api/ml/xi-status | jq -c '{run_id, ratings_through, ratings}'; }

if [ "$DRY_RUN" = "1" ]; then
  log "DRY RUN — nothing will be started"
  log "would POST ${API_URL}/ops/pipeline/run-plan  {\"plan\":\"${PLAN}\"}"
  log "server offers plans: ${known}"
  log "last plan: $(jq -r '.plan // "none"' <<<"$state") ($(jq -r '.finished_at // "unfinished"' <<<"$state"))"
  print_steps "$state"
  log "ratings now: $(freshness)"
  exit 0
fi

# The plan this endpoint reports before we start ours. /ops/pipeline/plan answers with
# the *latest* plan, so without this the first poll could read the previous run, see
# `running: false` and call our cadence complete before it had been recorded at all.
previous_start="$(jq -r '.started_at // "none"' <<<"$state")"

log "starting the plan"
api POST /ops/pipeline/run-plan "{\"plan\":\"${PLAN}\"}" >/dev/null ||
  die "the server refused to start the plan" 3

deadline=$(( $(date +%s) + TIMEOUT_MINUTES * 60 ))
ours=0
last=""
while :; do
  sleep "$POLL_SECONDS"
  state="$(api GET /ops/pipeline/plan)" || die "lost contact with ${API_URL} while waiting" 4

  if [ "$ours" = "0" ]; then
    if [ "$(jq -r '.started_at // "none"' <<<"$state")" = "$previous_start" ]; then
      [ "$(date +%s)" -lt "$deadline" ] || die "the plan never started within ${TIMEOUT_MINUTES}m" 4
      continue
    fi
    ours=1
  fi

  # Only the transitions, so an hour of polling is a handful of lines rather than 120.
  current="$(jq -c '[.running, [.steps[]?.status]]' <<<"$state")"
  if [ "$current" != "$last" ]; then
    print_steps "$state"
    last="$current"
  fi

  [ "$(jq -r '.running' <<<"$state")" = "false" ] && break
  [ "$(date +%s)" -lt "$deadline" ] || die "the plan was still running after ${TIMEOUT_MINUTES}m" 4
done

if jq -e '[.steps[]?.status] | any(. == "FAILED")' <<<"$state" >/dev/null; then
  jq -r '.steps[]? | select(.status == "FAILED") | "failed at \(.label): \(.error // "no reason recorded")"' <<<"$state" |
    while IFS= read -r line; do log "$line"; done
  die 'the plan stopped; nothing after that step ran, so `current` points where it did' 2
fi
if jq -e '[.steps[]?.status] | any(. == "CANCELLED")' <<<"$state" >/dev/null; then
  die "the plan was cancelled" 4
fi

log "plan complete"
verdict="$(freshness)"
log "ratings now: ${verdict}"

if [ "$(jq -r '.ratings.fresh' <<<"$verdict")" != "true" ]; then
  die "the run is served but its ratings are outside the H-11 limit — live predictions will be refused with RATINGS_STALE" 5
fi

log "done"
