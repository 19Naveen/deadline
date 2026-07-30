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
| `tab` | switch board ↔ analytics |
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
