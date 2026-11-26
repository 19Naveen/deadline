package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/loops"
	"gotodo/internal/task"
)

// logTailBytes is how much of a run log the viewer reads: the newest part
// is what matters, and agent logs can grow to megabytes.
const logTailBytes = 256 << 10

// logViewer is the popup showing a loop run's log. Lines are cleaned of
// terminal escapes once, at read time; wrapped rows are cached per width.
type logViewer struct {
	run     loops.RunRecord
	lines   []string // cleaned log lines
	size    int64    // file size at the last read
	clipped bool     // only the last logTailBytes were read
	err     string

	wrapW int      // width rows was wrapped at
	rows  []string // lines wrapped to wrapW

	offset  int  // first visible row
	follow  bool // pinned to the bottom: new lines scroll into view
	confirm bool // asking whether to stop the run
}

// readLogTail reads up to limit bytes from the end of path. When it cuts,
// the partial first line is dropped.
func readLogTail(path string, limit int64) (lines []string, size int64, clipped bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, false, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, 0, false, err
	}
	size = st.Size()
	start := int64(0)
	if size > limit {
		start, clipped = size-limit, true
	}
	buf := make([]byte, size-start)
	n, err := f.ReadAt(buf, start)
	if err != nil && err != io.EOF {
		return nil, 0, false, err
	}
	buf = buf[:n]
	size = start + int64(n)
	if clipped {
		if i := strings.IndexByte(string(buf), '\n'); i >= 0 {
			buf = buf[i+1:]
		}
	}
	text := strings.TrimRight(cleanLog(string(buf)), "\n")
	if text == "" {
		return nil, size, clipped, nil
	}
	return strings.Split(text, "\n"), size, clipped, nil
}

// cleanLog makes agent output safe to draw: it strips ANSI escape
// sequences (CSI, OSC and the rest), applies carriage returns the way a
// terminal would (a lone \r restarts the line), expands tabs, drops other
// control characters and replaces invalid UTF-8.
func cleanLog(s string) string {
	out := make([]byte, 0, len(s))
	lineStart := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == 0x1b:
			i = skipEscape(s, i)
		case c == '\r':
			if i+1 < len(s) && s[i+1] == '\n' {
				continue
			}
			out = out[:lineStart]
		case c == '\n':
			out = append(out, '\n')
			lineStart = len(out)
		case c == '\t':
			out = append(out, "    "...)
		case c < 0x20 || c == 0x7f:
			// bell, backspace and friends draw nothing
		default:
			out = append(out, c)
		}
	}
	return strings.ToValidUTF8(string(out), "�")
}

// skipEscape returns the index of the last byte of the escape sequence
// that starts at s[i] (an ESC).
func skipEscape(s string, i int) int {
	if i+1 >= len(s) {
		return i
	}
	switch s[i+1] {
	case '[': // CSI: parameters and intermediates, then a final byte
		for j := i + 2; j < len(s); j++ {
			if s[j] >= 0x40 && s[j] <= 0x7e {
				return j
			}
		}
		return len(s) - 1
	case ']', 'P', 'X', '^', '_': // OSC and other strings: until BEL or ST
		for j := i + 2; j < len(s); j++ {
			if s[j] == 0x07 {
				return j
			}
			if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
				return j + 1
			}
		}
		return len(s) - 1
	}
	// Two-byte escapes, with optional intermediates (ESC ( B and the like).
	j := i + 1
	for j < len(s)-1 && s[j] >= 0x20 && s[j] <= 0x2f {
		j++
	}
	return j
}

// wrapLog hard-wraps each line to width display cells; an empty line
// stays one empty row.
func wrapLog(lines []string, width int) []string {
	width = max(width, 1)
	rows := make([]string, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, wrapLogLine(line, width)...)
	}
	return rows
}

func wrapLogLine(s string, width int) []string {
	if len(s) <= width && isASCII(s) {
		return []string{s}
	}
	if isASCII(s) {
		var rows []string
		for len(s) > width {
			rows = append(rows, s[:width])
			s = s[width:]
		}
		return append(rows, s)
	}
	var rows []string
	var b strings.Builder
	w := 0
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		rw := lipgloss.Width(s[:n])
		if w+rw > width && w > 0 {
			rows = append(rows, b.String())
			b.Reset()
			w = 0
		}
		b.WriteRune(r)
		w += rw
		s = s[n:]
	}
	return append(rows, b.String())
}

