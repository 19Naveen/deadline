package agentsetup

// Agent CLI names accepted by InstallFor, in display order.
var AgentCLIs = []string{"claude", "codex", "opencode"}

// InstallFor registers the MCP server with only the named agents (claude,
// codex, opencode), in AgentCLIs order. Unknown names are ignored. Install
// is unchanged: it still covers every agent.
func InstallFor(e Env, agents []string) []Result {
	want := map[string]bool{}
	for _, a := range agents {
		want[a] = true
	}
	var out []Result
	for _, a := range AgentCLIs {
		if !want[a] {
			continue
		}
		switch a {
		case "claude":
			out = append(out, claude(e, false))
		case "codex":
			out = append(out, codex(e, false))
		case "opencode":
			out = append(out, opencode(e, false))
		}
	}
	return out
}
