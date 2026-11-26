package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/task"
)

// document is one path a task points at, tagged with where it came from.
type document struct {
	kind string // "attachment" or "note"
	path string
}

// taskDocuments lists a task's attachments, then its working notes.
func taskDocuments(t task.Task) []document {
	var docs []document
	for _, p := range t.Attachments {
		docs = append(docs, document{"attachment", p})
	}
	for _, p := range t.WorkingNotes {
		docs = append(docs, document{"note", p})
	}
	return docs
}

// projectRoot is the directory relative document paths resolve against: the
// parent of the board's .deadline directory. Empty for a board outside a
// project (relative paths then resolve against the working directory).
func projectRoot(boardPath string) string {
	if boardPath == "" {
		return ""
	}
	dir := filepath.Dir(boardPath)
	if filepath.Base(dir) == ".deadline" {
		return filepath.Dir(dir)
	}
	return ""
}

// resolveDocPath makes a document path absolute against the project root,
// expanding a leading ~/.
func resolveDocPath(path, root string) string {
	path = strings.TrimSpace(path)
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	if filepath.IsAbs(path) || root == "" {
		return path
	}
	return filepath.Join(root, path)
}

// textExts are opened in $EDITOR; anything else goes to the desktop opener.
var textExts = map[string]bool{
	".md": true, ".markdown": true, ".txt": true, ".text": true, ".log": true, ".rst": true,
	".json": true, ".yaml": true, ".yml": true, ".toml": true, ".ini": true, ".cfg": true, ".conf": true,
	".csv": true, ".tsv": true, ".xml": true, ".sql": true, ".env": true,
	".go": true, ".mod": true, ".sum": true, ".py": true, ".js": true, ".mjs": true, ".ts": true,
	".tsx": true, ".jsx": true, ".css": true, ".scss": true, ".html": true, ".sh": true, ".bash": true,
	".zsh": true, ".fish": true, ".ps1": true, ".bicep": true, ".tf": true, ".tfvars": true,
	".rs": true, ".c": true, ".h": true, ".cpp": true, ".hpp": true, ".cs": true, ".java": true,
	".kt": true, ".rb": true, ".php": true, ".lua": true, ".swift": true, ".vue": true, ".svelte": true,
}

// isTextDoc reports whether a path is a text-like file for the editor;
// extensionless names such as Makefile or README count as text.
func isTextDoc(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == "" || textExts[ext]
}

// editorCommand is $VISUAL or $EDITOR split into argv, falling back to vi.
func editorCommand(getenv func(string) string) []string {
	for _, v := range []string{"VISUAL", "EDITOR"} {
		if f := strings.Fields(getenv(v)); len(f) > 0 {
			return f
		}
	}
	return []string{"vi"}
}

// launcher is the desktop opener for this platform: open on macOS, wslview
// under WSL when installed, xdg-open elsewhere.
func launcher(goos string, wsl bool, lookPath func(string) (string, error)) string {
	switch {
	case goos == "darwin":
		return "open"
	case goos == "windows":
		return "explorer"
	case wsl:
		if _, err := lookPath("wslview"); err == nil {
			return "wslview"
		}
	}
	return "xdg-open"
}

// onWSL detects the Windows Subsystem for Linux.
func onWSL() bool {
	if os.Getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	v, err := os.ReadFile("/proc/version")
	return err == nil && strings.Contains(strings.ToLower(string(v)), "microsoft")
}

// docOpenedMsg reports how opening a document went; Err is shown in the
// footer.
type docOpenedMsg struct{ Err error }

// openDocument is the default opener: text files suspend the TUI for the
// editor, others go to the desktop opener without waiting.
func openDocument(path string) tea.Cmd {
	if isTextDoc(path) {
		argv := append(editorCommand(os.Getenv), path)
		cmd := exec.Command(argv[0], argv[1:]...)
		return tea.ExecProcess(cmd, func(err error) tea.Msg { return docOpenedMsg{err} })
	}
	return func() tea.Msg {
		cmd := exec.Command(launcher(runtime.GOOS, onWSL(), exec.LookPath), path)
		if err := cmd.Start(); err != nil {
			return docOpenedMsg{err}
		}
		go cmd.Wait() //nolint:errcheck // reap the detached opener
		return docOpenedMsg{}
	}
}

// openDoc resolves and opens one document, or reports why it can't.
func (m *BoardModel) openDoc(d document) tea.Cmd {
	path := resolveDocPath(d.path, projectRoot(m.board.Path()))
	if _, err := os.Stat(path); err != nil {
		m.err = "not found: " + d.path
		return nil
	}
	return m.opener(path)
}

// startOpen is o in the detail popup: open the only document, or pick one.
func (m *BoardModel) startOpen(t task.Task) tea.Cmd {
	docs := taskDocuments(t)
	switch len(docs) {
	case 0:
		m.err = "no attachments or working notes to open"
		return nil
	case 1:
		return m.openDoc(docs[0])
	}
	m.mode = modeOpen
	m.choice = 0
	return nil
}

// updateOpen drives the document chooser: j/k pick, enter opens, esc goes
// back to the detail popup.
func (m BoardModel) updateOpen(k tea.KeyMsg) (BoardModel, tea.Cmd) {
	t, ok := m.selectedTask()
	if !ok {
		m.mode = modeNormal
		return m, nil
	}
	docs := taskDocuments(t)
	m.err = ""
	switch k.String() {
	case "esc", "q":
		m.mode = modeDetail
	case "j", "down":
		m.choice = min(m.choice+1, len(docs)-1)
	case "k", "up":
		m.choice = max(m.choice-1, 0)
	case "enter", "o":
		if m.choice < len(docs) {
			cmd := m.openDoc(docs[m.choice])
			if m.err == "" {
				m.mode = modeDetail
			}
			return m, cmd
		}
	}
	return m, nil
}

// renderOpen is the document chooser popup.
func (m BoardModel) renderOpen() string {
	t, ok := m.selectedTask()
	if !ok {
		return ""
	}
	docs := taskDocuments(t)
	w := m.popupWidth()
	cw := w - 2
	rows := []string{TitleStyle.Render("Open document"), MutedStyle.Render(strings.Repeat("─", cw))}
	// Budget: tab row, borders, title, rule, footer rule and hints, error.
	room := len(docs)
	if m.height > 0 {
		room = max(min(room, m.height-3-4-1), 1)
	}
	start := 0
	if m.choice >= room {
		start = m.choice - room + 1
	}
	for i := start; i < min(start+room, len(docs)); i++ {
		prefix, style := "  ", lipgloss.NewStyle().Foreground(ColText)
		if i == m.choice {
			prefix, style = "› ", lipgloss.NewStyle().Foreground(ColAccent).Bold(true)
		}
		kind := MutedStyle.Render(fmt.Sprintf("  %s", docs[i].kind))
		path := truncate(docs[i].path, max(cw-2-lipgloss.Width(kind), 1))
		gap := max(cw-2-lipgloss.Width(path)-lipgloss.Width(kind), 0)
		rows = append(rows, style.Render(prefix+path)+strings.Repeat(" ", gap)+kind)
	}
	var hints []string
	if m.err != "" {
		hints = append(hints, errorLine(m.err))
	}
	hints = append(hints, keyHint("j/k", "Select"), keyHint("enter", "Open"), keyHint("esc", "Back"))
	rows = append(rows, popupFoot(cw, hints...)...)
	return ColumnStyle.Copy().Width(w).Render(strings.Join(rows, "\n"))
}
