---
name: gotodo-task-board
description: Task management and kanban board for coding agents via the gotodo CLI or its MCP server. Use for every software task, code change, feature, bug fix, refactor, implementation, test, or review: pick up work with claims, track it through todo, indev, testing-review, blocked, and shipped, record dependencies, PRs, attachments and working notes, and coordinate with other agents on the same board.
---

# GoTodo Task Board

Track dev work on a per-project kanban board with the `gotodo` CLI. Project and personal boards are resolved by the CLI; their persistence files are private implementation details. Several agents may share one board, so identify yourself, claim what you work on, and leave other agents' claimed cards alone.

## Setup

1. **Identify yourself:** `export GOTODO_AGENT=<your-name>` (e.g. `claude`, `codex`, `opencode`) before any gotodo command. Moves, claims and the Agents tab record you under that name; without it you count as the human.
2. **Initialize project tracking:** for work in a repository or project, run `gotodo init` from its project root. It reuses an existing ancestor board or creates one, and may add `.deadline/` to `.gitignore`. Do not initialize for unrelated personal tasks; respect higher-priority restrictions on writes.
3. **Inspect:** `gotodo board` prints the board type, columns, WIP limits and whether shipping needs approval. Never guess the pipeline. Personal boards use `todo → doing → done`; development boards use `todo → indev → testing-review → shipped`; both may use `blocked`.
4. **MCP:** if a `gotodo` MCP server is connected, its tools (`board`, `list_tasks`, `show_task`, `add_task`, `move_task`, `edit_task`, `delete_task`, `next_task`, `claim_task`, `release_task`, `task_context`) do the same as the commands below; prefer them over shelling out.

## Workflow

1. **Look first:** `gotodo list` (filters: `-status`, `-parent`, `-label`, `-priority`) and avoid duplicating a card that already exists.
2. **Pick up queued work:** `gotodo next -claim -start` chooses the highest-priority unblocked todo task nobody holds, claims it for you (2h; `-ttl 4h` for longer) and moves it to the active column. "no available tasks" means nothing is free; never take another agent's claimed card. `gotodo claim <id>` claims a specific card and extends your own claim on long work.
3. **Resume:** `gotodo context <id>` prints everything needed to continue a card — fields, parent, subtasks, blockers, working-note contents and recent history. Read it before continuing work someone else started.
4. **Log new work before starting, fully filled in:** a card is the hand-off to the human and to the next agent, so a title alone is never enough. Follow **Writing a complete card** below for every task and subtask. `gotodo add` prints the id; keep its short prefix.
5. **Group related work:** use `-parent <parent-id>` for subtasks; inspect with `gotodo show <parent-id>` or `gotodo list -parent <parent-id>`. Hierarchy is one level deep, and a parent cannot finish before all subtasks.
6. **Move it along:** `gotodo move <id-prefix> <status>` through the columns reported by `gotodo board`. Moving a card to the terminal column releases its claim and unblocks its dependants automatically.
7. **Update details:** `gotodo edit <id-prefix> [-title ...] [-desc ...] [-deadline DD/MM/YYYY] [-priority ...] [-label NAME]... [-blocked-by ID]... [-parent ID] [-worktree PATH]`. Repeatable flags replace the whole list on `edit`; an explicitly blank value clears a field. Use `move` for status.
8. **Stuck:** move to `blocked`, record the reason with `edit -desc`, and `gotodo release <id>`; move back to the active column when unblocked.
9. **Stopping or handing off:** `gotodo release <id>` so another agent can pick the card up.
10. **Wrong card:** `gotodo delete <id-prefix>` removes it — no confirm or undo. Deleting a parent detaches its subtasks.

## Writing a complete card

Before every `gotodo add` (task or subtask), gather and pass all of these. Skip a field only when it truly does not apply, never because it is effort.

| Field | Flag | What to put |
|---|---|---|
| Title | (argument) | Short, specific, imperative: what will be true when done. Include the ticket or issue reference if there is one. |
| Description | `-desc` | Always. Use the template below: goal, context, scope, acceptance criteria. |
| Priority | `-priority` | `high` (blocking or urgent), `medium` (normal), `low` (nice to have). |
| Labels | `-label` (repeatable) | Area and type, e.g. `backend`, `frontend`, `bug`, `docs`. |
| Parent | `-parent` | For subtasks: the top-level task they belong to. |
| Blockers | `-blocked-by` (repeatable) | Tasks that must ship first. |
| Deadline | `-deadline DD/MM/YYYY` | When the user gave a date or one is implied. |
| Worktree | `-worktree` | The worktree path or branch when not working in the main checkout. Check with `git rev-parse --show-toplevel` and `git branch --show-current`. |
| Attachments | `-attachment` (repeatable) | Every user-provided reference for this task: specs, .docx/.pdf/.xlsx, screenshots, tickets, files the user pointed you at. |
| Working notes | `-working-note` (repeatable) | Every plan or notes file you or another workflow wrote for this task (`docs/task.md`, `phase2.md`, `frontend.md`, handover docs). |

