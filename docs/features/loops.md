# Stage loops

## README section

### Stage loops: start an agent when a card enters a column

A loop starts a coding agent CLI (Claude Code, Codex or OpenCode) with a skill, a model and a prompt when a card **enters** a column. For example, `indev` can start an implementation skill and `testing-review` a review skill. When the indev agent moves the card to `testing-review`, the review loop starts next.

```sh
gotodo loops set indev -agent claude -model claude-sonnet-5-5 -skill tdd
gotodo loops set testing-review -agent codex -skill code-review -permissions readonly
gotodo loops on
gotodo loops                 # status: config, stages, skill installed ✓/✗, running and queued runs
gotodo loops logs 3fa1c9e2   # latest run log for a card (-stage S for one stage)
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

`prompt` is optional. The default prompt is: *Use the {skill} skill on gotodo task {id} ({title}), which just entered {stage}. Start by running `gotodo context {id}`…* You can use the placeholders `{skill}`, `{id}`, `{title}`, `{stage}` and `{board}`.

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

## Skill additions (for agents)

Add this to the gotodo task-board skill:

- **You may be running inside a loop.** If `GOTODO_LOOP=1`, gotodo started you because your card entered a column. `GOTODO_AGENT` is already `loop:<stage>` and the card is already claimed for you. Do not run `gotodo claim` or `gotodo next -claim`.
- **Find the board.** If gotodo started you in a worktree outside the project, the prompt says so. Run every command as `gotodo -file "$GOTODO_BOARD" ...`.
- **Moving the card is the hand-off.** When your stage's work is done and verified, move the card to the next column, for example `gotodo move <id> testing-review`. That move can start the next stage's loop, so move only when the card is ready. If you are stuck, move the card to `blocked` and say why with `gotodo edit <id> -desc ...`.
- **Do not reconfigure loops.** `gotodo loops on/off/set/unset/stop` are refused for agents and loops. Report problems on the card instead.
- **Logs.** `gotodo loops logs <id>` prints the latest run log for a card, and `gotodo loops` shows the live and queued runs.
