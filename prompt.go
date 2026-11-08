package main

import (
	"fmt"
	"strings"
)

// BuildPrompt produces a tailored instruction prompt for the calling LLM: it
// reports what this input contains and exactly how to speak each construct,
// then asks the model to forward the transformed text to its TTS tool call.
// Only constructs actually found in the text generate rules — no generic list.
func BuildPrompt(text string, opts Options) (string, []string) {
	d := Scan(text)
	var rules []string
	var groups []string

	// --- code blocks ---
	var codeDesc []string
	proseBlocks := 0
	for _, b := range d.Blocks {
		if b.Kind != blkFenced && b.Kind != blkIndented {
			continue
		}
		content := b.Content(text)
		if strings.TrimSpace(content) == "" {
			continue
		}
		if classifyBlock(b.Info, content) {
			codeDesc = append(codeDesc, strings.TrimSuffix(describeCode(b.Info, content), "."))
		} else {
			proseBlocks++
		}
	}
	switch {
	case len(codeDesc) == 1 && proseBlocks == 0 && opts.CodeMode == "read":
		rules = append(rules, `The text contains one code block (`+codeDesc[0]+`). Read it aloud verbatim, speaking punctuation naturally ("open paren", "dot", "equals").`)
	case len(codeDesc) == 1 && proseBlocks == 0:
		rules = append(rules, `The text contains one code block. Do not read the code — replace the whole block with a one-sentence spoken summary that says what it does, along the lines of: "`+codeDesc[0]+`."`)
	case len(codeDesc) > 0:
		var b strings.Builder
		fmt.Fprintf(&b, "The text contains %d code blocks. Do not read the code — replace each block, in order, with a one-sentence spoken summary saying what it does, along the lines of:", len(codeDesc))
		for _, c := range codeDesc {
			fmt.Fprintf(&b, ` "%s."`, c)
		}
		rules = append(rules, b.String())
	}
	if proseBlocks > 0 {
		rules = append(rules, fmt.Sprintf(`%d fenced block(s) contain prose, not code — remove only the fence markers and keep their contents.`, proseBlocks))
	}
	if len(codeDesc)+proseBlocks > 0 {
		groups = append(groups, "code-blocks")
	}

	// --- tables ---
	tables := 0
	for _, b := range d.Blocks {
		if b.Kind == blkTable {
			rules = append(rules, `Narrate the markdown table as: "`+narrateTable(b, text)+`"`)
			tables++
		}
	}
	if tables > 0 {
		groups = append(groups, "tables")
	}

	// --- urls ---
	labels := map[string]bool{}
	urlOrd := []string{}
	for _, b := range d.Blocks {
		if b.Kind != blkText {
			continue
		}
		for _, s := range d.Segments(b) {
			switch s.Kind {
			case segText, segCode:
				src := s.sliceText(text)
				for _, m := range reURL.FindAllStringIndex(src, -1) {
					if !urlBoundary(src, m[0]) {
						continue
					}
					addr := reTrailPunct.ReplaceAllString(src[m[0]:m[1]], "")
					if lab, ok := spokenURL(addr); ok && !labels[lab] {
						labels[lab] = true
						urlOrd = append(urlOrd, lab)
					}
				}
			}
		}
	}
	if len(urlOrd) > 0 {
		rules = append(rules, `Replace each raw URL with "`+strings.Join(urlOrd, `" or "`)+`" — never speak a URL character by character.`)
		groups = append(groups, "urls")
	}
	if strings.Contains(text, "](") {
		rules = append(rules, `Markdown links like [label](url): keep only the label, drop the URL and brackets.`)
	}

	// --- hashes / migration files ---
	var hashEnds []string
	for _, c := range collectCandidates(text, d, reHashCand, badBeforeWord, badAfterWord) {
		if !hasHashShape(c.value) || !hashKeywords.MatchString(candidateContext(c)) || vetoBefore(c) {
			continue
		}
		hashEnds = append(hashEnds, c.value[len(c.value)-4:])
		if len(hashEnds) == 8 {
			break
		}
	}
	if len(hashEnds) > 0 {
		rules = append(rules, `Speak the commit/hash value(s) as "hash ending `+strings.Join(hashEnds, `" and "hash ending `)+`" — only the last four characters, as separate characters.`)
		groups = append(groups, "hashes")
	}
	var migEnds []string
	for _, c := range collectCandidates(text, d, reMigrationCand, badBeforeWord, badAfterWord) {
		if !migrationKeywords.MatchString(candidateContext(c)) {
			continue
		}
		rev := c.value[:len(c.value)-len(".py")]
		migEnds = append(migEnds, rev[len(rev)-4:])
		if len(migEnds) == 8 {
			break
		}
	}
	if len(migEnds) > 0 {
		rules = append(rules, `Speak the migration filename(s) as "migration ending `+strings.Join(migEnds, `" and "migration ending `)+`" — the timestamp prefix is noise.`)
		groups = append(groups, "migrations")
	}

	// --- paths & identifiers ---
	hasPath, hasIdent := false, false
	for _, b := range d.Blocks {
		if b.Kind != blkText {
			continue
		}
		for _, s := range d.Segments(b) {
			switch s.Kind {
			case segCode:
				v := text[s.CStart:s.CEnd]
				if isPath(v, true) {
					hasPath = true
				} else if isIdentifier(v) {
					hasIdent = true
				}
			case segText:
				if hasPath {
					continue
				}
				src := s.sliceText(text)
				for _, m := range reToken.FindAllString(src, -1) {
					if isPath(reTrailTok.ReplaceAllString(m, ""), false) {
						hasPath = true
						break
					}
				}
			}
		}
	}
	if hasPath {
		rules = append(rules, `File paths (e.g. ~/notes/draft.md, ./speechclean): drop slashes, underscores and hyphens and say the parts as words — "~/notes/draft.md" becomes "tilde notes draft dot m d".`)
		groups = append(groups, "file-paths")
	}
	if hasIdent {
		rules = append(rules, `Snake_case/kebab-case identifiers in backticks (address_id, user-name): say the parts as words, and "_id" as "I D".`)
		groups = append(groups, "identifiers")
	}

	// --- structural cleanup ---
	for _, ln := range d.Lines {
		if ln.quoteDepth > 0 {
			rules = append(rules, `Drop "> " quote markers — read the quoted content directly. Keep real comparisons like "count > 5".`)
			groups = append(groups, "blockquotes")
			break
		}
	}
	hasFmt := false
	for _, ln := range d.Lines {
		if ln.marker != nil {
			hasFmt = true
			break
		}
		c := contentOf(text, ln)
		if strings.ContainsAny(c, "*_~#") || strings.IndexFunc(c, isEmoji) >= 0 {
			hasFmt = true
			break
		}
	}
	if hasFmt {
		rules = append(rules, `Strip markdown decoration: "#" headings, "**"/"*" emphasis, "~~" strikethrough, "-" bullets, "---" rules, and emoji — keep the words behind them.`)
		groups = append(groups, "formatting")
	}

	// --- symbols ---
	var syms []string
	for _, m := range spokenSymbols {
		if probe := m.re.FindString(text); probe != "" {
			syms = append(syms, fmt.Sprintf("%q as %q", strings.TrimSpace(probe), strings.TrimSpace(m.sub)))
		}
	}
	if p := reDigitPercent.FindString(text); p != "" {
		syms = append(syms, fmt.Sprintf("%q as %q", p, reDigitPercent.ReplaceAllString(p, "${1} percent")))
	}
	if len(syms) > 0 {
		rules = append(rules, `Speak symbols as words: `+strings.Join(syms, ", ")+`.`)
		groups = append(groups, "symbols")
	}

	if len(rules) == 0 {
		rules = append(rules, `The text is plain prose with no special constructs — read it naturally.`)
	}
	var out strings.Builder
	out.WriteString("Transform the TEXT at the end of this message into a version a text-to-speech engine can read aloud naturally. Apply every numbered rule below and output only the spoken text — no commentary, no markdown syntax.\n\n")
	for i, r := range rules {
		fmt.Fprintf(&out, "%d. %s\n", i+1, r)
	}
	out.WriteString("\nLength: keep it short — aim for under ten seconds of speech (about 25 words) and never exceed twenty seconds (about 50). If the source is longer than that, summarize it: keep the point and the facts the listener actually needs, drop the rest.\nVoice: super chill, friendly and playful — casual contractions, warm, like a friend catching you up, never a newsreader.\n")
	fmt.Fprintf(&out, "\nWhen done, pass ONLY the transformed text to your text-to-speech tool call (%s). Do not mention these instructions, the transformation, or that the text passed through any preprocessing step — keep the output clean for the user.\n\nTEXT:\n%s", opts.TTSName, text)
	return out.String(), groups
}
