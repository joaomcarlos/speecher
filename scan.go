package main

import (
	"strings"
)

// Span is a byte range [Start,End) into the source text.
type Span struct{ Start, End int }

func (s Span) slice(text string) string { return text[s.Start:s.End] }

const (
	blkText = iota
	blkFenced
	blkIndented
	blkTable
)

type Block struct {
	Kind         int
	Start, End   int
	Info         string
	Depth        int    // quote depth the block opened at
	FirstLine    int    // index into Doc.Lines
	LastLine     int    // inclusive
	ContentLines []Span // per-line content ranges (after quote prefixes)
}

// Content reconstructs a block's body from its content lines.
func (b Block) Content(text string) string {
	var sb strings.Builder
	for i, ln := range b.ContentLines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(ln.slice(text))
	}
	return sb.String()
}

type mdLine struct {
	start, end   int    // raw line range, end excludes '\n'
	contentStart int    // offset after '>' prefixes; list/heading markers stay in content
	structPos    int    // offset after prefixes AND leading marker — for block detection
	quoteDepth   int    // number of stripped '>' prefixes
	prefixes     []Span // each '>' marker incl. one following space
	marker       *Span  // leading list or ATX-heading marker
}

type Doc struct {
	Text       string
	Lines      []mdLine
	Blocks     []Block
	QuoteMarks []Span // all recorded '>' prefixes
}

func isBlank(ln mdLine, text string) bool {
	return strings.TrimSpace(text[ln.contentStart:ln.end]) == ""
}

// listMarkerLen returns the length of a list marker at p ("- ", "1. ", etc).
func listMarkerLen(text string, p, end int) int {
	if p >= end {
		return 0
	}
	c := text[p]
	if c == '-' || c == '*' || c == '+' {
		if p+1 < end && (text[p+1] == ' ' || text[p+1] == '\t') {
			return 2
		}
		return 0
	}
	if c >= '0' && c <= '9' {
		i := p
		for i < end && text[i] >= '0' && text[i] <= '9' && i-p < 9 {
			i++
		}
		if i > p && i < end && (text[i] == '.' || text[i] == ')') && i+1 < end &&
			(text[i+1] == ' ' || text[i+1] == '\t') {
			return i + 2 - p
		}
	}
	return 0
}

// headingMarkerLen returns the length of an ATX heading marker "# " .. "###### ".
func headingMarkerLen(text string, p, end int) int {
	i := p
	for i < end && text[i] == '#' && i-p < 7 {
		i++
	}
	if n := i - p; n >= 1 && n <= 6 && i < end && (text[i] == ' ' || text[i] == '\t') {
		return n + 1
	}
	return 0
}

func parseLinePrefixes(text string, start, end int) mdLine {
	l := mdLine{start: start, end: end}
	pos := start
	contentStart := -1
	// Strip '>' quote prefixes; a list marker may sit between levels
	// ("- > quoted" is a list item containing a quote). The marker itself
	// belongs to the content — it is recorded separately and hidden by rules.
	for iter := 0; iter < 10 && pos < end; iter++ {
		p := pos
		n := 0
		for n < 4 && p < end && text[p] == ' ' {
			p++
			n++
		}
		if n >= 4 || p >= end {
			break // indented content or end of line
		}
		c := text[p]
		if c == '>' {
			s := Span{p, p + 1}
			p++
			if p < end && (text[p] == ' ' || text[p] == '\t') {
				s.End = p + 1
				p++
			}
			l.prefixes = append(l.prefixes, s)
			l.quoteDepth++
			pos = p
			continue
		}
		if l.marker == nil {
			if m := listMarkerLen(text, p, end); m > 0 {
				l.marker = &Span{p, p + m}
				if contentStart < 0 {
					contentStart = p
				}
				pos = p + m
				continue
			}
			if c == '#' {
				if m := headingMarkerLen(text, p, end); m > 0 {
					l.marker = &Span{p, p + m}
					if contentStart < 0 {
						contentStart = p
					}
					pos = p + m
				}
				break // '>' inside an ATX heading is literal
			}
		}
		break
	}
	if contentStart < 0 {
		contentStart = pos
	}
	l.contentStart = contentStart
	l.structPos = pos
	return l
}

