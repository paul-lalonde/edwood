package main

// Rust keywords.
var rustKeywords = map[string]bool{
	"as": true, "async": true, "await": true,
	"break": true,
	"const": true, "continue": true, "crate": true,
	"dyn": true,
	"else": true, "enum": true, "extern": true,
	"false": true, "fn": true, "for": true,
	"if": true, "impl": true, "in": true,
	"let": true, "loop": true,
	"match": true, "mod": true, "move": true, "mut": true,
	"pub": true,
	"ref": true, "return": true,
	"self": true, "static": true, "struct": true, "super": true,
	"trait": true, "true": true, "type": true,
	"unsafe": true, "use": true,
	"where": true, "while": true,
}

// Rust builtin types and common std library types.
var rustBuiltins = map[string]bool{
	// Primitive types
	"bool": true, "char": true, "str": true,
	"i8": true, "i16": true, "i32": true, "i64": true, "i128": true,
	"u8": true, "u16": true, "u32": true, "u64": true, "u128": true,
	"isize": true, "usize": true,
	"f32": true, "f64": true,
	// Common std types
	"String": true, "Vec": true, "Option": true, "Result": true,
	"Box": true, "Rc": true, "Arc": true,
	"HashMap": true, "HashSet": true,
	"Some": true, "None": true, "Ok": true, "Err": true,
	"Self": true,
}

type rustToken struct {
	start, end int // byte offsets
	kind       int
}

// tokenizeRust lexes src as Rust source and returns colored regions.
func tokenizeRust(src string) []region {
	b2r := byteToRuneIndex(src)
	tokens := lexRust(src)

	var regions []region
	for _, t := range tokens {
		runeStart := b2r[t.start]
		runeEnd := b2r[t.end]
		if runeEnd <= runeStart {
			continue
		}
		var color string
		var bold bool
		switch t.kind {
		case tokKeyword:
			color, bold = colorKeyword, true
		case tokComment:
			color = colorComment
		case tokString:
			color = colorString
		case tokNumber:
			color = colorNumber
		case tokBuiltin:
			color = colorBuiltin
		}
		regions = append(regions, region{runeStart, runeEnd, color, bold})
	}

	return regions
}

// lexRust tokenizes Rust source and returns tokens for
// keywords, comments, strings, numbers, builtins, and lifetimes.
func lexRust(src string) []rustToken {
	var tokens []rustToken
	i := 0
	n := len(src)

	for i < n {
		c := src[i]

		// Whitespace — skip.
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}

		// Line comment: // to end of line.
		if c == '/' && i+1 < n && src[i+1] == '/' {
			start := i
			for i < n && src[i] != '\n' {
				i++
			}
			tokens = append(tokens, rustToken{start, i, tokComment})
			continue
		}

		// Block comment: /* ... */ with nesting.
		if c == '/' && i+1 < n && src[i+1] == '*' {
			start := i
			i += 2
			depth := 1
			for i < n && depth > 0 {
				if src[i] == '/' && i+1 < n && src[i+1] == '*' {
					depth++
					i += 2
				} else if src[i] == '*' && i+1 < n && src[i+1] == '/' {
					depth--
					i += 2
				} else {
					i++
				}
			}
			tokens = append(tokens, rustToken{start, i, tokComment})
			continue
		}

		// Byte string/char/raw-byte-string: b"...", b'x', br#"..."#
		if c == 'b' && i+1 < n {
			if src[i+1] == '"' {
				// Byte string b"..."
				start := i
				i += 2
				i = scanRustString(src, i, '"')
				tokens = append(tokens, rustToken{start, i, tokString})
				continue
			}
			if src[i+1] == '\'' {
				// Byte char b'x'
				start := i
				i += 2
				i = scanRustCharBody(src, i)
				tokens = append(tokens, rustToken{start, i, tokString})
				continue
			}
			if src[i+1] == 'r' {
				// Raw byte string br#"..."#
				if j := scanRustRawString(src, i+2); j > i+2 {
					start := i
					i = j
					tokens = append(tokens, rustToken{start, i, tokString})
					continue
				}
			}
		}

		// Raw string: r"..." or r#"..."#
		if c == 'r' && i+1 < n && (src[i+1] == '"' || src[i+1] == '#') {
			if j := scanRustRawString(src, i+1); j > i+1 {
				start := i
				i = j
				tokens = append(tokens, rustToken{start, i, tokString})
				continue
			}
		}

		// Regular string: "..."
		if c == '"' {
			start := i
			i++
			i = scanRustString(src, i, '"')
			tokens = append(tokens, rustToken{start, i, tokString})
			continue
		}

		// Char literal vs lifetime.
		if c == '\'' {
			start := i
			if tok, end := scanRustCharOrLifetime(src, i); tok >= 0 {
				tokens = append(tokens, rustToken{start, end, tok})
				i = end
				continue
			}
			// Not a char or lifetime — skip the tick.
			i++
			continue
		}

		// Number.
		if isDigit(c) || (c == '.' && i+1 < n && isDigit(src[i+1])) {
			start := i
			i = scanRustNumber(src, i)
			tokens = append(tokens, rustToken{start, i, tokNumber})
			continue
		}

		// Identifier, keyword, or builtin.
		if isIdentStart(c) {
			start := i
			for i < n && isIdentChar(src[i]) {
				i++
			}
			word := src[start:i]

			if rustKeywords[word] {
				tokens = append(tokens, rustToken{start, i, tokKeyword})
			} else if rustBuiltins[word] {
				tokens = append(tokens, rustToken{start, i, tokBuiltin})
			}
			continue
		}

		// Everything else: operators, punctuation, etc.
		i++
	}

	return tokens
}

