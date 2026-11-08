package main

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Rule struct {
	ID    string
	Apply func(text string) string
}

type Options struct {
	CodeMode string // "describe" (default) or "read" (unwrap every block verbatim)
}

type Result struct {
	Text    string
	Applied bool
	Changes []string
	Skipped []string
}

func Rules(opts Options) []Rule {
	return []Rule{
		{"code-blocks", ruleCodeBlocks(opts)},
		{"formatting", ruleFormatting},
		{"markdown-tables", ruleMarkdownTables},
		{"urls", ruleURLs},
		{"migration-files", ruleMigrationFiles},
		{"hashes", ruleHashes},
		{"file-paths", ruleFilePaths},
		{"separators", ruleSeparators},
		{"blockquotes", ruleBlockquotes},
		{"unwrap", ruleUnwrap},
	}
}

// Clean runs the ordered replacer pipeline. A panicking rule is skipped and
// its input text is kept, matching the original engine's failure semantics.
func Clean(text string, opts Options) Result {
	res := Result{Text: text}
	for _, r := range Rules(opts) {
		before := res.Text
		out, failed := func() (s string, failed bool) {
			defer func() {
				if recover() != nil {
					s = before
					failed = true
				}
			}()
			return r.Apply(before), false
		}()
		if failed {
			res.Skipped = append(res.Skipped, r.ID)
			continue
		}
		if out != before {
			res.Changes = append(res.Changes, r.ID)
		}
		res.Text = out
	}
	res.Applied = res.Text != text
	return res
}

// --- formatting: decoration removal for standalone speech output ---

var (
	reEmphRun       = regexp.MustCompile(`\*+|~~|__+`)
	reUnderscore    = regexp.MustCompile(`(^|[^\p{L}\p{N}])_([^\s_][^_]*)_`)
	reEscPunct      = regexp.MustCompile(`\\([!$%&'()*+,-./:;=?@<>\[\]{|}^"\\\x60])`)
	reThematicChars = regexp.MustCompile(`^[-*_ \t]+$`)
	reDigitTimes    = regexp.MustCompile(`(\d) \* (\d)`)
	reBulletOnly    = regexp.MustCompile(`^[-*+]\s`)
	reNumMarker     = regexp.MustCompile(`^\d+[.)]\s`)
)

func isEmoji(r rune) bool {
	return r >= 0x1F000 && r <= 0x1FAFF ||
		r >= 0x2600 && r <= 0x26FF ||
		r >= 0x2700 && r <= 0x27BF ||
		r >= 0x2B00 && r <= 0x2BFF ||
		r >= 0xFE00 && r <= 0xFE0F ||
		r == 0x200D || r == 0x20E3
}

// isThematicBreak reports whether s is a horizontal-rule line like "---".
func isThematicBreak(s string) bool {
	if !reThematicChars.MatchString(s) {
		return false
	}
	var c byte
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			continue
		}
		if c == 0 {
			c = s[i]
		} else if s[i] != c {
			return false
		}
		n++
	}
	return n >= 3
}

func stripEmoji(s string) string {
	if !strings.ContainsRune(s, rune(0x200D)) {
		quick := false
		for _, r := range s {
			if isEmoji(r) {
				quick = true
				break
			}
		}
		if !quick {
			return s
		}
	}
	return strings.Map(func(r rune) rune {
		if isEmoji(r) {
			return -1
		}
		return r
	}, s)
}

func stripEmphasis(s string) string {
	// runs of *, ~~, __ are emphasis — unless backslash-escaped
	var b strings.Builder
	cur := 0
	for _, m := range reEmphRun.FindAllStringIndex(s, -1) {
		b.WriteString(s[cur:m[0]])
		escaped := false
		for i := m[0] - 1; i >= 0 && s[i] == '\\'; i-- {
			escaped = !escaped
		}
		if escaped {
			b.WriteString(s[m[0]:m[1]])
		}
		cur = m[1]
	}
	b.WriteString(s[cur:])
	return reUnderscore.ReplaceAllString(b.String(), "$1$2")
}