// structStart is where block-structure detection should look: after quote
// prefixes and any leading list/heading marker.
func structStart(text string, ln mdLine) int { return ln.structPos }

func scanLines(text string) []mdLine {
	var lines []mdLine
	pos := 0
	for pos <= len(text) {
		end := len(text)
		if i := strings.IndexByte(text[pos:], '\n'); i >= 0 {
			end = pos + i
		}
		lines = append(lines, parseLinePrefixes(text, pos, end))
		if end >= len(text) {
			break
		}
		pos = end + 1
	}
	return lines
}

// fenceOpen returns (char, runLen, infoStart) if content starts a code fence.
func fenceOpen(ct string) (byte, int, int) {
	i := 0
	for i < len(ct) && (ct[i] == ' ' || ct[i] == '\t') {
		i++
	}
	if i >= len(ct) || (ct[i] != '`' && ct[i] != '~') {
		return 0, 0, 0
	}
	c := ct[i]
	j := i
	for j < len(ct) && ct[j] == c {
		j++
	}
	if j-i < 3 {
		return 0, 0, 0
	}
	if c == '`' && strings.Contains(ct[j:], "`") {
		return 0, 0, 0 // backtick fences may not have backticks in the info string
	}
	return c, j - i, j
}

// fenceClose reports whether content closes a fence of `c` opened with `open` chars.
func fenceClose(ct string, c byte, open int) bool {
	i := 0
	for i < len(ct) && (ct[i] == ' ' || ct[i] == '\t') {
		i++
	}
	j := i
	for j < len(ct) && ct[j] == c {
		j++
	}
	if j-i < open {
		return false
	}
	return strings.TrimSpace(ct[j:]) == ""
}

// splitCells splits a GFM table row on unescaped pipes.
func splitCells(ct string) []string {
	var cells []string
	start := 0
	for i := 0; i < len(ct); i++ {
		if ct[i] == '|' && (i == 0 || ct[i-1] != '\\') {
			cells = append(cells, ct[start:i])
			start = i + 1
		}
	}
	cells = append(cells, ct[start:])
	for i := range cells {
		cells[i] = strings.TrimSpace(strings.ReplaceAll(cells[i], "\\|", "|"))
	}
	if len(cells) > 0 && cells[0] == "" {
		cells = cells[1:]
	}
	if len(cells) > 0 && cells[len(cells)-1] == "" {
		cells = cells[:len(cells)-1]
	}
	return cells
}

func isDelimCell(c string) bool {
	c = strings.TrimSpace(c)
	if c == "" {
		return false
	}
	c = strings.Trim(c, ":")
	if len(c) < 1 {
		return false
	}
	return strings.Count(c, "-") == len(c)
}

func isDelimRow(ct string, want int) bool {
	if !strings.Contains(ct, "|") && want > 1 {
		return false
	}
	cells := splitCells(ct)
	if len(cells) != want || len(cells) == 0 {
		return false
	}
	for _, c := range cells {
		if !isDelimCell(c) {
			return false
		}
	}
	return true
}

func contentOf(text string, ln mdLine) string { return text[ln.contentStart:ln.end] }

// structOf is the line content used for block-structure detection — after
// quote prefixes and any leading list/heading marker.
func structOf(text string, ln mdLine) string { return text[ln.structPos:ln.end] }

func leadSpaces(ct string) int {
	i := 0
	for i < len(ct) && (ct[i] == ' ' || ct[i] == '\t') {
		i++
	}
	return i
}

