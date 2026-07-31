<div align="center">

# deadline

*Green. Amber. Red. Then a cross.*

![Go](https://img.shields.io/badge/go-1.22-00ADD8?logo=go&logoColor=white)
![Stars](https://img.shields.io/github/stars/majipa007/deadline?style=flat)
![Last commit](https://img.shields.io/github/last-commit/majipa007/deadline)

A terminal kanban board that never lets you forget when something is due.

</div>

---

## The board

Four columns. Three-line cards. Dates that change colour as they close in.

```
  BOARD    ANALYTICS    ARCHIVE    tab to switch
╭──────────────────────╮╭──────────────────────╮╭──────────────────────╮╭──────────────────────╮
│ TODO (3)             ││ DOING (1)            ││ BLOCKED (1)          ││ DONE (1)             │
│                      ││                      ││                      ││                      │
│ │ Ship the Q3 report ││  Rewrite the parser  ││  Waiting on design   ││  Drop the old API    │
│ │ Draft, review, se… ││  Split the lexer o…  ││  Mocks not landed    ││                      │
│ │ ● 09/08/2026       ││  ● 03/08/2026        ││  ● 06/08/2026        ││                      │
│                      ││                      ││                      ││                      │
│  Renew the TLS cert  ││                      ││                      ││                      │
│  Staging box         ││                      ││                      ││                      │
│  ● 01/08/2026        ││                      ││                      ││                      │
│                      ││                      ││                      ││                      │
│  Chase the invoice   ││                      ││                      ││                      │
│  Third follow-up     ││                      ││                      ││                      │
│  ● 27/07/2026 ✗      ││                      ││                      ││                      │
╰──────────────────────╯╰──────────────────────╯╰──────────────────────╯╰──────────────────────╯
focus: item (ctrl+t) · hjkl move · a add · e edit · d delete · m grab · tab analytics · ? help
```

No mouse. No config file. No account.

---

## Deadlines

The only thing on the card that changes colour.

| Time left | Colour | Looks like |
|---|---|---|
| more than 3 days | green | `● 09/08/2026` |
| 3 days or less | amber | `● 03/08/2026` |
| due today or tomorrow | red | `● 01/08/2026` |
| the day has passed | red, with a cross | `● 27/07/2026 ✗` |
| task is done | grey | `● 11/07/2026` |

A finished task never turns red. Being late is history by then, not an alarm.

Deadlines are optional. Leave the field blank and the card is two lines instead of three.

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
Mon ············
    ············
Wed ············
    ············
Fri ···········█
    ············
Sun ············
last 12 weeks, ending 31/07/2026
```

Cycle time runs from created to done. Time-per-column shows where work actually sits, which is rarely where you think.

---

## Archive

Third page. A task that has sat in Done for 14 days moves here on its own, at launch and once an hour while running. The board stays short without you pruning it.

It is read-only. Nothing you can press there will change a task.

Archived tasks still count in throughput, cycle time, streak and the heatmap. Hiding a task never erases it from your history — only the four column tiles and the board itself stop counting it.

---

## Install

Needs Go 1.22 or newer, and a terminal at least 80 columns wide.

```bash
git clone git@github.com:majipa007/deadline.git
cd deadline
go build -trimpath -ldflags "-s -w" -o ~/.local/bin/gotodo .
```

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
| `a` | add a task |
| `e` | edit the selected task |
| `d` | delete it, after a `y`/`n` confirm |
| `m` | grab it, then `h`/`l` to drag between columns, `enter` to drop, `esc` to cancel |
| `h` `l` | previous / next column |
| `j` `k` | previous / next task |
| `g` `G` | first / last task in the column |
| `ctrl+t` | switch between moving the cursor and moving between whole columns |
| `tab` | cycle Board, Analytics, Archive |
| `?` | key list |
| `q` | quit |

Adding and editing open the same three-field form. `tab` and `shift+tab` move between Title, Description and Deadline. `enter` saves from any field, `esc` throws it away.

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

One JSON file, `~/.config/gotodo/tasks.json`. Point somewhere else with `-file`:

```bash
gotodo -file ./work.json
```

Separate files are separate boards, which is the easiest way to keep work and personal apart.

Every change writes through a temporary file and a rename, so an interrupted write cannot leave you with half a board. A session where you changed nothing does not write at all, which matters if you ever have two copies open.

---

## Development

```bash
go test ./...
go vet ./...
gofmt -l .
```

Three packages. `internal/task` is the model and the JSON store, `internal/stats` is pure analytics, `internal/ui` is the three Bubble Tea pages. Nothing in `internal/task` or `internal/stats` calls `time.Now` — the clock is always passed in, which is what makes the tests deterministic.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss). Three dependencies, no more.

---

## FAQ

**Can I get a task back out of the archive?**
Not from inside the app. Edit the JSON and drop the `"archived": true` line.

**Why 14 days?**
Long enough that a finished task is still there when someone asks about it. Short enough that Done does not become a scrapbook.

**Does it sync?**
No. It is one file. Put it in a synced folder if you want it on two machines, but two copies open at once will overwrite each other.

**Why does it say my terminal is too narrow?**
Four columns and a date need 80 columns. Below that it tells you, instead of drawing a board that overlaps itself. Analytics and Archive are single-column and stay readable at any width.

**Do old task files still work?**
Yes. Files written before descriptions and deadlines existed load fine, with those fields empty.