// ruleFormatting removes decoration Claude's own cleanup handled downstream:
// emphasis, headings, list bullets, thematic breaks, emoji, escapes.
func ruleFormatting(text string) string {
	d := Scan(text)
	var reps []replacement
	var lines []mdLine
	for _, ln := range d.Lines {
		lines = append(lines, ln)
	}
	// thematic breaks: drop the whole content portion of the line
	for _, b := range d.Blocks {
		if b.Kind != blkText {
			continue
		}
		for _, ln := range d.Lines {
			if ln.end < b.Start || ln.start >= b.End {
				continue
			}
			ct := contentOf(text, ln)
			lead := leadSpaces(ct)
			if isThematicBreak(ct[lead:]) {
				reps = append(reps, replacement{ln.contentStart + lead, ln.end, ""})
			}
		}
	}
	for _, b := range d.Blocks {
		if b.Kind != blkText {
			continue
		}
		for _, s := range d.Segments(b) {
			if s.Kind != segText {
				continue
			}
			seg := s.sliceText(text)
			out := seg
			out = reDigitTimes.ReplaceAllString(out, "$1 times $2")
			out = stripEmphasis(out)
			out = stripEmoji(out)
			// unescape punctuation, except a structural char at line start
			out = reEscPunct.ReplaceAllStringFunc(out, func(m string) string {
				idx := strings.Index(seg, m)
				abs := s.Start + idx
				atLineStart := false
				for _, ln := range lines {
					if ln.contentStart == abs {
						atLineStart = true
						break
					}
				}
				if atLineStart {
					return m
				}
				return m[1:]
			})
			if out != seg {
				reps = append(reps, replacement{s.Start, s.End, out})
			}
		}
		// heading and bullet markers
		for _, ln := range d.Lines {
			if ln.end < b.Start || ln.start >= b.End || ln.marker == nil {
				continue
			}
			m := ln.marker.slice(text)
			switch {
			case strings.HasPrefix(strings.TrimLeft(m, " "), "#"):
				reps = append(reps, replacement{ln.marker.Start, ln.marker.End, ""})
			case reBulletOnly.MatchString(m):
				reps = append(reps, replacement{ln.marker.Start, ln.marker.End, ""})
			case reNumMarker.MatchString(m):
				// keep numbers — they read naturally
			}
		}
	}
	if len(reps) == 0 {
		return text
	}
	return splice(text, reps)
}

func (s Seg) sliceText(text string) string {
	if s.Kind == segCode || s.Kind == segAuto {
		return text[s.CStart:s.CEnd]
	}
	return text[s.Start:s.End]
}

// --- code blocks ---

func ruleCodeBlocks(opts Options) func(string) string {
	return func(text string) string {
		d := Scan(text)
		var reps []replacement
		for _, b := range d.Blocks {
			if b.Kind != blkFenced && b.Kind != blkIndented {
				continue
			}
			content := b.Content(text)
			if strings.TrimSpace(content) == "" {
				continue
			}
			read := opts.CodeMode == "read" || !classifyBlock(b.Info, content)
			var out string
			if read {
				out = readableBlock(content, 0)
			} else {
				out = describeCode(b.Info, content)
			}
			reps = append(reps, replacement{b.Start, b.End, out})
		}
		if len(reps) == 0 {
			return text
		}
		return splice(text, reps)
	}
}

// --- markdown tables ---

func ruleMarkdownTables(text string) string {
	if !strings.Contains(text, "|") || !strings.Contains(text, "-") {
		return text
	}
	d := Scan(text)
	var reps []replacement
	for _, b := range d.Blocks {
		if b.Kind == blkTable {
			reps = append(reps, replacement{b.Start, b.End, narrateTable(b, text)})
		}
	}
	if len(reps) == 0 {
		return text
	}
	return splice(text, reps)
}

// --- urls ---

var reURL = regexp.MustCompile(`(?:https?://|www\.)[^\s<>"'` + "`" + `\[\]{}]+`)

func spokenURL(value string) (string, bool) {
	if !regexp.MustCompile(`^(?i:https?://|www\.)\S+$`).MatchString(value) {
		return "", false
	}
	addr := value
	if strings.HasPrefix(strings.ToLower(addr), "www.") {
		addr = "https://" + addr
	}
	u, err := url.Parse(addr)
	if err != nil || u.Hostname() == "" {
		return "", false
	}
	if u.Hostname() == "github.com" &&
		regexp.MustCompile(`^/user-attachments/assets/[^/]+/?$`).MatchString(u.Path) {
		return "GitHub attachment", true
	}
	return "link to " + u.Hostname(), true
}

// urlBoundary replicates the original lookbehind: a URL must not be preceded
// by a letter, digit, _, @ or /.
func urlBoundary(text string, start int) bool {
	if start == 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(text[:start])
	return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '@' || r == '/')
}

var reTrailPunct = regexp.MustCompile(`[.,;:!?]+$`)

