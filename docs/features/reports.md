# Reports, PR links, web view and notifications

Four headless commands to report on the board and keep it current. They all
run locally: GitHub is reached only via your installed `gh` CLI, the web view
binds only to the loopback interface, and notifications go only to your
desktop.

## README section

### Standup summary

```sh
gotodo standup                      # last 24 hours, plain text
gotodo standup -since 7d            # any Go duration (90m, 36h) or whole days (7d)
gotodo standup -format markdown     # **bold** headings and `ids` for Teams/Slack
```

Prints a summary you can paste into chat. It has five sections, and any empty
section is left out:

| Section | What it lists |
|---|---|
| Done | Tasks that entered the terminal column (done/shipped) in the window, and who moved them |
| Moved | Every other column change in the window, `from → to`, by the agent name or "you" |
| Blocked now | Tasks in `blocked` and how long they have been there |
| Due today/overdue | Open tasks due today or past their deadline, most overdue first |
| In progress | Tasks in the working columns (`doing`, or `indev`/`testing-review`), with any live claim and PR |

When every section is empty, the output says "Nothing to report". Times use
24-hour HH:MM for today and DD/MM/YYYY HH:MM for older events. The command
only reads the board and never writes to it.

### Linking pull requests

```sh
gotodo pr 3fa1c9e2 https://github.com/acme/api/pull/42   # link by URL
gotodo pr 3fa1c9e2 42                                     # or by number ("#42" works too)
gotodo pr 3fa1c9e2                                        # show the link
gotodo pr 3fa1c9e2 ""                                     # clear it
```

A PR can be an `http(s)` URL or a number. Anything else is rejected. On a
project board, PR numbers resolve against the repository that contains
`.deadline/`.

### Syncing PR state from GitHub

```sh
gotodo pr sync -dry-run    # show what would change
gotodo pr sync             # apply it
```

For every open task with a linked PR, `pr sync` runs
`gh pr view <pr> --json state,mergedAt,statusCheckRollup,url` and then:

- **Merged** moves the task to the terminal column. If the board's approval
  gate is on and an agent (`GOTODO_AGENT`) is running the sync, the move is
  refused. The output then says the task awaits human approval, and a human
  ships it in the TUI.
- **A failing check** (conclusion or state `FAILURE`, `ERROR`, `CANCELLED`,
  `TIMED_OUT` or `STARTUP_FAILURE`) on an unmerged PR moves the task to
  `blocked`. It also appends `CI failing on PR <url> (DD/MM/YYYY)` to the
  description once. If that PR already has a note, no second one is added.
- Open PRs with green or pending checks, and closed unmerged PRs, are left
  alone.

The command prints one line per action, and dry-run lines start with
"would". Moves are recorded under `GOTODO_AGENT`, or as you when it is unset.
If `gh` is missing or not logged in, the command prints a hint
(`gh auth login`) and exits non-zero. It also exits non-zero if any PR could
not be checked. gotodo never reads or stores GitHub tokens.

### Read-only web board

```sh
gotodo serve                         # http://127.0.0.1:7777/
gotodo serve -addr 127.0.0.1:8080
```

Shows the board in a browser with a dark theme that matches the TUI. You get
the columns with their WIP limits, plus a card for each task with:

- a deadline countdown coloured like the TUI (green, then amber, then red)
- priority, labels and any live claim
- the PR link

`GET /api/board` returns the same data as JSON with these fields:

- `board`, `require_approval`, `generated_at` and `version`
- `columns[]`, each with `status`, `label`, `role`, `wip_limit` and `cards[]`

The page checks `/api/board` every 5 seconds and reloads when `version`
changes. Each request reads the board from disk again.

Security:

- The server only binds to a loopback address (`127.0.0.0/8`, `::1` or
  `localhost`). It refuses `0.0.0.0`, `:7777` and LAN IPs.
- It accepts only GET and HEAD. There are no write endpoints.
- Requests with a non-loopback `Host` header are refused, which guards
  against DNS rebinding.
- All task text is escaped by `html/template`. The page sends a strict
  Content-Security-Policy with a per-request nonce, plus `nosniff`,
  `no-referrer` and `DENY` framing headers.

Ctrl+C stops the server cleanly.

### Desktop notifications

```sh
gotodo notify                        # one pass
gotodo notify -watch                 # keep checking every minute until Ctrl+C
gotodo notify -watch -interval 5m    # minimum 10s
```

notify sends a notification when:

- a task becomes **blocked**
- a task enters the **review** column (`testing-review` on a dev board,
  `doing` on a personal board)
- a task is **due today**
- a task is **overdue**

Each event fires once. A task that is blocked again later fires again.
Delivery uses `notify-send` on Linux and `osascript` on macOS. On WSL, or
when neither tool is available, it prints to stdout instead. On the first run,
blocked and review events reach back only 24 hours, so old history is not
replayed.

The dedupe record is stored in
`<user cache dir>/gotodo/notify-<hash of board path>.json`, never in the
board. Delete that file to reset it. notify only reads the board and never
writes to it.

## Skill additions

Add to the gotodo-task-board skill and AGENTS snippet:

```md
- After opening a PR for a card: `gotodo pr <id> <pr-url>` so the board tracks it.
- Run `gotodo pr sync` before picking new work: merged PRs ship their cards
  (or report "awaits human approval" on gated boards; do not try to ship
  those yourself), and failing CI moves cards to blocked with a dated note.
  Use `-dry-run` first if unsure. It needs an authenticated `gh`; if it says
  gh is missing or not logged in, tell the human, do not try to log in.
- For a status update, run `gotodo standup -format markdown` and paste
  the output; do not hand-write summaries of board activity.
- `gotodo serve` and `gotodo notify -watch` are long-running human tools; do
  not start them from an agent session unless asked.
```

## Implementation notes

- `internal/task/pr.go`: `NormalizePR` and `Board.SetPR(id, pr, now)`.
- `internal/gh`: a `Runner` interface around `gh` (the `CLI` runner uses
  `exec.Command`), `CheckAuth`, `View` and `Sync`. `Sync` applies changes in
  memory, and the command decides whether to save (dry-run never saves).
- `internal/report`: `ParseSince`, `BuildStandup`, `Standup.Text` and
  `Standup.Markdown`.
- `internal/web`: `ValidateAddr`, `BuildView`, `Server.Handler` and
  `Serve(ctx, …)`. The template is embedded from `board.html.tmpl`.
- `internal/notify`: `Events`, `Notifier.Pass`, `Notifier.Watch`, the
  `Desktop` sender (system calls are injectable) and `StatePath`.
- Commands: `cmd_standup.go`, `cmd_pr.go`, `cmd_serve.go` and
  `cmd_notify.go`. Tests replace `newGHRunner` and `newNotifySender` with
  fakes, so they never call the real `gh` or send real notifications.
