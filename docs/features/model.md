# Priority, labels, dependencies and WIP limits

README-ready documentation for the task-model slice: priority, labels,
blocked-by dependencies with auto-blocking, and per-column WIP limits.

## Priority

Every card can carry a priority: `none` (default), `low`, `medium` or `high`.

```sh
gotodo add "Fix login crash" -priority high
gotodo edit 3fa1c9e2 -priority medium
gotodo edit 3fa1c9e2 -priority none        # clear
```

Accepted spellings (case-insensitive): `none`, `low`/`l`/`p3`,
`medium`/`med`/`m`/`p2`, `high`/`h`/`p1`/`p0`.

`gotodo list` shows highest priority first within each column; cards with the
same priority keep their board order.

## Labels

```sh
gotodo add "Parser tests" -label backend -label tests
gotodo edit 3fa1c9e2 -label ui             # replaces the whole list
gotodo edit 3fa1c9e2 -label ""             # clear
```

Labels are trimmed, lowercased, a leading `#` is dropped, and blanks and
duplicates are removed (`" Bug "`, `#bug` and `BUG` are all `bug`).

## Filtering the list

```sh
gotodo list -label backend
gotodo list -priority high
gotodo list -status indev -label ui -priority medium
```

Filters combine with `-status` and `-parent`.

## Dependencies (blocked-by)

A card can wait on other cards. Pass ID prefixes; they are stored as full IDs.

```sh
gotodo add "Wire API" -blocked-by 3fa1c9e2 -blocked-by 7b20d4aa
gotodo edit 9c1e0f33 -blocked-by 3fa1c9e2  # replaces the list
gotodo edit 9c1e0f33 -blocked-by ""        # clear
```

A card cannot block itself, every blocker must exist, and a change that would
create a cycle (directly or through other cards) is rejected without writing
anything:

```
gotodo: dependency cycle: 3fa1c9e2 → 9c1e0f33 → 3fa1c9e2
```

A blocker is **open** until it reaches the board's last column (`done` or
`shipped`).

### Auto-block and auto-unblock

On boards with a `blocked` column, `add` and `edit` apply two rules before
saving and print a receipt for each card they move:

- A card in the first column (`todo`) or the active column (`doing`/`indev`)
  with an open blocker moves to `blocked`:
  `auto-blocked 9c1e0f33 Wire API (waiting on 3fa1c9e2)`.
- A card that **the rule itself** moved into `blocked` returns to the column it
  came from once every blocker is done, or its blockers are cleared:
  `unblocked 9c1e0f33 Wire API → indev`.

Cards that a person or agent moved into `blocked` by hand are never moved out
automatically. Auto moves are recorded in the card history with the agent
`gotodo:deps`; the card's "last agent" keeps naming the last real actor.

Cards in later columns (`testing-review`) and archived cards are left alone.

> `gotodo move` does not yet run these rules; a card whose last blocker ships
> is unblocked by the next `add`/`edit` (or by the TUI once wired in).

## WIP limits

```sh
gotodo wip                 # each column: count and limit
gotodo wip indev 3         # cap indev at 3 cards
gotodo wip indev 0         # remove the cap
```

```
todo: 4 (no limit)
indev: 4/3 over limit
testing-review: 1/2
blocked: 0 (no limit)
shipped: 12 (no limit)
```

Moving into a full column is allowed. `gotodo list` starts with a warning for
every listed column over its cap:

```
! indev over WIP limit (4/3)
```

## List and show output

`list` lines gain these suffixes when set:

```
3fa1c9e2 [indev] Wire API · P:high · #backend #api · blocked by 7b20d4aa · claimed by claude · PR #42 · last claude
```

`blocked by` lists only open blockers. `show` adds:

```
priority: high
labels: backend, api
blocked by:
  7b20d4aa [todo] Schema migration
  1c2d3e4f [shipped] Auth tokens
claimed by: claude until 27/11/2026 09:30
pr: #42
last agent: claude
```

`show` lists every blocker with its status, including finished ones.

## Skill additions

Add to the agent skill (headless command reference and workflow):

- **Priority:** `gotodo add "title" -priority high|medium|low` and
  `gotodo edit <id> -priority none` to clear. Pick up `P:high` cards first;
  `list` already sorts each column by priority.
- **Labels:** `-label NAME` is repeatable on `add`/`edit`; on `edit` it replaces
  the list and `-label ""` clears it. Filter with `gotodo list -label NAME`.
- **Dependencies:** when a task needs another one shipped first, record it with
  `-blocked-by <id-prefix>` (repeatable; `edit` replaces, `""` clears). Do not
  move a dependent card into `blocked` yourself; gotodo auto-blocks it and
  moves it back when the blockers ship (watch for `auto-blocked` /
  `unblocked` lines in command output). Cycles are rejected.
- Check blockers with `gotodo show <id>` (`blocked by:` section) before
  starting a card; `list` shows ` · blocked by <ids>` for open blockers.
- **WIP limits:** run `gotodo wip` to see column caps. If `gotodo list` prints
  `! <column> over WIP limit (n/limit)`, finish or hand off work in that column
  before starting more. Only change limits (`gotodo wip <column> N`) when the
  human asks.

## Public API (internal/task)

```go
func (p Priority) String() string
func ParsePriority(s string) (Priority, error)
func NormalizeLabel(label string) string
func NormalizeLabels(labels []string) []string
func (t Task) HasLabel(label string) bool
func (b *Board) SetPriority(id string, p Priority, now time.Time) error
func (b *Board) SetLabels(id string, labels []string, now time.Time) error
func SortByPriority(tasks []Task)
func (b *Board) WIPLimit(s Status) int
func (b *Board) OverWIP(s Status) (count, limit int, over bool)

const DepsAgent = "gotodo:deps"
func (b *Board) SetBlockedBy(id string, blockers []string, now time.Time) error
func (b *Board) Blockers(id string) []Task
func (b *Board) OpenBlockers(id string) []Task
func (b *Board) SyncBlocked(now time.Time) []string
```

`Board.Save` does not call `SyncBlocked`. The TUI and `gotodo move` should call
`b.SyncBlocked(now)` after a move (or any mutation) and before saving.
