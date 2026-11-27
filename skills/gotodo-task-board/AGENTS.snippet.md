<!-- deadline:begin - managed by the gotodo installer, do not edit between the markers -->
## GoTodo task board

You are working in a project tracked on a `gotodo` kanban board. For EVERY
code change — feature, fix, refactor — log it before starting and move it
along as you work:

0. `export GOTODO_AGENT=<your-name>` (e.g. `claude`) so your moves and claims
   are recorded under your name.
1. `gotodo board` first, from inside the repo. Work in a code repository goes
   on that repo's project board; the personal board is for the user's
   personal tasks only. `board` prints the type, columns and
   `path: <file> (project|personal)`; if it says `personal` while you are in
   a repo, run `gotodo init` (gotodo also does this on your first change).
   Never assume columns; use the pipeline it prints:
   `todo → doing → done` (personal) or
   `todo → indev → testing-review → shipped` (development). Either may use
   `blocked` when stuck.
2. `gotodo list` and skip a duplicate card if one already covers the work.
3. `gotodo add "title" [-desc "detail"] [-status todo]` before writing code,
   or take queued work with `gotodo next -claim -start`; resume a card with
   `gotodo context <id>` and `gotodo release <id>` when you stop. Never take a
   card another agent has claimed.
   Never log a title-only card or subtask: always pass `-desc` (goal, context,
   scope, acceptance criteria), `-priority`, `-label`, and `-worktree`,
   `-attachment` (user-provided files) and `-working-note` (plans/notes you
   write) whenever they apply. Keep them updated with `gotodo edit` as you work.
   Linked files use project-relative paths inside the project (`docs/`,
   `notes/`, or `.deadline/notes/` if they shouldn't be committed), never
   `/tmp`, scratchpad, session or cache dirs: those disappear and the next
   agent can't read them. Copy a scratch draft into the project first.
4. Use `-parent <id-prefix>` when the work is a subtask of an existing card;
   inspect the family with `gotodo show <id-prefix>` or `list -parent`.
5. `gotodo move <id-prefix> <active-column>` when starting, then through the
   reported pipeline's review/terminal columns as work is verified and done.
   Move forward only when the stage is complete: every acceptance criterion
   ticked, checks run and passing in this session, a Verification section on
   the card, and every phase of the skill you were asked to use finished.
   Otherwise leave the card with a Progress section. A move can start a paid
   loop run, so never hand on half-done work.
6. `gotodo edit <id-prefix> [flags]` for title, description, deadline, priority,
   labels, `-blocked-by`, parent, worktree, attachments or working notes.
7. `gotodo delete <id-prefix>` only for cards logged by mistake — no undo.

Headless commands only (never run bare `gotodo`, it opens a TUI). If `gotodo`
is not on PATH, try exactly `~/.local/bin/gotodo`; do not search the filesystem
for it. If missing there too, tell the human to install it and continue without
tracking. Use the CLI only: never read, search, parse, or edit
`.deadline/board.json`, `tasks.json`, or any other board storage file directly.
<!-- deadline:end -->
