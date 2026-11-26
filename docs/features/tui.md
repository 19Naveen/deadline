# TUI: agent fields, search, documents, Agents tab, approval, undo

README-ready notes for the TUI side of the agent features. Dates use
DD/MM/YYYY, times use 24-hour HH:MM.

## What the board shows

- **Priority marker** before a card's title: `!!!` high (red), `!!` medium
  (amber), `!` low (muted). Cards with no priority show nothing.
- **Claim marker** next to the countdown: `◆ claude` while an agent holds the
  card. When the column is narrow it shortens to `◆ cl…` and then to a bare
  `◆`, so the title always keeps some room. Expired claims are hidden. Cards
  never get taller.
- **WIP limits** in column headers: `IN DEV (4/3)` means 4 cards against a
  limit of 3. Once a column is over its limit the count turns red. While a
  filter is on, the header reads `(1 · 4/3)`: 1 card shown, then cards/limit.
  Long headers are shortened rather than wrapped.
- **Detail popup** (`enter`) shows these one-row properties, each only when
  it is set:
  - Priority
  - Labels (`#api #ui`)
  - Approval (`awaiting approval`)
  - Blocked by: short ID, title and status, up to 3 rows then `+N more`
  - Claimed by, with `until 30/07/2026 15:30` (or `expired …`)
  - PR
  - Last agent

## Keys

Add these rows to the README keys table:

| Key | Where | Action |
| --- | --- | --- |
| `/` | board | Search. The footer becomes a filter input and cards filter as you type. `enter` keeps the filter, `esc` clears it |
| `esc` | board | Clear the active search filter |
| `u` | board | Undo the last TUI move, edit, delete, add or approval (up to 20 steps). The footer confirms with `undone: <what>` |
| `A` | board | Turn the approval gate on or off. The footer confirms the new state |
| `o` | detail popup | Open an attachment or working note. A task with one document opens it straight away; with more, a chooser opens (`j/k`, `enter`, `esc`) |
| `y` | detail popup | Approve: ship a card that is awaiting approval |
| `tab` | anywhere | Pages now cycle Board → Analytics → Archive → Calendar → **Agents** |

## Search syntax

Search terms are separated by spaces and a card must match all of them.
Matching ignores case.

| Term | Matches |
| --- | --- |
| `login` | Title, description or a label containing the word, or a task ID starting with it |
| `#api` | A label starting with `api` |
| `@claude` | The agent that holds the claim (ClaimedBy) or made the last move (LastAgent), by name prefix |

Search works together with the family filter (`f`): with both on, you see
only family members that match. The column counts read `shown/total`. If
the selected card is filtered out, the cursor moves to a card that is still
visible.

## Opening documents

- **Where relative paths start:** the project root, which is the folder that
  holds `.deadline/`. On a board outside a project, relative paths start
  from the current directory. Absolute paths and `~/` work as usual.
- **Text files:** `.md`, `.txt`, source code, config files and files with no
  extension open in `$VISUAL`, then `$EDITOR`, then `vi`. The TUI pauses
  until the editor exits.
- **Everything else** opens in the background with:
  - `open` on macOS
  - `wslview` on WSL, when it is installed
  - `xdg-open` everywhere else
- **Errors:** a missing file shows `! not found: <path>` in the popup footer.
  If the editor or opener fails, the footer shows `open failed: …`.

## Agents tab

The Agents tab is read-only and stays up to date when the board reloads
(about every 2 seconds).

- **Agents:** every agent named in move history, in a claim, or as a card's
  last agent. Each row shows:
  - how many cards it has claimed or in progress (up to 3 are listed, then
    `+N more`)
  - its last activity, for example `last 2h ago`
  - its moves in the last 7 days
- **Activity:** the 20 most recent moves across all tasks, newest first. Each
  line shows the time (`30/07 11:00`), the agent (or `you` for a human), the
  short ID, the title and `from → to`.

## Approval gate

When the gate is on (`A` in the TUI, or the CLI setting), agents cannot move
cards into the terminal column. Cards in the review column wait there:
testing-review on dev boards, doing on personal boards. Their detail popup
shows `awaiting approval` and the hint `y Approve`. Pressing `y` ships the
card as you, a human, even if `GOTODO_AGENT` is set in the terminal.

## Undo

Undo keeps up to 20 snapshots in memory, and only for this session. Each
change undoes like this:

- **Move or edit:** the task goes back to its earlier snapshot, which also
  removes the history entries the move added.
- **Delete:** the task comes back with its subtasks re-attached, and the
  next save keeps it.
- **Add:** the new task is deleted.

The delete confirmation now says `u` can bring the task back.
