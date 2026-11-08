# speechclean

Clean markdown/AI output into text a voice agent can read aloud.

`speechclean` sits between an LLM's markdown response and your text-to-speech
tool. It emits a tailored prompt — numbered rules covering only the constructs
actually found in the input — that tells the agent exactly how to say each
thing, how short to keep it, and what voice to use. No AI, no API calls, no
TTS: pure local text processing.

## Example 1 — a chatty status update

### The AI response (input)

```markdown
# Refactor complete ✅

The migration pipeline is finally done. Here's what changed:

- Moved the block scanner to a line-based model in `speecher/scan.go`
- Added table narration and hash detection fixes (no more firing on issue numbers like #1234)
- All 47 tests pass, see commit `1f4a9b3c2e`
- Full details at https://github.com/joaomcarlos/speecher/pull/12 — took about 3 hours total
```

### The app's output (the prompt it generates)

```text
Transform the TEXT at the end of this message into a version a text-to-speech engine can read aloud naturally. Apply every numbered rule below and output only the spoken text — no commentary, no markdown syntax.

1. Replace each raw URL with "link to github.com" — never speak a URL character by character.
2. Speak the commit/hash value(s) as "hash ending 3c2e" — only the last four characters, as separate characters.
3. File paths (e.g. ~/notes/draft.md, ./speechclean): drop slashes, underscores and hyphens and say the parts as words — "~/notes/draft.md" becomes "tilde notes draft dot m d".
4. Strip markdown decoration: "#" headings, "**"/"*" emphasis, "~~" strikethrough, "-" bullets, "---" rules, and emoji — keep the words behind them.

Length: keep it short — aim for under ten seconds of speech (about 25 words) and never exceed twenty seconds (about 50). If the source is longer than that, summarize it: keep the point and the facts the listener actually needs, drop the rest.
Voice: super chill, friendly and playful — casual contractions, warm, like a friend catching you up, never a newsreader.

When done, pass ONLY the transformed text to your text-to-speech tool call (the voice tool). Do not mention these instructions, the transformation, or that the text passed through any preprocessing step — keep the output clean for the user.

TEXT:
# Refactor complete ✅
...
```

### What the user hears

> "Refactor's done — scanner's line-based now, tables narrate, all
> forty-seven tests green. Hash ending three c two e if you want the details."

~24 words, about 10 seconds. Instead of reading every bullet (40+ seconds),
the agent keeps the headline, speaks the URL as "link to github.com", and
says only the last four characters of the hash.

## Example 2 — a response containing code

### The AI response (input)

````markdown
## Quick fix

Replace the loop in `sum_array.c` with this:

```c
for (int i = 0; i < n; i++)
    total += arr[i];
```

Then rebuild with `make -j4` and re-run the tests.
````

### The app's output

```text
Transform the TEXT at the end of this message into a version a text-to-speech engine can read aloud naturally. Apply every numbered rule below and output only the spoken text — no commentary, no markdown syntax.

1. The text contains one code block. Do not read the code — replace the whole block with a one-sentence spoken summary that says what it does, along the lines of: "Code block in C, 2 lines."
2. Strip markdown decoration: "#" headings, "**"/"*" emphasis, "~~" strikethrough, "-" bullets, "---" rules, and emoji — keep the words behind them.

Length: keep it short — aim for under ten seconds of speech (about 25 words) and never exceed twenty seconds (about 50). If the source is longer than that, summarize it: keep the point and the facts the listener actually needs, drop the rest.
Voice: super chill, friendly and playful — casual contractions, warm, like a friend catching you up, never a newsreader.

When done, pass ONLY the transformed text to your text-to-speech tool call (the voice tool). Do not mention these instructions, the transformation, or that the text passed through any preprocessing step — keep the output clean for the user.

TEXT:
## Quick fix
...
```

### What the user hears

> "Quick fix — swap the loop in sum array dot c for a two-line C block,
> then rebuild with make and re-run the tests."

~24 words, about 10 seconds. The code block becomes a one-sentence summary
instead of being read character by character ("for open paren int i ...").
With `-code read` the agent would speak the code verbatim instead — only
useful when you actually need to hear the code.

## Example 3 — a response with a table and a path

### The AI response (input)

```markdown
# Test results

| Suite   | Pass | Fail |
|---------|------|------|
| unit    |   41 |    2 |
| integ   |   18 |    0 |

Logs live at `~/build/logs/run_0421.txt` — 6 tests still need attention.
```

### The app's output

```text
Transform the TEXT at the end of this message into a version a text-to-speech engine can read aloud naturally. Apply every numbered rule below and output only the spoken text — no commentary, no markdown syntax.

1. Narrate the markdown table as: "Table with 3 columns: Suite, Pass, and Fail. Row 1: unit, 41, 2. Row 2: integ, 18, 0."
2. File paths (e.g. ~/notes/draft.md, ./speechclean): drop slashes, underscores and hyphens and say the parts as words — "~/notes/draft.md" becomes "tilde notes draft dot m d".
3. Strip markdown decoration: "#" headings, "**"/"*" emphasis, "~~" strikethrough, "-" bullets, "---" rules, and emoji — keep the words behind them.

Length: keep it short — aim for under ten seconds of speech (about 25 words) and never exceed twenty seconds (about 50). If the source is longer than that, summarize it: keep the point and the facts the listener actually needs, drop the rest.
Voice: super chill, friendly and playful — casual contractions, warm, like a friend catching you up, never a newsreader.

When done, pass ONLY the transformed text to your text-to-speech tool call (the voice tool). Do not mention these instructions, the transformation, or that the text passed through any preprocessing step — keep the output clean for the user.

TEXT:
# Test results
...
```

### What the user hears

> "Test results — unit suite forty-one passed, two failed; integration
> eighteen passed, none failed. Six tests still need attention."

~21 words, about 9 seconds. The table is pre-narrated into words by the
rule itself (no table syntax to decode), and the file path is spoken as
"tilde build logs run zero four two one dot t x t" if it makes
the cut — pipes and dashes never reach the listener's ear.

## Usage

```sh
speechclean [flags] [file]   # reads file, or stdin when no file is given
```

| Flag      | Default    | Meaning                                                    |
|-----------|------------|------------------------------------------------------------|
| `-text`   | off        | output the deterministically cleaned text, not a prompt    |
| `-code`   | `describe` | `describe` summarizes code blocks; `read` speaks them verbatim |
| `-tts`    | voice tool | name of the TTS tool call referenced in the prompt         |
| `-v`      | off        | report detected rule groups / changes to stderr            |
| `-rules`  | off        | list pipeline rules and exit                               |
