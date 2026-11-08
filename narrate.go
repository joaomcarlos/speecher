package main

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var langNames = map[string]string{
	"ts": "TypeScript", "typescript": "TypeScript", "tsx": "TypeScript", "mts": "TypeScript", "cts": "TypeScript",
	"js": "JavaScript", "javascript": "JavaScript", "jsx": "JavaScript", "mjs": "JavaScript",
	"py": "Python", "python": "Python", "pyw": "Python",
	"go": "Go", "golang": "Go",
	"rs": "Rust", "rust": "Rust",
	"rb": "Ruby", "ruby": "Ruby",
	"java": "Java", "kt": "Kotlin", "kotlin": "Kotlin", "scala": "Scala",
	"c": "C", "h": "C", "cpp": "C++", "c++": "C++", "cc": "C++", "cxx": "C++", "hpp": "C++",
	"cs": "C sharp", "csharp": "C sharp", "fsharp": "F sharp",
	"php": "PHP", "pl": "Perl", "perl": "Perl",
	"sh": "", "bash": "", "zsh": "", "shell": "", "console": "", "fish": "",
	"sql": "SQL", "json": "JSON", "jsonl": "JSON", "yaml": "YAML", "yml": "YAML",
	"toml": "TOML", "xml": "XML", "html": "HTML", "css": "CSS", "scss": "SCSS",
	"lua": "Lua", "vim": "Vim script", "hs": "Haskell", "haskell": "Haskell",
	"ex": "Elixir", "exs": "Elixir", "elixir": "Elixir", "erl": "Erlang",
	"swift": "Swift", "dart": "Dart", "zig": "Zig", "nix": "Nix",
	"r": "R", "ps1": "PowerShell", "powershell": "PowerShell",
	"dockerfile": "Dockerfile", "makefile": "Makefile", "tf": "Terraform",
	"terraform": "Terraform", "hcl": "HCL", "proto": "protobuf", "graphql": "GraphQL",
	"diff": "diff", "patch": "diff", "ini": "config", "cfg": "config", "conf": "config",
	"csv": "CSV", "groovy": "Groovy", "vue": "Vue", "svelte": "Svelte",
	"vbnet": "VB net", "vb": "Visual Basic", "asm": "assembly", "wasm": "WebAssembly",
}

// proseTags mark fence info strings that usually wrap prose, not code.
var proseTags = map[string]bool{
	"text": true, "txt": true, "md": true, "markdown": true,
	"plain": true, "plaintext": true, "prose": true, "asciidoc": true,
	"adoc": true, "rst": true, "tex": true,
}

func infoTag(info string) string {
	f := strings.Fields(info)
	if len(f) == 0 {
		return ""
	}
	return strings.ToLower(f[0])
}

func langName(info string) (name string, known bool) {
	tag := infoTag(info)
	if tag == "" {
		return "", false
	}
	n, ok := langNames[tag]
	return n, ok
}

var (
	// strong code signals: statement terminators, operators, structural markers
	reCodeStrong = regexp.MustCompile(
		`[;{}]\s*$|^\s*[{}]|=>|->|==|!=|<=|>=|&&|\|\||\+\+|--|\+=|-=|\*=|/=|<<=|>>=|::|` +
			`^#!|^#include|^<\?php|^<\?xml|</?[a-zA-Z][^>]*>`)
	// keyword-level signals (weight 1)
	reCodeWord = regexp.MustCompile(
		`(?i)\b(func|function|def|class|struct|enum|interface|impl|trait|fn|let|const|var|` +
			`import|export|package|use|pub|return|if|else|elif|for|while|switch|match|case|` +
			`default|try|catch|except|finally|throw|raise|new|typeof|instanceof|async|await|` +
			`yield|print|println|printf|echo|exit|break|continue|null|nil|none|void|int|` +
			`float|double|long|short|unsigned|static|public|private|protected|final|abstract|` +
			`override|virtual|extends|implements|namespace|template|require|include|defer|` +
			`select|insert|update|delete|from|where|join|create|drop|alter|begin|commit|` +
			`rollback|foreach|lambda|self|this|super)\b`)
	reShellCmd = regexp.MustCompile(
		`^\s*(\$\s+|\$ |sudo\s|apt(-get)?\s|apt-get\s|brew\s|pacman\s|yay\s|dnf\s|yum\s|` +
			`npm\s|npx\s|yarn\s|pnpm\s|bun\s|deno\s|pip3?\s|python3?\s|node\s|go\s+(run|build|test|mod|get)|` +
			`cargo\s|git\s|gh\s|cd\s|ls\s|ll\s|cat\s|grep\s|rg\s|find\s|curl\s|wget\s|chmod\s|` +
			`chown\s|mkdir\s|mv\s|cp\s|rm\s|touch\s|docker\s|kubectl\s|helm\s|systemctl\s|` +
			`make\s|cmake\s|tar\s|zip\s|unzip\s|ssh\s|scp\s|rsync\s|ps\s|kill\s|top\s|htop\s|` +
			`head\s|tail\s|less\s|man\s|which\s|whereis\s|export\s|source\s|alias\s|set\s)`)
	reAssign  = regexp.MustCompile(`^[\p{L}\p{N}_.\[\]'"]+\s*=([^=<>!]|$)`)
	reCallSig = regexp.MustCompile(`^\s*[A-Za-z_][\w.]*\s*\(.*\)\s*;?\s*$`) // call-shaped line
	reDataKV  = regexp.MustCompile(`^\s*"?[\p{L}\p{N}_-]+"?\s*[:=]\s*\S`)   // json/yaml key: value
	reComment = regexp.MustCompile(`^\s*(//|#|/\*|<!--|;+)\s*\S`)
	// prose signals: sentence-like lines
	reProseEnd  = regexp.MustCompile(`[.!?][)"']*\s*$`)
	reListItem  = regexp.MustCompile(`^\s{0,3}([-*+]|\d{1,9}[.)])\s+\S`)
	reWordyLine = regexp.MustCompile(`^\s*[\p{L}]`)
)

