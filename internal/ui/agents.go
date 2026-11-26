package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/loops"
	"gotodo/internal/task"
)

const (
	// activityFeedSize is how many recent transitions the feed lists.
	activityFeedSize = 20
	// agentTaskRows caps the task lines listed under each agent.
	agentTaskRows = 3
	// agentWeek is the window for the per-agent move count.
	agentWeek = 7 * 24 * time.Hour
	// loopRecentRows is how many finished loop runs the page lists.
	loopRecentRows = 5
	// loopQueuedRows caps the queued runs listed before "+N more".
	loopQueuedRows = 5
)

// AgentsModel is the read-only page of loop runs and agent activity. It
// reads the shared board and loop state on every View, so a live reload
// shows up with no extra wiring. j/k scroll it when it outgrows the screen.
type AgentsModel struct {
	board *task.Board
	watch *loopWatch
	now   func() time.Time

	scroll int
	width  int
	height int
}

// NewAgentsModel wires a board into the agents page.
func NewAgentsModel(b *task.Board) AgentsModel {
	return AgentsModel{board: b, watch: &loopWatch{}, now: time.Now}
}

// Update scrolls the page: j/k by a row, g/G to the ends, pgup/pgdn a page.
func (m AgentsModel) Update(msg tea.Msg) (AgentsModel, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	_, room := m.layout()
	switch k.String() {
	case "j", "down":
		m.scroll++
	case "k", "up":
		m.scroll--
	case "pgdown", "ctrl+d", " ":
		m.scroll += max(room-1, 1)
	case "pgup", "ctrl+u":
		m.scroll -= max(room-1, 1)
	case "g", "home":
		m.scroll = 0
	case "G", "end":
		m.scroll = 1 << 30
	}
	m.scroll = m.clampScroll(m.scroll)
	return m, nil
}

// clampScroll keeps the offset between the top and the last full screen.
func (m AgentsModel) clampScroll(offset int) int {
	lines, room := m.layout()
	return min(max(offset, 0), max(len(lines)-room, 0))
}

// SetSize records the terminal size for layout.
func (m *AgentsModel) SetSize(w, h int) { m.width, m.height = w, h }

// agentSummary is one agent's row on the page.
type agentSummary struct {
	name   string
	active []task.Task // claimed or in progress under this agent
	last   time.Time   // latest move, or update to a task it touched
	moves  int         // moves in the last agentWeek
}

// activity is one transition in the feed.
type activity struct {
	task task.Task
	tr   task.Transition
}

// inProgress reports whether s is a working column: neither the first, the
// terminal one, nor blocked.
func inProgress(b *task.Board, s task.Status) bool {
	cols := b.Statuses()
	return s != cols[0] && s != b.DoneStatus() && s != task.StatusBlocked
}

// summarizeAgents collects every agent named in history, claims or
// LastAgent, most recently active first.
func summarizeAgents(b *task.Board, now time.Time) []agentSummary {
	byName := map[string]*agentSummary{}
	get := func(name string) *agentSummary {
		s, ok := byName[name]
		if !ok {
			s = &agentSummary{name: name}
			byName[name] = s
		}
		return s
	}
	bump := func(s *agentSummary, at time.Time) {
		if at.After(s.last) {
			s.last = at
		}
	}
	for _, t := range b.Tasks {
		for _, tr := range t.History {
			if tr.Agent == "" {
				continue
			}
			s := get(tr.Agent)
			bump(s, tr.At)
			if d := now.Sub(tr.At); d >= 0 && d <= agentWeek {
				s.moves++
			}
		}
		claimed := activeClaim(t, now)
		if t.ClaimedBy != "" {
			bump(get(t.ClaimedBy), t.UpdatedAt)
		}
		if t.LastAgent != "" {
			bump(get(t.LastAgent), t.UpdatedAt)
		}
		if t.Archived || t.Status == b.DoneStatus() {
			continue
		}
		switch {
		case claimed:
			s := get(t.ClaimedBy)
			s.active = append(s.active, t)
		case t.LastAgent != "" && inProgress(b, t.Status):
			s := get(t.LastAgent)
			s.active = append(s.active, t)
		}
	}
	out := make([]agentSummary, 0, len(byName))
	for _, s := range byName {
		out = append(out, *s)
	}
	slices.SortFunc(out, func(a, b agentSummary) int {
		if c := b.last.Compare(a.last); c != 0 {
			return c
		}
		return strings.Compare(a.name, b.name)
	})
	return out
}

