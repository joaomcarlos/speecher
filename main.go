package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func usage() {
	fmt.Fprintf(os.Stderr, `speechclean — clean markdown/AI output into text a voice agent can read.

Usage: speechclean [flags] [file]

Reads from file, or stdin when no file is given. Writes cleaned text to stdout.
No AI, no API calls, no TTS — pure local text processing.

Flags:
`)
	flag.PrintDefaults()
}

func main() {
	verbose := flag.Bool("v", false, "report which rules changed the text / prompt groups detected (stderr)")
	codeMode := flag.String("code", "describe", "code block handling: describe | read (read unwraps every block verbatim)")
	direct := flag.Bool("text", false, "output the deterministically cleaned text instead of an LLM prompt")
	tts := flag.String("tts", "the voice tool", "name of the TTS tool call referenced in prompt output")
	list := flag.Bool("rules", false, "list pipeline rules and exit")
	flag.Usage = usage
	flag.Parse()

	if *list {
		for _, r := range Rules(Options{CodeMode: *codeMode}) {
			fmt.Println(r.ID)
		}
		return
	}
	if *codeMode != "describe" && *codeMode != "read" {
		fmt.Fprintln(os.Stderr, "invalid -code value: want describe or read")
		os.Exit(2)
	}

	var in []byte
	var err error
	if flag.NArg() > 0 && flag.Arg(0) != "-" {
		in, err = os.ReadFile(flag.Arg(0))
	} else {
		in, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}

	text := strings.ReplaceAll(string(in), "\r\n", "\n")
	opts := Options{CodeMode: *codeMode, TTSName: *tts}
	if *direct {
		res := Clean(text, opts)
		if *verbose {
			fmt.Fprintf(os.Stderr, "changes: %s\n", strings.Join(res.Changes, ", "))
		}
		os.Stdout.WriteString(res.Text)
		return
	}
	prompt, groups := BuildPrompt(text, opts)
	if *verbose {
		fmt.Fprintf(os.Stderr, "detected: %s\n", strings.Join(groups, ", "))
	}
	os.Stdout.WriteString(prompt)
}
