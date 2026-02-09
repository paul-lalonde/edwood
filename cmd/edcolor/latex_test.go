package main

import (
	"testing"
)

func TestLexLatex(t *testing.T) {
	src := `\documentclass{article}
\usepackage{amsmath}
% A comment
\begin{document}
\section{Introduction}
Hello, $x^2$ and $$E = mc^2$$.
\end{document}
`
	tokens := lexLatex(src)

	type want struct {
		text string
		kind int
	}
	var got []want
	for _, tok := range tokens {
		got = append(got, want{src[tok.start:tok.end], tok.kind})
	}

	wantTokens := []want{
		{`\documentclass`, tokKeyword},
		{`\usepackage`, tokKeyword},
		{"% A comment", tokComment},
		{`\begin`, tokKeyword},
		{"document", tokBuiltin},
		{`\section`, tokKeyword},
		{"$x^2$", tokString},
		{"$$E = mc^2$$", tokString},
		{`\end`, tokKeyword},
		{"document", tokBuiltin},
	}

	for _, w := range wantTokens {
		found := false
		for gi, g := range got {
			if g.text == w.text && g.kind == w.kind {
				found = true
				got = append(got[:gi], got[gi+1:]...)
				break
			}
		}
		if !found {
			t.Errorf("missing token: %q kind=%d", w.text, w.kind)
		}
	}
}

func TestLexLatexComments(t *testing.T) {
	t.Run("line comment", func(t *testing.T) {
		src := "hello % this is a comment\nworld"
		tokens := lexLatex(src)

		var commentFound bool
		for _, tok := range tokens {
			if tok.kind == tokComment {
				got := src[tok.start:tok.end]
				if got != "% this is a comment" {
					t.Errorf("comment = %q, want %q", got, "% this is a comment")
				}
				commentFound = true
			}
		}
		if !commentFound {
			t.Error("no comment token found")
		}
	})

	t.Run("escaped percent", func(t *testing.T) {
		src := `10\% discount`
		tokens := lexLatex(src)

		for _, tok := range tokens {
			if tok.kind == tokComment {
				t.Errorf("unexpected comment token: %q", src[tok.start:tok.end])
			}
		}
		// \% should be a keyword (single-char command).
		var found bool
		for _, tok := range tokens {
			if tok.kind == tokKeyword {
				got := src[tok.start:tok.end]
				if got == `\%` {
					found = true
				}
			}
		}
		if !found {
			t.Error(`expected \% as keyword token`)
		}
	})
}

func TestLexLatexMath(t *testing.T) {
	t.Run("inline math", func(t *testing.T) {
		src := "text $a + b$ more"
		tokens := lexLatex(src)

		var mathFound bool
		for _, tok := range tokens {
			if tok.kind == tokString {
				got := src[tok.start:tok.end]
				if got != "$a + b$" {
					t.Errorf("math = %q, want %q", got, "$a + b$")
				}
				mathFound = true
			}
		}
		if !mathFound {
			t.Error("no math token found")
		}
	})

	t.Run("display math", func(t *testing.T) {
		src := "text $$a + b$$ more"
		tokens := lexLatex(src)

		var mathFound bool
		for _, tok := range tokens {
			if tok.kind == tokString {
				got := src[tok.start:tok.end]
				if got != "$$a + b$$" {
					t.Errorf("math = %q, want %q", got, "$$a + b$$")
				}
				mathFound = true
			}
		}
		if !mathFound {
			t.Error("no display math token found")
		}
	})

	t.Run("escaped dollar in math", func(t *testing.T) {
		src := `$cost = \$5$`
		tokens := lexLatex(src)

		var mathFound bool
		for _, tok := range tokens {
			if tok.kind == tokString {
				got := src[tok.start:tok.end]
				if got != src {
					t.Errorf("math = %q, want %q", got, src)
				}
				mathFound = true
			}
		}
		if !mathFound {
			t.Error("no math token found with escaped dollar")
		}
	})
}

func TestLexLatexEnvironments(t *testing.T) {
	src := `\begin{itemize}
\item Hello
\end{itemize}`
	tokens := lexLatex(src)

	type want struct {
		text string
		kind int
	}
	var got []want
	for _, tok := range tokens {
		got = append(got, want{src[tok.start:tok.end], tok.kind})
	}

	wantTokens := []want{
		{`\begin`, tokKeyword},
		{"itemize", tokBuiltin},
		{`\item`, tokKeyword},
		{`\end`, tokKeyword},
		{"itemize", tokBuiltin},
	}

	for _, w := range wantTokens {
		found := false
		for gi, g := range got {
			if g.text == w.text && g.kind == w.kind {
				found = true
				got = append(got[:gi], got[gi+1:]...)
				break
			}
		}
		if !found {
			t.Errorf("missing token: %q kind=%d", w.text, w.kind)
		}
	}
}

func TestColorizeLatex(t *testing.T) {
	src := "\\section{Hello}\n% comment\n$x^2$\n"
	spans := colorize(src, tokenizeLatex, 0, 0)

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
