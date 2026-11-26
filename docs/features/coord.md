# Agent coordination: claims, `next`, `context`, approval

Status: implemented on `feat/coord`. The README-ready section below can be
pasted into README.md; the "Skill additions" lines belong in
`skills/gotodo-task-board/SKILL.md`.

---

## README section

### Working with coding agents

Several agents (and humans) can share one board without stepping on each
other. Agents identify themselves with an environment variable:

```sh
export GOTODO_AGENT=claude      # or codex, gemini, claude-2, ...
```

With `GOTODO_AGENT` set, every move an agent makes is recorded with its name
in the task history (shown by `gotodo context`). Without it, the CLI acts as
a human, as the TUI always does.

#### Picking up work: `gotodo next`

```sh
gotodo next                       # peek: which task should be worked next?
gotodo next -claim                # ...and claim it for $GOTODO_AGENT
gotodo next -claim -start         # ...and move it to the active column
gotodo next -start -ttl 4h        # longer claim (default 2h)
```

`next` prints the chosen task in the same format as `gotodo show`, or
`no available tasks`. It picks from the **first column** (todo) and skips:

- archived tasks;
- tasks with a live claim by another agent (your own claims stay eligible);
- parents that still have open subtasks (work the subtasks instead);
- tasks blocked by any task that has not reached the terminal column
  (`done`/`shipped`). A subtask also inherits its parent's blockers. A
  deleted blocker no longer blocks.

The remaining tasks are ordered by priority (high first), then earliest
deadline (tasks without one last), then oldest created.

`-start` moves the task to the board's active column (`doing` on personal
boards, `indev` on development boards). When an agent runs `-start` it always
claims the task too; a human (no `GOTODO_AGENT`) running `-start` just moves
it. `-claim` without `GOTODO_AGENT` is an error.

Pick, claim and start run as one transaction under the board lock, so two
agents running `next -claim` at the same moment always get different tasks.

#### Claims

```sh
gotodo claim 3fa1c9e2             # claim for $GOTODO_AGENT for 2h
gotodo claim 3fa1c9e2 -ttl 30m    # re-claiming your own task extends it
gotodo release 3fa1c9e2           # drop the claim when you stop
```

- A claim lasts until its TTL expires (`claim_until`); after that anyone may
  take the task over. Expired claims are shown as `(expired)` in `context`.
- Claiming a task another agent holds fails with
  `task is claimed by <agent> until DD/MM/YYYY HH:MM`.
- Claims only gate other agents' **claims and `next`**. They never block
  `move`: a human (or agent) can always move a card.
- An agent can release its own claim or an expired one. A human (no
  `GOTODO_AGENT`) can release anyone's claim.
- `move` does not touch claims. A claim left on a card that reached the
  terminal column is dropped automatically by the next `next`, `claim` or
  `release` on that board, so agents should still `release` explicitly when
  they stop.

#### Resuming work: `gotodo context <id>`

Prints one markdown document an agent can read to resume a task: title,
status, priority, labels, deadline, worktree, PR, claim, last agent, parent
(with status), subtasks with statuses, blocked-by tasks with statuses and
whether they are done, attachment paths, working notes, description and the
last 10 status transitions with who made them and when
(DD/MM/YYYY HH:MM).

Working notes are inlined in a fenced block when the file exists and is under
64 KB. Relative note paths are resolved against the project root (the
directory containing `.deadline/`) first, then the current directory, so
`context` works from any subdirectory. Missing, oversized, directory or
binary notes are listed with the reason instead. Attachment contents are
never printed, only their paths.

`context` is read-only.

#### Human approval gate

```sh
gotodo approval status            # on or off
gotodo approval on                # agents can no longer move cards to done/shipped
gotodo approval off
```

With approval on, a move into the terminal column by an agent
(`GOTODO_AGENT` set) is refused; a human ships it from the TUI or a plain
CLI. `approval on` and `approval off` are themselves refused when
`GOTODO_AGENT` is set, so an agent cannot lift the gate that reviews it.
`approval status` works for everyone.

---

## Skill additions

Lines to add to `skills/gotodo-task-board/SKILL.md` (e.g. after the numbered
workflow):

```markdown
## Coordinating with other agents

- Identify yourself before any gotodo command: `export GOTODO_AGENT=<your-name>`
  (e.g. `claude`). Your moves are then recorded under that name.
- To pick up work, run `gotodo next -claim -start`: it chooses the
  highest-priority unblocked todo task nobody else holds, claims it for you
  (2h by default, `-ttl 4h` for longer) and moves it to the active column.
  "no available tasks" means there is nothing for you; do not take claimed
  cards.
- To resume a task, run `gotodo context <id>` and read it fully: it includes
  your working notes, blockers, subtasks and recent history.
- If `gotodo claim` says a task is claimed by another agent, leave it alone
  and pick something else with `gotodo next`.
- Re-run `gotodo claim <id>` to extend your claim on long work.
- When you stop working on a task (finished, handing off, or blocked), run
  `gotodo release <id>`.
- If `move` to done/shipped is refused because the board requires approval,
  move the card to testing-review and tell the human; never try to change
  `gotodo approval` (it is refused for agents).
```

---

## Implementation notes (for the coordinator)

- `internal/task/claim.go`: `Task.ClaimHeld`, `Board.Claim`, `Board.Release`,
  `Board.ReleaseFinishedClaims`, `Board.BlockingTasks`, `Board.NextTask`,
  `DefaultClaimTTL`, and `UpdateLocked(path, actor, fn)`: lock, fresh load,
  mutate, write. `Save` lets the last writer win, which is wrong for
  check-then-set claims, hence the separate transaction.
  `Board.writeLocked` duplicates the temp-file+rename tail of `Save`; when
  merging, consider making `Save` call it to remove the duplication.
- `internal/task/context.go`: `Board.Context(id, ContextOptions)`.
- `runMove` was not modified (owned elsewhere). To release claims right when
  a card ships, add after a successful `board.Move` in `runMove`:
  `board.ReleaseFinishedClaims(time.Now())`.
- `Board.Move` is used by `next -start`, so any WIP-limit check added to
  `Move` applies there too (the whole next transaction aborts with the
  error and nothing is claimed).
