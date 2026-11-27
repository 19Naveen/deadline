package wizard

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/loops"
)

// maxCard is the widest the card grows on a wide terminal; it is centred.
const maxCard = 100

func (m Model) size() (int, int) {
	w, h := m.width, m.height
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	return w, h
}

// View is a header with the stepper, one card for the current screen and
// the key bar pinned to the bottom. The card's body is clipped, never the
// footer, so every screen fits the terminal.
func (m Model) View() string {
	if m.quit {
		return ""
	}
	w, h := m.size()
	cw := min(w, maxCard)
	inner := cw - 6 // border and two cells of padding each side
	header := m.header(cw)
	footer := hintBar(cw, m.hints()...)
	room := h - lipgloss.Height(header) - 1 - 2 - 1 - lipgloss.Height(footer)
	if room < 1 {
		room = 1
	}
	title, sub, body := m.content(inner, room)

	lines := strings.Split(title, "\n")
	spacer := len(body) > 0
	need := len(lines) + len(body)
	if sub != "" {
		need++
	}
	if spacer {
		need++
	}
	if need > room && spacer {
		spacer, need = false, need-1
	}
	if need > room && sub != "" {
		sub = ""
	}
	if sub != "" {
		lines = append(lines, mutedStyle.Render(sub))
	}
	if spacer {
		lines = append(lines, "")
	}
	lines = append(lines, body...)
	if len(lines) > room {
		lines = lines[:room]
	}
	if len(lines)+2 <= room { // breathing room when the terminal has it
		lines = append(append([]string{""}, lines...), "")
	}
	for i, l := range lines {
		if lipgloss.Width(l) > inner {
			lines[i] = lipgloss.NewStyle().MaxWidth(inner).Render(l)
		}
	}
	card := cardStyle.Copy().Width(cw - 2).Render(strings.Join(lines, "\n"))
	gap := h - lipgloss.Height(header) - 1 - lipgloss.Height(card) - lipgloss.Height(footer) + 1
	if gap < 1 {
		gap = 1
	}
	above := 1 + (gap-1)/3 // a little nearer the top than the middle
	out := header + strings.Repeat("\n", above) + "\n" + card + strings.Repeat("\n", gap-above+1) + footer
	if w > cw { // centre the whole layout
		left := strings.Repeat(" ", (w-cw)/2)
		all := strings.Split(out, "\n")
		for i, l := range all {
			if l != "" {
				all[i] = left + l
			}
		}
		out = strings.Join(all, "\n")
	}
	return out
}

