package main

import (
	"testing"
)

func TestLexRust(t *testing.T) {
	src := `use std::collections::HashMap;

// A comment
fn main() {
    let mut x: i32 = 42;
    let s = String::from("hello");
    let v: Vec<i32> = vec![1, 2, 3];
    if x > 0 {
        println!("positive");
    }
    match x {
        0 => false,
        _ => true,
    }
}
`
	tokens := lexRust(src)

	type want struct {
		text string
		kind int
	}
	var got []want
	for _, tok := range tokens {
		got = append(got, want{src[tok.start:tok.end], tok.kind})
	}

	wantTokens := []want{
		{"use", tokKeyword},
		{"// A comment", tokComment},
		{"fn", tokKeyword},
		{"let", tokKeyword},
		{"mut", tokKeyword},
		{"i32", tokBuiltin},
		{"42", tokNumber},
		{"String", tokBuiltin},
		{`"hello"`, tokString},
		{"Vec", tokBuiltin},
		{"if", tokKeyword},
		{`"positive"`, tokString},
		{"match", tokKeyword},
		{"false", tokKeyword},
		{"true", tokKeyword},
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

func TestLexRustStrings(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"double", `"hello"`, `"hello"`},
		{"escape", `"he said \"hi\""`, `"he said \"hi\""`},
		{"raw", `r"no\escape"`, `r"no\escape"`},
		{"raw hash", `r#"has "quotes" inside"#`, `r#"has "quotes" inside"#`},
		{"raw 2 hash", `r##"has "# inside"##`, `r##"has "# inside"##`},
		{"byte string", `b"data"`, `b"data"`},
		{"byte char", `b'x'`, `b'x'`},
		{"raw byte", `br#"raw\bytes"#`, `br#"raw\bytes"#`},
		{"char", `'a'`, `'a'`},
		{"char escape", `'\n'`, `'\n'`},
		{"char hex", `'\x41'`, `'\x41'`},
		{"char unicode", `'\u{1F600}'`, `'\u{1F600}'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := lexRust(tt.src)
			if len(tokens) != 1 {
				for i, tok := range tokens {
					t.Logf("  token[%d]: %q kind=%d", i, tt.src[tok.start:tok.end], tok.kind)
				}
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

func TestLexRustNumbers(t *testing.T) {
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
		{"underscore", "1_000_000", "1_000_000"},
		{"suffixed u32", "42u32", "42u32"},
		{"suffixed f64", "3.14f64", "3.14f64"},
		{"suffixed i8", "255i8", "255i8"},
		{"suffixed usize", "0usize", "0usize"},
		{"hex suffixed", "0xFFu8", "0xFFu8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := lexRust(tt.src)
			if len(tokens) != 1 {
				for i, tok := range tokens {
					t.Logf("  token[%d]: %q kind=%d", i, tt.src[tok.start:tok.end], tok.kind)
				}
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

func TestLexRustComments(t *testing.T) {
	t.Run("line comment", func(t *testing.T) {
		src := "x = 1; // inline comment\ny = 2"
		tokens := lexRust(src)

		var commentFound bool
		for _, tok := range tokens {
			if tok.kind == tokComment {
				got := src[tok.start:tok.end]
				if got != "// inline comment" {
					t.Errorf("comment = %q, want %q", got, "// inline comment")
				}
				commentFound = true
			}
		}
		if !commentFound {
			t.Error("no comment token found")
		}
	})

	t.Run("nested block comment", func(t *testing.T) {
		src := "/* outer /* inner */ still outer */"
		tokens := lexRust(src)
		if len(tokens) != 1 {
			t.Fatalf("got %d tokens, want 1", len(tokens))
		}
		got := src[tokens[0].start:tokens[0].end]
		if got != src {
			t.Errorf("got %q, want %q", got, src)
		}
		if tokens[0].kind != tokComment {
			t.Errorf("got kind %d, want tokComment", tokens[0].kind)
		}
	})

	t.Run("simple block comment", func(t *testing.T) {
		src := "/* simple */"
		tokens := lexRust(src)
		if len(tokens) != 1 {
			t.Fatalf("got %d tokens, want 1", len(tokens))
		}
		got := src[tokens[0].start:tokens[0].end]
		if got != src {
			t.Errorf("got %q, want %q", got, src)
		}
	})
}

func TestLexRustLifetimes(t *testing.T) {
	src := "fn foo<'a>(x: &'a str) -> &'a str {"
	tokens := lexRust(src)

	type want struct {
		text string
		kind int
	}
	var got []want
	for _, tok := range tokens {
		got = append(got, want{src[tok.start:tok.end], tok.kind})
	}

	// Lifetimes should be colored as keywords.
	wantTokens := []want{
		{"fn", tokKeyword},
		{"'a", tokKeyword},
		{"'a", tokKeyword},
		{"str", tokBuiltin},
		{"'a", tokKeyword},
		{"str", tokBuiltin},
	}

	for _, w := range wantTokens {
		found := false
		for gi, g := range got {
			if g.text == w.text && g.kind == w.kind {
				found = true
				// Remove so we can match duplicates.
				got = append(got[:gi], got[gi+1:]...)
				break
			}
		}
		if !found {
			t.Errorf("missing token: %q kind=%d", w.text, w.kind)
		}
	}
}

func TestColorizeRust(t *testing.T) {
	src := "fn main() {\n    let x = 42;\n}\n"
	spans := colorize(src, tokenizeRust, 0, 0)

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