// scanRustString scans past a regular string body (after the opening quote).
// Handles backslash escapes. Returns the byte offset past the closing quote.
func scanRustString(src string, i int, quote byte) int {
	n := len(src)
	for i < n {
		if src[i] == '\\' && i+1 < n {
			i += 2
			continue
		}
		if src[i] == quote {
			return i + 1
		}
		i++
	}
	return i // unterminated
}

// scanRustRawString scans a raw string starting at src[pos], which should
// point to the first '#' or '"' after 'r' (or 'br'). Returns the byte offset
// past the closing delimiter, or pos if no raw string was found.
func scanRustRawString(src string, pos int) int {
	n := len(src)
	i := pos

	// Count leading '#'s.
	hashes := 0
	for i < n && src[i] == '#' {
		hashes++
		i++
	}

	// Must have opening '"'.
	if i >= n || src[i] != '"' {
		return pos
	}
	i++ // skip opening '"'

	// Scan for closing '"' followed by the same number of '#'s.
	for i < n {
		if src[i] == '"' {
			// Check for matching '#'s.
			j := i + 1
			count := 0
			for j < n && count < hashes && src[j] == '#' {
				count++
				j++
			}
			if count == hashes {
				return j
			}
		}
		i++
	}
	return i // unterminated
}

// scanRustCharBody scans the body of a char literal after the opening
// single-tick (which has already been consumed along with any prefix like b).
// Returns the byte offset past the closing tick.
func scanRustCharBody(src string, i int) int {
	n := len(src)
	if i >= n {
		return i
	}
	// Escape sequence.
	if src[i] == '\\' {
		i++ // skip backslash
		if i < n {
			i++ // skip escaped char
			// For \x, \u{...} etc., consume until closing quote.
			for i < n && src[i] != '\'' {
				i++
			}
		}
	} else {
		i++ // skip the literal char
	}
	if i < n && src[i] == '\'' {
		return i + 1
	}
	return i
}

