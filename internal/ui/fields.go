package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/task"
)

// priorityMarker is a card's compact priority: !!! high in red, !! medium in
// amber, ! low muted; empty when unset.
func priorityMarker(p task.Priority) string {
	switch p {
	case task.PriorityHigh:
		return lipgloss.NewStyle().Bold(true).Foreground(colBlocked).Render("!!!")
	case task.PriorityMedium:
		return lipgloss.NewStyle().Bold(true).Foreground(colDoing).Render("!!")
	case task.PriorityLow:
		return MutedStyle.Render("!")
	}
	return ""
}

// priorityWord names a priority for the detail popup; empty when unset.
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

// propKeyWidth is the key column of the compact one-row detail props.
const propKeyWidth = 12

// propRow is one compact detail property: a muted bold key, then the value
// clipped to the row.
func propRow(key, value string, width int) string {
	k := MutedStyle.Copy().Bold(true).Width(propKeyWidth).Render(key)
	return k + lipgloss.NewStyle().MaxWidth(max(width-propKeyWidth, 1)).Render(value)
}

// maxBlockerRows caps the Blocked by list so the fixed props stay short.
const maxBlockerRows = 3

// agentProps are the one-row props agents fill in: priority, labels,
// approval, blockers, claim, PR and last agent. Only set fields appear.
func (m BoardModel) agentProps(t task.Task, width int) []string {
	var rows []string
	if w := priorityWord(t.Priority); w != "" {
		rows = append(rows, propRow("Priority", priorityMarker(t.Priority)+" "+w, width))
	}
	if l := labelList(t.Labels); l != "" {
		rows = append(rows, propRow("Labels", truncate(l, width-propKeyWidth), width))
	}
	if m.awaitingApproval(t) {
		rows = append(rows, propRow("Approval", statusTag("awaiting approval")+MutedStyle.Render("  y to ship"), width))
	}
	for i, id := range t.BlockedBy {
		key := ""
		if i == 0 {
			key = "Blocked by"
		}
		if i == maxBlockerRows {
			rows = append(rows, propRow("", MutedStyle.Render(fmt.Sprintf("+%d more", len(t.BlockedBy)-i)), width))
			break
		}
		rows = append(rows, propRow(key, m.blockerLine(id, width-propKeyWidth), width))
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
		rows = append(rows, propRow("Claimed by", v, width))
	}
	if t.PR != "" {
		rows = append(rows, propRow("PR", truncate(t.PR, width-propKeyWidth), width))
	}
	if t.LastAgent != "" {
		rows = append(rows, propRow("Last agent", truncate(t.LastAgent, width-propKeyWidth), width))
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

// awaitingApproval reports whether t sits in review on a board that needs a
// human to ship it.
func (m BoardModel) awaitingApproval(t task.Task) bool {
	return m.board.RequireApproval && !t.Archived && t.Status == m.reviewStatus() &&
		m.reviewStatus() != m.board.DoneStatus()
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
