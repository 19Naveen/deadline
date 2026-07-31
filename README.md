# gotodo

A keyboard-driven terminal kanban board with a built-in analytics page.

## Install

```bash
go build -o gotodo .
```

## Run

```bash
./gotodo                        # uses ~/.config/gotodo/tasks.json
./gotodo -file ./mytasks.json   # or point it anywhere
```

## Keys

| Key | Action |
|---|---|
| `tab` | cycle Board → Analytics → Archive |
| `ctrl+t` | toggle column / item focus |
| `h` `l` | previous / next column |
| `j` `k` | previous / next task (item focus) |
| `g` `G` | first / last task in column |
| `a` | add a task |
| `e` | edit the selected task |
| `d` | delete the selected task (confirms with `y`) |
| `m` | grab a task; `h`/`l` to move it, `enter` to drop, `esc` to cancel |
| `?` | toggle help |
| `q` | quit |

## Task fields

Each task has a title, an optional one-line description, and an optional
deadline. Press `a` to open the form, `tab` and `shift+tab` to move between
the three fields, `enter` to save from anywhere, `esc` to cancel.

Deadlines are typed and displayed as `DD/MM/YYYY` — leave the field blank for
no deadline. The date on the card is colour-coded by how long you have left:

| Colour | Meaning |
|---|---|
| green | more than 3 days left |
| amber | 3 days or less |
| red | due today or tomorrow |
| red with ✗ | the deadline has passed |

A completed task shows its deadline in grey — finishing late is history, not
an ongoing emergency.

## Archive

Done tasks move to the Archive 14 days after you finish them, so the board
stays clean without losing anything. Sweeps run at startup and hourly while
gotodo is open. Press `tab` twice to browse the Archive; it is read-only.

Archived tasks still count toward every analytic — throughput, streak, cycle
time and the heatmap all keep their full history. Only the four column tiles
and the board itself hide them.

## Storage

One JSON file, written atomically after every change. Every status change is
appended to the task's history, which is what the analytics page reads.

## Analytics

- Task counts per column
- 14-day completion throughput sparkline
- Cycle time: mean and median created→done, plus mean time per column
- Blocked report: how long each blocked task has been stuck
- Completion streak and a 12-week heatmap

Dates are shown as `DD/MM/YYYY`.
