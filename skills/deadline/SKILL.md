---
name: deadline
description: Track implementation work on a project kanban board (todo, indev, testing-review, blocked, shipped) via the gotodo CLI. Use when starting, progressing, blocking, or finishing dev tasks.
---

# Deadline — project task board

Each project has its own board in `./.deadline/` (gitignored, invisible to other projects). The human's personal board lives elsewhere — never touch files outside the current project.

## Setup (once per project, skip if `.deadline/` exists)

`gotodo init` — creates `./.deadline/board.json` with the dev pipeline.

## Commands (run in the project dir; plain text in and out)

- `gotodo add "title" [-desc "detail"] [-deadline DD/MM/YYYY] [-status todo]` — prints the id
- `gotodo list [-status blocked]` — prints `id [status] title · due DD/MM/YYYY`
- `gotodo move <id-prefix> <status>` — moves the card; any unique id prefix works

Columns: `todo, indev, testing-review, blocked, shipped`.

## Rules

- Log each work item with `add` before starting it, then `move` it along: `todo → indev → testing-review → shipped`.
- Park stuck items in `blocked` with the reason in the description; move back to `indev` when unblocked.
- Bare `gotodo` opens the interactive TUI — never run that here; headless commands only. The human sees your writes live in their open board.