// header is the wordmark and the stepper, on one line when they fit.
func (m Model) header(w int) string {
	mark := accentStyle.Render("◆ gotodo") + mutedStyle.Render(" setup")
	steps := m.stepper(false)
	if lipgloss.Width(mark)+3+lipgloss.Width(steps) <= w {
		return mark + strings.Repeat(" ", w-lipgloss.Width(mark)-lipgloss.Width(steps)) + steps
	}
	if lipgloss.Width(steps) > w {
		steps = m.stepper(true)
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(mark + "\n" + steps)
}

func (m Model) stepIndex() int {
	for i, c := range crumbs {
		for _, s := range c.scr {
			if s == m.scr {
				return i
			}
		}
	}
	return len(crumbs) // apply: every step is behind
}

func (m Model) visited(i int) bool {
	for _, b := range m.back {
		for _, s := range crumbs[i].scr {
			if b == s {
				return true
			}
		}
	}
	return false
}

// stepper shows done ✓, current ●, skipped – and upcoming ○ steps joined
// by lines that turn green behind the current one. Compact keeps only the
// current label.
func (m Model) stepper(compact bool) string {
	cur := m.stepIndex()
	var b strings.Builder
	for i, c := range crumbs {
		if i > 0 {
			st := ruleStyle
			if i <= cur {
				st = okStyle.Copy().Bold(false)
			}
			b.WriteString(st.Render(" ─ "))
		}
		icon, label := mutedStyle.Render("○"), mutedStyle.Render(c.label)
		switch {
		case i == cur:
			icon, label = accentStyle.Render("●"), accentStyle.Render(c.label)
		case i < cur && m.visited(i):
			icon, label = okStyle.Render("✓"), textStyle.Render(c.label)
		case i < cur:
			icon = mutedStyle.Render("–")
		}
		b.WriteString(icon)
		if !compact || i == cur {
			b.WriteString(" " + label)
		}
	}
	return b.String()
}

func (m Model) hints() []string {
	switch {
	case m.edit != nil:
		return []string{keyHint("tab", "Suggestion"), keyHint("enter", "Use"), keyHint("esc", "Cancel")}
	case m.src != nil:
		return []string{keyHint("enter", "Add pack"), keyHint("esc", "Cancel")}
	}
	switch m.scr {
	case scrWelcome:
		return []string{keyHint("enter", "Start"), keyHint("q", "Quit")}
	case scrMCP:
		return []string{keyHint("↑↓", "Move"), keyHint("space", "Toggle"), keyHint("enter", "Next"), keyHint("esc", "Back")}
	case scrPacks:
		it, _ := m.packItem()
		switch it.kind {
		case itemAdd:
			return []string{keyHint("↑↓", "Move"), keyHint("enter", "Add a skill"), keyHint("esc", "Back")}
		case itemCustom:
			return []string{keyHint("↑↓", "Move"), keyHint("d", "Remove"), keyHint("enter", "Next"), keyHint("esc", "Back")}
		}
		return []string{keyHint("↑↓", "Choose"), keyHint("enter", "Next"), keyHint("esc", "Back")}
	case scrLoops, scrExisting:
		return []string{keyHint("↑↓", "Choose"), keyHint("enter", "Next"), keyHint("esc", "Back")}
	case scrStages:
		return []string{keyHint("↑↓", "Stage"), keyHint("←→", "Field"), keyHint("space", "Change"), keyHint("enter", "Next"), keyHint("esc", "Back")}
	case scrSummary:
		return []string{keyHint("enter", "Apply"), keyHint("esc", "Back"), keyHint("q", "Quit")}
	case scrApply:
		if m.finished {
			return []string{keyHint("enter", "Quit")}
		}
		return []string{keyHint("ctrl+c", "Abort")}
	}
	return nil
}

// content is the current screen's styled title, plain subtitle and body.
func (m Model) content(w, room int) (title, sub string, body []string) {
	switch m.scr {
	case scrWelcome:
		return m.welcomeView(w, room)
	case scrMCP:
		return m.mcpView(w)
	case scrLoops:
		return m.loopsView(w)
	case scrPacks:
		return m.packsView(w)
	case scrExisting:
		return m.existingView(w)
	case scrStages:
		return m.stagesView(w)
	case scrSummary:
		return m.summaryView(w)
	case scrApply:
		if m.finished {
			return m.doneView(w, room)
		}
		return m.applyView(w, room)
	}
	return "", "", nil
}

// logo is the gotodo wordmark in box-drawing letters.
var logo = []string{
	"┌─┐┌─┐┌┬┐┌─┐┌┬┐┌─┐",
	"│ ┬│ │ │ │ │ │││ │",
	"└─┘└─┘ ┴ └─┘─┴┘└─┘",
}

// welcomeView explains what gotodo is before asking anything, then what
// setup will do. The logo and footnote drop first on short terminals.
func (m Model) welcomeView(w, room int) (string, string, []string) {
	sub := "A kanban board in your terminal that you and your coding agents share."

	// The card flow, minimal: each column in its board colour, and who
	// moves the card there underneath.
	steps := []struct {
		col, who string
		st       lipgloss.Style
	}{{"todo", "you", textStyle}, {"in dev", "agent", amberStyle}, {"review", "agent", accentStyle}, {"shipped", "you", okStyle}}
	flow, whom := "  ", "  "
	for i, s := range steps {
		cell := len(s.col) + 5 // " ─▶  " between columns
		if i == len(steps)-1 {
			cell = len(s.col)
		}
		flow += s.st.Render(s.col)
		if i < len(steps)-1 {
			flow += mutedStyle.Render("  →  ")
		}
		whom += mutedStyle.Render(pad(s.who, cell))
	}

	var agents []string
	for _, a := range loops.Agents {
		if _, ok := m.paths[a]; ok {
			agents = append(agents, okStyle.Render("✓ ")+textStyle.Render(a))
		} else {
			agents = append(agents, mutedStyle.Render("· "+a))
		}
	}
	num := func(n, s string) string { return "  " + accentStyle.Render(n) + "  " + textStyle.Render(s) }

	build := func(withLogo, withFoot bool) (string, []string) {
		title := titleStyle.Render("Welcome to gotodo")
		if withLogo {
			var l []string
			for _, row := range logo {
				l = append(l, accentStyle.Render(row))
			}
			title = strings.Join(l, "\n")
		}
		b := []string{flow, whom, "",
			"  " + mutedStyle.Render("agents  ") + strings.Join(agents, "  "), "",
			section("This setup will"),
			num("1", "Connect gotodo to your agents, so they can read and move cards"),
			num("2", "Optionally start an agent when a card enters a column (loops)"),
			num("3", "Optionally install recommended skills for those agents"),
		}
		if withFoot {
			b = append(b, "", mutedStyle.Render("Nothing changes until you confirm on the Summary step."))
		}
		return title, b
	}
	for _, v := range [][2]bool{{true, true}, {true, false}, {false, true}, {false, false}} {
		title, b := build(v[0], v[1])
		if len(strings.Split(title, "\n"))+2+len(b) <= room {
			return title, sub, b
		}
	}
	title, b := build(false, false)
	return title, sub, b
}

var mcpWhere = map[string]string{
	"claude":   "Claude Code, user scope via claude mcp add",
	"codex":    "Codex, ~/.codex/config.toml",
	"opencode": "OpenCode, ~/.config/opencode/opencode.json",
}

func checkStyle(on bool) (lipgloss.Style, string) {
	if on {
		return accentStyle, "[✓]"
	}
	return mutedStyle, "[ ]"
}

func (m Model) mcpView(w int) (string, string, []string) {
	if len(m.detected) == 0 {
		return errorStyle.Render("No coding agents found"), "", []string{
			errorLine("No coding agent CLI found on PATH (claude, codex, opencode)."),
			"",
			textStyle.Render("Install one, then run gotodo setup again."),
		}
	}
	var b []string
	for i, a := range m.detected {
		r := m.newRow(i == m.cur[scrMCP])
		r.add(checkStyle(m.mcpOn[a])).space(1).cell(boldStyle, a, 11).add(mutedStyle, mcpWhere[a])
		b = append(b, r.line(w))
	}
	b = append(b, "", mutedStyle.Render("Agents load it on their next start. Unticked agents are left as they are."))
	return titleStyle.Render("MCP server"), "Register gotodo with your agents so they can read and move cards.", b
}

func (m Model) loopsView(w int) (string, string, []string) {
	sel := m.cur[scrLoops]
	opt := func(i int, label, desc string) []string {
		r := m.newRow(sel == i)
		st := textStyle
		if sel == i {
			st = boldStyle
		}
		r.add(radioStyle(sel == i)).space(1).add(st, label)
		return []string{r.line(w), "    " + mutedStyle.Render(desc)}
	}
	b := append(opt(0, "No, just the MCP server", "Agents touch the board only when you ask them to."),
		opt(1, "Yes, set up stage loops", "Pick an agent, model and skill for each column next.")...)
	b = append(b, "", mutedStyle.Render("Change them any time with gotodo loops or gotodo setup."))
	return titleStyle.Render("Stage loops"), "When a card enters a column, gotodo can start an agent with a skill on it.", b
}

func radioStyle(on bool) (lipgloss.Style, string) {
	if on {
		return accentStyle, "◉"
	}
	return mutedStyle, "○"
}

func (m Model) packsView(w int) (string, string, []string) {
	cur, _ := m.packItem()
	var b []string
	missing := map[string][]string{} // agent → packs Recommended would add
	for _, i := range m.offered() {
		c := m.cells[i]
		missing[c.Agent] = append(missing[c.Agent], c.Pack)
	}
	if len(missing) > 0 {
		var what []string
		for _, a := range m.detected {
			if ps := missing[a]; len(ps) > 0 {
				what = append(what, a+": "+strings.Join(ps, " + "))
			}
		}
		opt := func(kind int, on bool, label, detail string) string {
			return m.newRow(cur.kind == kind).add(radioStyle(on)).space(1).
				cell(boldStyle, label, 22).add(mutedStyle, trunc(detail, w-30)).line(w)
		}
		b = append(b,
			opt(itemRecommend, m.recommend, "Install recommended", strings.Join(what, " · ")),
			opt(itemSkip, !m.recommend, "Skip", "use the skills you already have"))
	} else {
		b = append(b, m.newRow(cur.kind == itemContinue).add(radioStyle(cur.kind != itemAdd && cur.kind != itemCustom)).space(1).
			cell(boldStyle, "Continue", 22).add(mutedStyle, "use the skills you have").line(w))
	}
	for ci, c := range m.customs {
		b = append(b, m.newRow(cur.kind == itemCustom && cur.custom == ci).add(accentStyle, "◆ ").
			cell(boldStyle, trunc(c.Label, 26), 28).add(m.pillStyle("will install", colAccent)).line(w))
	}
	switch {
	case m.src != nil:
		b = append(b, "  "+m.src.view(w-2))
	case !m.npx:
		b = append(b, m.newRow(cur.kind == itemAdd).add(mutedStyle, "+ Add my own skill…  needs npx (Node.js)").line(w))
	default:
		b = append(b, m.newRow(cur.kind == itemAdd).add(accentStyle, "+ Add my own skill…").
			add(mutedStyle, "  GitHub owner/repo, git URL or a folder").line(w))
	}
	if m.srcErr != "" {
		b = append(b, "  "+errorLine(m.srcErr))
	}
	for _, c := range m.cells {
		if c.State == packNeedsNpx {
			b = append(b, "  "+amberStyle.Render(trunc("! npx not found: install Node.js, then rerun gotodo setup for mattpocock", w-2)))
			break
		}
	}
	// What each agent has now, compactly.
	b = append(b, "", section("Already on this machine"))
	for _, a := range m.detected {
		r := m.newRow(false).cell(boldStyle, a, 10)
		for _, c := range m.cells {
			if c.Agent != a {
				continue
			}
			if c.State == packInstalled {
				r.add(okStyle, "✓ "+c.Pack).space(2)
			} else {
				r.add(mutedStyle, "· "+c.Pack).space(2)
			}
		}
		if other := otherSkills(m.skills[a]); len(other) > 0 {
			r.add(mutedStyle, trunc(fmt.Sprintf("+ %d own: %s", len(other), strings.Join(other, ", ")), w-44))
		}
		b = append(b, r.line(w))
	}
	sub := "Install the recommended skills, skip, or add your own."
	if len(missing) == 0 {
		sub = "Every agent has the recommended skills. Continue, or add your own."
	}
	return titleStyle.Render("Skills"), sub, b
}

func (m Model) existingView(w int) (string, string, []string) {
	ex := m.existing
	sel := m.cur[scrExisting]
	stages := textStyle.Render("stages: " + strings.Join(ex.Stages, ", "))
	if len(ex.Stages) == 0 {
		stages = mutedStyle.Render("no stages")
	}
	if ex.Err != nil {
		stages = errorStyle.Render("unreadable: " + trunc(ex.Err.Error(), 60))
	}
	opt := func(i int, label string) string {
		return m.newRow(sel == i).add(radioStyle(sel == i)).space(1).add(textStyle, label).line(w)
	}
	b := []string{
		"  " + boldStyle.Render(tilde(m.d.ConfigPath, m.d.Home)) + mutedStyle.Render(" · modified "+ex.Mod.Format("02/01/2006 15:04")),
		"  " + stages,
		"",
		opt(0, "Keep it, leave the file untouched"),
		opt(1, "Replace it with the stages set up next"),
	}
	return titleStyle.Render("Existing config found"), "Stage loops are already set up on this machine.", b
}

// stage table column widths: stage, on, agent, model, skill, access.
var colW = []int{15, 3, 9, 18, 16, 11}

func (m Model) stagesView(w int) (string, string, []string) {
	row, col := m.cur[scrStages], m.stageCol
	b := []string{headStyle.Render("  " + pad("STAGE", colW[0]) + pad("ON", colW[1]) + pad("AGENT", colW[2]) +
		pad("MODEL", colW[3]) + pad("SKILL", colW[4]) + "ACCESS")}
	for i, r := range m.rows {
		ln := m.newRow(i == row)
		focus := func(c int) bool { return i == row && c == col+1 }
		dim := func(st lipgloss.Style) lipgloss.Style {
			if !r.On {
				return mutedStyle
			}
			return st
		}
		text := func(c int, s string, st lipgloss.Style) {
			if focus(c) {
				st = cellStyle
			} else {
				st = dim(st)
			}
			ln.cell(st, s, colW[c])
		}
		ln.cell(dim(boldStyle), r.Name, colW[0])
		on, onSt := "·", mutedStyle
		if r.On {
			on, onSt = "✓", okStyle
		}
		text(1, on, onSt)
		text(2, r.Agent, textStyle)
		model, modelSt := r.Model, textStyle
		if model == "" {
			model, modelSt = "default", mutedStyle
		}
		text(3, model, modelSt)
		skill, skillSt := r.Skill, textStyle
		if skill == "" {
			skill, skillSt = "pick one", amberStyle
		}
		text(4, skill, skillSt)
		c := permColor(r.Perm)
		if !r.On {
			c = colBorder
		}
		tag := m.pill(r.Perm, c)
		if focus(5) {
			tag = cellStyle.Render("▸") + tag
		}
		ln.raw(tag, colW[5])
		b = append(b, ln.line(w))
	}
	b = append(b, "") // breathing room between the table and its notes
	switch {
	case m.edit != nil:
		b = append(b, m.edit.view(w))
	case m.errMsg != "":
		b = append(b, errorLine(m.errMsg))
	case row < len(m.rows) && m.rows[row].Skill != "" && !m.installed(m.rows[row].Agent, m.rows[row].Skill):
		r := m.rows[row]
		b = append(b, mutedStyle.Render(trunc(r.Skill, 20)+" is not installed for "+r.Agent+": loops will skip until it is"))
	default:
		b = append(b, mutedStyle.Render(m.fieldHelp(row, col)))
	}
	if m.flash != "" {
		b = append(b, errorLine(m.flash))
	}
	b = append(b, mutedStyle.Render("Personal boards use doing/done: set those with gotodo loops set."))
	return titleStyle.Render("Stages"), "When a card enters a column, gotodo starts the agent with the skill.", b
}

func (m Model) fieldHelp(row, col int) string {
	if row >= len(m.rows) {
		return ""
	}
	r := m.rows[row]
	switch col {
	case colOn:
		return "space turns " + r.Name + " on or off"
	case colAgent:
		return "space cycles the detected agents: " + strings.Join(m.detected, ", ")
	case colModel:
		switch r.Agent {
		case "claude":
			return "space edits: " + strings.Join(ModelSuggestions["claude"], ", ")
		case "codex":
			return "space edits; blank uses codex's default model"
		}
		return "space edits: provider/model, blank uses opencode's default"
	case colSkill:
		where := ""
		if len(m.proj[r.Agent]) > 0 {
			where = " (with this project's)"
		}
		return fmt.Sprintf("space picks from %d skills %s can load%s, or type your own", len(m.available(r.Agent)), r.Agent, where)
	case colPerm:
		return "readonly: read + gotodo · edits: change files (default) · full: no prompts"
	}
	return ""
}

func (m Model) summaryView(w int) (string, string, []string) {
	label := func(s string) string { return headStyle.Render(pad(strings.ToUpper(s), 13)) }
	mcp := textStyle.Render(strings.Join(m.mcpChosen(), ", "))
	if len(m.mcpChosen()) == 0 {
		mcp = mutedStyle.Render("none")
	}
	b := []string{label("MCP server") + mcp}
	packs, shared := m.packLines()
	if len(packs) == 0 {
		packs = []string{mutedStyle.Render("none")}
	}
	for i, p := range packs {
		l := label("")
		if i == 0 {
			l = label("Skill packs")
		}
		b = append(b, l+p)
	}
	path := tilde(m.d.ConfigPath, m.d.Home)
	review := false
	switch {
	case !m.loops:
		b = append(b, label("Loops")+mutedStyle.Render("off, "+path+" not touched"))
	case !m.writeConfig():
		b = append(b, label("Loops")+textStyle.Render("keep existing "+path))
	default:
		verb := "write "
		if m.existing != nil {
			verb = "replace "
		}
		b = append(b, label("Loops")+textStyle.Render(verb+path),
			label("")+mutedStyle.Render(fmt.Sprintf("max_parallel %d · max_runs_per_task %d",
				loops.DefaultMaxParallel, loops.DefaultMaxRunsPerTask)), "")
		for _, r := range m.rows {
			if !r.On {
				continue
			}
			model := r.Model
			if model == "" {
				model = "default"
			}
			review = review || (r.Name == "testing-review" && r.Perm == loops.PermEdits)
			b = append(b, "  "+boldStyle.Render(pad(r.Name, colW[0]))+textStyle.Render(pad(r.Agent, colW[2])+
				pad(model, colW[3])+pad(r.Skill, colW[4]))+m.pill(r.Perm, permColor(r.Perm)))
		}
	}
	var notes []string
	if shared || m.sharedPresent() {
		notes = append(notes, mutedStyle.Render("Update npx-installed packs later with: npx skills update -g"))
	}
	if review {
		notes = append(notes, mutedStyle.Render("testing-review uses edits so the reviewer can run gotodo to move the card."))
	}
	if len(notes) > 0 {
		b = append(append(b, ""), notes...)
	}
	return titleStyle.Render("Review and apply"), "Nothing has changed yet. Enter applies everything below.", b
}

// packLines is one summary line per chosen pack, and whether an npx
// install is among them.
func (m Model) packLines() (lines []string, shared bool) {
	for _, u := range m.installUnits() {
		a := u.Pack + " → " + strings.Join(u.Agents, ", ")
		if u.Needs != "" {
			shared = true
		}
		line := textStyle.Render(a)
		if len(u.Agents) > 1 && u.Pack == PackMattpocock {
			line += mutedStyle.Render(" (one shared install)")
		}
		lines = append(lines, line)
	}
	return lines, shared
}

// sharedPresent reports whether mattpocock already sits in ~/.agents/skills.
func (m Model) sharedPresent() bool {
	for _, c := range m.cells {
		if isShared(c.Pack, c.Agent) && c.State == packInstalled {
			return true
		}
	}
	return false
}

// stepMark is a step's state glyph; running steps show the spinner.
func (m Model) stepMark(s step) string {
	switch s.State {
	case stepRunning:
		return m.spin.View()
	case stepOK:
		return okStyle.Render("✓")
	case stepFailed:
		return errorStyle.Render("✗")
	case stepSkipped:
		return amberStyle.Render("–")
	}
	return mutedStyle.Render("·")
}

func took(d time.Duration) string {
	switch {
	case d <= 0:
		return ""
	case d < 10*time.Second:
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

// progress is a hand-drawn bar of finished steps.
func progress(done, total, w int) string {
	if total == 0 {
		total = 1
	}
	count := fmt.Sprintf(" %d/%d", done, total)
	bw := w - len(count)
	fill := bw * done / total
	return accentStyle.Render(strings.Repeat("━", fill)) + ruleStyle.Render(strings.Repeat("─", bw-fill)) + mutedStyle.Render(count)
}

func (m Model) applyView(w, room int) (string, string, []string) {
	done, running := 0, 0
	for i, s := range m.steps {
		switch s.State {
		case stepOK, stepFailed, stepSkipped:
			done++
		case stepRunning:
			running = i
		}
	}
	b := []string{progress(done, len(m.steps), w), ""}
	// A window of steps that keeps the running one in view.
	fit := max(1, room-3-len(b))
	start := 0
	if len(m.steps) > fit {
		start = min(max(0, running-fit/2), len(m.steps)-fit)
	}
	for i := start; i < len(m.steps) && i < start+fit; i++ {
		s := m.steps[i]
		detail := map[stepState]string{stepPending: "waiting", stepRunning: "running"}[s.State]
		if detail == "" {
			detail = s.Detail
		}
		t := took(s.Took)
		label := textStyle
		if s.State == stepRunning {
			label = boldStyle
		}
		line := m.stepMark(s) + " " + label.Render(pad(s.Label, 28)) + mutedStyle.Render(trunc(detail, w-30-8))
		b = append(b, padR(line, w-len(t))+mutedStyle.Render(t))
	}
	return amberStyle.Render("Applying…"), "Steps run one at a time; a failed step does not stop the rest.", b
}

func (m Model) doneView(w, room int) (string, string, []string) {
	failed := 0
	var mcp []string
	var packs, rest []step
	for _, s := range m.steps {
		if s.State == stepFailed {
			failed++
		}
		switch s.Group {
		case "mcp":
			mcp = append(mcp, m.stepMark(s)+" "+textStyle.Render(strings.TrimPrefix(s.Label, "MCP · ")))
		case "pack":
			packs = append(packs, s)
		default:
			rest = append(rest, s)
		}
	}
	label := func(s string) string { return "  " + headStyle.Render(pad(s, 12)) }
	stepLine := func(l string, s step) string {
		detail := ""
		switch {
		case s.Label == "":
			detail = s.Detail
		case s.State != stepOK:
			detail = "  " + s.Detail
		}
		return l + m.stepMark(s) + " " + textStyle.Render(s.Label) + mutedStyle.Render(detail)
	}
	var b []string
	if len(m.steps) == 0 {
		b = append(b, mutedStyle.Render("Nothing to do: no agent or pack was chosen."))
	} else {
		b = append(b, section("What changed"))
	}
	if len(mcp) > 0 {
		b = append(b, label("MCP server")+strings.Join(mcp, mutedStyle.Render(" · ")))
	}
	next := []string{"", section("Try next"),
		"  " + accentStyle.Render(pad("gotodo", 14)) + mutedStyle.Render("open the board"),
		"  " + accentStyle.Render(pad("gotodo loops", 14)) + mutedStyle.Render("see stage loops and their runs"),
		"  " + accentStyle.Render(pad("gotodo help", 14)) + mutedStyle.Render("every command")}
	budget := room - 3 - len(b) - len(rest) - len(next)
	if budget < len(packs) { // tight: lose the blank lines instead
		next = next[1:]
		budget = room - 2 - len(b) - len(rest) - len(next)
	}
	budget = max(1, budget)
	for i, s := range packs {
		l := label("")
		if i == 0 {
			l = label("Skill packs")
		}
		if len(packs) > budget && i == budget-1 {
			b = append(b, l+mutedStyle.Render(fmt.Sprintf("+ %d more, listed when setup closes", len(packs)-i)))
			break
		}
		b = append(b, stepLine(l, s))
	}
	for _, s := range rest {
		l := label("Loops")
		s.Label = ""
		b = append(b, stepLine(l, s))
	}
	b = append(b, next...)
	title := okStyle.Render("✓ gotodo is set up")
	if failed > 0 {
		title = errorStyle.Render(fmt.Sprintf("✗ Set up with %d failed step(s)", failed))
	}
	return title, "Done · " + m.appliedAt.Format("02/01/2006 15:04") + " · restart open agent sessions to load changes", b
}

// pillStyle returns a pill as a (style, text) pair for row builders.
func (m Model) pillStyle(text string, c lipgloss.AdaptiveColor) (lipgloss.Style, string) {
	return lipgloss.NewStyle(), m.pill(text, c)
}
