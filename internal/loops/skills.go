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
			// Git plugins are cached as npm packages:
			// ~/.cache/opencode/npm/<plugin>/<stamp>/node_modules/<pkg>/[plugins/<name>/]skills
			[]string{filepath.Join(home, ".cache", "opencode", "npm", "*", "*", "node_modules", "*", "skills"),
				filepath.Join(home, ".cache", "opencode", "npm", "*", "*", "node_modules", "*", "plugins", "*", "skills")}
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
	return skillNames(dirs)
}

// skillNames lists the skills in dirs (up to two levels down), sorted and
// deduplicated.
func skillNames(dirs []string) []string {
	lists := make([][]string, 0, len(dirs))
	for _, dir := range dirs {
		lists = append(lists, skillsUnder(dir, 2))
	}
	return mergeNames(lists...)
}

// mergeNames merges name lists, sorted and deduplicated.
func mergeNames(lists ...[]string) []string {
	seen := map[string]bool{}
	for _, l := range lists {
		for _, name := range l {
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

// projectSkillDirs lists where agent loads skills from inside a project.
func projectSkillDirs(agent, root string) []string {
	j := func(parts ...string) string { return filepath.Join(append([]string{root}, parts...)...) }
	switch agent {
	case "claude":
		return []string{j(".claude", "skills")}
	case "codex":
		return []string{j(".agents", "skills"), j(".codex", "skills")}
	case "opencode":
		return []string{j(".opencode", "skills"), j(".claude", "skills"), j(".agents", "skills")}
	}
	return nil
}

// ProjectSkills lists the skills agent loads from the project at root,
// sorted and deduplicated; none when root is empty.
func ProjectSkills(agent, root string) []string {
	if root == "" {
		return nil
	}
	return skillNames(projectSkillDirs(agent, root))
}

// InstalledSkillsFor is InstalledSkills plus the skills of the project at
// projectRoot (empty for none), sorted and deduplicated.
func InstalledSkillsFor(agent, projectRoot string) []string {
	return mergeNames(InstalledSkills(agent), ProjectSkills(agent, projectRoot))
}

// SkillsIn lists the skills in a skill pack folder: the folder itself when
// it holds SKILL.md, plus skill folders up to two levels below it.
func SkillsIn(dir string) []string {
	var out []string
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err == nil {
		out = append(out, filepath.Base(dir))
	}
	return mergeNames(out, skillsUnder(dir, 2))
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

// HasSkill reports whether agent can load skill from the user's skill
// directories and plugins. Plugin skills may be addressed with a namespace
// ("pstack:poteto-mode"); the part after the last colon is matched.
func HasSkill(agent, skill string) bool {
	return hasSkillName(InstalledSkills(agent), skill)
}

// HasSkillIn is HasSkill that also looks in the project at projectRoot.
func HasSkillIn(agent, skill, projectRoot string) bool {
	return HasSkill(agent, skill) || hasSkillName(ProjectSkills(agent, projectRoot), skill)
}

func hasSkillName(names []string, skill string) bool {
	if i := strings.LastIndex(skill, ":"); i >= 0 {
		skill = skill[i+1:]
	}
	for _, s := range names {
		if s == skill {
			return true
		}
	}
	return false
}
