package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// helpEntry documents one subcommand for `gotodo help`.
type helpEntry struct {
	name, usage, summary string
	details              []string // flag and behaviour notes, one per line
	examples             []string
}

// helpGroups orders the overview; every command appears exactly once.
var helpGroups = []struct {
	title    string
	commands []helpEntry
}{
	{"Boards", []helpEntry{
		{name: "init", usage: "gotodo init", summary: "create a project board in ./.deadline/ (reuses one above you)",
			details: []string{"Adds .deadline/ to .gitignore. Run once from the project root."}},
		{name: "board", usage: "gotodo board", summary: "show the board type, columns, WIP limits and approval gate"},
		{name: "wip", usage: "gotodo wip [<status> <limit>]", summary: "show or set per-column WIP limits",
			details:  []string{"A limit of 0 clears it. list and move warn when a column is over its limit."},
			examples: []string{"gotodo wip", "gotodo wip indev 3"}},
		{name: "approval", usage: "gotodo approval on|off|status", summary: "require a human to ship cards",
			details: []string{"When on, agents (GOTODO_AGENT set) cannot move cards to the terminal column; a human approves with y in the TUI. Agents cannot change this setting."}},
	}},
	{"Tasks", []helpEntry{
		{name: "add", usage: `gotodo add "title" [flags]`, summary: "add a task (prints its ID)",
			details: []string{
				`-desc TEXT            description (Markdown)`,
				`-deadline DD/MM/YYYY  due date`,
				`-status NAME          starting column (default: the first)`,
				`-parent ID            make it a subtask of a top-level task`,
				`-priority LEVEL       none, low, medium or high`,
				`-label NAME           label (repeatable)`,
				`-blocked-by ID        task that must ship first (repeatable)`,
				`-worktree PATH        git worktree path or branch`,
				`-attachment PATH      user-provided reference file (repeatable)`,
				`-working-note PATH    agent working document (repeatable)`,
			},
			examples: []string{
				`gotodo add "Rewrite the parser" -desc "Split lexer from parser" -priority high -label backend`,
				`gotodo add "Parser tests" -parent 3fa1c9e2 -blocked-by 3fa1c9e2 -working-note docs/parser.md`,
			}},
		{name: "list", usage: "gotodo list [-status NAME] [-parent ID] [-label NAME] [-priority LEVEL]",
			summary: "list active tasks, highest priority first in each column"},
		{name: "show", usage: "gotodo show <id>", summary: "show one task with its parent, subtasks, blockers and documents"},
		{name: "edit", usage: "gotodo edit <id> [flags]", summary: "change a task's fields (use move for status)",
			details: []string{
				"Takes add's flags plus -title. Omitted flags keep their values; a blank value clears.",
				"Repeatable flags (-label, -blocked-by, -attachment, -working-note) replace the whole list.",
			},
			examples: []string{`gotodo edit 3fa1c9e2 -desc "Ready for review" -working-note docs/parser.md`}},
		{name: "move", usage: "gotodo move <id> <status>", summary: "move a task to another column",
			details:  []string{"Shipping a card unblocks its dependants and releases its claim."},
			examples: []string{"gotodo move 3fa1c9e2 testing-review"}},
		{name: "delete", usage: "gotodo delete <id>", summary: "delete a task logged by mistake (no undo)",
			details: []string{"Subtasks of a deleted parent are detached, not deleted."}},
	}},
	{"Agents", []helpEntry{
		{name: "next", usage: "gotodo next [-claim] [-start] [-ttl 2h]", summary: "pick the best unclaimed, unblocked todo task",
			details:  []string{"-claim claims it for $GOTODO_AGENT; -start also moves it to the active column."},
			examples: []string{"GOTODO_AGENT=claude gotodo next -claim -start"}},
		{name: "claim", usage: "gotodo claim <id> [-ttl 2h]", summary: "claim a task so other agents skip it (needs GOTODO_AGENT)"},
		{name: "release", usage: "gotodo release <id>", summary: "release a claim"},
		{name: "context", usage: "gotodo context <id>", summary: "print everything needed to resume a task, incl. working notes"},
		{name: "mcp", usage: "gotodo mcp", summary: "run the MCP server on stdio (agents start it themselves)",
			details: []string{"You never run this by hand: gotodo setup registers it and each agent starts it per session."}},
		{name: "setup", usage: "gotodo setup [-yes] [-loops|-no-loops] [-remove] [-dry-run]", summary: "setup wizard: MCP server, skill packs and stage loops",
			details: []string{
				"In a terminal it opens a wizard; -yes (or no terminal) only registers the MCP server everywhere.",
				"-loops/-no-loops preset the stage-loops question; with -yes, -loops writes defaults if no loops.json exists.",
				"Runs automatically from install.sh. Safe to repeat; skips agents that are not installed.",
				"Claude Code: claude mcp add -s user. Codex: a marked block in ~/.codex/config.toml.",
				"OpenCode: merges mcp.gotodo into ~/.config/opencode/opencode.json (left alone if it can't be parsed).",
			}},
		{name: "loops", usage: "gotodo loops [on|off|set|unset|logs|stop] ...", summary: "start a coding agent with a skill when a card enters a column",
			details: []string{
				"gotodo loops                  status: config in use, stages, skill installed ✓/✗, running and queued runs",
				"on|off [-project]             switch loops on or off (project .deadline/loops.json if it exists, else ~/.config/gotodo/loops.json)",
				"set <stage> -agent A [-model M] [-skill S] [-prompt P] [-permissions readonly|edits|full] [-enabled=false] [-project]",
				"unset <stage> [-project]      remove a stage's loop",
				"logs <id> [-stage S]          print the latest run log for a card",
				"stop <id>                     kill a card's live runs and release their claims",
				"dispatch                      start runs for new card moves and drain the queue now (also runs after every save)",
				"run <id> <stage>              internal: what a dispatched loop executes",
				"Runs claim the card as loop:<stage>, start in the card's worktree (else the project root), and stop after timeout_minutes (default 120).",
				"Limits: max_parallel (default 2) runs at once, max_runs_per_task (default 3) per card and stage. Agents cannot change loops.",
			},
			examples: []string{
				"gotodo loops set indev -agent claude -model claude-sonnet-5-5 -skill tdd",
				"gotodo loops set testing-review -agent codex -skill code-review -permissions readonly",
				"gotodo loops on",
			}},
	}},
	{"Reports", []helpEntry{
		{name: "standup", usage: "gotodo standup [-since 24h|7d] [-format text|markdown]", summary: "summarise recent activity"},
		{name: "pr", usage: "gotodo pr <id> <url|number> | gotodo pr sync [-dry-run]", summary: "link PRs; sync merged/failing PRs via gh",
			details: []string{"sync ships cards whose PR merged and blocks cards whose CI fails. Needs an authenticated gh CLI."}},
		{name: "notify", usage: "gotodo notify [-watch] [-interval 1m]", summary: "desktop alerts for blocked, review-ready and due cards"},
	}},
	{"Other", []helpEntry{
		{name: "help", usage: "gotodo help [command]", summary: "show this help, or one command's flags and examples"},
		{name: "version", usage: "gotodo version", summary: "print the build revision"},
	}},
}