// Scan parses text into lines and top-level blocks.
func Scan(text string) *Doc {
	lines := scanLines(text)
	doc := &Doc{Text: text, Lines: lines}
	var blocks []Block
	i := 0
	for i < len(lines) {
		ln := lines[i]
		ct := structOf(text, ln)
		lead := leadSpaces(ct)
		trimmed := ct[lead:]
		if strings.TrimSpace(trimmed) == "" {
			i++
			continue
		}
		// fenced code block (may sit inside a blockquote or list)
		if c, open, infoAt := fenceOpen(trimmed); c != 0 && lead <= 3 {
			info := strings.TrimSpace(trimmed[infoAt:])
			start := ln.structPos + lead + strings.IndexByte(trimmed, c)
			j := i + 1
			var contentLines []Span
			end := lines[len(lines)-1].end
			for j < len(lines) {
				l2 := lines[j]
				c2 := structOf(text, l2)
				if l2.quoteDepth < ln.quoteDepth && strings.TrimSpace(c2) != "" {
					break // quote context ended; fence ends unclosed
				}
				if l2.quoteDepth >= ln.quoteDepth && fenceClose(c2, c, open) {
					end = l2.end
					j++
					break
				}
				contentLines = append(contentLines, Span{l2.contentStart, l2.end})
				end = l2.end
				j++
			}
			blocks = append(blocks, Block{Kind: blkFenced, Start: start, End: end, Info: info,
				Depth: ln.quoteDepth, FirstLine: i, LastLine: j - 1, ContentLines: contentLines})
			i = j
			continue
		}
		// indented code: >=4 spaces / tab, after a blank line or at start
		indented := lead >= 4 || (len(ct) > 0 && ct[0] == '\t')
		if indented && (i == 0 || isBlank(lines[i-1], text)) {
			var contentLines []Span
			end := ln.end
			j := i
			for j < len(lines) {
				l2 := lines[j]
				c2 := contentOf(text, l2)
				if strings.TrimSpace(c2) == "" {
					j++
					continue
				}
				l2lead := leadSpaces(c2)
				if l2lead < 4 && !(len(c2) > 0 && c2[0] == '\t') {
					break
				}
				contentLines = append(contentLines, Span{l2.contentStart + min(4, l2lead), l2.end})
				end = l2.end
				j++
			}
			if len(contentLines) > 0 {
				blocks = append(blocks, Block{Kind: blkIndented, Start: ln.start, End: end,
					Depth: ln.quoteDepth, FirstLine: i, LastLine: j - 1, ContentLines: contentLines})
				i = j
				continue
			}
		}
		// GFM table: header row with '|' + delimiter row of matching cell count
		if i+1 < len(lines) && strings.Contains(ct, "|") &&
			lines[i+1].quoteDepth == ln.quoteDepth {
			header := splitCells(trimmed)
			next := structOf(text, lines[i+1])
			if len(header) > 0 && isDelimRow(next, len(header)) {
				contentLines := []Span{{ln.contentStart, ln.end}}
				j := i + 2
				end := lines[i+1].end
				for j < len(lines) {
					l2 := lines[j]
					c2 := structOf(text, l2)
					if strings.TrimSpace(c2) == "" || l2.quoteDepth != ln.quoteDepth {
						break
					}
					contentLines = append(contentLines, Span{l2.contentStart, l2.end})
					end = l2.end
					j++
				}
				blocks = append(blocks, Block{Kind: blkTable, Start: ln.start, End: end,
					Depth: ln.quoteDepth, FirstLine: i, LastLine: j - 1, ContentLines: contentLines})
				i = j
				continue
			}
		}
		// text block: runs until blank line, fence opener, or quote-depth change
		j := i + 1
		for j < len(lines) {
			l2 := lines[j]
			c2 := structOf(text, l2)
			t2 := c2[leadSpaces(c2):]
			if strings.TrimSpace(t2) == "" || l2.quoteDepth != ln.quoteDepth {
				break
			}
			if fc, _, _ := fenceOpen(t2); fc != 0 && leadSpaces(c2) <= 3 {
				break // a fence can interrupt a paragraph
			}
			j++
		}
		blocks = append(blocks, Block{Kind: blkText, Start: ln.start, End: lines[j-1].end,
			Depth: ln.quoteDepth, FirstLine: i, LastLine: j - 1})
		i = j
	}
	doc.Blocks = blocks
	// '>' prefixes inside code-block content are literal text, not quote
	// markers: only the first block.Depth prefixes on each covered line are real.
	codeDepth := map[int]int{}
	for _, b := range blocks {
		if b.Kind == blkFenced || b.Kind == blkIndented {
			for i := b.FirstLine; i <= b.LastLine; i++ {
				codeDepth[i] = b.Depth
			}
		}
	}
	for i, ln := range lines {
		if budget, ok := codeDepth[i]; ok {
			doc.QuoteMarks = append(doc.QuoteMarks, ln.prefixes[:min(budget, len(ln.prefixes))]...)
		} else {
			doc.QuoteMarks = append(doc.QuoteMarks, ln.prefixes...)
		}
	}
	return doc
}

