package main

import (
	_ "embed"

	"gotodo/internal/agentsetup"
)

// The agent skill and instruction block ship inside the binary, so
// `gotodo setup` installs the copy that matches the gotodo version.
var (
	//go:embed skills/gotodo-task-board/SKILL.md
	skillMD []byte
	//go:embed skills/gotodo-task-board/AGENTS.snippet.md
	agentsSnippet []byte
)

func agentAssets() agentsetup.Assets {
	return agentsetup.Assets{Skill: skillMD, Snippet: agentsSnippet}
}