// wordCount counts tokens made mostly of letters.
func wordCount(s string) int {
	n := 0
	for _, w := range strings.Fields(s) {
		letters := 0
		for _, r := range w {
			if unicode.IsLetter(r) {
				letters++
			}
		}
		if letters >= 2 {
			n++
		}
	}
	return n
}

// classifyBlock decides whether fenced/indented content is code/data to
// describe or natural-language prose to read verbatim. Deterministic scoring
// replaces the original Jev classification; fence language tags are hints only.
func classifyBlock(info, content string) bool /* isCode */ {
	codeScore, proseScore := 0, 0
	for _, ln := range strings.Split(content, "\n") {
		t := strings.TrimSpace(ln)
		if t == "" {
			continue
		}
		code := 0
		if reCodeStrong.MatchString(t) {
			code += 2
		}
		if reCodeWord.MatchString(t) {
			code++
			if len(reCodeWord.FindAllString(t, -1)) > 1 {
				code++
			}
		}
		if reShellCmd.MatchString(t) {
			code += 2
		}
		if reCallSig.MatchString(t) {
			code += 2
		}
		if reAssign.MatchString(t) {
			code++
		}
		if reDataKV.MatchString(t) {
			code++
		}
		if reComment.MatchString(t) {
			code++
		}
		prose := 0
		if reProseEnd.MatchString(t) {
			prose++
		}
		if reListItem.MatchString(t) {
			prose++
		}
		if wordCount(t) >= 5 {
			prose++
		}
		codeScore += code
		proseScore += prose
	}
	tag := infoTag(info)
	if proseTags[tag] {
		proseScore += 2
	} else if _, ok := langNames[tag]; ok && tag != "" {
		codeScore++ // a language label is a hint, never proof
	}
	return codeScore >= 3 && codeScore > proseScore
}

