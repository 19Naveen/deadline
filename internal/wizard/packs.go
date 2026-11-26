package wizard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gotodo/internal/loops"
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
// agent order, then the custom packs in the order added. Shared agents
// collapse into one job; every job runs from home.
func units(chosen map[string]bool, agents []string, home string, customs []customPack) []installUnit {
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
	for _, c := range customs {
		var on []string
		for _, a := range agents {
			if c.Agents[a] {
				on = append(on, a)
			}
		}
		if len(on) > 0 {
			out = append(out, installUnit{Pack: c.Label, Agents: on, Dir: home, Needs: "npx",
				Cmds: [][]string{customArgv(c.Source, on)}})
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

// customPack is a skill pack the person added by source on the packs
// screen, installed with the skills CLI for the ticked agents.
type customPack struct {
	Source string          // passed to the skills CLI; absolute for folders
	Label  string          // as shown
	Local  bool            // a folder on this machine
	Skills []string        // skills a local folder ships, else unknown
	Agents map[string]bool // agents to install for
}

// skillsAgentID maps an agent to the skills CLI's --agent id.
var skillsAgentID = map[string]string{"claude": "claude-code", "codex": "codex", "opencode": "opencode"}

// customArgv installs source for agents with the skills CLI. Codex and
// OpenCode both read ~/.agents/skills, where the CLI keeps its canonical
// copy, so they share one id: opencode when chosen (as mattpocock does),
// else codex.
func customArgv(source string, agents []string) []string {
	var ids []string
	shared := ""
	for _, a := range agents {
		switch {
		case !sharedAgents[a]:
			ids = append(ids, skillsAgentID[a])
		case shared == "" || a == "opencode":
			shared = skillsAgentID[a]
		}
	}
	if shared != "" {
		ids = append(ids, shared)
	}
	argv := []string{"npx", "-y", "skills@latest", "add", source, "-g", "-a"}
	argv = append(argv, ids...)
	return append(argv, "-s", "*", "-y")
}

var ownerRepo = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9_.-]+(@[A-Za-z0-9_./-]+)?$`)

// parseSource checks a pack source typed on the packs screen: a GitHub
// owner/repo, a git or https URL, or a folder (relative to cwd, ~ allowed)
// holding at least one SKILL.md up to two levels down.
func parseSource(in, cwd, home string) (customPack, error) {
	s := strings.TrimSpace(in)
	switch {
	case s == "":
		return customPack{}, errors.New("type a GitHub owner/repo, a git URL or a folder")
	case strings.HasPrefix(s, "https://"), strings.HasPrefix(s, "http://"), strings.HasPrefix(s, "git@"),
		strings.HasPrefix(s, "ssh://"), strings.HasPrefix(s, "git://"):
		return customPack{Source: s, Label: s}, nil
	}
	pathLike := strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~") || strings.HasPrefix(s, ".") ||
		strings.HasPrefix(s, `\`) || filepath.VolumeName(s) != ""
	if !pathLike && ownerRepo.MatchString(s) {
		return customPack{Source: s, Label: s}, nil
	}
	dir := s
	if dir == "~" || strings.HasPrefix(dir, "~/") {
		dir = filepath.Join(home, strings.TrimPrefix(dir, "~"))
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(cwd, dir)
	}
	dir = filepath.Clean(dir)
	st, err := os.Stat(dir)
	switch {
	case err != nil && !pathLike:
		return customPack{}, fmt.Errorf("%s is not a GitHub owner/repo, a git URL or a folder", s)
	case err != nil:
		return customPack{}, fmt.Errorf("no folder at %s", tilde(dir, home))
	case !st.IsDir():
		return customPack{}, fmt.Errorf("%s is a file, not a folder", tilde(dir, home))
	}
	skills := loops.SkillsIn(dir)
	if len(skills) == 0 {
		return customPack{}, fmt.Errorf("no SKILL.md in %s or two levels below", tilde(dir, home))
	}
	return customPack{Source: dir, Label: tilde(dir, home), Local: true, Skills: skills}, nil
}

// packSkills are the skills pstack and mattpocock ship, so the packs screen
// can count everything else as the person's own. principle-* is pstack's.
var packSkills = func() map[string]bool {
	set := map[string]bool{}
	for _, s := range strings.Fields(`
		poteto-mode arena architect automate-me babysit benchmark-checklist blast-radius bro correct
		create-formal-verification-skill create-verification-skill deslop figure-it-out fix-ci
		fix-merge-conflicts get-pr-comments how in-your-own-words interrogate ketchup ketchup-video
		maintain-formal-verification-skill maintain-verification-skill make-pr-easy-to-review
		no-comments poteto-help recall reflect reframe setup-pstack sharpen show-me-your-work swarm
		statechart tdd teach technical-writing thermo-nuclear-code-quality-review thermo-nuclear-review
		thermos typescript-best-practices unslop what-did-i-get-done why
		grill-me grilling handoff to-questionnaire writing-for-agents wait-what git-guardrails-claude-code
		setup-pre-commit scaffold-exercises migrate-to-shoehorn writing-shape setup-ts-deep-modules
		claude-handoff writing-beats writing-fragments loop-me chief-of-staff diagnosing-bugs ask-matt pr
		triage wayfinder to-spec grill-with-docs code-review codebase-design implement-spec
		setup-matt-pocock-skills prototype to-tickets research implement wizard
		improve-codebase-architecture domain-modeling retro`) {
		set[s] = true
	}
	return set
}()

// otherSkills are the installed skills that come from neither pack.
func otherSkills(skills []string) []string {
	var out []string
	for _, s := range skills {
		if !packSkills[s] && !strings.HasPrefix(s, "principle-") {
			out = append(out, s)
		}
	}
	return out
}
