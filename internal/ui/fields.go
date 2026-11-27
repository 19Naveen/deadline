package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"gotodo/internal/task"
)

// priorityColour is a priority's colour: red high, amber medium, blue low.
func priorityColour(p task.Priority) (lipgloss.AdaptiveColor, bool) {
	switch p {
	case task.PriorityHigh:
		return colBlocked, true
	case task.PriorityMedium:
		return colDoing, true
	case task.PriorityLow:
		return colTodo, true
	}
	return lipgloss.AdaptiveColor{}, false
}

// priorityDot is a card's priority: a single coloured ●, or under a
// colourless profile (NO_COLOR, pipes) the letter H, M or L, so the level
// still reads. Empty when unset.
func priorityDot(p task.Priority) string {
	c, ok := priorityColour(p)
	if !ok {
		return ""
	}
	if lipgloss.ColorProfile() == termenv.Ascii {
		return strings.ToUpper(priorityWord(p)[:1])
	}
	return lipgloss.NewStyle().Foreground(c).Render("●")
}

// priorityFact is the detail popup's "● high", dot and word in the
// priority's colour; empty when unset.
func priorityFact(p task.Priority) string {
	c, ok := priorityColour(p)
	if !ok {
		return ""
	}
	return lipgloss.NewStyle().Bold(p == task.PriorityHigh).Foreground(c).Render("● " + priorityWord(p))
}

// priorityWord names a priority; empty when unset.
func priorityWord(p task.Priority) string {
	switch p {
	case task.PriorityHigh:
		return "high"
	case task.PriorityMedium:
		return "medium"
	case task.PriorityLow:
		return "low"
	}
	return ""
}

// activeClaim reports whether an agent holds t right now; an expired claim
// no longer counts.
func activeClaim(t task.Task, now time.Time) bool {
	return t.ClaimedBy != "" && (t.ClaimUntil == nil || t.ClaimUntil.After(now))
}

// claimStyle marks an agent's name wherever it appears.
var claimStyle = lipgloss.NewStyle().Foreground(ColAccent)

// cardClaim fits the "◆ agent" marker into budget cells: the full name when
// it fits, shortened, or a bare diamond; empty without an active claim.
func cardClaim(t task.Task, now time.Time, budget int) string {
	if !activeClaim(t, now) || budget < 1 {
		return ""
	}
	if budget < 4 {
		return claimStyle.Render("◆")
	}
	return claimStyle.Render("◆ " + truncate(t.ClaimedBy, budget-2))
}

// cardMarker is the agent marker beside a card's countdown: "⟳ loop:stage"
// while a loop runs on the card (its claim is the same loop), else the
// "◆ agent" claim.
func (m BoardModel) cardMarker(t task.Task, budget int) string {
	if run, ok := m.watch.running(t.ID); ok {
		return cardLoop(run, budget)
	}
	return cardClaim(t, m.now(), budget)
}

// hasLoopLog reports whether t has a run log the L viewer can open.
func (m BoardModel) hasLoopLog(t task.Task) bool {
	run, ok := m.watch.latest(t.ID)
	return ok && fileExists(run.Log)
}

// formatStamp is a date and time in the Singapore layout, DD/MM/YYYY HH:MM.
func formatStamp(t time.Time) string { return t.Format("02/01/2006 15:04") }

// labelList renders labels as "#a #b".
func labelList(labels []string) string {
	tags := make([]string, 0, len(labels))
	for _, l := range labels {
		if l = strings.TrimSpace(l); l != "" {
			tags = append(tags, "#"+strings.TrimPrefix(l, "#"))
		}
	}
	return strings.Join(tags, " ")
}

// detailFacts are the short facts under the detail popup's meta line:
// priority, deadline with its countdown, labels and the approval state.
// Only the set ones appear; none at all means no line.
func (m BoardModel) detailFacts(t task.Task) []string {
	var facts []string
	if p := priorityFact(t.Priority); p != "" {
		facts = append(facts, p)
	}
	if t.Deadline != nil {
		now := m.now()
		done := m.board.DoneStatus()
		due := "due " + FormatDate(t.Deadline.In(now.Location()))
		if c := countdownText(t, now, done); c != "" {
			due += " · " + c
		}
		u := task.DeadlineUrgency(t, now, done)
		facts = append(facts, lipgloss.NewStyle().Foreground(UrgencyColor(u)).Bold(u >= task.UrgencySoon).Render(due))
	}
	if l := labelList(t.Labels); l != "" {
		facts = append(facts, MutedStyle.Render(l))
	}
	switch {
	case m.awaitingApproval(t):
		facts = append(facts, statusTag("awaiting approval"))
	case m.inReview(t):
		facts = append(facts, statusTag("ready to verify"))
	}
	return facts
}

// joinWrap lays parts out separated by sep, starting a new row before a
// part that would push the row past width. A part wider than width is
// clipped.
func joinWrap(parts []string, sep string, width int) []string {
	clip := lipgloss.NewStyle().MaxWidth(max(width, 1))
	var rows []string
	row := ""
	for _, p := range parts {
		p = clip.Render(p)
		if row != "" && lipgloss.Width(row)+lipgloss.Width(sep)+lipgloss.Width(p) > width {
			rows = append(rows, row)
			row = ""
		}
		if row != "" {
			row += sep
		}
		row += p
	}
	if row != "" {
		rows = append(rows, row)
	}
	return rows
}

// agentKeyWidth is the key column of the Agent section's rows.
const agentKeyWidth = 12

