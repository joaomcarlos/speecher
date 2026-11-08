package main

import "testing"

func clean(t *testing.T, in string) string {
	t.Helper()
	return Clean(in, Options{CodeMode: "describe"}).Text
}

func table(t *testing.T, name string, cases [][2]string) {
	t.Helper()
	for _, c := range cases {
		if got := clean(t, c[0]); got != c[1] {
			t.Errorf("%s: clean(%q) = %q, want %q", name, c[0], got, c[1])
		}
	}
}

// Ported from markdown-speech.test.ts path cases; backticks/link syntax are
// unwrapped by the final rule since there is no downstream cleanup.
func TestFilePaths(t *testing.T) {
	table(t, "filePaths", [][2]string{
		{"`km/h` `x/y` `A/B`", "km/h x/y A/B"},
		{"`./src/components` `src/components/`", ". src components src components"},
		{
			"**Draft replies to Pedro** (not posted; full text in sol-4174-research/pedro-review-replies-draft.md)",
			"Draft replies to Pedro (not posted; full text in sol 4174 research pedro review replies draft.md)",
		},
		{"Open `some/file-name.bob` and `src/main.ts`.", "Open some file name.bob and src main.ts."},
		{
			"See **src/my_file.ts**, ../docs/read-me.md; then ~/notes/draft.md.",
			"See src my file.ts, .. docs read me.md; then ~ notes draft.md.",
		},
		{"[src/my_file.ts](https://example.com/src/my_file.ts)", "src my file.ts"},
		{
			"https://example.com/my_file.ts example.com/my_file.ts /api/v1 A/B 10/2 km/h",
			"link to example.com example.com/my_file.ts /api/v1 A/B 10/2 km/h",
		},
		{"`https://example.com/my_file.ts` `cat src/my_file.ts`", "link to example.com cat src/my_file.ts"},
	})
}

// Ported from markdown-speech.test.ts URL cases.
func TestURLs(t *testing.T) {
	attachment := "https://github.com/user-attachments/assets/ffdad1a6-fb67-4bd8-a7dc-45fb013acaf0"
	table(t, "urls", [][2]string{
		{
			"SOL-3972 is ready. 3 min video: [" + attachment + "](" + attachment + ")",
			"SOL-3972 is ready. 3 min video: GitHub attachment",
		},
		{"Video: " + attachment, "Video: GitHub attachment"},
		{"<" + attachment + ">", "GitHub attachment"},
		{"[Watch the **demo**](" + attachment + ")", "Watch the demo"},
		{
			"See https://example.com/private/token?key=secret#hash, then www.example.org/video.",
			"See link to example.com, then link to www.example.org.",
		},
		{"(https://example.com/a_(b)). Next.", "(link to example.com). Next."},
		{
			"https://github.com.evil.example/user-attachments/assets/abc https://github.com/owner/repo/commit/abcdef12345",
			"link to github.com.evil.example link to github.com",
		},
		{"`" + attachment + "` and `curl " + attachment + "`", "GitHub attachment and curl " + attachment},
		{
			"[" + attachment + "][video]\n\n[video]: " + attachment,
			"GitHub attachment\n\n",
		},
		{"Broken https:// and email me@example.com.", "Broken https:// and email me@example.com."},
	})
}

// Ported from the blockquote cases.
func TestBlockquotes(t *testing.T) {
	table(t, "blockquotes", [][2]string{
		{"> What about a visit that grows into the next stop?", "What about a visit that grows into the next stop?"},
		{
			"> Call `enqueue_route_optimizations_after_commit`; count > 5.\n> Then review.",
			"Call enqueue route optimizations after commit; count > 5.\nThen review.",
		},
		{"> Outer\n>> Inner\n>\n> End", "Outer\nInner\n\nEnd"},
		{"- > Quoted\n  > reply", "Quoted\n  reply"},
		{"> First\nlazy continuation\n\nAfter", "First\nlazy continuation\n\nAfter"},
		{"Compare count > 5, `x >= 3`, and `items >> 1`.", "Compare count > 5, x >= 3, and items >> 1."},
		{"\\> literal\n\n```txt\n> literal\n```\n\n    > literal", "\\> literal\n\nliteral\n\nliteral"},
		{"> ```txt\n> x > 5\n> ```", "x > 5"},
	})
}

// Ported from pipeline.test.ts separator cases; output drops backticks.
func TestSeparators(t *testing.T) {
	table(t, "separators", [][2]string{
		{
			"Call `enqueue_route_optimizations_after_commit` and `submit_route_optimization`.",
			"Call enqueue route optimizations after commit and submit route optimization.",
		},
		{
			"Use `address_id`, `user_ID`, `id_lookup`, and `get_id()`.",
			"Use address I D, user I D, I D lookup, and get I D().",
		},
		{
			"`identity` `identity_key` `valid_id2` `id` and plain address_id.",
			"identity identity key valid id2 id and plain address_id.",
		},
		{"Open `some/file-name.bob`.", "Open some file name.bob."},
		{
			"Use `my_file-name.ts` and `~/café_notes/version-2/`.",
			"Use my file name.ts and ~ café notes version 2.",
		},
		{"See **`./my_project/read-me.md`** and ``other_file.md``.", "See . my project read me.md and other file.md."},
		{"Plain my_project/release-notes.md and follow-up.", "Plain my project release notes.md and follow-up."},
		{
			"`-12` `-£12` `-.5` `3-5` `1e-3` `some_variable` `--dry-run`",
			"-12 -£12 -.5 3-5 1e-3 some variable --dry-run",
		},
		{
			"`git diff --stat file-name.ts` and `https://example.com/my_file`",
			"git diff --stat file-name.ts and link to example.com",
		},
	})
}

