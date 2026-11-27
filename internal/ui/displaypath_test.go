package ui

import "testing"

func TestDisplayPath(t *testing.T) {
	root := "/home/u/Projects/billing-ode"
	cases := map[string]string{
		"docs/design.md": "docs/design.md",
		"/home/u/Projects/billing-ode/docs/design.md":        "docs/design.md",
		"/home/u/Projects/billing-ode":                       ".",
		"/home/u/Projects/billing-ode-wt":                    "../billing-ode-wt",
		"/home/u/Projects/billing-ode-wt/notes/a.md":         "../billing-ode-wt/notes/a.md",
		"/tmp/claude-1000/-home-x/e5f9/scratchpad/design.md": "…/scratchpad/design.md",
		"/etc/hosts":       "/etc/hosts",
		"feat/branch-name": "feat/branch-name",
	}
	for in, want := range cases {
		if got := displayPath(in, root); got != want {
			t.Errorf("displayPath(%q) = %q, want %q", in, got, want)
		}
	}
	if got := displayPath("/tmp/a/b/c.md", ""); got != "…/b/c.md" {
		t.Errorf("no project root: %q", got)
	}
}
