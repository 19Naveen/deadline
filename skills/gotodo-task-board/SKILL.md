---
name: gotodo-task-board
description: Task management and kanban board for coding agents via the gotodo CLI. Use for every software task, code change, feature, bug fix, refactor, implementation, test, or review: track work through todo, indev, testing-review, blocked, and shipped.
---

# GoTodo Task Board

Track dev work on a per-project kanban board with the `gotodo` CLI. Project and personal boards are resolved by the CLI; their persistence files are private implementation details.

## Workflow

1. **Initialize project tracking:** For work in a repository or project, run `gotodo init` from its project root before inspecting the board. It reuses an existing ancestor project board or creates one, and may add `.deadline/` to `.gitignore`. Do not initialize for unrelated personal tasks; respect higher-priority restrictions on writes.
2. **Inspect and look first:** `gotodo board` prints the board type and accepted columns; `gotodo list` prevents duplicate cards. Never guess the pipeline. Personal boards use `todo → doing → done`; development boards use `todo → indev → testing-review → shipped`; both may use `blocked`.
3. **Log before starting:** `gotodo add "title" [-desc "detail"] [-deadline DD/MM/YYYY] [-status todo]` — it prints the id; keep its short prefix for later moves.
4. **Group related work:** when work belongs under an existing parent task, use `gotodo add "title" -parent <parent-id>`. Inspect a family with `gotodo show <parent-id>` or `gotodo list -parent <parent-id>`. Keep hierarchy one level deep.
5. **Move it along:** `gotodo move <id-prefix> <status>` through the exact columns reported by `gotodo board`. Any unique id prefix works. A parent cannot finish before all subtasks.
6. **Update details through the CLI:** `gotodo edit <id-prefix> [-title "..."] [-desc "..."] [-deadline DD/MM/YYYY] [-parent <id>] [-worktree PATH]`. Pass `-worktree` (also on `add`) when the work lives in a git worktree. An explicitly blank description, deadline, parent, or worktree clears it; use `move` for status.
7. **Stuck:** move to `blocked` and record the reason with `edit -desc`; move back to the reported active column (`doing` or `indev`) when unblocked.
8. **Wrong card:** `gotodo delete <id-prefix>` removes it headless — no confirm or undo. Deleting a parent detaches its subtasks; archived cards cannot be deleted headless.

## Task documents

Record relevant user-provided reference documents with repeatable `-attachment PATH` flags, and agent-prepared plans or working documents (including `docs/task.md`, `phase2.md`, or `frontend.md` created by other workflow skills) with repeatable `-working-note PATH` flags on `add` or `edit`. Use project-relative paths where practical; files are referenced, not copied. On `edit`, flags replace the entire corresponding list, so use `gotodo show <id>` first and include existing paths to keep them; `-attachment ""` or `-working-note ""` clears the respective list. Do not create or link unrelated documents merely to fill a field.

## Rules

- One card per work item; `list` first, then `add` only when it is really new.
- Headless commands only — bare `gotodo` opens the interactive TUI, never run that here.
- CLI only: never read, search, parse, or edit `.deadline/board.json`, `tasks.json`, or any board storage file directly. Use `gotodo board`, `list`, `show`, `add`, `move`, `edit`, and `delete`.
- If `gotodo` is not on PATH, try exactly `~/.local/bin/gotodo`. Do not search the filesystem for it. If that is also missing, tell the human to install it and continue without tracking.
