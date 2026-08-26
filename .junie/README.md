### Junie Slash Commands

This directory contains **skills** — reusable automation workflows triggered by slash commands in the Junie chat.

> **`skills/*/SKILL.md` are generated. Do not edit them by hand.**
>
> The same seven workflows exist for Cursor in `.cursor/skills/`, in a different file
> format (YAML frontmatter vs. the trigger line Junie expects), so they cannot be one
> file and symlinking would break one tool. `.cursor/skills/` is the source; these are
> generated from it by `scripts/sync-junie-skills.py`.
>
> ```bash
> make sync-skills          # regenerate
> make sync-skills-check    # fail if out of date
> ```
>
> They previously drifted apart by hand, and not only cosmetically: the Junie copy of
> `/run-check-all-incremental` still described three components and omitted the
> `frontend-backend-sync-check` step that CI enforces.

### Available Commands

| Command | Description |
|---------|-------------|
| `/fix-gemini-reviews [PR#]` | Fetch unresolved Gemini code review threads on a PR, implement the suggested fixes, and resolve the threads via GraphQL. Uses current branch PR if no number given. |
| `/gemini-review-check-iterate` | Full quality cycle: run Gemini code review → fix issues → pass all local checks → re-review until clean → commit and push. |
| `/publish-feature` | Publish the current feature branch through a review loop: pass all checks, trigger Gemini PR review, fix comments, and repeat (up to 10 cycles or until no unresolved threads remain). |
| `/run-check-all-incremental` | Run all checks (lint, format, tests) per component in order (frontend → go-app → ml-service). On failure, re-run only the failed step until it passes, then continue. |
| `/run-github-workflows-local` | Verify that all GitHub CI workflows pass locally before pushing, by mapping each workflow to equivalent local commands. |
| `/update-deps` | Update dependencies across all three components: go-app (Go modules), ml-service (pip-tools), and frontend (npm). |
| `/update-deps-prs` | Same dependency bumps as `/update-deps`, but opens **one PR per component** off `main` (go-app, ml-service, frontend). |

### How to Use

1. Open the Junie chat in your IDE.
2. Type the slash command (e.g., `/update-deps`).
3. Junie reads the corresponding `SKILL.md` and executes the workflow automatically.

Some commands accept arguments:
- `/fix-gemini-reviews 53` — targets PR #53
- `/fix-gemini-reviews` — auto-detects the PR for the current branch

### Prerequisites

Most commands require:
- **GitHub CLI (`gh`)** authenticated with `repo` scope
- **Go 1.26+**, **Python 3.12**, **Node 20.x**
- **Docker** (for integration tests and Postgres)

See each skill's `SKILL.md` for detailed prerequisites.

### Directory Structure

```
.junie/
├── README.md                          # This file
├── guidelines.md                      # Junie behavior directives
└── skills/
    ├── fix-gemini-reviews/SKILL.md
    ├── gemini-review-check-iterate/SKILL.md
    ├── publish-feature/SKILL.md
    ├── run-check-all-incremental/SKILL.md
    ├── run-github-workflows-local/SKILL.md
    ├── update-deps/SKILL.md
    └── update-deps-prs/SKILL.md
```

### Adding a New Skill

1. Create the skill under **`.cursor/skills/<name>/SKILL.md`**, with frontmatter:
   ```
   ---
   name: my-command
   description: One line; this becomes the Junie trigger line.
   ---
   ```
2. Document prerequisites, step-by-step instructions, and acceptance criteria.
3. Run `make sync-skills` to generate the Junie copy under `skills/<name>/`.
4. Update this README with the new command.