// recentActivity is the newest transitions across all tasks, archived ones
// included, newest first.
func recentActivity(b *task.Board, n int) []activity {
	var all []activity
	for _, t := range b.Tasks {
		for _, tr := range t.History {
			all = append(all, activity{task: t, tr: tr})
		}
	}
	slices.SortStableFunc(all, func(a, b activity) int { return b.tr.At.Compare(a.tr.At) })
	if len(all) > n {
		all = all[:n]
	}
	return all
}

// pageHints is the footer: a scroll hint when the page overflows.
func (m AgentsModel) pageHints(scrolls bool) []string {
	lead := MutedStyle.Render("read-only")
	if scrolls {
		lead = keyHint("j/k", "Scroll")
	}
	return append([]string{lead}, globalHints()...)
}

// layout is every body line and the rows pageFrame leaves for them.
func (m AgentsModel) layout() (lines []string, room int) {
	lines = m.bodyLines()
	room = len(lines)
	if m.height > 0 {
		width := m.width
		if width <= 0 {
			width = 80
		}
		room = max(m.height-2-lipgloss.Height(hintBar(width, m.pageHints(true)...)), 0)
	}
	return lines, room
}

// View renders the loop runs, the agents list and the activity feed,
// scrolled to the current offset.
func (m AgentsModel) View() string {
	lines, room := m.layout()
	scrolls := m.height > 0 && len(lines) > room
	if scrolls {
		lines = lines[m.clampScroll(m.scroll):]
	}
	return pageFrame(m.width, m.height, strings.Join(lines, "\n"), m.pageHints(scrolls)...)
}

// bodyLines is the whole page before scrolling: Loops, Agents, Activity.
func (m AgentsModel) bodyLines() []string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	now := m.now()
	agents := summarizeAgents(m.board, now)
	feed := recentActivity(m.board, activityFeedSize)

	lines := m.loopLines(now, width)
	lines = append(lines, "", TitleStyle.Render("Agents")+MutedStyle.Render(" ("+strconv.Itoa(len(agents))+")"))
	if len(agents) == 0 {
		lines = append(lines, MutedStyle.Render(truncate("no agent activity yet; agents name themselves with "+task.AgentEnv, width)))
	}
	for _, a := range agents {
		lines = append(lines, m.agentLines(a, now, width)...)
	}

	lines = append(lines, "", TitleStyle.Render("Activity"))
	if len(feed) == 0 {
		lines = append(lines, MutedStyle.Render("no moves yet"))
	}
	for _, f := range feed {
		lines = append(lines, m.feedLine(f, now, width))
	}
	return lines
}

// loopLines is the Loops section: live runs (⟳), queued runs (…) and the
// last few finished ones, or one muted line when loops are off.
func (m AgentsModel) loopLines(now time.Time, width int) []string {
	w := m.watch
	if w == nil {
		w = &loopWatch{}
	}
	r := w.report
	head := TitleStyle.Render("Loops")
	if w.on || len(r.Running) > 0 || len(r.Queued) > 0 {
		head += MutedStyle.Render(fmt.Sprintf(" (%d running · %d queued)", len(r.Running), len(r.Queued)))
	}
	lines := []string{head}
	if w.err != "" {
		lines = append(lines, errorLine(truncate("loops: "+w.err, width-2)))
	}
	if !w.on {
		lines = append(lines, MutedStyle.Render(truncate("  Loops are off · gotodo setup or gotodo loops on", width)))
	} else if len(r.Running)+len(r.Queued)+len(r.Recent) == 0 {
		lines = append(lines, MutedStyle.Render(truncate("  no runs yet: moving a card into a looped column starts one", width)))
	}

	stageW := 0
	recent := r.Recent[:min(len(r.Recent), loopRecentRows)]
	for _, list := range [][]loops.RunRecord{r.Running, recent} {
		for _, run := range list {
			stageW = max(stageW, lipgloss.Width(run.Stage))
		}
	}
	for _, q := range r.Queued {
		stageW = max(stageW, lipgloss.Width(q.Stage))
	}
	stageW = min(stageW, 14)
	stage := func(s string) string { return fmt.Sprintf("%-*s", stageW, truncate(s, stageW)) }

	for _, run := range r.Running {
		right := MutedStyle.Render(runAgent(run)+"  ") + loopStyle.Render(runDuration(runTook(run, now)))
		lines = append(lines, m.loopRow(runMarker(run.Status)+" "+loopStyle.Render(stage(run.Stage)), run.TaskID, right, width))
	}
	for i, q := range r.Queued {
		if i == loopQueuedRows {
			lines = append(lines, MutedStyle.Render(fmt.Sprintf("    +%d more queued", len(r.Queued)-i)))
			break
		}
		right := MutedStyle.Render("queued " + ago(max(now.Sub(q.QueuedAt), 0)))
		lines = append(lines, m.loopRow(MutedStyle.Render("… "+stage(q.Stage)), q.TaskID, right, width))
	}
	for _, run := range recent {
		end := run.Started
		if run.Ended != nil {
			end = *run.Ended
		}
		right := MutedStyle.Render(fmt.Sprintf("%s %s  %s", run.Status, end.In(now.Location()).Format("02/01 15:04"), runDuration(runTook(run, now))))
		lines = append(lines, m.loopRow(runMarker(run.Status)+" "+MutedStyle.Render(stage(run.Stage)), run.TaskID, right, width))
	}
	return lines
}