var (
	reDefName = regexp.MustCompile(
		`(?m)^\s*(?:export\s+|public\s+|private\s+|protected\s+|static\s+|async\s+|pub\s+|` +
			`open\s+|internal\s+|abstract\s+|final\s+|data\s+|sealed\s+)*` +
			`(?:function|def|fn|func|class|struct|interface|enum|trait|impl|type|module|` +
			`namespace|record|object|protocol|extension)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	reArrowName = regexp.MustCompile(
		`(?m)^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)` +
			`\s*=\s*(?:async\s+)?(?:\(|function|[A-Za-z_$][A-Za-z0-9_$]*\s*=>)`)
	reShellFn = regexp.MustCompile(`(?m)^\s*([A-Za-z_][A-Za-z0-9_]*)\s*\(\)\s*\{`)
	reGoMeth  = regexp.MustCompile(`(?m)^\s*func\s*\([^)]*\)\s*([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	rePyMeth  = regexp.MustCompile(`(?m)^\s*def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
)

// speakName renders an identifier as speakable words:
// "totalCost" -> "total cost", "getHTTPClient" -> "get H T T P client",
// "enqueue_route" -> "enqueue route".
func speakName(name string) string {
	name = strings.TrimSuffix(name, "()")
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, string(cur))
			cur = nil
		}
	}
	runes := []rune(name)
	for i, r := range runes {
		if r == '_' || r == '-' || r == '$' {
			flush()
			continue
		}
		if unicode.IsUpper(r) && len(cur) > 0 {
			prev := runes[i-1]
			next := rune(0)
			if i+1 < len(runes) {
				next = runes[i+1]
			}
			if unicode.IsLower(prev) || (unicode.IsUpper(prev) && unicode.IsLower(next)) {
				flush()
			}
		}
		cur = append(cur, r)
	}
	flush()
	for i, w := range words {
		if !isAllUpper(w) {
			words[i] = strings.ToLower(w)
		}
	}
	return strings.Join(words, " ")
}

func isAllUpper(s string) bool {
	hasLetter := false
	for _, r := range s {
		if unicode.IsLetter(r) && !unicode.IsUpper(r) {
			return false
		}
		if unicode.IsLetter(r) {
			hasLetter = true
		}
	}
	return hasLetter
}

// extractDefs finds named definitions in source, in order, capped at max.
func extractDefs(content string, max int) []string {
	seen := map[string]bool{}
	var names []string
	for _, re := range []*regexp.Regexp{reDefName, reArrowName, reShellFn, reGoMeth, rePyMeth} {
		for _, m := range re.FindAllStringSubmatch(content, -1) {
			n := m[1]
			if n == "" || seen[strings.ToLower(n)] || n == "_" {
				continue
			}
			seen[strings.ToLower(n)] = true
			names = append(names, n)
		}
	}
	if len(names) > max {
		names = names[:max]
	}
	return names
}

func joinWords(words []string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	case 2:
		return words[0] + " and " + words[1]
	default:
		return strings.Join(words[:len(words)-1], ", ") + ", and " + words[len(words)-1]
	}
}

// describeCode produces the deterministic spoken summary for a code block,
// replacing the original LLM narration.
func describeCode(info, content string) string {
	lines := 0
	for _, ln := range strings.Split(content, "\n") {
		if strings.TrimSpace(ln) != "" {
			lines++
		}
	}
	word := "lines"
	if lines == 1 {
		word = "line"
	}
	tag := infoTag(info)
	if tag == "sh" || tag == "bash" || tag == "zsh" || tag == "shell" || tag == "console" || tag == "fish" {
		return "Shell commands, " + strconv.Itoa(lines) + " " + word + "."
	}
	name, known := langName(info)
	var b strings.Builder
	b.WriteString("Code block")
	if known && name != "" {
		b.WriteString(" in ")
		b.WriteString(name)
	}
	b.WriteString(", ")
	b.WriteString(strconv.Itoa(lines))
	b.WriteString(" ")
	b.WriteString(word)
	var spoken []string
	for _, n := range extractDefs(content, 3) {
		if s := speakName(n); s != "" {
			spoken = append(spoken, s)
		}
	}
	if len(spoken) > 0 {
		b.WriteString(", defining ")
		b.WriteString(joinWords(spoken))
	}
	b.WriteString(".")
	return b.String()
}

var reTickRuns = regexp.MustCompile("`{3,}")
var reFenceRuns = regexp.MustCompile("[`~]{3,}")

// readableBlock recursively unwraps fenced/indented blocks inside s so prose
// reads verbatim, mirroring the original readableBlock.
func readableBlock(s string, depth int) string {
	if depth >= 8 {
		return reFenceRuns.ReplaceAllString(s, "")
	}
	d := Scan(s)
	var reps []replacement
	for _, b := range d.Blocks {
		if b.Kind != blkFenced && b.Kind != blkIndented {
			continue
		}
		inner := b.Content(s)
		if b.Kind == blkFenced {
			reps = append(reps, replacement{b.Start, b.End, readableBlock(inner, depth+1)})
		} else {
			reps = append(reps, replacement{b.Start, b.End, inner})
		}
	}
	out := s
	if len(reps) > 0 {
		out = splice(s, reps)
	}
	return reTickRuns.ReplaceAllString(out, "")
}

// narrateTable turns a GFM table into spoken prose, replacing LLM narration.
func narrateTable(block Block, text string) string {
	if len(block.ContentLines) < 2 {
		return block.sliceText(text)
	}
	header := splitCells(block.ContentLines[0].slice(text))
	var rows [][]string
	for _, ln := range block.ContentLines[1:] {
		ct := ln.slice(text)
		if strings.TrimSpace(ct) == "" {
			continue
		}
		rows = append(rows, splitCells(ct))
	}
	cleanCell := func(c string) string {
		c = stripEmphasis(c)
		c = stripEmoji(c)
		c = strings.ReplaceAll(c, "`", "")
		return strings.Join(strings.Fields(c), " ")
	}
	for i := range header {
		header[i] = cleanCell(header[i])
	}
	var b strings.Builder
	b.WriteString("Table with ")
	b.WriteString(strconv.Itoa(len(header)))
	if len(header) == 1 {
		b.WriteString(" column: ")
	} else {
		b.WriteString(" columns: ")
	}
	b.WriteString(joinWords(header))
	b.WriteString(".")
	const maxRows = 20
	shown := rows
	if len(shown) > maxRows {
		shown = shown[:maxRows]
	}
	for i, row := range shown {
		b.WriteString(" Row ")
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(": ")
		var cells []string
		for _, c := range row {
			c = cleanCell(c)
			if c == "" {
				cells = append(cells, "blank")
			} else {
				cells = append(cells, c)
			}
		}
		b.WriteString(strings.Join(cells, ", "))
		b.WriteString(".")
	}
	if len(rows) > len(shown) {
		b.WriteString(" And ")
		b.WriteString(strconv.Itoa(len(rows) - len(shown)))
		b.WriteString(" more rows.")
	}
	return b.String()
}

func (b Block) sliceText(text string) string { return text[b.Start:b.End] }
