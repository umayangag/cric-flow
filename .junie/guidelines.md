# Junie Directives — Condensed

## Core
- Be an efficient, pragmatic engineer. Ship clean, tested, maintainable code. Avoid edit loops.

## Hard Rules
- Never loop on edits; pause and ask to adjust plan.
- Never commit to main/master; always use a feature branch if not on a feature branch already.
- Do not contact support; solve with provided tools.
- Use existing packages and tools from opensource sources to avoid reinventing the wheel.
- Use context_index.json as the entry point to understand the project structure and context when needed. Use it smartly to minimize token usage.
- Always keep the code clean and readable for humans. Use descriptive names for files, variables, methods, and functions. Short functions are better than long ones.
- Never suppress errors. Log errors and return.
- Use the available tools to minimize token usage and improve productivity.

## SOP (do this order)
1) Use the available MCPs like serena, sequential-thinking, gopls and gemini-cli to minimize credit and token usage and improve productivity.
2) Analyze & Plan: Create the plan file: `.junie_plans/{master_plan_name}/{timestamp}-{plan_number}-{slug}.md` with files, changes, tests, acceptance criteria + exact commands. Lock plan; execute in phases.
3) Code & Test: Follow project standards. Use table test style. TDD: write failing test, then code. Tests independent with clear assertions. Keep code small and clean.
4) Use descriptive names for variables, functions, packages, etc. Keep code easily readable and comprehensive to the human eye. Prefer simplicity to cleverness.
5) Plan Hierarchy & Anti-Drift Rule:
* Assign a stable unique Plan ID to the main plan for the task: `X` (e.g., `1-{plan_name}`). All sub-plans must derive from this ID.
* All sub plans must be created under the same directory as the main plan.
* Number sub-plans as `X.y` for step `y` of the main plan; deeper levels continue as `X.y.z` and so on.
* At the start of any sub-plan, record its parent path (breadcrumb) explicitly: `Parent: X` or `Parent: X.y`.
* After completing a sub-plan, update the status of the subplan, immediately return to its parent plan and reconcile:
- Update the parent's status for the corresponding step.
- Verify the parent's acceptance criteria affected by the sub-plan.
- Ensure no sibling subtasks are left untracked.
* Never start a new top-level plan while `X` is active. If scope changes, request approval to revise `X` rather than creating a new top-level plan.
* In all status updates and PR descriptions, include the active path (e.g., `Active path: X -> X.2 -> X.2.1`).
* Close the main plan `X` only after all direct steps and sub-plans under its hierarchy are marked complete and verified.
* Try to complete sub-plans one by one, one step at a time in the strict order specified in the plan.
* If a plan gets too complex or big, break it down into smaller sub-plans.
* Do not use serena/think_about_task_adherence more than once consecutively. Stick to the already documented plan.

## Engineering Principles to follow
- KISS, DRY.
- OOP: Encapsulation, Abstraction, Inheritance, Polymorphism.
- SOLID: SRP, OCP, LSP, ISP, DIP.
- Prefer simple, readable, minimal code and package flow.

## Code Quality instructions
- Prefer clarity; descriptive long names (no abbreviations).
- Restructure when it simplifies flow.
- Reuse code; keep functions small; modular design.
- Robust error handling; never suppress errors. Log and return.
- Comments explain why.
- Singleton logger per project; log success and error paths.
- Use available formating commands. eg: `make lint`, `make fmt`

## Defaults
- Branching: never to main/master; branches `type/short-slug`; Conventional Commits; focused PRs.
- Testing:
  - Use interfaces + mockery for mocks (no fakes).
  - For unit tests, use table-driven tests following the AAA pattern (arrange, act, assert). Also prefer using *_test package for unit tests. avoid using if conditions inside t.Run. Instead move conditional checks inside assert function.
  - error handling and error logging is a must. do not suppress errors.
  - Go: 1.25+, std `testing`, table‑driven, `make test` or `go test ./...`, use `httptest`, avoid ifs in tests; use asserts; name the file `{pkg}_test.go` and append `_test` to the package name`.
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
- at the completion of a plan, make sure all unit tests and linters pass.

## Project-Specific
- `src/` is prototype reference; do not modify.
- Project not live; prefer clarity/maintainability over legacy.
- No need of backward compatibility or support for legacy features.
- Restructure when it streamlines (outside `src/`).
- Ensure code is tested and documented (Makefile, README).
- All python code is in ml-service/
- All go code is in go-app/
- No need to be backward compatible. no backfilling or preserving old features. It is always easier to start fresh.