func findHelp(name string) (helpEntry, bool) {
	for _, g := range helpGroups {
		for _, e := range g.commands {
			if e.name == name {
				return e, true
			}
		}
	}
	return helpEntry{}, false
}

// commandNames lists every subcommand, for the unknown-command error.
func commandNames() string {
	var names []string
	for _, g := range helpGroups {
		for _, e := range g.commands {
			names = append(names, e.name)
		}
	}
	return strings.Join(names, ", ")
}

// runHelp prints the overview, or one command's details.
func runHelp(args []string) error {
	if len(args) == 0 {
		printHelp(os.Stdout)
		return nil
	}
	if len(args) > 1 {
		return fmt.Errorf("usage: gotodo help [command]")
	}
	e, ok := findHelp(args[0])
	if !ok {
		return fmt.Errorf("no help for %q (commands: %s)", args[0], commandNames())
	}
	fmt.Printf("usage: %s\n\n%s.\n", e.usage, strings.ToUpper(e.summary[:1])+e.summary[1:])
	if len(e.details) > 0 {
		fmt.Println()
		for _, d := range e.details {
			fmt.Println("  " + d)
		}
	}
	if len(e.examples) > 0 {
		fmt.Println("\nexamples:")
		for _, ex := range e.examples {
			fmt.Println("  " + ex)
		}
	}
	return nil
}

func printHelp(w io.Writer) {
	fmt.Fprint(w, `gotodo — a terminal kanban board that coding agents can drive.

usage:
  gotodo                     open the interactive board (project board, else personal)
  gotodo <command> [args]    run a command headless
  gotodo -file PATH ...      use a specific board file (before the command)
`)
	width := 0
	for _, g := range helpGroups {
		for _, e := range g.commands {
			width = max(width, len(e.name))
		}
	}
	for _, g := range helpGroups {
		fmt.Fprintf(w, "\n%s:\n", strings.ToLower(g.title))
		for _, e := range g.commands {
			fmt.Fprintf(w, "  %-*s  %s\n", width, e.name, e.summary)
		}
	}
	fmt.Fprint(w, `
Agents: export GOTODO_AGENT=<name> so moves and claims are recorded as you.
IDs: any unique prefix works (the 8-character short ID is enough).
Dates: DD/MM/YYYY.

Run 'gotodo help <command>' for flags and examples.
`)
}
