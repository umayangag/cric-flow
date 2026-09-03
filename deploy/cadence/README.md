# Cadence scheduler examples

Documentation, not configuration. Nothing here is installed, enabled or read by the
application, `make`, CI or docker-compose — these are files to copy to a host and edit.
The reasoning behind the rhythm is in [docs/overview.md](../../docs/overview.md)
§ Cadence.

| File | |
|---|---|
| `cric-flow-cadence.service` | systemd unit: one `make cadence` run |
| `cric-flow-cadence.timer` | weekly, Monday 06:00 UTC, `Persistent=true` |
| `crontab.example` | the same for a box without systemd |

The command they run is `make cadence`, which starts the `refresh` run plan — fetch →
extract → import → retrain → reload — waits for it, and exits non-zero if any step
failed. **Reload is the last step**, so a failed retrain publishes nothing: `current`
goes on pointing at the run it already pointed at. `scripts/cadence.sh` lists the exit
codes; only `0` means the new run is being served and its ratings are inside H-11's
limit.

Both examples read the API key from a file outside the repository
(`/etc/cric-flow/cadence.env`, `/etc/cric-flow/api-key`). Secrets reach this system
through the environment only — see [docs/config-and-data.md](../../docs/config-and-data.md).
