package main

import (
	"testing"
)

func TestTokenizeGo(t *testing.T) {
	src := `package main

import "fmt"

// greet prints a greeting.
func greet(name string) {
	x := 42
	fmt.Println("Hello, " + name)
}
`
	regions := tokenizeGo(src)

	type want struct {
		text  string
		color string
		bold  bool
	}
	// Collect actual region texts.
	runes := []rune(src)
	var got []want
	for _, r := range regions {
		got = append(got, want{string(runes[r.runeStart:r.runeEnd]), r.color, r.bold})
	}

	wantRegions := []want{
		{"package", colorKeyword, true},
		{`"fmt"`, colorString, false},
		{"// greet prints a greeting.", colorComment, false},
		{"func", colorKeyword, true},
		{"string", colorBuiltin, false},
		{"42", colorNumber, false},
		{`"Hello, "`, colorString, false},
	}

	for _, w := range wantRegions {
		found := false
		for _, g := range got {
			if g.text == w.text && g.color == w.color && g.bold == w.bold {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing region: %q color=%s bold=%v", w.text, w.color, w.bold)
		}
	}
}

func TestColorizeGo(t *testing.T) {
	src := "package main\n\nfunc f() int { return 42 }\n"
	spans := colorize(src, tokenizeGo, 0, 0)

	// Verify spans are contiguous and cover the whole source.
	totalRunes := 0
	for range src {
		totalRunes++
	}

	covered := 0
	for i, s := range spans {
		if s.offset != covered {
			t.Errorf("span %d: offset=%d, expected %d (gap)", i, s.offset, covered)
		}
		covered += s.length
	}
	if covered != totalRunes {
		t.Errorf("spans cover %d runes, source has %d", covered, totalRunes)
	}
}

func TestColorizeGoViewport(t *testing.T) {
	// Source with multiple lines to test viewport filtering.
	src := "package main\n\nimport \"fmt\"\n\nfunc f() { fmt.Println(42) }\n"
	totalRunes := 0
	for range src {
		totalRunes++
	}

	// Full file (viewOrg=0, viewEnd=0) should cover everything.
	full := colorize(src, tokenizeGo, 0, 0)
	fullCovered := 0
	for _, s := range full {
		fullCovered += s.length
	}
	if fullCovered != totalRunes {
		t.Errorf("full colorize covers %d runes, want %d", fullCovered, totalRunes)
	}

	// Viewport in the middle: only covers a subset (plus margin).
	// viewOrg=14, viewEnd=26 covers "import \"fmt\"\n" (12 runes visible).
	// With 1x margin (12 runes above, 12 below), clip = [2, 38].
	spans := colorize(src, tokenizeGo, 14, 26)
	if len(spans) == 0 {
		t.Fatal("viewport colorize returned no spans")
	}

	// First span should start at clipStart (max(14-12, 0) = 2).
	if spans[0].offset != 2 {
		t.Errorf("first span offset = %d, want 2", spans[0].offset)
	}

	// Last span should end at clipEnd (min(26+12, totalRunes) = 38).
	last := spans[len(spans)-1]
	lastEnd := last.offset + last.length
	if lastEnd != 38 {
		t.Errorf("last span ends at %d, want 38", lastEnd)
	}

	// Spans must be contiguous within the clip range.
	cursor := spans[0].offset
	for i, s := range spans {
		if s.offset != cursor {
			t.Errorf("span %d: offset=%d, expected %d (gap)", i, s.offset, cursor)
		}
		cursor += s.length
	}

	// Total runes covered should be less than full file.
	viewCovered := 0
	for _, s := range spans {
		viewCovered += s.length
	}
	if viewCovered >= totalRunes {
		t.Errorf("viewport colorize covers %d runes, should be less than full %d", viewCovered, totalRunes)
	}
}

func TestGoTokenStyle(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		color string
		bold  bool
	}{
		{"keyword", "func", colorKeyword, true},
		{"string", `"hello"`, colorString, false},
		{"number", "42", colorNumber, false},
		{"comment", "// comment", colorComment, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regions := tokenizeGo(tt.src)
			if len(regions) == 0 {
				t.Fatal("no regions returned")
			}
			r := regions[0]
			if r.color != tt.color {
				t.Errorf("color = %q, want %q", r.color, tt.color)
			}
			if r.bold != tt.bold {
				t.Errorf("bold = %v, want %v", r.bold, tt.bold)
			}
		})
	}
}
