# speechclean

Clean markdown/AI output into text a voice agent can read aloud.

`speechclean` sits between an LLM's markdown response and your text-to-speech
tool. It emits a tailored prompt — numbered rules covering only the constructs
actually found in the input — that tells the agent exactly how to say each
thing, how short to keep it, and what voice to use. No AI, no API calls, no
TTS: pure local text processing.

## Example

### 1. The AI response (input)

```markdown
# Refactor complete ✅

The migration pipeline is finally done. Here's what changed:

- Moved the block scanner to a line-based model in `speecher/scan.go`
- Added table narration and hash detection fixes (no more firing on issue numbers like #1234)
- All 47 tests pass, see commit `1f4a9b3c2e`
- Full details at https://github.com/joaomcarlos/speecher/pull/12 — took about 3 hours total
```

### 2. The app's output (the prompt it generates)

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

### 3. What the user hears

> "Refactor's done — scanner's line-based now, tables narrate, all
> forty-seven tests green. Hash ending three c two e if you want the details."

~24 words, about 10 seconds. Instead of reading every bullet (40+ seconds),
the agent keeps the headline, speaks the URL as "link to github.com", and
says only the last four characters of the hash.

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
