## MCP server

`gotodo mcp` serves the board to coding agents over the [Model Context Protocol](https://modelcontextprotocol.io). Agents call typed tools such as `add_task` and `move_task` instead of building shell commands. The server uses the stdio transport: newline-delimited JSON-RPC 2.0 on stdin and stdout. It uses only the Go standard library, so it adds no dependencies.

```sh
gotodo mcp                                   # nearest project board, else the personal board
gotodo -file path/to/.deadline/board.json mcp  # pin one board (-file goes before mcp)
```

- **Board resolution** follows the CLI rules. The server looks for the nearest `.deadline/board.json`, walking up from the directory the client starts it in. If it finds none, it uses the personal board. When your client starts servers outside the project, pin the board with `-file`.
- **Always fresh.** Each tool call reloads the board from disk, so the TUI, the CLI and other agents can change it while the server runs. Changes are saved after each call that modifies the board, using the same locking merge as the CLI.
- **Attribution.** Moves are recorded under `GOTODO_AGENT` when it is set. Otherwise they use the client's `clientInfo.name` in lowercase, such as `claude-code`, `opencode` or `codex-mcp-client`. If neither is available, the name is `mcp`. An MCP session never counts as the human, so on boards that require approval, agents cannot ship through MCP either.
- **Output.** Stdout carries only protocol messages. Diagnostics go to stderr.
- **Protocol versions.** The server echoes `2025-06-18`, `2025-03-26` or `2024-11-05` when the client asks for one of them, and answers `2025-06-18` otherwise.

### Tools

| Tool | Arguments | Does |
|---|---|---|
| `board` | none | Board type, columns in order, WIP limits, approval gate, and the name moves are recorded under |
| `list_tasks` | `status?`, `parent?` | One line per active task, like `gotodo list` |
| `show_task` | `id` | Full card: parent, deadline, worktree, documents, agent fields, subtasks, history |
| `add_task` | `title`, `description?`, `deadline?` (DD/MM/YYYY), `status?`, `parent?`, `worktree?`, `attachments?[]`, `working_notes?[]` | Creates a card and returns its short ID |
| `move_task` | `id`, `status` | Moves a card and records the transition |
| `edit_task` | `id` plus any of `title`, `description`, `deadline`, `parent`, `worktree`, `attachments[]`, `working_notes[]` | Changes only the fields you pass. `""` clears a text field; arrays replace the whole list, and `[]` clears it |
| `delete_task` | `id` | Deletes a card for good and detaches its subtasks |

IDs accept any unique prefix. Tool failures come back as tool results with `isError: true` and a readable message, so the model can correct itself. Examples are an unknown status, an ambiguous ID, a bad date, an unknown argument or the approval gate.

### Claude Code

```sh
claude mcp add gotodo -- gotodo mcp
# share it with the repo (.mcp.json) and name the agent:
claude mcp add --scope project gotodo -e GOTODO_AGENT=claude -- gotodo mcp
```

Or add this JSON to `.mcp.json` in the project, or to any client that reads an `mcpServers` block (Claude Desktop, Cursor and others):

```json
{
  "mcpServers": {
    "gotodo": {
      "command": "gotodo",
      "args": ["mcp"],
      "env": { "GOTODO_AGENT": "claude" }
    }
  }
}
```

### Codex

In `~/.codex/config.toml`:

```toml
[mcp_servers.gotodo]
command = "gotodo"
args = ["mcp"]
env = { GOTODO_AGENT = "codex" }
```

### OpenCode

In `opencode.json`, either at the project root or in `~/.config/opencode/opencode.json`:

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

If `gotodo` is not on the client's `PATH`, use the absolute path, for example `~/.local/bin/gotodo` expanded to a full path. To pin a board, put `-file` before `mcp`, as in `["-file", "/abs/project/.deadline/board.json", "mcp"]`.

### Skill additions

Add the following to `skills/gotodo-task-board/SKILL.md`, after **Workflow**, and add a short version to `AGENTS.snippet.md`:

```markdown
## MCP tools

If the session has gotodo MCP tools (`board`, `list_tasks`, `show_task`, `add_task`,
`move_task`, `edit_task`, `delete_task`), prefer them over the CLI and follow the same workflow:

| CLI | MCP tool |
|---|---|
| `gotodo board` | `board` |
| `gotodo list [-status S] [-parent ID]` | `list_tasks {status?, parent?}` |
| `gotodo show ID` | `show_task {id}` |
| `gotodo add "T" -desc … -deadline DD/MM/YYYY -status S -parent ID -worktree P -attachment … -working-note …` | `add_task {title, description?, deadline?, status?, parent?, worktree?, attachments?, working_notes?}` |
| `gotodo move ID S` | `move_task {id, status}` |
| `gotodo edit ID …` | `edit_task {id, …}`: only the fields you pass change; `""` clears; arrays replace the whole list |
| `gotodo delete ID` | `delete_task {id}` |

- `gotodo init` has no tool. Run it from the CLI once per project, then restart the MCP server
  if it was already running on the personal board.
- A result with `isError: true` is a rejected call (unknown status, ambiguous id, approval gate).
  Read the message and fix the call. Do not retry it unchanged.
- Moves are recorded under GOTODO_AGENT or the client name. On boards that require approval,
  leave the card in review and ask the human to ship it.
```
