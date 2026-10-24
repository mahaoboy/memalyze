package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/mahaoboy/memalyze/internal/govern"
)

type redactHit struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type redactResult struct {
	Mode     string      `json:"mode"`
	Rules    int         `json:"rules"`
	Hits     []redactHit `json:"hits"`
	Changed  bool        `json:"changed"`
	InputLen int         `json:"input_len"`
	Output   string      `json:"output"`
}

func runRedact(args []string) int {
	fs := flag.NewFlagSet("redact", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	text := fs.String("text", "", "text to redact")
	file := fs.String("file", "", "read text from file instead of --text")
	rules := fs.String("rules", "", "custom rules JSON (default: built-in)")
	mode := fs.String("mode", string(govern.ModeEnforce), "enforce or dry-run")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	m := govern.Mode(*mode)
	if !m.Valid() || m == govern.ModeOff {
		fmt.Fprintf(os.Stderr, "memalyze: --mode must be enforce or dry-run, got %q\n", *mode)
		return 2
	}

	src := *text
	if *file != "" {
		data, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "memalyze: read %s: %v\n", *file, err)
			return 1
		}
		src = string(data)
	}
	if src == "" {
		fmt.Fprintln(os.Stderr, "memalyze: --text or --file is required")
		return 2
	}

	r, err := govern.Load(*rules)
	if err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: %v\n", err)
		return 1
	}
	out, hits := r.Redact(src)

	res := redactResult{
		Mode:     string(m),
		Rules:    len(r.Names()),
		Hits:     make([]redactHit, 0, len(hits)),
		Changed:  out != src,
		InputLen: len(src),
		Output:   out,
	}
	for _, h := range hits {
		res.Hits = append(res.Hits, redactHit{Name: h.Name, Count: h.Count})
	}
	if m == govern.ModeDryRun {
		// dry-run 语义：只报告"本应脱敏"，输出保持原文
		res.Output = src
		res.Changed = false
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(res); err != nil {
		fmt.Fprintf(os.Stderr, "memalyze: encode: %v\n", err)
		return 1
	}
	return 0
}
