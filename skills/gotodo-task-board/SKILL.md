---
name: gotodo-task-board
description: Task management and kanban board for coding agents via the gotodo CLI. Use for every software task, code change, feature, bug fix, refactor, implementation, test, or review: track work through todo, indev, testing-review, blocked, and shipped.
---

# GoTodo Task Board

Track dev work on a per-project kanban board with the `gotodo` CLI. Project and personal boards are resolved by the CLI; their persistence files are private implementation details.

## Workflow

1. **Inspect the board:** `gotodo board` prints the board type and accepted columns. Never guess the pipeline. Personal boards use `todo → doing → done`; development boards use `todo → indev → testing-review → shipped`; both may use `blocked`.
2. **Look first:** `gotodo list` and avoid duplicating a card that already exists. Run `gotodo init` only when a project-local development board is wanted and none exists.
3. **Log before starting:** `gotodo add "title" [-desc "detail"] [-deadline DD/MM/YYYY] [-status todo]` — it prints the id; keep its short prefix for later moves.
4. **Move it along:** `gotodo move <id-prefix> <status>` through the exact columns reported by `gotodo board`. Any unique id prefix works. The human sees writes live.
5. **Update details through the CLI:** `gotodo edit <id-prefix> [-title "..."] [-desc "..."] [-deadline DD/MM/YYYY]`. An explicitly blank description or deadline clears it; use `move` for status.
6. **Stuck:** move to `blocked` and record the reason with `edit -desc`; move back to the reported active column (`doing` or `indev`) when unblocked.
7. **Wrong card:** `gotodo delete <id-prefix>` removes it headless — no confirm or undo. `list` first; archived cards cannot be deleted headless.

## Rules

- One card per work item; `list` first, then `add` only when it is really new.
- Headless commands only — bare `gotodo` opens the interactive TUI, never run that here.
- CLI only: never read, search, parse, or edit `.deadline/board.json`, `tasks.json`, or any board storage file directly. Use `gotodo board`, `list`, `add`, `move`, `edit`, and `delete`.
- If `gotodo` is not on PATH, try exactly `~/.local/bin/gotodo`. Do not search the filesystem for it. If that is also missing, tell the human to install it and continue without tracking.