// loopRow is one run: marker and stage, short ID and title, then right
// (agent, status, times) flush right; the title gives way first.
func (m AgentsModel) loopRow(lead, taskID, right string, width int) string {
	title := "(deleted)"
	if t, ok := m.board.TaskByID(taskID); ok {
		title = t.Title
	}
	head := "  " + lead + "  " + MutedStyle.Render(shortTaskID(taskID)) + "  "
	room := width - lipgloss.Width(head) - lipgloss.Width(right) - 2
	line := head + truncate(title, max(room, 1))
	if gap := width - lipgloss.Width(line) - lipgloss.Width(right); gap >= 2 {
		line += strings.Repeat(" ", gap) + right
	} else {
		line += "  " + right
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}

// agentLines is an agent's heading row and its current tasks.
func (m AgentsModel) agentLines(a agentSummary, now time.Time, width int) []string {
	last := "never"
	if !a.last.IsZero() {
		if d := now.Sub(a.last); d >= 0 {
			last = ago(d)
		} else {
			last = "just now"
		}
	}
	stats := MutedStyle.Render(fmt.Sprintf("  %s active   last %s   %s this week",
		strconv.Itoa(len(a.active)), last, plural(a.moves, "move")))
	name := claimStyle.Copy().Bold(true).Render("◆ " + truncate(a.name, max(width-lipgloss.Width(stats)-4, 4)))
	rows := []string{lipgloss.NewStyle().MaxWidth(width).Render("  " + name + stats)}
	for i, t := range a.active {
		if i == agentTaskRows {
			rows = append(rows, MutedStyle.Render(fmt.Sprintf("      +%d more", len(a.active)-i)))
			break
		}
		status := HeaderStyle(t.Status).Render(statusWord(t.Status))
		head := "    " + MutedStyle.Render(shortTaskID(t.ID)) + "  "
		title := truncate(t.Title, max(width-lipgloss.Width(head)-lipgloss.Width(status)-2, 1))
		rows = append(rows, head+title+"  "+status)
	}
	return rows
}

// feedLine is one transition: DD/MM HH:MM, who, short ID, title, from → to.
func (m AgentsModel) feedLine(f activity, now time.Time, width int) string {
	who := "you"
	style := MutedStyle
	if f.tr.Agent != "" {
		who, style = f.tr.Agent, claimStyle
	}
	stamp := MutedStyle.Render(f.tr.At.In(now.Location()).Format("02/01 15:04"))
	whoCell := style.Render(fmt.Sprintf("%-10s", truncate(who, 10)))
	move := lipgloss.NewStyle().Foreground(AccentFor(f.tr.From)).Render(statusWord(f.tr.From)) +
		MutedStyle.Render(" → ") +
		lipgloss.NewStyle().Foreground(AccentFor(f.tr.To)).Render(statusWord(f.tr.To))
	head := "  " + stamp + "  " + whoCell + "  " + MutedStyle.Render(shortTaskID(f.task.ID)) + "  "
	title := truncate(f.task.Title, max(width-lipgloss.Width(head)-lipgloss.Width(move)-2, 1))
	return lipgloss.NewStyle().MaxWidth(width).Render(head + title + "  " + move)
}