Pass a multi-line description with a quoted heredoc:

```sh
gotodo add "Show Xero tax rates in the category dropdown" -priority high -label frontend -label xero \
  -parent 5fa76639 -worktree ../app-tax-rates \
  -attachment docs/specs/tax-rates-uat.docx -working-note docs/FRONTEND_HANDOVER_TAX_RATES.md \
  -desc "$(cat <<'CARD'
**Goal:** users pick a Xero tax rate per category instead of typing a code.

**Context:** UAT 25/11 items #2 and #7; the backend endpoint is GET /xero/tax-rates (task 7e3de89e).

**Scope:**
- dropdown fed by /xero/tax-rates, cached per session
- unknown codes rejected with an inline error

**Acceptance criteria:**
- [ ] dropdown lists active Xero rates with name and percentage
- [ ] saving sends the picked TaxType
- [ ] unit tests for the mapper; manual check against the demo org
CARD
)"
```

Bad: `gotodo add "tax rates frontend"`. Good: the command above. Subtasks get their own description with their own acceptance criteria; never leave them title-only because the parent has a description.

**Keep the card current as you work** (`gotodo edit`; run `gotodo show <id>` first, because list flags replace the whole list):
- When you write or update a plan or notes file, add it with `-working-note`.
- When the user shares a file, add it with `-attachment`.
- When scope changes, a decision is made or you hit a blocker, update `-desc` (keep the template; add a **Progress** or **Decisions** section).
- Before moving to review, update `-desc` with what changed and how it was verified (tests run, results).

## Priority, dependencies and WIP

- Work `P:high` cards first; `list` sorts each column by priority.
- When a task needs another shipped first, record it with `-blocked-by <id>`. Do not move dependent cards into `blocked` yourself: gotodo auto-blocks them and moves them back when the blockers ship (watch for `auto-blocked` / `unblocked` lines). Cycles are rejected.
- If output shows `! <column> over WIP limit (n/limit)`, finish or hand off work in that column before starting more. Change limits (`gotodo wip <column> N`) only when the human asks.

## Approval gate

If a move to done/shipped is refused because the board requires approval, leave the card in `testing-review` and tell the human; they approve it in the TUI. Never try to change `gotodo approval` — it is refused for agents.

## Task documents

Record user-provided reference documents with repeatable `-attachment PATH` flags, and agent-prepared plans or working documents (including `docs/task.md`, `phase2.md`, or `frontend.md` created by other workflow skills) with repeatable `-working-note PATH` flags on `add` or `edit`. Use project-relative paths; files are referenced, not copied. On `edit`, these flags replace the whole list, so check `gotodo show <id>` first and include paths to keep; `-attachment ""` or `-working-note ""` clears a list. Keep working notes current — `gotodo context` shows them to the next agent. Do not create or link unrelated documents merely to fill a field.

## Pull requests and reports

- After opening a PR for a card: `gotodo pr <id> <pr-url>`.
- `gotodo pr sync` (needs an authenticated `gh`) ships cards whose PRs merged and blocks cards whose CI fails. Use `-dry-run` if unsure. If it says `gh` is missing or not logged in, tell the human; do not try to log in.
- For a status update, run `gotodo standup -format markdown` and paste the output rather than hand-writing a summary.
- `gotodo notify -watch` is a long-running human tool; do not start it unless asked.

## Rules

- One card per work item; `list` first, then `add` only when it is really new.
- Never create a title-only card or subtask: every card gets a description in the template, plus priority, labels and any worktree, attachments and working notes that apply.
- Headless commands only — bare `gotodo` opens the interactive TUI, never run that here.
- CLI or MCP only: never read, search, parse, or edit `.deadline/board.json`, `tasks.json`, or any board storage file directly.
- If `gotodo` is not on PATH, try exactly `~/.local/bin/gotodo`. Do not search the filesystem for it. If that is also missing, tell the human to install it and continue without tracking.
