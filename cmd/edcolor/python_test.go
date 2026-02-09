package main

import (
	"testing"
)

func TestLexPython(t *testing.T) {
	src := `import os
# a comment
def hello(name):
    print(f"Hello, {name}!")
    x = 42 + 3.14
    return True
`
	tokens := lexPython(src)

	type want struct {
		text string
		kind int
	}
	// Collect actual token texts.
	var got []want
	for _, tok := range tokens {
		got = append(got, want{src[tok.start:tok.end], tok.kind})
	}

	// Check that all expected tokens that should appear do appear.
	// We check a subset since the lexer skips plain identifiers.
	wantTokens := []want{
		{"import", tokKeyword},
		{"# a comment", tokComment},
		{"def", tokKeyword},
		{"print", tokBuiltin},
		{`f"Hello, {name}!"`, tokString},
		{"42", tokNumber},
		{"3.14", tokNumber},
		{"return", tokKeyword},
		{"True", tokKeyword},
	}

	for _, w := range wantTokens {
		found := false
		for _, g := range got {
			if g.text == w.text && g.kind == w.kind {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing token: %q kind=%d", w.text, w.kind)
		}
	}
}

func TestLexStrings(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"double", `"hello"`, `"hello"`},
		{"single", `'hello'`, `'hello'`},
		{"triple double", `"""multi\nline"""`, `"""multi\nline"""`},
		{"triple single", `'''multi\nline'''`, `'''multi\nline'''`},
		{"escape", `"he said \"hi\""`, `"he said \"hi\""`},
		{"raw", `r"no\escape"`, `r"no\escape"`},
		{"fstring", `f"val={x}"`, `f"val={x}"`},
		{"bytes", `b"data"`, `b"data"`},
		{"raw bytes", `rb"\n"`, `rb"\n"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := lexPython(tt.src)
			if len(tokens) != 1 {
				t.Fatalf("got %d tokens, want 1", len(tokens))
			}
			got := tt.src[tokens[0].start:tokens[0].end]
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			if tokens[0].kind != tokString {
				t.Errorf("got kind %d, want tokString", tokens[0].kind)
			}
		})
	}
}

func TestLexNumbers(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"int", "42", "42"},
		{"float", "3.14", "3.14"},
		{"leading dot", ".5", ".5"},
		{"exponent", "1e10", "1e10"},
		{"neg exp", "1e-5", "1e-5"},
		{"hex", "0xFF", "0xFF"},
		{"octal", "0o77", "0o77"},
		{"binary", "0b1010", "0b1010"},
		{"complex", "3j", "3j"},
		{"float complex", "3.14j", "3.14j"},
		{"underscore", "1_000_000", "1_000_000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := lexPython(tt.src)
			if len(tokens) != 1 {
				t.Fatalf("got %d tokens, want 1", len(tokens))
			}
			got := tt.src[tokens[0].start:tokens[0].end]
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			if tokens[0].kind != tokNumber {
				t.Errorf("got kind %d, want tokNumber", tokens[0].kind)
			}
		})
	}
}

func TestLexComment(t *testing.T) {
	src := "x = 1 # inline comment\ny = 2"
	tokens := lexPython(src)

	var commentFound bool
	for _, tok := range tokens {
		if tok.kind == tokComment {
			got := src[tok.start:tok.end]
			if got != "# inline comment" {
				t.Errorf("comment = %q, want %q", got, "# inline comment")
			}
			commentFound = true
		}
	}
	if !commentFound {
		t.Error("no comment token found")
	}
}

func TestColorizePython(t *testing.T) {
	src := "def f():\n    pass\n"
	spans := colorize(src, tokenizePython, 0, 0)

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