func shortenURLsIn(source string, base int) []replacement {
	var reps []replacement
	for _, m := range reURL.FindAllStringIndex(source, -1) {
		if !urlBoundary(source, m[0]) {
			continue
		}
		match := source[m[0]:m[1]]
		addr := reTrailPunct.ReplaceAllString(match, "")
		for strings.HasSuffix(addr, ")") && strings.Count(addr, ")") > strings.Count(addr, "(") {
			addr = addr[:len(addr)-1]
		}
		if label, ok := spokenURL(addr); ok {
			reps = append(reps, replacement{base + m[0], base + m[0] + len(addr), label})
		}
	}
	return reps
}

func ruleURLs(text string) string {
	if !strings.Contains(text, "//") && !strings.Contains(text, "www.") {
		return text
	}
	d := Scan(text)
	var reps []replacement
	for _, b := range d.Blocks {
		if b.Kind != blkText {
			continue
		}
		for _, s := range d.Segments(b) {
			switch s.Kind {
			case segAuto:
				if label, ok := spokenURL(text[s.CStart:s.CEnd]); ok {
					reps = append(reps, replacement{s.Start, s.End, label})
				}
			case segCode:
				if label, ok := spokenURL(text[s.CStart:s.CEnd]); ok {
					reps = append(reps, replacement{s.CStart, s.CEnd, label})
				}
			case segText:
				reps = append(reps, shortenURLsIn(text[s.Start:s.End], s.Start)...)
			}
		}
	}
	if len(reps) == 0 {
		return text
	}
	return splice(text, reps)
}

// --- visible candidates (hashes, migration files) ---

type candidate struct {
	start, end int
	value      string
	visOff     int // offset of the match inside the paragraph's visible text
	visText    string
}

var reMaskURL = regexp.MustCompile(`\b(?:[a-zA-Z][a-zA-Z\d+.-]*://|www\.)\S+`)

func maskURLs(s string) string {
	return reMaskURL.ReplaceAllStringFunc(s, func(m string) string {
		return strings.Repeat(" ", len(m))
	})
}

// collectCandidates finds pattern matches in visible paragraph text (including
// inline code and link labels) with a 32-candidate cap, mirroring
// visibleCandidates.
func collectCandidates(text string, d *Doc, re *regexp.Regexp,
	badBefore, badAfter func(string, int) bool) []candidate {
	var cands []candidate
	for _, b := range d.Blocks {
		if b.Kind != blkText {
			continue
		}
		var visible strings.Builder
		var pending []struct {
			start, end int
			value      string
			off        int
		}
		for _, s := range d.Segments(b) {
			if s.Kind != segText && s.Kind != segCode {
				continue
			}
			src := maskURLs(s.sliceText(text))
			for _, m := range re.FindAllStringIndex(src, -1) {
				if len(cands)+len(pending) >= 32 {
					break
				}
				abs := s.CStart + m[0]
				// boundary checks run against the real text, not the mask
				if badBefore(text, abs) || badAfter(text, s.CStart+m[1]) {
					continue
				}
				pending = append(pending, struct {
					start, end int
					value      string
					off        int
				}{abs, abs + m[1] - m[0], text[abs : abs+m[1]-m[0]], visible.Len() + m[0]})
			}
			visible.WriteString(src)
			visible.WriteString(" ")
		}
		vis := visible.String()
		for _, p := range pending {
			cands = append(cands, candidate{p.start, p.end, p.value, p.off, vis})
		}
	}
	return cands
}

func badBeforeWord(text string, s int) bool {
	if s == 0 {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(text[:s])
	return unicode.IsLetter(r) || unicode.IsDigit(r) ||
		strings.ContainsRune("_/@#.-", r)
}

func badAfterWord(text string, e int) bool {
	if e >= len(text) {
		return false
	}
	r, n := utf8.DecodeRuneInString(text[e:])
	if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_/-", r) {
		return true
	}
	if r == '.' && e+n < len(text) {
		r2, _ := utf8.DecodeRuneInString(text[e+n:])
		return unicode.IsLetter(r2) || unicode.IsDigit(r2)
	}
	return false
}

var reHashCand = regexp.MustCompile(`[a-fA-F0-9]{7,64}`)
var reMigrationCand = regexp.MustCompile(`\d{10}_[a-fA-F0-9]{7,64}\.py`)