// Deterministic hash shortening via keyword context.
func TestHashes(t *testing.T) {
	table(t, "hashes", [][2]string{
		{
			"Migration `2bde899ec541` follows `b310dd58c6da`.",
			"Migration hash ending c541 follows hash ending c6da.",
		},
		{"Merged commit `fe7e5536bdd2f263784cb46b76c67c3a455781bd`.", "Merged commit hash ending 81bd."},
		{"Commit DEADBEEF is live.", "Commit hash ending BEEF is live."},
		{
			"The invoice number is 12345678, the color is deadbeef, and the quantity is 10000000.",
			"The invoice number is 12345678, the color is deadbeef, and the quantity is 10000000.",
		},
		{"Checksum sha256=a1b2c3d4e5 verified.", "Checksum sha256=hash ending d4e5 verified."},
		{
			"I re-pointed our migration and renamed it to `1791970000_b310dd58c6da.py`.",
			"I re-pointed our migration and renamed it to migration ending c6da.",
		},
		{"It is in `backup_1791970000_b310dd58c6da.py` file.", "It is in backup 1791970000 b310dd58c6da.py file."},
		{"abcdef123456 on its own.", "abcdef123456 on its own."},
		{
			"[abcdef123456](https://example.com/private/abcdef123456) is the commit.",
			"hash ending 3456 is the commit.",
		},
		{"See https://example.com/abcdef123456 for the commit.", "See link to example.com for the commit."},
	})
}

func TestCodeBlocks(t *testing.T) {
	table(t, "codeBlocks", [][2]string{
		{
			"```ts\nconst x = 1;\n```",
			"Code block in TypeScript, 1 line.",
		},
		{
			"Before.\n\n```go\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n```\n\nAfter.",
			"Before.\n\nCode block in Go, 3 lines, defining main.\n\nAfter.",
		},
		{
			"```sh\nsystemctl restart nginx\njournalctl -u nginx -f\n```",
			"Shell commands, 2 lines, running systemctl restart and journalctl.",
		},
		{
			"```sh\nCGO_ENABLED=0 go build -ldflags='-s -w' . && ./speechclean -v\nsudo systemctl reload nginx | grep ok\n```",
			"Shell commands, 2 lines, running go build, speechclean, systemctl reload, and grep.",
		},
		{
			"````vbnet\nHere are the proposed additions:\n\n```\n1. First.\n2. Second.\n```\n````",
			"Here are the proposed additions:\n\n1. First.\n2. Second.",
		},
		{"    print('hello')", "Code block, 1 line."},
	})
}

func TestCodeModeRead(t *testing.T) {
	res := Clean("```go\nfunc main() {}\n```", Options{CodeMode: "read"})
	if res.Text != "func main() {}" {
		t.Fatalf("read mode = %q", res.Text)
	}
}

func TestTables(t *testing.T) {
	table(t, "tables", [][2]string{
		{
			"| Site | Cost |\n| --- | ---: |\n| A | -£12 |\n| B | £34 |",
			"Table with 2 columns: Site and Cost. Row 1: A, -£12. Row 2: B, £34.",
		},
		{
			"Before.\n\n| Env | Ver |\n|---|---|\n| prod | 2.0 |\n\nAfter.",
			"Before.\n\nTable with 2 columns: Env and Ver. Row 1: prod, 2.0.\n\nAfter.",
		},
		{"Not | a | table\njust text", "Not | a | table\njust text"},
	})
}

func TestFormatting(t *testing.T) {
	table(t, "formatting", [][2]string{
		{"**Bold** and *italic* and `code`.", "Bold and italic and code."},
		{"# Heading\nBody text.", "Heading\nBody text."},
		{"- one\n- two", "one\ntwo"},
		{"1. first\n2. second", "1. first\n2. second"},
		{"Done ✅ next 🚀 step.", "Done  next  step."},
		{"Above.\n\n---\n\nBelow.", "Above.\n\n\n\nBelow."},
		{"multiply 4 * 5 ok", "multiply 4 times 5 ok"},
		{"say \"hi\" it's fine", "say \"hi\" it's fine"},
	})
}

func TestUnwrapLinks(t *testing.T) {
	table(t, "unwrap", [][2]string{
		{"[demo](https://example.com/x) done.", "demo done."},
		{"![alt text](image.png) rest.", "alt text rest."},
		{"See [docs](https://example.com \"title\") now.", "See docs now."},
		{"plain [text] trailing.", "plain text trailing."},
	})
}

func TestNoChange(t *testing.T) {
	res := Clean("Hello there.", Options{})
	if res.Applied || len(res.Changes) != 0 {
		t.Fatalf("expected no changes, got %+v", res)
	}
	if res.Text != "Hello there." {
		t.Fatalf("text changed: %q", res.Text)
	}
}
