#!/bin/sh
# Wrapper to run the API binary and log exit reason for crash diagnosis.
# Do not use exec so we can log exit code when the process exits or is killed.
# Exit code 137 = SIGKILL, often from kernel OOM killer.
/api
exitCode=$?
echo "go-api exited with code $exitCode" >&2
if [ "$exitCode" = 137 ]; then
  echo "Exit 137 usually indicates OOM kill (SIGKILL by kernel). Check memory limits and MEM_STATS_INTERVAL logs." >&2
fi
exit $exitCode