var hashKeywords = regexp.MustCompile(
	`(?i)\b(commits?|hash(es|ed|ing)?|sha-?1|sha-?256|md5|checksum|revision|changeset|` +
		`digest|fingerprint|merg(ed|e|ing)|migration|alembic|rebase[sd]?|deploy(ed|s|ing)?|` +
		`tag(ged)?|release[sd]?|version|signed|signature|hmac|blake|sha3|keccak|git)\b`)

var hashVeto = regexp.MustCompile(
	`(?i)\b(colou?r|invoice|order|phone|amount|price|total|quantity|count|serial|sku|port)\b`)

var migrationKeywords = regexp.MustCompile(
	`(?i)\b(migration|alembic|revision|schema|upgrade|downgrade|database|migrate)\b`)

// hasHashShape requires a digit+letter mix, or all-caps hex letters: pure
// numbers are usually amounts or IDs, and lowercase pure-letter strings are
// usually words or colours. Screaming hex like DEADBEEF is a hash convention.
func hasHashShape(v string) bool {
	var digit, letter bool
	for _, r := range v {
		switch {
		case r >= '0' && r <= '9':
			digit = true
		case (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F'):
			letter = true
		default:
			return false
		}
	}
	if digit && letter {
		return true
	}
	return letter && v == strings.ToUpper(v)
}

// candidateContext returns the ±160-char context window like the original.
func candidateContext(c candidate) string {
	start := c.visOff - 160
	if start < 0 {
		start = 0
	}
	end := c.visOff + len(c.value) + 160
	if end > len(c.visText) {
		end = len(c.visText)
	}
	return strings.Join(strings.Fields(c.visText[start:end]), " ")
}

// vetoBefore checks ~45 visible chars before the candidate for veto words.
func vetoBefore(c candidate) bool {
	start := c.visOff - 45
	if start < 0 {
		start = 0
	}
	return hashVeto.MatchString(c.visText[start:c.visOff])
}

func ruleHashes(text string) string {
	if !reHashCand.MatchString(text) {
		return text
	}
	d := Scan(text)
	cands := collectCandidates(text, d, reHashCand, badBeforeWord, badAfterWord)
	var reps []replacement
	for _, c := range cands {
		if !hasHashShape(c.value) {
			continue
		}
		ctx := candidateContext(c)
		if !hashKeywords.MatchString(ctx) || vetoBefore(c) {
			continue
		}
		reps = append(reps, replacement{c.start, c.end,
			"hash ending " + c.value[len(c.value)-4:]})
	}
	if len(reps) == 0 {
		return text
	}
	return splice(text, reps)
}

func ruleMigrationFiles(text string) string {
	if !reMigrationCand.MatchString(text) {
		return text
	}
	d := Scan(text)
	cands := collectCandidates(text, d, reMigrationCand, badBeforeWord, badAfterWord)
	var reps []replacement
	for _, c := range cands {
		if !migrationKeywords.MatchString(candidateContext(c)) {
			continue
		}
		rev := c.value[:len(c.value)-len(".py")]
		reps = append(reps, replacement{c.start, c.end,
			"migration ending " + rev[len(rev)-4:]})
	}
	if len(reps) == 0 {
		return text
	}
	return splice(text, reps)
}

// --- file paths ---

var (
	rePathChars = regexp.MustCompile(`^(?:~/)?[\p{L}\p{N}_./][\p{L}\p{N}_./-]*$`)
	reHasExt    = regexp.MustCompile(`\.[\p{L}][\p{L}\p{N}]*$`)
	rePathPfx   = regexp.MustCompile(`^(?:\.{1,2}/|~/|/)|/$`)
	reDomainPfx = regexp.MustCompile(`^[^/]*\p{L}\.[\p{L}\p{N}-]+/`)
	reLetter    = regexp.MustCompile(`\p{L}`)
	reToken     = regexp.MustCompile(`[^\s()\[\]{}<>"'` + "`" + `]+`)
	reFileRef   = regexp.MustCompile(`^(?:~/)?[\p{L}\p{N}_./][\p{L}\p{N}_./-]*$`)
	reIdent     = regexp.MustCompile(`^[\p{L}_][\p{L}\p{N}_]*_[\p{L}\p{N}_]+(?:\(\))?$`)
	reSepRuns   = regexp.MustCompile(`[-_]+`)
	reSlashRuns = regexp.MustCompile(`[/_-]+`)
	reIDSeg     = regexp.MustCompile(`(?i)(^|_)id(_|\(\)$|$)`)
	reUnders    = regexp.MustCompile(`_+`)
	reTrailTok  = regexp.MustCompile(`[.,;:!?]+$`)
)

func isPath(value string, inline bool) bool {
	if !rePathChars.MatchString(value) || !strings.Contains(value, "/") ||
		!reLetter.MatchString(value) {
		return false
	}
	if inline && rePathPfx.MatchString(value) {
		return true
	}
	return reHasExt.MatchString(value) && !reDomainPfx.MatchString(value)
}

func speakPath(value string) string {
	return strings.TrimSpace(reSlashRuns.ReplaceAllString(value, " "))
}

func isFileReference(value string) bool {
	return reFileRef.MatchString(value) && reLetter.MatchString(value) &&
		(strings.Contains(value, "/") || reHasExt.MatchString(value))
}

func isIdentifier(value string) bool { return reIdent.MatchString(value) }

func ruleFilePaths(text string) string {
	if !strings.Contains(text, "/") {
		return text
	}
	d := Scan(text)
	var reps []replacement
	for _, b := range d.Blocks {
		if b.Kind != blkText {
			continue
		}
		for _, s := range d.Segments(b) {
			switch s.Kind {
			case segCode:
				v := text[s.CStart:s.CEnd]
				if isPath(v, true) {
					reps = append(reps, replacement{s.CStart, s.CEnd, speakPath(v)})
				}
			case segText:
				src := s.sliceText(text)
				var inner []replacement
				for _, m := range reToken.FindAllStringIndex(src, -1) {
					tok := src[m[0]:m[1]]
					path := reTrailTok.ReplaceAllString(tok, "")
					if isPath(path, false) {
						inner = append(inner, replacement{s.Start + m[0], s.Start + m[0] + len(path), speakPath(path)})
					}
				}
				reps = append(reps, inner...)
			}
		}
	}
	if len(reps) == 0 {
		return text
	}
	return splice(text, reps)
}

// --- separators ---

func ruleSeparators(text string) string {
	if !strings.Contains(text, "`") || !(strings.Contains(text, "-") || strings.Contains(text, "_")) {
		return text
	}
	d := Scan(text)
	var reps []replacement
	for _, b := range d.Blocks {
		if b.Kind != blkText {
			continue
		}
		for _, s := range d.Segments(b) {
			if s.Kind != segCode {
				continue
			}
			v := text[s.CStart:s.CEnd]
			if isIdentifier(v) {
				n := reIDSeg.ReplaceAllString(v, "${1}I D${2}")
				n = reUnders.ReplaceAllString(n, " ")
				if n != v {
					reps = append(reps, replacement{s.CStart, s.CEnd, n})
				}
			} else if isFileReference(v) {
				n := reSepRuns.ReplaceAllString(v, " ")
				if n != v {
					reps = append(reps, replacement{s.CStart, s.CEnd, n})
				}
			}
		}
	}
	if len(reps) == 0 {
		return text
	}
	return splice(text, reps)
}

// --- blockquotes ---

func ruleBlockquotes(text string) string {
	if !strings.Contains(text, ">") {
		return text
	}
	d := Scan(text)
	var reps []replacement
	for _, m := range d.QuoteMarks {
		reps = append(reps, replacement{m.Start, m.End, ""})
	}
	if len(reps) == 0 {
		return text
	}
	return splice(text, reps)
}

// --- unwrap: no downstream cleanup, so drop residual markdown syntax ---
// Removes link brackets/destinations and reference definitions, unwraps
// non-URL autolinks, and strips inline-code backticks.

func ruleUnwrap(text string) string {
	d := Scan(text)
	var reps []replacement
	for _, ln := range d.Lines {
		if isLinkDefLine(contentOf(text, ln)) {
			reps = append(reps, replacement{ln.start, ln.end, ""})
		}
	}
	for _, b := range d.Blocks {
		if b.Kind != blkText {
			continue
		}
		for _, s := range d.Segments(b) {
			switch s.Kind {
			case segCode:
				reps = append(reps, replacement{s.Start, s.End, text[s.CStart:s.CEnd]})
			case segAuto:
				inner := text[s.CStart:s.CEnd]
				if scheme := strings.IndexByte(inner, ':'); scheme > 0 &&
					strings.EqualFold(inner[:scheme], "mailto") {
					inner = inner[scheme+1:]
				}
				reps = append(reps, replacement{s.Start, s.End, inner})
			case segHidden:
				reps = append(reps, replacement{s.Start, s.End, ""})
			}
		}
	}
	if len(reps) == 0 {
		return text
	}
	return splice(text, reps)
}
