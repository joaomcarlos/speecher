package main

import (
	"strings"
	"testing"
)

func TestBuildPromptTailored(t *testing.T) {
	in := "# Hi\n\n```go\nfunc main() {}\n```\n\ncommit `1f4a9b3c` and see https://ex.com/a → ok\n"
	p, groups := BuildPrompt(in, Options{CodeMode: "describe", TTSName: "speak"})
	for _, want := range []string{
		"code block", "hash ending 9b3c", "link to ex.com",
		"text-to-speech tool call (speak)",
		"Do not mention", "TEXT:\n" + in,
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q\ngroups=%v\n%s", want, groups, p)
		}
	}
	for _, g := range []string{"code-blocks", "urls", "hashes", "formatting", "symbols"} {
		found := false
		for _, have := range groups {
			found = found || have == g
		}
		if !found {
			t.Errorf("missing group %q in %v", g, groups)
		}
	}
}

func TestBuildPromptMinimal(t *testing.T) {
	p, _ := BuildPrompt("just a sentence.\n", Options{TTSName: "speak"})
	if !strings.Contains(p, "read it naturally") || strings.Contains(p, "code block") {
		t.Errorf("plain input should get a single natural-read rule:\n%s", p)
	}
}

func TestBuildPromptNoGeneric(t *testing.T) {
	p, _ := BuildPrompt("No urls or code here.\n", Options{TTSName: "speak"})
	if strings.Contains(p, "URL") || strings.Contains(p, "hash ending") {
		t.Errorf("absent constructs must not produce rules:\n%s", p)
	}
}

func TestBuildPromptBrevityAndVoice(t *testing.T) {
	p, _ := BuildPrompt("just a sentence.\n", Options{TTSName: "speak"})
	for _, want := range []string{"under ten seconds", "never exceed twenty seconds", "summarize", "chill, friendly and playful"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q:\n%s", want, p)
		}
	}
}
