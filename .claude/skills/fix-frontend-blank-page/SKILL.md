---
name: fix-frontend-blank-page
description: Diagnoses and fixes a blank/white page in the frontend dev server. Use when the Vite app renders nothing, the tab is white after a frontend change, or the console shows "<name>_default is not a function", "Failed to fetch dynamically imported module", or a 504 Outdated Optimize Dep. Distinguishes a stale Vite dependency cache from a real application error instead of guessing.
---

# Fix a blank frontend page

A white page has exactly two causes. **Identify which one before changing anything** — the
recovery for a stale cache does nothing for an application bug, and vice versa.

Run everything from the repository root.

## 1. Read the actual error first

Do not skip this. Open the app and read the browser console.

Since the bootstrap guard landed (`frontend/index.html`), a startup failure paints a diagnostic
panel naming the cause and the fix. **If you see that panel, follow what it says and stop.** A
genuinely blank page now means the guard itself did not run — check that the inline `<script>`
in `frontend/index.html` is intact.

If you cannot open a browser, reproduce from the shell:

```sh
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:5173/
curl -s http://localhost:5173/src/main.tsx | head -20
```

## 2. Decide which cause it is

**Stale Vite dependency cache** — the console error matches one of:

- `<something>_default is not a function` (most often `styled_default`)
- `Failed to fetch dynamically imported module`
- `504 (Outdated Optimize Dep)`
- `does not provide an export named ...`

**Application error** — anything else (a real `TypeError` in a component, a bad import path, a
failed API call at module scope). Fix the code; the steps below will not help.

## 3. Recover from a stale cache

```sh
cd frontend && npm run dev:clean
```

Then **hard-reload** the browser tab (Cmd+Shift+R). A normal reload can reuse the stale module
graph the tab already holds.

That is the whole fix. It takes about 10 seconds.

## 4. Confirm the cache is healthy

A healthy cache has few entries. Many chunk files means the package got split across many
pre-bundle entries, which is the condition that causes this failure.

```sh
cd frontend
ls node_modules/.vite/deps/chunk-*.js | wc -l          # expect ~10, not ~90
python3 -c "import json; m=json.load(open('node_modules/.vite/deps/_metadata.json')); \
print('mui entries:', [k for k in m['optimized'] if k.startswith('@mui/material')])"
```

`mui entries` must be exactly `['@mui/material']`. More than one means a subpath import crept
back in — see below.

## 5. Stop it recurring

The root cause is **pre-bundle entry count**. Every distinct specifier into a package is its own
esbuild entry; many entries force the package to be code-split across shared chunks, and any
re-optimization re-splits them all. An already-open tab then holds chunk URLs whose files no
longer agree, and a lazy `init_` binding goes missing — the `styled_default` crash.

Re-optimization is triggered by Vite discovering a **new** dependency entry, which is why this
used to fire right after a batch of frontend changes.

So the rule is:

- **Import MUI through the barrel**: `import { Button, Box } from '@mui/material';`
- **Never** `import Button from '@mui/material/Button';` — that adds an entry.
- Adding a component to an existing barrel import needs no re-optimization, so the failure
  cannot be triggered.
- `@mui/icons-material/<Icon>` subpaths are the deliberate exception: that package is far too
  large to barrel-import.

Check before finishing:

```sh
grep -rn "from '@mui/material/" frontend/src | grep -v icons-material   # expect no output
```

If that prints anything, fold those imports into the file's `@mui/material` barrel import and
re-run `npm run dev:clean`.

## 6. Verify

```sh
cd frontend && npm run typecheck && npm run lint && npm test
```

Then load the app and confirm it renders with a clean console.
