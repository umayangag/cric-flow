# Surface probes

Scripts that measure a **shipped surface** against a **running stack**, rather than a model
against a frame. An experiment under `scripts/experiments/xi/` answers "is this model
better?"; a probe here answers "does the product behave the way this document says it
does?", and it answers it by making the calls a client makes.

They are run by hand, against a stack serving the run under test, and they write nothing —
no artifact, no database row. Each one prints a table and exits non-zero when its assertion
fails, so it can be re-run after any change to the surface it covers.

| probe | what it asserts | recorded in |
|---|---|---|
| `p1_2_play_mode.py` | Pinning the eleven Optimise returned reproduces Optimise's own numbers; a swap for a player who dominates on every as-of vector never lowers the displayed probability; and one Play-mode re-score's end-to-end latency, per format, at the served draw count | `docs/PRODUCT_ROADMAP.md` § 10, P1-2 |

## Running one

The probe needs `ml-service` on the import path (it reads the served run's artifacts to
choose which swap is an upgrade) and a stack to call:

```sh
PATH="$(pwd)/ml-service/.venv/bin:$PATH" PYTHONPATH=ml-service \
  python scripts/probes/p1_2_play_mode.py \
    --api http://127.0.0.1:8080 --ml http://127.0.0.1:8000 \
    --models-dir output/ml-service --repeats 30 --json probe.json
```

A latency figure describes the stack it was measured on, and the two are not separable: the
same `/simulate` payload measured 317 ms against an ml-service running as a host process and
145 ms against the same run in its container. Record the configuration beside the number.
