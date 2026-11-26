package wizard

import (
	"fmt"
	"sort"
	"strings"
)

// Skill packs the wizard can install. Each is detected per agent from the
// skills that agent can already load, whatever installed them, so a pack
// present from any source is never offered again (it would load twice).
const (
	PackPstack     = "pstack"
	PackMattpocock = "mattpocock"
)

// Packs lists the pack IDs in display order.
var Packs = []string{PackPstack, PackMattpocock}

// packSource is the upstream repo shown next to each pack.
var packSource = map[string]string{
	PackPstack:     "michael-denyer/pstack-claude (community port of pstack)",
	PackMattpocock: "mattpocock/skills",
}

// packProvides lists skills a pack is known to ship, so stage defaults can
// count a pack the user chose to install as if it were already there.
var packProvides = map[string][]string{
	PackPstack:     {"poteto-mode"},
	PackMattpocock: {"code-review", "tdd"},
}

// mattpocockOnly are skills only the mattpocock pack ships. tdd is not
// among them: pstack has its own tdd, so tdd marks mattpocock only when
// pstack is absent.
var mattpocockOnly = []string{"implement", "grilling", "codebase-design", "domain-modeling", "diagnosing-bugs"}

// HasPack reports whether an agent's installed skills include pack.
func HasPack(pack string, skills []string) bool {
	set := map[string]bool{}
	for _, s := range skills {
		set[s] = true
	}
	switch pack {
	case PackPstack:
		return set["poteto-mode"]
	case PackMattpocock:
		for _, s := range mattpocockOnly {
			if set[s] {
				return true
			}
		}
		return set["tdd"] && !set["poteto-mode"]
	}
	return false
}

// sharedAgents read ~/.agents/skills, so mattpocock for them is one shared
// `npx skills add -g` install rather than a per-agent plugin.
var sharedAgents = map[string]bool{"codex": true, "opencode": true}

// isShared reports whether pack for agent uses the shared install.
func isShared(pack, agent string) bool { return pack == PackMattpocock && sharedAgents[agent] }

// installUnit is one install job: the commands run in order in Dir, and the
// agents it covers. Needs names an extra binary the commands require.
type installUnit struct {
	Pack   string
	Agents []string
	Dir    string
	Needs  string
	Cmds   [][]string
}

func (u installUnit) label() string {
	return u.Pack + " → " + strings.Join(u.Agents, ", ")
}

// MattpocockShared is the one install that covers Codex and OpenCode.
var MattpocockShared = []string{"npx", "-y", "skills@latest", "add", "mattpocock/skills", "-g", "-a", "opencode", "-s", "*", "-y"}

// recipe returns the commands installing pack for one agent (shared
// mattpocock is handled by units).
func recipe(pack, agent string) [][]string {
	switch pack + "/" + agent {
	case "pstack/claude":
		return [][]string{
			{"claude", "plugin", "marketplace", "add", "michael-denyer/pstack-claude"},
			{"claude", "plugin", "install", "pstack@pstack-claude"},
		}
	case "pstack/codex":
		return [][]string{
			{"codex", "plugin", "marketplace", "add", "michael-denyer/pstack-claude"},
			{"codex", "plugin", "add", "pstack@pstack-claude"},
		}
	case "pstack/opencode":
		return [][]string{{"opencode", "plugin", "add", "github:michael-denyer/pstack-claude#main"}}
	case "mattpocock/claude":
		return [][]string{{"claude", "plugin", "install", "mattpocock-skills@claude-plugins-official"}}
	}
	return nil
}

// units turns the chosen pack/agent pairs into install jobs, in Packs then
// agent order. Shared mattpocock agents collapse into one job run from home.
func units(chosen map[string]bool, agents []string, home string) []installUnit {
	var out []installUnit
	for _, p := range Packs {
		var shared []string
		for _, a := range agents {
			if !chosen[cellKey(p, a)] {
				continue
			}
			if isShared(p, a) {
				shared = append(shared, a)
				continue
			}
			out = append(out, installUnit{Pack: p, Agents: []string{a}, Dir: home, Cmds: recipe(p, a)})
		}
		if len(shared) > 0 {
			out = append(out, installUnit{Pack: p, Agents: shared, Dir: home, Needs: "npx",
				Cmds: [][]string{MattpocockShared}})
		}
	}
	return out
}

func cellKey(pack, agent string) string { return pack + "/" + agent }

// runUnit runs a job's commands, stopping at the first failure. Output
// saying the marketplace or plugin is already there counts as success.
func runUnit(u installUnit, run Runner, lookPath func(string) (string, error)) (stepState, string) {
	if u.Needs != "" {
		if _, err := lookPath(u.Needs); err != nil {
			return stepSkipped, u.Needs + " not found; install Node.js, then run gotodo setup again"
		}
	}
	for _, c := range u.Cmds {
		out, err := run(u.Dir, c[0], c[1:]...)
		if err != nil && !strings.Contains(strings.ToLower(out), "already") {
			msg := lastLine(out)
			if msg == "" {
				msg = err.Error()
			}
			return stepFailed, fmt.Sprintf("%s: %s", strings.Join(c[:min(len(c), 3)], " "), msg)
		}
	}
	return stepOK, "installed"
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// union merges skill lists, sorted and deduplicated.
func union(lists ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range lists {
		for _, s := range l {
			if s != "" && !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	sort.Strings(out)
	return out
}