// scanRustCharOrLifetime disambiguates 'x' (char literal) from 'a (lifetime).
// Returns (tokString, end) for a char, (tokKeyword, end) for a lifetime,
// or (-1, 0) if neither.
func scanRustCharOrLifetime(src string, i int) (int, int) {
	n := len(src)
	if i >= n || src[i] != '\'' {
		return -1, 0
	}
	start := i
	i++ // skip opening tick

	if i >= n {
		return -1, 0
	}

	// Try char literal: 'x', '\n', '\x41', '\u{1F600}'
	if src[i] == '\\' {
		// Escape in char literal — scan to closing tick.
		j := i + 1
		if j >= n {
			return -1, 0
		}
		j++ // skip the escaped character
		// For multi-char escapes (\x41, \u{...}), consume until tick.
		for j < n && src[j] != '\'' && src[j] != '\n' {
			j++
		}
		if j < n && src[j] == '\'' {
			return tokString, j + 1
		}
		return -1, 0
	}

	// Non-escape: could be char 'x' or lifetime 'ident
	if !isIdentStart(src[i]) && src[i] != '_' {
		// Character like '(' or '0' — look for closing tick.
		j := i + 1
		if j < n && src[j] == '\'' {
			return tokString, j + 1
		}
		return -1, 0
	}

	// Identifier-like after tick: could be 'a' (char) or 'abc (lifetime).
	j := i
	for j < n && isIdentChar(src[j]) {
		j++
	}
	ident := src[i:j]

	// If followed by a closing tick and the ident is 1 char, it's a char literal.
	if j < n && src[j] == '\'' && len(ident) == 1 {
		return tokString, j + 1
	}

	// Otherwise it's a lifetime 'ident (no closing tick needed).
	if len(ident) > 0 {
		_ = start
		return tokKeyword, j
	}

	return -1, 0
}

// scanRustNumber scans a Rust numeric literal starting at src[i].
// Handles decimal, hex (0x), octal (0o), binary (0b), floats,
// underscore separators, and type suffixes (u8, i32, f64, etc.).
func scanRustNumber(src string, i int) int {
	n := len(src)

	// Leading dot: .5, .5e10
	if src[i] == '.' {
		i++
		for i < n && (isDigit(src[i]) || src[i] == '_') {
			i++
		}
		i = scanRustExponent(src, i)
		i = scanRustTypeSuffix(src, i)
		return i
	}

	// Hex, octal, binary.
	if src[i] == '0' && i+1 < n {
		switch src[i+1] {
		case 'x', 'X':
			i += 2
			for i < n && (isHexDigit(src[i]) || src[i] == '_') {
				i++
			}
			i = scanRustTypeSuffix(src, i)
			return i
		case 'o', 'O':
			i += 2
			for i < n && ((src[i] >= '0' && src[i] <= '7') || src[i] == '_') {
				i++
			}
			i = scanRustTypeSuffix(src, i)
			return i
		case 'b', 'B':
			i += 2
			for i < n && (src[i] == '0' || src[i] == '1' || src[i] == '_') {
				i++
			}
			i = scanRustTypeSuffix(src, i)
			return i
		}
	}

	// Decimal digits.
	for i < n && (isDigit(src[i]) || src[i] == '_') {
		i++
	}

	// Fractional part.
	if i < n && src[i] == '.' && (i+1 >= n || src[i+1] != '.') {
		// Check that the next char after '.' is a digit or '_' or 'e'/'E'
		// to distinguish from range syntax (1..10). Also allow trailing dot (1.).
		next := i + 1
		if next >= n || isDigit(src[next]) || src[next] == '_' || src[next] == 'e' || src[next] == 'E' {
			i++
			for i < n && (isDigit(src[i]) || src[i] == '_') {
				i++
			}
		}
	}

	i = scanRustExponent(src, i)
	i = scanRustTypeSuffix(src, i)
	return i
}

// scanRustExponent scans an optional exponent (e10, E-5, etc.).
func scanRustExponent(src string, i int) int {
	n := len(src)
	if i < n && (src[i] == 'e' || src[i] == 'E') {
		i++
		if i < n && (src[i] == '+' || src[i] == '-') {
			i++
		}
		for i < n && (isDigit(src[i]) || src[i] == '_') {
			i++
		}
	}
	return i
}

// scanRustTypeSuffix scans an optional Rust numeric type suffix
// like u8, i32, f64, usize, isize, etc.
func scanRustTypeSuffix(src string, i int) int {
	n := len(src)
	if i >= n {
		return i
	}

	// Type suffixes start with u, i, or f.
	if src[i] != 'u' && src[i] != 'i' && src[i] != 'f' {
		return i
	}

	// Try to match known suffixes.
	suffixes := []string{
		"u8", "u16", "u32", "u64", "u128", "usize",
		"i8", "i16", "i32", "i64", "i128", "isize",
		"f32", "f64",
	}
	for _, s := range suffixes {
		if i+len(s) <= n && src[i:i+len(s)] == s {
			// Make sure the suffix isn't part of a longer identifier.
			end := i + len(s)
			if end < n && isIdentChar(src[end]) {
				continue
			}
			return end
		}
	}
	return i
}