// agentRows are the Agent section: the loop run, the claim, the PR and the
// last agent, one aligned "key  value" row each, values that are too long
// wrapping under the value column. Empty when none is set.
func (m BoardModel) agentRows(t task.Task, width int) []string {
	type row struct{ key, value string }
	var rows []row
	if run, ok := m.watch.latest(t.ID); ok {
		rows = append(rows, row{"Loop", loopSummary(run, m.now())})
	}
	if t.ClaimedBy != "" {
		v := claimStyle.Render("◆ " + t.ClaimedBy)
		if t.ClaimUntil != nil {
			until := "until " + formatStamp(t.ClaimUntil.In(m.now().Location()))
			if !activeClaim(t, m.now()) {
				until = "expired " + formatStamp(t.ClaimUntil.In(m.now().Location()))
			}
			v += MutedStyle.Render("  " + until)
		}
		rows = append(rows, row{"Claimed by", v})
	}
	if t.PR != "" {
		rows = append(rows, row{"PR", t.PR})
	}
	if t.LastAgent != "" {
		rows = append(rows, row{"Last agent", t.LastAgent})
	}
	valueW := max(width-agentKeyWidth, 1)
	pad := strings.Repeat(" ", agentKeyWidth)
	key := MutedStyle.Copy().Bold(true).Width(agentKeyWidth)
	var out []string
	for _, r := range rows {
		lines := []string{lipgloss.NewStyle().MaxWidth(valueW).Render(r.value)}
		if r.key == "PR" || r.key == "Last agent" {
			lines = wrapPath(r.value, valueW)
		}
		for i, line := range lines {
			if i == 0 {
				out = append(out, key.Render(r.key)+line)
			} else {
				out = append(out, pad+line)
			}
		}
	}
	return out
}

// wrapPath breaks a path or URL into rows of at most width cells. It breaks
// after a "/" (or at a space) when one fits on the row, and mid-word only
// when a single segment is wider than the row.
func wrapPath(s string, width int) []string {
	width = max(width, 1)
	var rows []string
	rs := []rune(s)
	for len(rs) > 0 {
		w, fit, brk := 0, 0, -1
		for fit < len(rs) {
			cw := lipgloss.Width(string(rs[fit]))
			if w+cw > width {
				break
			}
			w += cw
			fit++
			if rs[fit-1] == '/' || rs[fit-1] == ' ' {
				brk = fit
			}
		}
		if fit == len(rs) {
			rows = append(rows, string(rs))
			break
		}
		cut := fit
		if brk > 0 {
			cut = brk
		}
		if cut == 0 {
			cut = 1 // a rune wider than the row
		}
		rows = append(rows, strings.TrimRight(string(rs[:cut]), " "))
		rs = []rune(strings.TrimLeft(string(rs[cut:]), " "))
	}
	return rows
}

// blockerLine is one dependency: short ID, title and status, done ones ticked.
func (m BoardModel) blockerLine(id string, width int) string {
	b, ok := m.board.TaskByID(id)
	if !ok {
		return MutedStyle.Render(shortTaskID(id) + "  missing")
	}
	status := statusWord(b.Status)
	mark := lipgloss.NewStyle().Foreground(AccentFor(b.Status)).Render("○")
	if b.Status == m.board.DoneStatus() {
		mark = lipgloss.NewStyle().Foreground(colDone).Render("✓")
	}
	right := MutedStyle.Render("  " + status)
	head := mark + " " + MutedStyle.Render(shortTaskID(b.ID)) + "  "
	title := truncate(b.Title, max(width-lipgloss.Width(head)-lipgloss.Width(right), 1))
	return head + title + right
}

// reviewStatus is the column cards wait in for approval: the last column
// before the terminal one, skipping blocked (testing-review, or doing).
func (m BoardModel) reviewStatus() task.Status {
	cols := m.board.Statuses()
	for i := len(cols) - 2; i >= 0; i-- {
		if cols[i] != task.StatusBlocked {
			return cols[i]
		}
	}
	return cols[0]
}

// inReview reports whether t sits in the review column, ready for a human
// to verify and ship with v.
func (m BoardModel) inReview(t task.Task) bool {
	return !t.Archived && t.Status == m.reviewStatus() && m.reviewStatus() != m.board.DoneStatus()
}

// awaitingApproval reports whether t sits in review on a board that needs a
// human to ship it.
func (m BoardModel) awaitingApproval(t task.Task) bool {
	return m.board.RequireApproval && m.inReview(t)
}

// columnHeader is the column title with its count: "(n)", "(shown/total)"
// while filtered, and "(cards/limit)" under a WIP limit, red once over it.
// The label is shortened rather than letting the header wrap.
func (m BoardModel) columnHeader(s task.Status, shown, width int) string {
	total := len(m.board.ByStatus(s))
	var count string
	if limit := m.board.WIPLimits[s]; limit > 0 {
		style := MutedStyle
		if total > limit {
			style = ErrorStyle
		}
		lead := " ("
		if m.filtering() {
			lead += strconv.Itoa(shown) + " · "
		}
		count = MutedStyle.Render(lead) + style.Render(fmt.Sprintf("%d/%d", total, limit)) + MutedStyle.Render(")")
	} else {
		c := strconv.Itoa(shown)
		if m.filtering() {
			c += "/" + strconv.Itoa(total)
		}
		count = MutedStyle.Render(" (" + c + ")")
	}
	room := max(width-2-lipgloss.Width(count), 1) // ColumnStyle padding
	return HeaderStyle(s).Render(truncate(s.Label(), room)) + count
}
