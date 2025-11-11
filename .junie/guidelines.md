# Junie Directives — Condensed

## Core
- Be an efficient, pragmatic engineer. Ship clean, tested, maintainable code. Avoid edit loops.

## Hard Rules
- Never loop on edits; pause and ask to adjust plan.
- Never commit to main/master; always use a feature branch.
- Do not contact support; solve with provided tools.

## SOP (do this order)
1) Analyze & Plan: Create the plan file: `.junie_plans/{chat_title}/{timestamp}-{plan_number}-{slug}.md` with files, changes, tests, acceptance criteria + exact commands. Lock plan; execute in phases.
2) Plan hierarchy: Main ID `X` (e.g., `1`); subplans `X.y`, `X.y.z`. Start each subplan with `Parent: ...`. After each subplan step is completed update status: update parent status once all its subplans are done, verify affected acceptance criteria, ensure no sibling drift. Keep Active path in updates/PRs. Do not start a new top‑level while `X` is active. Close `X` only when all children verified.Try to focus on completing one task at a time.
3) Code & Test: Follow project standards. TDD: write failing test, then code. Tests independent with clear assertions. Keep code small and clean.
4) Code & Test: Follow project standards. TDD: write failing test, then code. Tests independent with clear assertions. Keep code small and clean.
5) Commit & PR: Feature branch + Conventional Commits. Open small PRs per phase.

## Engineering Principles
- KISS, DRY.
- OOP: Encapsulation, Abstraction, Inheritance, Polymorphism.
- SOLID: SRP, OCP, LSP, ISP, DIP.
- Prefer simple, readable, minimal code and package flow.

## Code Quality
- Prefer clarity; descriptive names (no abbreviations).
- Restructure when it simplifies flow.
- Reuse code; keep functions small; modular design.
- Robust error handling; never suppress errors.
- Comments explain why.
- Singleton logger per project; log success and error paths.

## Defaults
- Branching: never to main/master; branches `type/short-slug`; Conventional Commits; focused PRs.
- Testing:
  - Use interfaces + mockery for mocks (no fakes).
  - Go: 1.25+, std `testing`, table‑driven, `make test` or `go test ./...`, use `httptest`, avoid ifs in tests; use asserts; name `{pkg}_test.go`.
  - Python: 3.10+, `pytest` under `tests/` with `test_*.py`, deps via `requirements.txt`, `make test` or `pytest -q`, no ifs in tests; table‑driven.
- Execution: Prefer Makefile targets and docker-compose. Default to unit tests; run integration via `docker compose up` only when planned.
- Data/Artifacts (ML): small deterministic fixtures (`tests/fixtures/` or `data/sample/`), no large downloads; temp under `output/`; set seeds.
- Network: tests offline by default; mock externals; allow internet only if plan says.
- Plans: every plan file must include explicit acceptance criteria and exact verification commands.
- Security: secrets via env vars; provide `.env.example`; never commit real secrets; document required env vars.
- Lint/Format: Go `gofmt -s`, `go vet`, `golangci-lint` (if configured); Python `black`, `isort`, `ruff`/`flake8`; prefer `make lint`/`make fmt`.
- CI: align with existing; if none and needed, propose minimal workflow in plan.
- Runtime config: prefer env vars; CLIs support flags; put config files under `configs/`.
- Docs: when behavior/commands change, update `README.md` and `docs/` in same branch and plan.

## Project-Specific
- `src/` is prototype reference; do not modify.
- Project not live; prefer clarity/maintainability over legacy.
- Restructure when it streamlines (outside `src/`).
- Ensure code is tested and documented (Makefile, README).