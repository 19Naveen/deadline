package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Markdown subset for task descriptions, rendered in the detail popup.
//
// Supported: `#`/`##`/`###` headings, `**bold**`, `*italic*`/`_italic_`,
// “ `code` “, ``` fenced blocks (preformatted, truncated — never wrapped),
// `-`/`*`/`1.` lists with indented continuations, `[text](url)` links, and
// `---` rules. Everything else renders as plain wrapped text. The form
// textarea keeps editing the raw source; one-line card previews stay raw.
//
// Two safety rules keep this predictable: inline spans never cross a line
// boundary, and unmatched markers render literally — `5 * 3` and
// `upload_original_pdf` are never eaten. Wrapping is measured with
// lipgloss.Width before styling (markers only ever shrink a line), so output
// never exceeds the given width.
var (
	mdHeadingStyle = lipgloss.NewStyle().Bold(true).Foreground(ColAccent)
	mdBoldStyle    = lipgloss.NewStyle().Bold(true).Foreground(ColText)
	mdItalicStyle  = lipgloss.NewStyle().Italic(true).Foreground(ColText)
	mdCodeStyle    = lipgloss.NewStyle().Foreground(colDoing)
	mdLinkStyle    = lipgloss.NewStyle().Underline(true).Foreground(ColAccent)
)

// Plain prose with no markup is where unreadable cards come from, so the
// renderer gives explicit structure a push before anything else runs:
//
//   - `…end. (1) Capital…` gains its own paragraph: enumerated points read
//     as blocks, not buried mid-sentence. Only digits in the parens, only
//     after sentence-ending punctuation, only before a capital — `(session
//     abc)`, `options (1) and (2)`, and `(see fig. 3)` never fire.
//   - `…end. Fix: …` (likewise Note, TODO, Warning, Important) gains its own
//     paragraph with a bold label. A bare `a quick fix: later` or lowercase
//     `end. fix: this` never fires.
//
// Both only trigger mid-line (spaces, never across newlines), so already
// structured text passes through untouched. Display-only: the source keeps
// whatever the author wrote.
var (
	mdEnumRe  = regexp.MustCompile(`([.!?)]) +\((\d+)\) +([A-Z])`)
	mdAdmonRe = regexp.MustCompile(`([.!?]) +(Fix|Note|Notes|TODO|Warning|Important):`)
)

// structureProse breaks enumerated points and admonition labels out of
// running prose into their own paragraphs.
func structureProse(src string) string {
	src = mdEnumRe.ReplaceAllString(src, "$1\n\n($2) $3")
	src = mdAdmonRe.ReplaceAllString(src, "$1\n\n**$2:** ")
	return src
}

// renderMarkdown renders src into styled rows that each fit width display
// columns. The returned string holds no newlines except the row separators.
func renderMarkdown(src string, width int) string {
	if width < 1 {
		width = 1
	}
	lines := strings.Split(structureProse(src), "\n")
	var rows []string
	for i := 0; i < len(lines); {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			i++
			for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
				rows = append(rows, mdCodeStyle.Render(truncate(lines[i], width)))
				i++
			}
			if i < len(lines) { // closing fence
				i++
			}
			continue
		}
		if text, ok := mdHeading(line); ok {
			rows = append(rows, wrapStyled(text, width, 0, func(s string) string {
				return mdHeadingStyle.Render(s)
			})...)
			i++
			continue
		}
		if mdIsRule(trimmed) {
			rows = append(rows, MutedStyle.Render(strings.Repeat("─", width)))
			i++
			continue
		}
		if indent, marker, text, ok := mdListItem(line); ok {
			text, i = mdCollectContinuation(lines, i+1, text)
			rows = append(rows, mdListRows(indent, marker, text, width)...)
			continue
		}
		if marker, text, ok := mdEnumItem(trimmed); ok {
			text, i = mdCollectContinuation(lines, i+1, text)
			rows = append(rows, mdListRows("", marker, text, width)...)
			continue
		}
		if trimmed == "" {
			if len(rows) > 0 && rows[len(rows)-1] != "" {
				rows = append(rows, "")
			}
			i++
			continue
		}

		// Markdown treats a single newline inside a paragraph as a soft
		// wrap. Join those source lines before reflowing so text pasted from
		// terminals/editors does not produce random flush-left fragments.
		paragraph := trimmed
		i++
		for i < len(lines) && !mdStartsBlock(lines[i]) {
			paragraph += " " + strings.TrimSpace(lines[i])
			i++
		}
		rows = append(rows, wrapStyled(paragraph, width, 0, nil)...)
	}
	for len(rows) > 0 && rows[len(rows)-1] == "" {
		rows = rows[:len(rows)-1]
	}
	return strings.Join(rows, "\n")
}

// mdStartsBlock reports whether line starts a new markdown block. A blank
// line is a boundary too. Inline markup deliberately does not count.
func mdStartsBlock(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "```") || mdIsRule(trimmed) {
		return true
	}
	if _, ok := mdHeading(line); ok {
		return true
	}
	if _, _, _, ok := mdListItem(line); ok {
		return true
	}
	_, _, ok := mdEnumItem(trimmed)
	return ok
}

