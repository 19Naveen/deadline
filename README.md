<div align="center">

<img src="banner.png" alt="deadline" width="100%">

*Green. Amber. Red. Then a cross.*

![Go](https://img.shields.io/badge/go-1.22-00ADD8?logo=go&logoColor=white)
![Stars](https://img.shields.io/github/stars/19Naveen/deadline?style=flat)
![Last commit](https://img.shields.io/github/last-commit/19Naveen/deadline)

A terminal kanban board that never lets you forget when something is due.

</div>

---

## The board

Four columns. Two-line cards. A countdown on each card that gets louder as the day closes in.

```
  Board    Analytics    Archive    Calendar    Agents
╭──────────────────────╮╭──────────────────────╮╭──────────────────────╮╭──────────────────────╮
│ TODO (3)             ││ DOING (1)            ││ BLOCKED (1)          ││ DONE (1)             │
│                      ││                      ││                      ││                      │
│ ▌ ● Ship the QBR… 6d ││   Rewri… ◆ claude 2d ││   Waiting on de… 12d ││   Drop the old API   │
│ ▌ Draft, review, se… ││   Split the lexer o… ││   Mocks not landed   ││                      │
│                      ││                      ││                      ││                      │
│   Renew the TLS … 1d ││                      ││                      ││                      │
│   Staging box        ││                      ││                      ││                      │
│                      ││                      ││                      ││                      │
│   Chase the… late 3d ││                      ││                      ││                      │
│   Third follow-up    ││                      ││                      ││                      │
│                      ││                      ││                      ││                      │
│                      ││                      ││                      ││                      │
│                      ││                      ││                      ││                      │
╰──────────────────────╯╰──────────────────────╯╰──────────────────────╯╰──────────────────────╯

enter Open  a Add  e Edit  d Delete  u Undo  / Search  m Grab  f Family  ctrl+t Focus
tab Switch  ? Help  q Quit
```

A card can carry a priority dot before its title (`●` red for high, amber for
medium, blue for low; with `NO_COLOR` or a plain terminal it reads `H`, `M` or
`L` instead) and, next to the countdown, `◆ claude` while
an agent holds a claim on it. In a narrow column the claim shortens to `◆ cl…`
and then a bare `◆`; cards never get taller. A column with a WIP limit shows it
in the header: `IN DEV (4/3)`, with the count in red once it is over.

No mouse. No config file. No account.

---

## Deadlines

Each card with a deadline ends its title row
with a countdown, and it only gets louder as the day gets closer:

| Time left | Colour | Looks like |
|---|---|---|
| more than 3 days | green | `9d` |
| 3 days or less | amber, bold | `3d` |
| due today or tomorrow | red, bold | `today`, `1d` |
| the day has passed | red, bold | `late 3d` |
| task is done | no countdown | |

The date itself, in DD/MM/YYYY, is in the detail view and on the Calendar
page. A finished task never turns red: being late is history by then, not an
alarm. The selected card, focused column and active tab are marked in blue.

Deadlines are optional. Leave the field blank and the card has no countdown.

---

## Analytics

Second page. Everything is derived from each task's transition history, so it is measured rather than tallied.

```
╭────────╮╭─────────╮╭───────────╮╭────────╮
│    3   ││    1    ││     1     ││    1   │
│  TODO  ││  DOING  ││  BLOCKED  ││  DONE  │
╰────────╯╰─────────╯╰───────────╯╰────────╯
THROUGHPUT
             █
1 completed over 14 days · 18/07/2026 → 31/07/2026
CYCLE TIME
mean 3d 22h · median 3d 22h · over 1 completed
mean time spent per column:
todo     ████████████████████████ 3d 22h
doing    ░░░░░░░░░░░░░░░░░░░░░░░░ 0m
blocked  ░░░░░░░░░░░░░░░░░░░░░░░░ 0m
done     ░░░░░░░░░░░░░░░░░░░░░░░░ 0m
BLOCKED
1 currently blocked
  Waiting on design                        2h
STREAK
current 1 day · longest 1 day
          May 2026                      June 2026                      July 2026
 Mo  Tu  We  Th  Fr  Sa  Su     Mo  Tu  We  Th  Fr  Sa  Su     Mo  Tu  We  Th  Fr  Sa  Su
                  1   2   3      1   2   3   4   5   6   7              1   2   3   4   5
  4   5   6   7   8   9  10      8   9  10  11  12  13  14      6   7   8   9  10  11  12
 11  12  13  14  15  16  17     15  16  17  18 ▄19 ▄20  21     13  14  15  16  17  18  19
 18  19  20 ▄21  22  23  24     22  23  24  25  26  27  28    ▄20 ▄21  22  23  24  25  26
 25  26  27  28  29  30  31     29  30                        █27 ▄28 ▄29  30  31
▁▄▓█ more finished that day · underline is today, 30/07/2026
```

Cycle time runs from created to done. Time-per-column shows where work actually sits, which is rarely where you think.

The streak is up to six real months, not an anonymous strip: a day you finished something is shaded, darker the more you closed, and today is underlined. Narrow the terminal and it shows fewer months, down to one.

---

## Archive

Third page. A task that has sat in Done for 14 days moves here on its own, at launch and once an hour while running. The board stays short without you pruning it.

It is read-only. Nothing you can press there will change a task.

Archived tasks still count in throughput, cycle time and the streak. Hiding a task never erases it from your history — only the four column tiles and the board itself stop counting it.

---

## Calendar

Fourth page. Every open deadline on the board, on real months, in the colour it already wears on its card.

```
DEADLINES
         July 2026                     August 2026                   September 2026
 Mo  Tu  We  Th  Fr  Sa  Su     Mo  Tu  We  Th  Fr  Sa  Su     Mo  Tu  We  Th  Fr  Sa  Su
          1   2   3   4   5                          1   2          1   2   3   4   5   6
  6   7   8   9  10  11  12      3 ● 4   5   6   7   8   9      7   8   9  10  11  12 ●13
 13  14  15  16  17  18  19     10  11 ●12  13  14  15  16     14  15  16  17  18  19  20
 20  21  22  23  24  25  26     17  18  19  20  21  22  23     21  22  23  24  25  26  27
●27  28  29 ●30 ●31             24  25  26 ●27  28  29  30     28  29  30
● due · green >3 days · amber ≤3 · red ≤1 or overdue · underline is today
UPCOMING
● 27/07/2026  Chase the invoice                     3 days late
● 30/07/2026  Renew the TLS cert                    today
● 04/08/2026  Ship the report                       in 5 days
```

`h` and `l` step a month either way, `t` comes back to this one. A day with more than one deadline takes the colour of its most pressing task.

Done tasks are left off: a deadline you already met is history, and this page is about what is coming. Each month needs about 31 columns, so a wide terminal shows up to six side by side; 90 columns fit three, and below 59 just one. The upcoming list fills whatever rows are left and says how many it could not fit.

---

## Agents tab

Fifth page. Who is working on the board, read-only, refreshed whenever the board reloads (about every 2 seconds). `j`/`k` scroll it when it is taller than the terminal (`g`/`G` for the ends).

- **Loops** — stage loop runs: each live run (`⟳ indev  3fa1c9e2  Fix parser   claude · opus  12m`), queued runs (`… testing-review`), then the last 5 finished runs with their result (`✓ done`, `✗ failed`, `■ stopped`, `– skipped`), end time (`26/11 13:11`) and duration. With loops off or not set up it says `Loops are off · gotodo setup or gotodo loops on`.
- **Agents** — every agent named in move history, a claim, or as a card's last agent. Each row shows the cards it has claimed or in progress (up to 3, then `+N more`), its last activity (`last 2h ago`) and its moves over the last 7 days.
- **Activity** — the 20 most recent moves across all tasks, newest first: time (`30/07 11:00`), agent (or `you`), short ID, title and `from → to`.

---

## Install

Needs Go 1.22 or newer, and a terminal at least 80 columns wide.

**Quick install** (Debian, Ubuntu, Arch, Fedora, openSUSE, Alpine — sets up `~/.bashrc` and `~/.zshrc` automatically):

```bash
curl -fsSL https://raw.githubusercontent.com/19Naveen/deadline/main/install.sh | bash
```

The script detects your distro (`apt`, `pacman`, `dnf`/`yum`, `zypper`, `apk`), installs `git` and Go ≥ 1.22 if missing (falling back to an official Go toolchain in `~/.local/go` when the distro package is too old or root is unavailable), builds `gotodo` into `~/.local/bin`, and adds it to `PATH` in both `~/.bashrc` and `~/.zshrc` without duplicating lines. It also installs the `deadline` agent skill for Claude Code, Codex, OpenCode and other Agent-Skills-compatible tools, so coding agents can log work to project boards. Restart your terminal afterwards, then run `gotodo`.

**Setup wizard.** When the installer has a terminal it finishes with `gotodo setup`, a short full-screen wizard (nothing changes until you confirm on its summary screen):

1. **Welcome**: which agent CLIs were found (`claude`, `codex`, `opencode`).
2. **MCP**: tick the agents to register the gotodo MCP server with.
3. **Stage loops?** (default No): when a card enters a column, gotodo starts an agent with a skill on it. Change them any time with `gotodo loops` or `gotodo setup`.
4. **Skill packs**: per agent, whether [pstack](https://github.com/michael-denyer/pstack-claude) and [mattpocock/skills](https://github.com/mattpocock/skills) are present; tick missing ones to install. A pack already present from any source is never offered again. An *other skills* column counts the skills each agent already has from anywhere else, including the current project. **+ add a skill pack…** installs your own pack: a GitHub `owner/repo`, a git or https URL, or a local folder with at least one `SKILL.md` (up to two levels down), for the agents you tick, with `npx skills add <source> -g -a <agents> -s '*' -y`. Add as many as you like; `d` removes one. Codex and OpenCode packs are one shared install in `~/.agents/skills` (needs `npx`; update later with `npx skills update -g`).
5. **Stages**: per development column (todo, indev, testing-review, blocked, shipped): on/off, agent, model, skill and access (`readonly`, `edits`, `full`, shown as a coloured tag). The skill field suggests every skill the agent can load, user and project (marked `(project)`), or takes any name; a skill that is not installed gets a quiet note that loops will skip it until it is. Switching a stage to `full` shows a one-line warning until the next key. Defaults: indev and testing-review on with Claude, `edits` access. Personal boards (doing/done) are set with `gotodo loops set`.
6. **Summary**: confirm, then each step shows a spinner, its result and how long it took, under a progress bar; a failed pack install does not stop the rest. An existing `~/.config/gotodo/loops.json` is only replaced if you choose to. The last screen lists what changed and the commands to try next.

The wizard follows `NO_COLOR` (tags become `[brackets]`) and fits an 80x18 terminal.

Without a terminal (CI, `--yes`) the installer runs `gotodo setup -yes -no-loops`, which only registers the MCP server. Installer flags pass through: `--yes` skips the wizard, `--loops` / `--no-loops` preset the loops question (with `--yes`, `--loops` writes default loops when no config exists). Run `gotodo setup` again any time; `gotodo setup -dry-run` previews the MCP changes.

```bash
curl -fsSL https://raw.githubusercontent.com/19Naveen/deadline/main/install.sh | bash -s -- --yes --no-loops
```

**Manual install:**

```bash
git clone https://github.com/19Naveen/deadline.git
cd deadline
go build -trimpath -ldflags "-s -w" -o ~/.local/bin/gotodo .
```

**Upgrade:** re-run the installer — it rebuilds only when `main` moved past your installed copy (checked via `gotodo version`), otherwise it just re-syncs the skill and PATH lines.

**Uninstall:**

```bash
curl -fsSL https://raw.githubusercontent.com/19Naveen/deadline/main/install.sh | bash -s -- --uninstall
```

This removes the binary, the agent skills and the PATH lines it added. Boards are data, not installation, so `~/.config/gotodo` and `.deadline/` in projects are kept — delete them too for a full wipe.

Then run it:

```bash
gotodo
```

If you get `gotodo: command not found`, `~/.local/bin` is not on your `PATH`:

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc   # or ~/.bashrc
```

---

## Keys

| Key | What it does |
|---|---|
| `enter` | expand the selected task in a centred popup — `e` edit, `d` delete, `o` open a document, `v` verify & ship, `esc` close |
| `a` | add a task |
| `s` | add a subtask under the selected task's family |
| `f` | focus the board on the selected parent and its subtasks; press again to clear |
| `e` | edit the selected task |
| `d` | delete it, after a `y`/`n` confirm popup that names the task |
| `m` | grab it, then `h`/`l` to drag between columns, `enter` to drop, `esc` to cancel |
| `/` | search; cards filter as you type, `enter` keeps the filter, `esc` clears it |
| `v` | verify & ship: move the selected review card to the terminal column as you |
| `u` | undo the last move, edit, delete, add or approval (up to 20 steps) |
| `A` | turn the approval gate on or off |
| `L` | the selected card's newest loop run log (see [Watching loops](#watching-loops)) |
| `h` `l` | previous / next column |
| `j` `k` | previous / next task |
| `g` `G` | first / last task in the column |
| `ctrl+t` | switch between moving the cursor and moving between whole columns |
| `tab` | cycle Board, Analytics, Archive, Calendar, Agents |
| `h` `l` `t` | on the Calendar page: previous month, next month, back to today |
| `?` | key list |
| `q` | quit |

`enter` on a card opens it centred on the screen. The top stays put: the
full title, the status line (column, ID, ages, moves), then one facts line with
whichever of priority (`● high` in its colour), deadline (`due 30/11/2026 · 3d`
in its countdown colour), labels and `awaiting approval` (or `ready to verify`)
are set. Below it, in this order and only when set: the description, the
subtask checklist, blocked by, the parent, the worktree, attachments, working
notes, and an Agent section with the loop run (`Loop  indev · claude · running
12m`, or `indev · done 26/11 13:11`), the claim (`until 30/07/2026 15:30`, or
`expired`), the PR and the last agent. Long paths wrap at `/`. The body
reserves at least ten rows when space permits; long bodies scroll with
`j`/`k`. It is read-only, but `s`, `e` and `d` work from there.

**Search.** Terms are separated by spaces, a card must match all of them, and
case is ignored:

| Term | Matches |
|---|---|
| `login` | title, description or a label containing it, or an ID starting with it |
| `#api` | a label starting with `api` |
| `@claude` | the agent holding the claim or that made the last move, by name prefix |

Search stacks with the family filter (`f`); column counts then read
`shown/total`, and the WIP header `(1 · 4/3)`.

**Opening documents.** `o` in the popup opens an attachment or working note
straight away, or a chooser when there is more than one. Relative paths start
at the project root (the folder holding `.deadline/`), or the current directory
for a board outside a project. Text files (`.md`, `.txt`, source, config, no
extension) open in `$VISUAL`, then `$EDITOR`, then `vi`, and the board waits
for the editor to exit. Everything else goes to `open` (macOS), `wslview` (WSL)
or `xdg-open`. A missing file says `! not found: <path>` in the footer.

**Undo.** Up to 20 snapshots, in memory, for this session only. A move or edit
returns the task to its earlier state, history entries included; a delete
brings the task back with its subtasks re-attached; an add deletes the new
task. The footer confirms with `undone: <what>`.

**Verify & ship.** A card in the review column (testing-review on project
boards, doing on personal ones) offers `v Verify` in the board footer and
`v Verify & ship` in its popup. `v` moves it to the terminal column as you, a
human, even if `GOTODO_AGENT` is set in that terminal, and `u` undoes it.

**Approval.** With the gate on (`A`), agents cannot move cards into the
terminal column, so finished work waits in review with `awaiting approval` in
its popup (`ready to verify` when the gate is off). `v` ships it; `y` still
works as an alias while the gate is on. Dropping or approving a card
re-checks dependencies at once, the same as `gotodo move` (see below).

Adding and editing open the same form, centred in the same place. `tab` and
`shift+tab` move between Title, Description, Deadline, Worktree, Parent,
Attachments and Working notes. Sections sit one blank row apart (on a short
terminal every gap goes at once, never just one), only the focused field shows
a hint, and fields you are not in show the start of their value, so a long
title or parent stays recognisable. The footer lists the keys for the field
you are on.
Worktree is the git worktree path or branch the work lives in, and a new
subtask starts with its parent's. Inside a git repository the field opens a
dropdown of the repository's worktrees (with their branches) and local
branches, fuzzy-matched as you type (prefix first, then word starts, then any
substring, then scattered letters). `up`/`down` (or `ctrl+p`/`ctrl+n`)
highlight and `enter` picks: a worktree fills in its path, a branch its name.
A name that matches nothing exactly adds a last row,
`+ create worktree ../<repo>-<slug> on new branch <name>`; only highlighting
that row and pressing `enter` runs `git worktree add -b <name> <path>`, and
any git error shows in the form. Outside a repository the field is plain text.
Attachments are paths to user-provided reference files; Working notes are
paths to agent-created documents. Typing fuzzy-searches the files inside the
project root (`git ls-files`, so ignored files stay out; a bounded folder walk
outside git), `enter` adds the highlighted file as a chip (`docs/plan.md ×`),
and `backspace` on an empty search removes the last chip. A path that matches
no file, or one outside the project (`/…`, `~/…`, `../…`), is added as typed.
Paths are references, not copies.
On Parent, type a title or ID to filter the visible dropdown, use `up`/`down`
to highlight a match, and press `enter` to choose it. Clear the text for no
parent. The dropdown shows titles and short IDs together, so nothing must be
memorized. After choosing, `enter` saves; `esc` throws the form away.

The Description is multi-line: `ctrl+j` starts a new line, and `enter` still
saves. The editor opens four rows tall and grows with what you type, up to
15 rows, giving rows back only when the terminal cannot fit them.

Parent cards show a progress bar (`▰▱ 1/2 subtasks`). Subtasks keep their own
column and deadline, show `↳ parent` on the card, and appear in the parent's
detail checklist. Selecting any card marks the rest of its family with a dim
bar, across columns. A parent cannot enter the terminal column until every
subtask is complete. Deleting a parent safely detaches rather than deletes its
subtasks.

Dates go in as `DD/MM/YYYY`. A date it cannot read keeps the form open and tells you the format, rather than quietly dropping what you typed.

Or do not type them at all. Tab to the Deadline field and a calendar is
already there. `hjkl` moves a day or a week and fills the field as you go,
`t` jumps back to today. Today is green; the selected day is bracketed.

The calendar and the text box share the keyboard rather than fighting over
it — `hjkl` steer the calendar because those letters are never part of a
date, while digits and `backspace` go to the field and the calendar follows
along. `tab` and `enter` behave exactly as they do on the other two fields.

---

## Storage

Two boards, never mixed. Outside any project, `gotodo` opens your personal board, one JSON file at `~/.config/gotodo/tasks.json`. Inside an initialised project (any directory at or under one containing `.deadline/board.json`), it opens that project's board instead — one project never sees another project's tasks. Point anywhere else with `-file`:

```bash
gotodo -file ./work.json
```

Every change writes through a temporary file and a rename, so an interrupted write cannot leave you with half a board. A session where you changed nothing does not write at all. Concurrent writers merge by task under a brief file lock, so an agent logging while your board is open never drops your cards — and vice versa; two sessions editing the same card resolve last-writer-wins.

Everything lives in the board file, with one exception: `gotodo notify` keeps
its "already told you" record in `<user cache dir>/gotodo/notify-<hash of board path>.json`.
Delete that file to reset it.

---

## Project boards

A project board is the dev pipeline: `TODO → IN DEV → TESTING/REVIEW → BLOCKED → SHIPPED`, with shipped playing the role done plays on the personal board (grey deadlines, throughput, streak, and the 14-day archive sweep). The personal board keeps the original four columns. Five columns need a wider terminal — about 100 columns instead of 80.

Create one per project:

```bash
cd my-project
gotodo init
```

This writes `.deadline/board.json` and adds `.deadline/` to the project's `.gitignore`, creating the file when the project has none yet. Running `init` twice, or inside a subdirectory, reuses the existing board instead of nesting a new one. (A nested board is still possible by hand-creating `.deadline/board.json`; the nearest file always wins.)

Separate files are separate boards, which is the easiest way to keep work and personal apart — and project boards are how agents log development in parallel with your own list (see below).

---

## Priority, labels and dependencies

```bash
gotodo add "Fix login crash" -priority high
gotodo add "Parser tests" -label backend -label tests
gotodo add "Wire API" -blocked-by 3fa1c9e2 -blocked-by 7b20d4aa
gotodo edit 9c1e0f33 -priority none -label "" -blocked-by ""   # clear all three
gotodo list -status indev -label ui -priority medium
```

| Field | Notes |
|---|---|
| Priority | `none` (default), `low`, `medium`, `high`; also `l`/`m`/`h`, `med`, `p0`–`p3`, any case. `list` sorts each column highest first. |
| Labels | Repeat `-label`. Trimmed, lowercased, a leading `#` dropped, duplicates removed — `#Bug` is `bug`. |
| Blocked by | Repeat `-blocked-by` with ID prefixes. A card cannot block itself, blockers must exist, and a cycle is refused without writing: `gotodo: dependency cycle: 3fa1c9e2 → 9c1e0f33 → 3fa1c9e2`. |

On `edit`, each of these flags replaces the whole list and `""` clears it.

A blocker stays open until it reaches the last column (`done` or `shipped`).
On boards with a `blocked` column, every change that can affect that — `add`,
`edit`, `move`, a drop or approval in the TUI, `pr sync`, the MCP tools — re-checks
dependencies before saving and prints a line for each card it moves:

```
auto-blocked 9c1e0f33 Wire API (waiting on 3fa1c9e2)
unblocked 9c1e0f33 Wire API → indev
```

A todo or active card with an open blocker moves to `blocked`; once its last
blocker ships (in the same command) or its blockers are cleared, it goes back
to the column it came from. Only cards the rule itself blocked are moved back —
a card you blocked by hand stays put. Later columns and archived cards are left
alone. Automatic moves are recorded as `gotodo:deps`, and the card's "last
agent" keeps naming the last real actor. The same re-check drops claims on
cards that just finished.

### WIP limits

```bash
gotodo wip               # each column: count and limit
gotodo wip indev 3       # cap indev at 3 cards
gotodo wip indev 0       # remove the cap
```

```
todo: 4 (no limit)
indev: 4/3 over limit
testing-review: 1/2
```

A limit is a warning, not a wall: moving into a full column still works, `move`
says so, and `gotodo list` opens with `! indev over WIP limit (4/3)` for each column over
its cap.

`list` and `show` print the new fields when set:

```
3fa1c9e2 [indev] Wire API · P:high · #backend #api · blocked by 7b20d4aa · claimed by claude · PR #42 · last claude
```

`list` names only open blockers; `show` lists every blocker with its status.

---

## Agents

Coding agents can't drive the interactive board, so `gotodo` has headless commands with plain-text output. They run against the project board when invoked in the project directory:

```bash
gotodo help                                    # every command; gotodo help add for one
gotodo init                                    # once per project (skip if .deadline/ exists)
gotodo board                                   # board type + accepted columns
gotodo add "Rewrite the parser" -desc "Split the lexer" -deadline 04/08/2026 -priority high
gotodo add "Write parser tests" -parent 3fa1c9e2 -label tests -blocked-by 7b20d4aa
gotodo list
gotodo list -status blocked
gotodo list -parent 3fa1c9e2
gotodo show 3fa1c9e2                         # details, parent and subtasks
gotodo move 3fa1c9e2 testing-review          # any unique id prefix works
gotodo edit 3fa1c9e2 -parent 91bc2f0a         # attach; -parent "" detaches
gotodo edit 3fa1c9e2 -worktree ../app-parser   # where the work lives; "" clears
gotodo edit 3fa1c9e2 -attachment "brief.docx" -working-note docs/task.md
gotodo delete 3fa1c9e2                       # no confirm; list first, there is no undo

gotodo next -claim -start                    # pick, claim and start the next task
gotodo claim 3fa1c9e2 -ttl 30m               # hold a specific card
gotodo release 3fa1c9e2                      # let it go
gotodo context 3fa1c9e2                      # everything needed to resume, as markdown
gotodo wip indev 3                           # cap a column
gotodo approval on                           # agents can no longer ship (humans only)
gotodo pr 3fa1c9e2 42                        # link a pull request
gotodo pr sync                               # ship merged PRs, block failing CI (needs gh)
gotodo standup -format markdown              # what happened in the last 24 hours
gotodo mcp                                   # serve the board over MCP (stdio)
gotodo notify -watch                         # desktop alerts for blocked, review, due
```

Repeat `-attachment` or `-working-note` to specify multiple paths. On `edit`,
each supplied flag replaces that entire list; supply all paths to keep, or an
empty value to clear it. `gotodo show` displays the recorded paths.

`gotodo help` lists every command by group; `gotodo help <command>` shows its flags and examples (`gotodo -h` and `--help` work too). The global `-file` flag must precede the subcommand (`gotodo -file ./work.json list`). Help never changes anything.

The installer drops a `gotodo-task-board` skill (`skills/gotodo-task-board/SKILL.md` in this repo) into the Claude, Codex, OpenCode and shared agent skills directories. Its task-management and kanban keywords make it easy for agents to discover for any coding workflow. An open board picks up agent writes within a couple of seconds — no restart needed.

### Who is moving cards

```bash
export GOTODO_AGENT=claude      # or codex, gemini, claude-2, ...
```

With `GOTODO_AGENT` set, every move is recorded under that name — in the task
history, the card's "last agent", `context` and the Agents tab. Without it the
CLI acts as a human, as the TUI always does.

### Picking up work: `next`

```bash
gotodo next                       # peek: which task should be worked next?
gotodo next -claim                # ...and claim it for $GOTODO_AGENT
gotodo next -claim -start         # ...and move it to the active column
gotodo next -start -ttl 4h        # longer claim (default 2h)
```

`next` prints the task like `show` does, or `no available tasks`. It picks from
the first column and skips archived cards, cards another agent holds a live
claim on, parents with open subtasks (work those instead), and cards with an
open blocker (a subtask inherits its parent's). What is left is ordered by
priority, then earliest deadline (none last), then oldest.

`-start` moves the card to the active column (`doing` or `indev`); an agent
running it always claims too, a human just moves it. `-claim` without
`GOTODO_AGENT` is an error. Pick, claim and start happen as one transaction
under the board lock, so two agents running `next -claim` at the same moment
get different cards.

### Claims

```bash
gotodo claim 3fa1c9e2             # claim for $GOTODO_AGENT for 2h
gotodo claim 3fa1c9e2 -ttl 30m    # re-claiming your own card extends it
gotodo release 3fa1c9e2           # drop the claim when you stop
```

- A claim lasts until its TTL runs out; after that anyone may take the card.
- Claiming a card another agent holds fails: `task is claimed by claude until 27/11/2026 09:30`.
- Claims gate other agents' `claim` and `next`, never `move`: anyone can always move a card.
- An agent can release its own claim or an expired one; a human can release anyone's.
- A card reaching `done`/`shipped` loses its claim in the same command. Release explicitly when you stop early.

### Resuming work: `context`

`gotodo context <id>` prints one markdown document: title, status, priority,
labels, deadline, worktree, PR, claim (or `(expired)`), last agent, parent,
subtasks and blockers with their statuses, attachment paths, description and
the last 10 transitions with who made them and when (DD/MM/YYYY HH:MM).

Working notes under 64 KB are inlined in a fenced block. Relative paths resolve
against the project root first, then the current directory, so it works from
any subdirectory. Missing, oversized, binary or directory notes are listed with
the reason; attachment contents are never printed. `context` is read-only.

### Approval gate

```bash
gotodo approval status            # on or off
gotodo approval on                # agents can no longer move cards to done/shipped
gotodo approval off
```

With approval on, an agent's move into the terminal column is refused; a human
ships it from the TUI (`v`) or a plain CLI. `approval on` and `off` are refused
when `GOTODO_AGENT` is set, so an agent cannot lift the gate that reviews it.
`approval status` works for everyone.

### MCP server

`gotodo mcp` serves the board over the [Model Context Protocol](https://modelcontextprotocol.io),
so agents call typed tools instead of building shell commands. It speaks stdio
(newline-delimited JSON-RPC 2.0) and uses only the standard library.

```bash
gotodo mcp                                     # nearest project board, else the personal board
gotodo -file path/to/.deadline/board.json mcp  # pin one board (-file goes before mcp)
```

- **Board resolution** follows the CLI: nearest `.deadline/board.json` from the
  directory the client starts it in, else the personal board. Pin with `-file`
  if your client starts servers elsewhere.
- **Always fresh.** Each call reloads the board from disk and saves with the
  same locking merge as the CLI, so the TUI, CLI and other agents can work
  alongside it. Changes go through the same dependency and claim re-check.
- **Attribution.** Moves are recorded under `GOTODO_AGENT`, else the client's
  name in lowercase (`claude-code`, `opencode`, `codex-mcp-client`), else
  `mcp`. An MCP session is never the human, so the approval gate applies.
- Stdout carries only protocol messages; diagnostics go to stderr. Protocol
  versions `2025-06-18`, `2025-03-26` and `2024-11-05` are accepted.

| Tool | Arguments | Does |
|---|---|---|
| `board` | none | Board type, columns, WIP limits, approval gate, and the name moves are recorded under |
| `list_tasks` | `status?`, `parent?` | One line per active task, like `gotodo list` |
| `show_task` | `id` | The full card, like `gotodo show` |
| `add_task` | `title`, `description?`, `deadline?` (DD/MM/YYYY), `status?`, `parent?`, `worktree?`, `attachments?[]`, `working_notes?[]`, `priority?`, `labels?[]`, `blocked_by?[]`, `pr?` | Creates a card and returns its short ID |
| `move_task` | `id`, `status` | Moves a card and records the transition |
| `edit_task` | `id` plus any field `add_task` takes except `status` | Changes only what you pass; `""` clears a text field, arrays replace the whole list and `[]` clears it |
| `delete_task` | `id` | Deletes a card for good and detaches its subtasks |
| `next_task` | `claim?`, `start?`, `ttl_minutes?` | Like `gotodo next` |
| `claim_task` | `id`, `ttl_minutes?` | Like `gotodo claim` |
| `release_task` | `id` | Like `gotodo release` |
| `task_context` | `id` | Like `gotodo context` |

IDs accept any unique prefix. Failures (unknown status, ambiguous ID, bad date,
the approval gate) come back as tool results with `isError: true` and a
readable message, so the model can fix the call.

**You don't need to set anything up.** `install.sh` runs `gotodo setup`,
which registers the server with every agent it finds: Claude Code (user
scope, via `claude mcp add`), Codex (a marked block in
`~/.codex/config.toml`) and OpenCode (merged into
`~/.config/opencode/opencode.json`), each with its own `GOTODO_AGENT`. Each
agent then starts `gotodo mcp` itself in every session. Run `gotodo setup`
again after installing a new agent; it is safe to repeat, never touches your
other settings, and leaves a config it cannot parse alone. `gotodo setup
-remove` (also run by `--uninstall`) takes it out again.

The manual equivalents, if you prefer to wire it yourself:

**Claude Code:**

```bash
claude mcp add --scope project gotodo -e GOTODO_AGENT=claude -- gotodo mcp
```

Or in `.mcp.json` (also read by Claude Desktop, Cursor and other `mcpServers` clients):

```json
{
  "mcpServers": {
    "gotodo": { "command": "gotodo", "args": ["mcp"], "env": { "GOTODO_AGENT": "claude" } }
  }
}
```

**Codex**, in `~/.codex/config.toml`:

```toml
[mcp_servers.gotodo]
command = "gotodo"
args = ["mcp"]
env = { GOTODO_AGENT = "codex" }
```

**OpenCode**, in `opencode.json` at the project root or `~/.config/opencode/opencode.json`:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "gotodo": {
      "type": "local",
      "command": ["gotodo", "mcp"],
      "enabled": true,
      "environment": { "GOTODO_AGENT": "opencode" }
    }
  }
}
```

If `gotodo` is not on the client's `PATH`, give the full path to
`~/.local/bin/gotodo`. To pin a board, put `-file` before `mcp`:
`["-file", "/abs/project/.deadline/board.json", "mcp"]`.

---

## Stage loops: start an agent when a card enters a column

A loop starts a coding agent CLI (Claude Code, Codex or OpenCode) with a skill, a model and a prompt when a card **enters** a column. For example, `indev` can start an implementation skill and `testing-review` a review skill. When the indev agent moves the card to `testing-review`, the review loop starts next.

```sh
gotodo loops set indev -agent claude -model claude-sonnet-5-5 -skill tdd
gotodo loops set testing-review -agent codex -skill code-review -permissions readonly
gotodo loops on
gotodo loops                 # status: config, stages, skill installed ✓/✗, running and queued runs
gotodo loops logs 3fa1c9e2   # latest run log for a card (-stage S for one stage, -f to follow)
gotodo loops stop 3fa1c9e2   # kill a card's live runs and release their claims
gotodo loops dispatch        # start runs for new moves and drain the queue now
```

**Config.** `~/.config/gotodo/loops.json` applies to every board. A project file at `.deadline/loops.json` replaces it completely for that project (no merging). `on`, `off`, `set` and `unset` write the project file if it exists, else the global one. Add `-project` to create or write the project file. A new project file starts as a copy of the config in effect.

```json
{
  "enabled": true,
  "max_parallel": 2,
  "max_runs_per_task": 3,
  "timeout_minutes": 120,
  "stages": {
    "indev": { "agent": "claude", "model": "claude-sonnet-5-5", "skill": "tdd", "permissions": "edits" },
    "testing-review": { "agent": "codex", "skill": "code-review", "permissions": "readonly", "enabled": false }
  }
}
```

`prompt` is optional. The default prompt asks for one complete pass, because every move can start another paid run: follow the skill's whole workflow, tick every acceptance criterion, run the checks in that run and record them in a **Verification** section, and only then `gotodo move` the card to the next column. An agent that cannot finish leaves the card where it is with a **Progress** section, or moves it to `blocked` with the exact question. Placeholders: `{skill}`, `{id}`, `{title}`, `{stage}`, `{next}` (the column after this one, skipping blocked) and `{board}`. A custom `prompt` replaces all of this, so keep those rules in it.

**What a run does.**

1. Claims the card as `loop:<stage>`. If someone else holds the claim, the run is skipped.
2. Starts the agent in the card's worktree if it exists (a relative path resolves from the project root). Otherwise the agent starts in the project root.
3. Sets `GOTODO_AGENT=loop:<stage>`, `GOTODO_LOOP=1` and `GOTODO_BOARD=<board path>` for the agent.
4. Writes a header (DD/MM/YYYY HH:MM, the command with its prompt, the cwd) and all agent output to `.deadline/loops/<id>-<stage>-<n>.log`.
5. Stops the agent's process group after `timeout_minutes`.
6. Releases the claim, then starts any queued runs.

Loop decisions that have no run of their own go to `.deadline/loops/loops.log`: skips, queueing, drops and stops.

**Permissions.**

| level | claude | codex | opencode |
|---|---|---|---|
| `readonly` | `-p --permission-mode plan --allowedTools "Bash(gotodo *)" mcp__gotodo` | `exec --sandbox read-only` | `run` (permissions come from your opencode config) |
| `edits` (default) | `-p --permission-mode acceptEdits --allowedTools "Bash(gotodo *)" mcp__gotodo` | `exec --sandbox workspace-write --add-dir <board dir>` | `run` (permissions come from your opencode config) |
| `full` | `-p --dangerously-skip-permissions` | `exec --dangerously-bypass-approvals-and-sandbox` | `run --auto` |

`-model` maps to `--model` (claude) or `-m` (codex, and opencode's `provider/model`). Codex always gets `--skip-git-repo-check`. These flags were checked against claude 2.1.332, codex-cli 0.171.0 and opencode 2.4.2. This codex version has no `--full-auto` flag, so `edits` uses `--sandbox workspace-write`.

With `edits`, Claude Code can edit files but cannot run shell commands other than `gotodo`, because nobody is there to approve them. Codex's `read-only` sandbox cannot write the board. Use `full` only in a sandboxed or throwaway checkout.

**Safety rails.**

- Loops are off until you run `gotodo loops on`. Turning them on never replays old moves: only moves made after that point start runs.
- Moves made by the dependency sync (`gotodo:deps`) never start runs, and neither do archived or deleted cards.
- `max_runs_per_task` (default 3) caps runs per card per stage, so a card that bounces between columns cannot loop forever.
- `max_parallel` (default 2) caps live runs. Extra runs wait in a queue, and a card's second stage waits for its first run to finish. A queued run is dropped if its card moves on before a slot frees.
- If a stage's skill is not installed for its agent, the run is not started. The card's description gets one line: `loop skipped: skill <s> not found for <agent> (run gotodo setup)`.
- A card claimed by another agent is skipped.
- Agents (`GOTODO_AGENT` set) cannot run `on`, `off`, `set` or `unset`. A loop (`GOTODO_LOOP=1`) also cannot run `stop`.
- Prompts are passed as one argv entry, never through a shell.
- A runner that dies is marked `lost` and its slot is freed. A run that overruns its timeout by 15 minutes is killed.

Loops start after every change from the CLI, the TUI, the MCP server and `pr sync`. An agent started by a loop finds the board through `GOTODO_BOARD`, even from a worktree with no `.deadline` of its own. A loop started by a sandboxed agent (for example Codex running `gotodo move`) inherits that sandbox.

The setup wizard (`gotodo setup`, opened by `install.sh`) asks whether you want loops, offers to install the pstack and mattpocock skill packs for agents that lack them (and any pack of your own by GitHub `owner/repo`, URL or folder), and lets you pick the agent, model, skill and permissions for each stage.

Loops find skills in the agent's user folders and plugins and in the project: `.claude/skills` for Claude Code; `.agents/skills` and `.codex/skills` for Codex; `.opencode/skills`, `.claude/skills` and `.agents/skills` for OpenCode, under the project root and the card's worktree. A skill committed to the repository is enough.

### Watching loops

- **Agents tab** (`tab` to the fifth page): the Loops section at the top lists live, queued and recently finished runs. It refreshes about every 2 seconds.
- **On the board**: a card with a live run shows `⟳ loop:indev` (or `⟳ indev` in a narrow column) where a claim would show `◆ agent`. Its detail popup has a `Loop` line.
- **`L` on a card** (on the board or in its detail popup, where the hint `L Loop log` appears once a log exists) opens the newest run log for that card. The viewer opens at the bottom and keeps following new output while you stay there. `j`/`k`, `g`/`G` and `pgup`/`pgdn` scroll. Terminal colour codes are stripped and long lines wrap; only the last 256 KB of a large log is read. `x` stops a live run after a `y`/`n` confirm (the same as `gotodo loops stop`). `esc` goes back.
- **From a shell**: `gotodo loops` for status, and `gotodo loops logs <id> -f` to print the newest log for a card and follow it until the run ends (`Ctrl+C` stops following, not the run).

---

## Reports and integrations

All local. GitHub is reached only through your installed `gh` CLI, and
notifications go only to your desktop.

### Standup

```bash
gotodo standup                      # last 24 hours, plain text
gotodo standup -since 7d            # any Go duration (90m, 36h) or whole days
gotodo standup -format markdown     # bold headings and `ids`, for chat
```

| Section | Lists |
|---|---|
| Done | cards that reached done/shipped in the window, and who moved them |
| Moved | every other column change, `from → to`, by agent or "you" |
| Blocked now | cards in `blocked` and how long they have sat there |
| Due today/overdue | open cards due today or late, most overdue first |
| In progress | cards in the working columns, with any live claim and PR |

Empty sections are left out; if all are empty it says `Nothing to report`.
Times are HH:MM for today and DD/MM/YYYY HH:MM before that. Read-only.

### Pull requests

```bash
gotodo pr 3fa1c9e2 https://github.com/[org]/[repo]/pull/42   # link by URL
gotodo pr 3fa1c9e2 42                                         # or by number ("#42" works too)
gotodo pr 3fa1c9e2                                            # show the link
gotodo pr 3fa1c9e2 ""                                         # clear it
gotodo pr sync -dry-run                                       # what would change
gotodo pr sync                                                # apply it
```

A PR is an `http(s)` URL or a number; numbers resolve against the repository
holding `.deadline/`. `pr sync` runs `gh pr view` for every open card with a
PR, then:

- **Merged** moves the card to the terminal column — refused, with "awaits
  human approval", when the gate is on and an agent is running the sync.
- **A failing check** (`FAILURE`, `ERROR`, `CANCELLED`, `TIMED_OUT`,
  `STARTUP_FAILURE`) on an unmerged PR moves the card to `blocked` and appends
  `CI failing on PR <url> (DD/MM/YYYY)` to its description, once per PR.
- Green or pending checks, and closed unmerged PRs, are left alone.

Shipping a card here unblocks its dependants in the same run. One line per
action, `would …` in dry-run. If `gh` is missing or not logged in it says so
(`gh auth login`) and exits non-zero, as it does when any PR could not be
checked. gotodo never reads or stores GitHub tokens.

### Desktop notifications

```bash
gotodo notify                        # one pass
gotodo notify -watch                 # check every minute until Ctrl+C
gotodo notify -watch -interval 5m    # minimum 10s
```

Fires once per event when a card becomes blocked, enters review
(`testing-review`, or `doing` on a personal board), is due today, or is
overdue; a card blocked again later fires again. Uses `notify-send` on Linux
and `osascript` on macOS, and prints to stdout on WSL or when neither exists.
The first run looks back only 24 hours for blocked and review events. It never
writes to the board (see Storage for where its record lives).

---

## Development

```bash
go test ./...
go vet ./...
gofmt -l .
```

`internal/task` is the model and the JSON store, `internal/stats` is pure analytics, and `internal/ui` contains the five Bubble Tea tabs: Board, Analytics, Archive, Calendar and Agents. Around them sit `internal/mcp` (the MCP server), `internal/gh` (the `gh` wrapper behind `pr sync`), `internal/report` (standup) and `internal/notify`. Nothing in `internal/task` or `internal/stats` calls `time.Now` — the clock is always passed in, which is what makes the tests deterministic. Tests never call the real `gh` or send real notifications.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss). Three dependencies, no more — the MCP server is standard library.

---

## FAQ

**Can I get a task back out of the archive?**
Not from inside the app. Edit the JSON and drop the `"archived": true` line.

**Why 14 days?**
Long enough that a finished task is still there when someone asks about it. Short enough that Done does not become a scrapbook.

**Does it sync?**
No. It is one file. Put it in a synced folder if you want it on two machines. Two sessions open at once merge by task instead of overwriting each other, with the later save winning any card both touched.

**Why does it say my terminal is too narrow?**
Four columns and a date need 80 columns (about 100 for a five-column project board). Below that it tells you, instead of drawing a board that overlaps itself. Press `tab` for Analytics, Archive and Calendar, which stay readable on narrower terminals.

**Do old task files still work?**
Yes. Files written before descriptions and deadlines existed load fine, with those fields empty.
