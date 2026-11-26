package loops

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// skillRoots lists where each agent loads skills from, relative to home:
// plain skill directories plus plugin caches, whose skills sit under
// <plugin>/<version>/skills/<name>/SKILL.md.
func skillRoots(agent, home, configHome string) (dirs, pluginGlobs []string) {
	shared := filepath.Join(home, ".agents", "skills")
	switch agent {
	case "claude":
		return []string{filepath.Join(home, ".claude", "skills")},
			[]string{filepath.Join(home, ".claude", "plugins", "cache", "*", "*", "*", "skills")}
	case "codex":
		codexHome := os.Getenv("CODEX_HOME")
		if codexHome == "" {
			codexHome = filepath.Join(home, ".codex")
		}
		return []string{filepath.Join(codexHome, "skills"), shared},
			[]string{filepath.Join(codexHome, "plugins", "cache", "*", "*", "*", "skills")}
	case "opencode":
		// OpenCode also reads Claude Code and shared skill directories.
		return []string{filepath.Join(configHome, "opencode", "skills"), filepath.Join(home, ".claude", "skills"), shared},
			[]string{filepath.Join(home, ".cache", "opencode", "packages", "*", "*", "skills"),
				filepath.Join(home, ".cache", "opencode", "packages", "*", "*", "plugins", "*", "skills")}
	}
	return nil, nil
}

// InstalledSkills lists the skill names an agent can load, sorted and
// deduplicated. A skill is a directory holding SKILL.md, found directly or
// one level down (skill packs that group by category).
func InstalledSkills(agent string) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	return installedSkillsIn(agent, home, configHome)
}

func installedSkillsIn(agent, home, configHome string) []string {
	dirs, globs := skillRoots(agent, home, configHome)
	for _, g := range globs {
		matches, _ := filepath.Glob(g)
		dirs = append(dirs, matches...)
	}
	seen := map[string]bool{}
	for _, dir := range dirs {
		for _, name := range skillsUnder(dir, 2) {
			seen[name] = true
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// skillsUnder finds directories containing SKILL.md up to depth levels down.
func skillsUnder(dir string, depth int) []string {
	entries, err := os.ReadDir(dir)
	if err != nil || depth == 0 {
		return nil
	}
	var out []string
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if st, err := os.Stat(path); err != nil || !st.IsDir() { // follows symlinked skills
			continue
		}
		if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err == nil {
			out = append(out, e.Name())
			continue
		}
		out = append(out, skillsUnder(path, depth-1)...)
	}
	return out
}

// HasSkill reports whether agent can load skill. Plugin skills may be
// addressed with a namespace ("pstack:poteto-mode"); the part after the
// last colon is matched.
func HasSkill(agent, skill string) bool {
	if i := strings.LastIndex(skill, ":"); i >= 0 {
		skill = skill[i+1:]
	}
	for _, s := range InstalledSkills(agent) {
		if s == skill {
			return true
		}
	}
	return false
}
