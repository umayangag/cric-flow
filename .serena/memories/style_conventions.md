### Style and Conventions

General:
- `src/` is a reference prototype and must not be modified. Implementations live in `go-app/` and `ml-service/`.
- Prefer small, focused commits. Keep changes scoped to one component when possible.

Go (go-app/):
- Language: Go 1.25+
- Formatting: `gofumpt` + `golines` via `make fmt`; verify with `make fmt-check`
- Linting: `golangci-lint` via `make lint`; also `make vet` available
- Structure: `cmd/<tool>` for entrypoints, `internal/` for packages (db, features, repos, clients, etc.)
- Config precedence: CLI flags > env vars > `go-app/config.json` > built-in defaults
- Error handling: idiomatic Go errors, no panics for expected flows; return contextual `fmt.Errorf` wrapping where helpful.
- Logging: follow existing patterns (check `internal` packages and `cmd` for logger setup)

Python (ml-service/):
- Runtime: Python 3.10+
- Web: FastAPI served by `uvicorn`
- Virtualenv: `.venv` (created by `make init`)
- Dependencies: managed via `requirements.txt`
- Formatting: `isort` + `black` (line length default 100 from Makefile)
- Lint: `ruff` (auto-fix for unused imports), `flake8` for checks
- API: `app/main.py` defines FastAPI app; follow Pydantic models and route patterns used there
- Scripts: `ml/` contains training and validation scripts; keep separation between training and serving

Git & CI:
- Run formatters and linters before PR
- Ensure unit tests pass (`go test ./...`), and ML scripts execute without errors when changed
- Keep docker images reproducible with provided Dockerfiles

Documentation:
- Update component `README.md` when adding new entrypoints or changing flags/envs
- Add concise docstrings/comments for non-trivial logic; keep in sync across Go and Python implementations where relevant.