// mdCollectContinuation joins the soft-wrapped lines following a list item
// until the next real block. It returns the joined text and next unread row.
func mdCollectContinuation(lines []string, i int, text string) (string, int) {
	for i < len(lines) && !mdStartsBlock(lines[i]) {
		text += " " + strings.TrimSpace(lines[i])
		i++
	}
	return text, i
}

// mdHeading strips one to three leading `#` markers. ok is false when the
// line is not a heading.
func mdHeading(line string) (text string, ok bool) {
	rest := line
	level := 0
	for level < 3 && strings.HasPrefix(rest, "#") {
		rest = rest[1:]
		level++
	}
	if level == 0 || !strings.HasPrefix(rest, " ") {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

// mdIsRule reports a horizontal rule: three or more `-` (or `*`), the line
// holding nothing else.
func mdIsRule(trimmed string) bool {
	if len(trimmed) < 3 {
		return false
	}
	for _, r := range trimmed {
		if r != '-' && r != '*' {
			return false
		}
	}
	return true
}

// mdEnumItem splits a `(1) …` row — the shape structureProse breaks
// enumerated points into — into marker and text, reusing the list's hanging
// indent. Anything else (session ids, `(see …)`) is not an item.
func mdEnumItem(trimmed string) (marker, text string, ok bool) {
	if !strings.HasPrefix(trimmed, "(") {
		return "", "", false
	}
	end := strings.IndexByte(trimmed, ')')
	if end < 0 {
		return "", "", false
	}
	for _, r := range trimmed[1:end] {
		if r < '0' || r > '9' {
			return "", "", false
		}
	}
	rest, found := strings.CutPrefix(trimmed[end+1:], " ")
	if !found || rest == "" {
		return "", "", false
	}
	// Parenthesized source points render as conventional ordered-list
	// markers; less punctuation reads more cleanly in a narrow panel.
	return trimmed[1:end] + ".", rest, true
}

// mdListItem splits a list row into its indent, marker and text. ok is false
// for non-list lines — including `*` emphasis mid-text, which never sits at
// the line start followed by a space.
func mdListItem(line string) (indent, marker, text string, ok bool) {
	stripped := strings.TrimLeft(line, " \t")
	indent = line[:len(line)-len(stripped)]
	if rest, found := strings.CutPrefix(stripped, "- "); found {
		return indent, "-", rest, true
	}
	if rest, found := strings.CutPrefix(stripped, "* "); found {
		return indent, "*", rest, true
	}
	for i := 0; i < len(stripped); i++ {
		c := stripped[i]
		if c < '0' || c > '9' {
			if (c == '.' || c == ')') && i > 0 {
				if rest, found := strings.CutPrefix(stripped[i+1:], " "); found {
					return indent, stripped[:i+1], rest, true
				}
			}
			return "", "", "", false
		}
	}
	return "", "", "", false
}

// mdListRows wraps one list item: the first row carries the marker, the rest
// hang underneath it. Narrow widths fall back to a single truncated row
// rather than a sliver of text.
func mdListRows(indent, marker, text string, width int) []string {
	if marker == "-" || marker == "*" {
		marker = "•"
	}
	prefix := indent + marker + " "
	cont := strings.Repeat(" ", lipgloss.Width(prefix))
	avail := width - lipgloss.Width(prefix)
	if avail < 10 {
		return []string{truncate(prefix+text, width)}
	}
	var rows []string
	for i, row := range wrapWords(text, avail) {
		if i == 0 {
			rows = append(rows, indent+mdHeadingStyle.Render(marker)+" "+renderInline(row))
			continue
		}
		rows = append(rows, renderInline(cont+row))
	}
	return rows
}

// wrapStyled wraps plain text and styles every row. Styling runs after
// wrapping on purpose: the markers it removes only shrink rows, so the width
// measured on the raw text is a safe upper bound. A nil style leaves rows
// as inline-rendered.
func wrapStyled(text string, width, indent int, style func(string) string) []string {
	pad := strings.Repeat(" ", indent)
	avail := width - indent
	if avail < 1 {
		avail = 1
	}
	var rows []string
	for _, row := range wrapWords(text, avail) {
		line := renderInline(row)
		if style != nil {
			line = style(line)
		}
		rows = append(rows, pad+line)
	}
	return rows
}

// wrapWords is a greedy word wrap measured in display columns. Overlong
// words (hashes, URLs, session ids) break mid-word so no row overflows.
func wrapWords(text string, width int) []string {
	if width < 1 {
		width = 1
	}
	var rows []string
	var cur strings.Builder
	curWidth := 0
	flush := func() {
		if cur.Len() > 0 {
			rows = append(rows, cur.String())
			cur.Reset()
			curWidth = 0
		}
	}
	for _, word := range strings.Fields(text) {
		ww := lipgloss.Width(word)
		if ww > width {
			flush()
			rows = append(rows, breakWord(word, width)...)
			continue
		}
		add := ww
		if curWidth > 0 {
			add++ // the space
		}
		if curWidth+add > width {
			flush()
		}
		if curWidth > 0 {
			cur.WriteByte(' ')
			curWidth++
		}
		cur.WriteString(word)
		curWidth += ww
	}
	flush()
	if len(rows) == 0 {
		return []string{""}
	}
	return rows
}

// breakWord splits an overlong word into rows that each fit width columns.
func breakWord(word string, width int) []string {
	var rows []string
	var cur strings.Builder
	curWidth := 0
	for _, r := range word {
		rw := lipgloss.Width(string(r))
		if curWidth+rw > width && curWidth > 0 {
			rows = append(rows, cur.String())
			cur.Reset()
			curWidth = 0
		}
		cur.WriteRune(r)
		curWidth += rw
	}
	if cur.Len() > 0 {
		rows = append(rows, cur.String())
	}
	return rows
}

// renderInline styles one line's spans. Markers without a same-line partner
// fall through as literal text.
func renderInline(line string) string {
	r := []rune(line)
	var b strings.Builder
	i := 0
	for i < len(r) {
		switch {
		case r[i] == '`':
			if j := indexRune(r, '`', i+1); j > i+1 {
				b.WriteString(mdCodeStyle.Render(string(r[i+1 : j])))
				i = j + 1
			} else {
				b.WriteRune(r[i])
				i++
			}
		case r[i] == '*' && i+1 < len(r) && r[i+1] == '*':
			if j := indexPair(r, "**", i+2); j >= 0 {
				b.WriteString(mdBoldStyle.Render(string(r[i+2 : j])))
				i = j + 2
			} else {
				b.WriteString("**")
				i += 2
			}
		case r[i] == '[':
			if text, url, next, ok := parseLink(r, i); ok {
				b.WriteString(mdLinkStyle.Render(renderInline(text)))
				b.WriteString(MutedStyle.Render(" (" + url + ")"))
				i = next
			} else {
				b.WriteRune(r[i])
				i++
			}
		case r[i] == '*' || r[i] == '_':
			if j, ok := matchItalic(r, i); ok {
				b.WriteString(mdItalicStyle.Render(string(r[i+1 : j])))
				i = j + 1
			} else {
				b.WriteRune(r[i])
				i++
			}
		default:
			b.WriteRune(r[i])
			i++
		}
	}
	return b.String()
}

// indexRune is strings.IndexRune for a rune slice from position from.
func indexRune(r []rune, target rune, from int) int {
	for j := from; j < len(r); j++ {
		if r[j] == target {
			return j
		}
	}
	return -1
}

// indexPair finds "**" at or after from. -1 when there is no partner.
func indexPair(r []rune, pair string, from int) int {
	pr := []rune(pair)
	for j := from; j+len(pr) <= len(r); j++ {
		match := true
		for k, c := range pr {
			if r[j+k] != c {
				match = false
				break
			}
		}
		if match {
			return j
		}
	}
	return -1
}

// matchItalic pairs a `*` or `_` opener with the next same marker that sits
// on a word boundary — the pairs in `*fix*` and `(see _note_)` match, while
// the underscores in `snake_case` and the star in `5 * 3` never do. A `*`
// adjacent to another `*` is a bold marker, never an italic one.
func matchItalic(r []rune, i int) (int, bool) {
	if !leftBoundary(r, i) {
		return 0, false
	}
	m := r[i]
	for j := i + 1; j < len(r); j++ {
		if r[j] != m {
			continue
		}
		if m == '*' && ((j+1 < len(r) && r[j+1] == '*') || r[j-1] == '*') {
			continue
		}
		if !rightBoundary(r, j) {
			continue
		}
		if j == i+1 {
			return 0, false // empty span: literal markers
		}
		return j, true
	}
	return 0, false
}

// leftBoundary reports whether a marker at i opens a span: start of line or
// preceded by a non-word character.
func leftBoundary(r []rune, i int) bool {
	return i == 0 || !isWordChar(r[i-1])
}

// rightBoundary reports whether a marker at j closes a span: end of line or
// followed by a non-word character.
func rightBoundary(r []rune, j int) bool {
	return j+1 == len(r) || !isWordChar(r[j+1])
}

func isWordChar(r rune) bool {
	return r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

// parseLink reads `[text](url)` at i. ok is false for anything else —
// `[brackets]` without a paren target stay literal.
func parseLink(r []rune, i int) (text, url string, next int, ok bool) {
	close := indexRune(r, ']', i+1)
	if close < 0 || close == i+1 || close+1 >= len(r) || r[close+1] != '(' {
		return "", "", 0, false
	}
	end := indexRune(r, ')', close+2)
	if end < 0 || end == close+2 {
		return "", "", 0, false
	}
	return string(r[i+1 : close]), string(r[close+2 : end]), end + 1, true
}