// truncateLeft keeps the end of s (a path's file name matters most),
// cutting from the left with an ellipsis.
func truncateLeft(s string, n int) string {
	n = max(n, 1)
	if lipgloss.Width(s) <= n {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width("…"+string(r)) > n {
		r = r[1:]
	}
	return "…" + string(r)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// logWidth is the log viewer's content width: wider than the other
// popups, since logs are long lines.
func (m BoardModel) logWidth() int {
	w := m.width
	if w <= 0 {
		w = 80
	}
	return max(min(w-6, 120), minColumnWidth)
}

// logHeadRows is the header above the log: title, run line, path, rule.
const logHeadRows = 4

// logWindow is the number of log rows the viewer shows.
func (m BoardModel) logWindow() int {
	budget := m.detailBudget()
	if m.height <= 0 {
		budget = 22
	}
	return max(budget-logHeadRows-rowsHeight(m.logFoot("999999–999999 / 999999")), 1)
}

// logRows is the wrapped log at the viewer's current width.
func (m BoardModel) logRows() []string {
	cw := m.logWidth() - 2
	if m.logView.wrapW == cw {
		return m.logView.rows
	}
	return wrapLog(m.logView.lines, cw)
}

// rewrapLog refreshes the cached rows after a read or a resize, keeping a
// following viewer at the bottom.
func (m *BoardModel) rewrapLog() {
	cw := m.logWidth() - 2
	m.logView.wrapW = cw
	m.logView.rows = wrapLog(m.logView.lines, cw)
	m.clampLog()
}

// clampLog keeps the offset within the rows; a following viewer sits on
// the last window, and reaching the bottom by hand starts following.
func (m *BoardModel) clampLog() {
	last := max(len(m.logRows())-m.logWindow(), 0)
	if m.logView.follow {
		m.logView.offset = last
	}
	m.logView.offset = min(max(m.logView.offset, 0), last)
	m.logView.follow = m.logView.offset == last
}

// openLog opens the viewer on t's newest run log. It reports false (and
// leaves the mode alone) when the card has no log yet.
func (m *BoardModel) openLog(t task.Task) bool {
	run, ok := m.watch.latest(t.ID)
	if !ok || !fileExists(run.Log) {
		return false
	}
	m.logFrom = m.mode
	m.mode = modeLog
	m.logView = logViewer{run: run, follow: true}
	m.readLog()
	return true
}

// readLog re-reads the log tail into the viewer.
func (m *BoardModel) readLog() {
	lines, size, clipped, err := readLogTail(m.logView.run.Log, logTailBytes)
	if err != nil {
		m.logView.err = err.Error()
		return
	}
	m.logView.err = ""
	m.logView.lines, m.logView.size, m.logView.clipped = lines, size, clipped
	m.rewrapLog()
}

// refreshLog runs on the refresh tick while the viewer is open: it picks
// up the run's new status and re-reads the log if it changed size.
func (m *BoardModel) refreshLog() {
	if m.mode != modeLog {
		return
	}
	if r, ok := m.watch.find(m.logView.run); ok {
		m.logView.run = r
		if r.Status != loops.RunRunning {
			m.logView.confirm = false
		}
	}
	st, err := os.Stat(m.logView.run.Log)
	if err != nil {
		m.logView.err = err.Error()
		return
	}
	if st.Size() != m.logView.size {
		m.readLog()
	}
}

// closeLog returns to wherever the viewer was opened from.
func (m *BoardModel) closeLog() {
	m.mode = m.logFrom
	if m.mode != modeDetail {
		m.mode = modeNormal
	}
	m.logView = logViewer{}
}

// updateLog handles the log viewer: j/k/g/G/pgup/pgdn scroll, x stops a
// live run after a y/n confirm, esc goes back.
func (m BoardModel) updateLog(k tea.KeyMsg) (BoardModel, tea.Cmd) {
	m.err = ""
	if m.logView.confirm {
		m.logView.confirm = false
		if k.String() == "y" && m.logView.run.Status == loops.RunRunning {
			m.stopRun()
		}
		return m, nil
	}
	page := max(m.logWindow()-1, 1)
	scroll := func(delta int) {
		m.logView.follow = false
		m.logView.offset += delta
		m.clampLog()
	}
	switch k.String() {
	case "esc", "q", "L":
		m.closeLog()
	case "j", "down":
		scroll(1)
	case "k", "up":
		scroll(-1)
	case "pgdown", "ctrl+d", " ":
		scroll(page)
	case "pgup", "ctrl+u":
		scroll(-page)
	case "g", "home":
		m.logView.follow = false
		m.logView.offset = 0
		m.clampLog()
	case "G", "end":
		m.logView.follow = true
		m.clampLog()
	case "x":
		if m.logView.run.Status == loops.RunRunning {
			m.logView.confirm = true
		}
	}
	return m, nil
}

// stopRun stops the viewed run's task through the injectable stopper and
// refreshes the shared loop state so every screen sees it end.
func (m *BoardModel) stopRun() {
	path := m.board.Path()
	stopped, err := m.stopLoop(path, m.logView.run.TaskID, m.now())
	if err != nil {
		m.err = "stop: " + err.Error()
		return
	}
	for _, r := range stopped {
		if r.Stage == m.logView.run.Stage && r.N == m.logView.run.N {
			m.logView.run = r
		}
	}
	if m.watch != nil && path != "" {
		*m.watch = readLoopWatch(path)
	}
	if r, ok := m.watch.find(m.logView.run); ok {
		m.logView.run = r
	}
	if len(stopped) == 0 {
		m.err = "the run had already ended"
	}
}

// logFoot is the viewer's hint footer. counter is the visible row range.
func (m BoardModel) logFoot(counter string) []string {
	cw := m.logWidth() - 2
	if m.logView.confirm {
		return popupFoot(cw, ErrorStyle.Render("Stop this run?"), keyHint("y", "Stop"), keyHint("n", "Cancel"))
	}
	var hints []string
	if m.err != "" {
		hints = append(hints, errorLine(m.err))
	}
	hints = append(hints, keyHint("j/k", "Scroll"), keyHint("g/G", "Top/end"))
	if m.logView.run.Status == loops.RunRunning {
		hints = append(hints, keyHint("x", "Stop run"))
	}
	hints = append(hints, keyHint("esc", "Back"))
	if counter != "" {
		hints = append(hints, MutedStyle.Render(counter))
	}
	return popupFoot(cw, hints...)
}

// renderLog draws the log viewer: the run's header, the visible window of
// wrapped log rows (padded so the panel keeps its height) and the footer.
func (m BoardModel) renderLog() string {
	w := m.logWidth()
	cw := w - 2
	v := m.logView
	r := v.run

	title := TitleStyle.Render("Loop log") + MutedStyle.Render("  "+shortTaskID(r.TaskID))
	if t, ok := m.board.TaskByID(r.TaskID); ok {
		title += "  " + truncate(t.Title, max(cw-lipgloss.Width(title)-2, 1))
	}
	state := runMarker(r.Status) + " " + r.Status
	if r.Status == loops.RunRunning {
		state = loopStyle.Render("⟳ running " + runDuration(runTook(r, m.now())))
	} else if r.Ended != nil {
		state += MutedStyle.Render(" " + r.Ended.In(m.now().Location()).Format("02/01 15:04") + " · took " + runDuration(runTook(r, m.now())))
	}
	meta := r.Stage + MutedStyle.Render(fmt.Sprintf(" · run %d · ", r.N)) + state + MutedStyle.Render(" · "+runAgent(r))
	path := r.Log
	if v.clipped {
		path = "last 256 KB of " + path
	}
	head := []string{
		lipgloss.NewStyle().MaxWidth(cw).Render(title),
		lipgloss.NewStyle().MaxWidth(cw).Render(meta),
		MutedStyle.Render(truncateLeft(path, cw)),
		MutedStyle.Render(strings.Repeat("─", cw)),
	}

	rows := m.logRows()
	window := m.logWindow()
	start := min(max(v.offset, 0), max(len(rows)-window, 0))
	end := min(start+window, len(rows))
	body := make([]string, 0, window)
	switch {
	case v.err != "":
		body = append(body, errorLine(truncate(v.err, cw-2)))
	case len(rows) == 0:
		body = append(body, MutedStyle.Render("the log is empty so far"))
	default:
		body = append(body, rows[start:end]...)
	}
	for len(body) < window {
		body = append(body, "")
	}
	counter := ""
	if len(rows) > window {
		counter = fmt.Sprintf("%d–%d / %d", start+1, end, len(rows))
	}
	if v.follow && r.Status == loops.RunRunning {
		counter = strings.TrimSpace(counter + " following")
	}
	all := append(append(head, body...), m.logFoot(counter)...)
	return ColumnStyle.Copy().Width(w).Render(strings.Join(all, "\n"))
}