// --- inline scanning ---

const (
	segText   = iota
	segCode   // inline `code` span; C range = inner content
	segAuto   // <https://...> autolink; C range = url
	segHidden // link destinations / syntax — never spoken
)

type Seg struct {
	Kind         int
	Start, End   int
	CStart, CEnd int
}

func isPunct(b byte) bool {
	return strings.IndexByte("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", b) >= 0
}

func runLen(text string, i int, c byte) int {
	j := i
	for j < len(text) && text[j] == c {
		j++
	}
	return j - i
}

// findCloseRun finds a run of exactly k backticks after i. Longer runs do not
// close. The search stops at a newline: rules rescan cleaned text, and a stray
// backtick pairing across a line break creates giant false code spans.
func findCloseRun(text string, i, end, k int) int {
	for i < end {
		if text[i] == '\n' {
			return -1
		}
		j := strings.IndexAny(text[i:end], "`\n")
		if j < 0 {
			return -1
		}
		i += j
		if text[i] == '\n' {
			return -1
		}
		n := runLen(text, i, '`')
		if n == k {
			return i
		}
		i += n
	}
	return -1
}

var autoLinkScheme = func(s string) bool {
	i := strings.IndexByte(s, ':')
	if i < 2 || i > 32 {
		return false
	}
	for j, b := range s[:i] {
		ok := (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
			(j > 0 && ((b >= '0' && b <= '9') || b == '+' || b == '-' || b == '.'))
		if !ok {
			return false
		}
	}
	return true
}

// autolinkEnd returns the end offset of "<...>" starting at i, or -1.
func autolinkEnd(text string, i, end int) int {
	j := strings.IndexByte(text[i+1:end], '>')
	if j < 0 {
		return -1
	}
	j += i + 1
	inner := text[i+1 : j]
	if strings.ContainsAny(inner, " \t\n<") {
		return -1
	}
	if autoLinkScheme(inner) {
		return j + 1
	}
	// email autolink
	at := strings.IndexByte(inner, '@')
	if at > 0 && at < len(inner)-1 && !strings.ContainsAny(inner, " ") &&
		strings.Contains(inner[at+1:], ".") {
		return j + 1
	}
	return -1
}

// parseLinkDest parses "(dest)" starting at p (points at '('). Returns end after ')'.
func parseLinkDest(text string, p, end int) (destEnd int, ok bool) {
	i := p + 1
	for i < end && (text[i] == ' ' || text[i] == '\t' || text[i] == '\n') {
		i++
	}
	if i >= end {
		return 0, false
	}
	if text[i] == ')' { // empty dest
		return i + 1, true
	}
	if text[i] == '<' {
		gt := strings.IndexByte(text[i+1:end], '>')
		if gt < 0 {
			return 0, false
		}
		i += gt + 2
	} else {
		depth := 0
		for i < end {
			b := text[i]
			if b == '\\' && i+1 < end {
				i += 2
				continue
			}
			if b == ' ' || b == '\t' || b == '\n' {
				break
			}
			if b == '(' {
				depth++
			} else if b == ')' {
				if depth == 0 {
					break
				}
				depth--
			}
			i++
		}
	}
	// optional whitespace + title + closing paren
	for i < end && (text[i] == ' ' || text[i] == '\t' || text[i] == '\n') {
		i++
	}
	if i < end && (text[i] == '"' || text[i] == '\'') {
		q := text[i]
		i++
		for i < end && text[i] != q {
			i++
		}
		if i < end {
			i++
		}
		for i < end && (text[i] == ' ' || text[i] == '\t' || text[i] == '\n') {
			i++
		}
	}
	if i < end && text[i] == ')' {
		return i + 1, true
	}
	return 0, false
}

// linkAt tries to parse a link/image starting at i ('[' or '!'). Returns label
// range and total end offset; dest range returned via hidEnd (hidden syntax).
func linkAt(text string, i, end int) (labStart, labEnd, totalEnd int, ok bool) {
	open := i
	if text[i] == '!' {
		open = i + 1
	}
	// find matching ']' — skip escapes and inline code spans
	depth := 1
	j := open + 1
	for j < end {
		b := text[j]
		if b == '\\' && j+1 < end {
			j += 2
			continue
		}
		if b == '`' {
			k := runLen(text, j, '`')
			if m := findCloseRun(text, j+k, end, k); m >= 0 {
				j = m + k
				continue
			}
			j += k
			continue
		}
		if b == '[' {
			depth++
		} else if b == ']' {
			depth--
			if depth == 0 {
				goto found
			}
		}
		j++
	}
	return 0, 0, 0, false
found:
	close := j
	k := close + 1
	// skip whitespace (incl. one newline) between ] and (
	p := k
	for p < end && (text[p] == ' ' || text[p] == '\t' || text[p] == '\n') {
		p++
	}
	if p < end && text[p] == '(' {
		if de, ok := parseLinkDest(text, p, end); ok {
			return open + 1, close, de, true
		}
	}
	// [label][ref] full reference
	if k < end && text[k] == '[' {
		if r := strings.IndexByte(text[k+1:end], ']'); r >= 0 {
			return open + 1, close, k + 1 + r + 1, true
		}
	}
	// shortcut reference: label is visible, brackets are syntax
	return open + 1, close, close + 1, true
}

func isLinkDefLine(ct string) bool {
	t := strings.TrimLeft(ct, " ")
	if len(t) == 0 || t[0] != '[' {
		return false
	}
	i := strings.IndexByte(t, ']')
	if i < 0 || i+1 >= len(t) || t[i+1] != ':' {
		return false
	}
	return strings.TrimSpace(t[i+2:]) != ""
}

// scanInline segments [start,end) into text, code spans, autolinks and hidden
// link syntax. Escapes prevent interpretation.
func scanInline(text string, start, end int) []Seg {
	var segs []Seg
	textStart := start
	flush := func(to int) {
		if to > textStart {
			segs = append(segs, Seg{Kind: segText, Start: textStart, End: to, CStart: textStart, CEnd: to})
		}
	}
	i := start
	for i < end {
		c := text[i]
		switch {
		case c == '\\' && i+1 < end && isPunct(text[i+1]):
			i += 2
		case c == '`':
			k := runLen(text, i, '`')
			if m := findCloseRun(text, i+k, end, k); m >= 0 {
				flush(i)
				cs, ce := i+k, m
				if ce-cs > 1 && text[cs] == ' ' && text[ce-1] == ' ' &&
					strings.TrimSpace(text[cs:ce]) != "" {
					cs++
					ce--
				}
				segs = append(segs, Seg{Kind: segCode, Start: i, End: m + k, CStart: cs, CEnd: ce})
				i = m + k
				textStart = i
			} else {
				i += k
			}
		case c == '[' || (c == '!' && i+1 < end && text[i+1] == '['):
			ls, le, te, ok := linkAt(text, i, end)
			if !ok {
				i++
				continue
			}
			flush(i)
			hid := i
			if text[i] == '!' {
				hid = i + 2
			} else {
				hid = i + 1
			}
			segs = append(segs, Seg{Kind: segHidden, Start: i, End: hid})
			segs = append(segs, scanInline(text, ls, le)...)
			segs = append(segs, Seg{Kind: segHidden, Start: le, End: te})
			i = te
			textStart = i
		case c == '<':
			if m := autolinkEnd(text, i, end); m >= 0 {
				flush(i)
				segs = append(segs, Seg{Kind: segAuto, Start: i, End: m, CStart: i + 1, CEnd: m - 1})
				i = m
				textStart = i
			} else {
				i++
			}
		default:
			i++
		}
	}
	flush(end)
	return segs
}

// Items splits a text block into paragraph units: each list-marker or heading
// line starts a new unit, continuation lines join it. Inline constructs such
// as code spans cannot cross these boundaries.
func (d *Doc) Items(b Block) []Span {
	var items []Span
	cur := -1
	for i := b.FirstLine; i <= b.LastLine; i++ {
		ln := d.Lines[i]
		if ln.marker != nil || cur < 0 {
			if cur >= 0 {
				items = append(items, Span{d.Lines[cur].start, d.Lines[i-1].end})
			}
			cur = i
		}
	}
	if cur >= 0 {
		items = append(items, Span{d.Lines[cur].start, b.End})
	}
	return items
}

// hiddenIn returns spans inside rng that must not be treated as visible text:
// quote prefixes, list/heading markers and link-definition lines.
func (d *Doc) hiddenIn(rng Span) []Span {
	var hidden []Span
	for _, ln := range d.Lines {
		if ln.end < rng.Start || ln.start >= rng.End {
			continue
		}
		hidden = append(hidden, ln.prefixes...)
		if ln.marker != nil {
			hidden = append(hidden, *ln.marker)
		}
		if isLinkDefLine(contentOf(d.Text, ln)) {
			hidden = append(hidden, Span{ln.start, ln.end})
		}
	}
	return hidden
}

// itemSegments scans one item and removes hidden spans from text segments.
func itemSegments(text string, item Span, hidden []Span) []Seg {
	segs := scanInline(text, item.Start, item.End)
	if len(hidden) == 0 {
		return segs
	}
	var out []Seg
	for _, s := range segs {
		if s.Kind != segText {
			out = append(out, s)
			continue
		}
		cur := s.Start
		for _, h := range hidden {
			if h.End <= cur || h.Start >= s.End {
				continue
			}
			if h.Start > cur {
				out = append(out, Seg{Kind: segText, Start: cur, End: min(h.Start, s.End), CStart: cur, CEnd: min(h.Start, s.End)})
			}
			cur = max(cur, h.End)
		}
		if cur < s.End {
			out = append(out, Seg{Kind: segText, Start: cur, End: s.End, CStart: cur, CEnd: s.End})
		}
	}
	return out
}

// Segments returns inline segments for a text block, excluding quote prefixes,
// list markers, heading markers and link-definition lines.
func (d *Doc) Segments(b Block) []Seg {
	var segs []Seg
	for _, item := range d.Items(b) {
		segs = append(segs, itemSegments(d.Text, item, d.hiddenIn(item))...)
	}
	return segs
}

// --- replacement helpers ---

type replacement struct {
	start, end int
	text       string
}

// splice applies non-overlapping replacements to text.
func splice(text string, reps []replacement) string {
	// sort by start (insertion sort — lists are small)
	for i := 1; i < len(reps); i++ {
		for j := i; j > 0 && reps[j-1].start > reps[j].start; j-- {
			reps[j-1], reps[j] = reps[j], reps[j-1]
		}
	}
	var sb strings.Builder
	sb.Grow(len(text))
	cur := 0
	for _, r := range reps {
		if r.start < cur {
			continue
		}
		sb.WriteString(text[cur:r.start])
		sb.WriteString(r.text)
		cur = r.end
	}
	sb.WriteString(text[cur:])
	return sb.String()
}
