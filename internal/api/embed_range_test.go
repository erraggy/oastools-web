package api

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// emgithubCall matches an emgithubURL template action that pins a line range,
// e.g. {{with emgithubURL "internal/api/fix.go" 101 158}}.
var emgithubCall = regexp.MustCompile(`emgithubURL\s+"internal/api/(\w+\.go)"\s+(\d+)\s+(\d+)`)

// TestEmbedLineRangesCoverTheirBlock guards the "How it's done" code embeds.
//
// Those sections pin a line range of a handler and render it from GitHub, so
// the range silently stops describing the block it names as soon as anything
// above it grows. Nothing in a normal build reads these numbers, which is
// exactly why they rot unnoticed: the page keeps rendering, just showing the
// wrong lines. This asserts each range still starts on the comment it claims to
// start on and still contains the library call it exists to demonstrate.
func TestEmbedLineRangesCoverTheirBlock(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		source      string
		startMarker string
		mustContain []string
	}{
		{
			name:        "fix page embeds the fixer configuration and FixParsed",
			template:    "../templates/fix.html",
			source:      "fix.go",
			startMarker: "// Configure fixer based on form options",
			mustContain: []string{"fixer.New()", "f.EnabledFixes = enabledFixes", "f.FixParsed("},
		},
		{
			name:        "join page embeds the joiner configuration and JoinParsed",
			template:    "../templates/join.html",
			source:      "join.go",
			startMarker: "// Configure joiner with collision strategy",
			mustContain: []string{"joiner.DefaultConfig()", "config.DeduplicationMode", "j.JoinParsed("},
		},
		{
			name:        "explore page embeds the parse and walker collection",
			template:    "../templates/explore.html",
			source:      "explore.go",
			startMarker: "// Parse spec using oastools",
			mustContain: []string{"parser.ParseWithOptions(", "walker.Collect"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := os.ReadFile(tt.template)
			if err != nil {
				t.Fatalf("failed to read template: %v", err)
			}

			// All matches, not just the first: a template that grows a second
			// ranged embed would otherwise have only its first one checked,
			// and the new range would rot unnoticed — the very failure this
			// test exists to prevent.
			matches := emgithubCall.FindAllStringSubmatch(string(tmpl), -1)
			if len(matches) == 0 {
				t.Fatalf("no emgithubURL call with a line range found in %s", tt.template)
			}
			if len(matches) > 1 {
				t.Fatalf("%s has %d ranged emgithubURL calls; this test checks one per template, so extend the table",
					tt.template, len(matches))
			}

			match := matches[0]
			if match[1] != tt.source {
				t.Fatalf("embed references %s, want %s", match[1], tt.source)
			}

			start, _ := strconv.Atoi(match[2])
			end, _ := strconv.Atoi(match[3])
			if start >= end {
				t.Fatalf("line range %d-%d is not ascending", start, end)
			}

			src, err := os.ReadFile(tt.source)
			if err != nil {
				t.Fatalf("failed to read source: %v", err)
			}
			lines := strings.Split(string(src), "\n")
			if end > len(lines) {
				t.Fatalf("line range ends at %d but %s has only %d lines", end, tt.source, len(lines))
			}

			// Lines are 1-based in the embed, so shift for the slice.
			if got := strings.TrimSpace(lines[start-1]); got != tt.startMarker {
				t.Errorf("range starts at line %d (%q), want it to start on %q\n"+
					"the block moved: update the line range in %s", start, got, tt.startMarker, tt.template)
			}

			snippet := strings.Join(lines[start-1:end], "\n")
			for _, want := range tt.mustContain {
				if !strings.Contains(snippet, want) {
					t.Errorf("range %d-%d does not include %q\n"+
						"the embed no longer shows what it claims: update the line range in %s",
						start, end, want, tt.template)
				}
			}
		})
	}
}
