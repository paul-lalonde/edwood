package main

import "strings"

// Python keywords (3.12+).
var pyKeywords = map[string]bool{
	"False": true, "None": true, "True": true,
	"and": true, "as": true, "assert": true, "async": true, "await": true,
	"break": true, "class": true, "continue": true,
	"def": true, "del": true,
	"elif": true, "else": true, "except": true,
	"finally": true, "for": true, "from": true,
	"global": true, "if": true, "import": true, "in": true, "is": true,
	"lambda": true, "nonlocal": true, "not": true,
	"or": true, "pass": true, "raise": true, "return": true,
	"try": true, "while": true, "with": true, "yield": true,
}

// Python builtin functions and types.
var pyBuiltins = map[string]bool{
	// Types
	"bool": true, "bytearray": true, "bytes": true, "complex": true,
	"dict": true, "float": true, "frozenset": true, "int": true,
	"list": true, "memoryview": true, "object": true, "set": true,
	"slice": true, "str": true, "tuple": true, "type": true,
	// Functions
	"abs": true, "all": true, "any": true, "ascii": true,
	"bin": true, "breakpoint": true, "callable": true, "chr": true,
	"classmethod": true, "compile": true, "delattr": true, "dir": true,
	"divmod": true, "enumerate": true, "eval": true, "exec": true,
	"filter": true, "format": true, "getattr": true, "globals": true,
	"hasattr": true, "hash": true, "help": true, "hex": true,
	"id": true, "input": true, "isinstance": true, "issubclass": true,
	"iter": true, "len": true, "locals": true, "map": true,
	"max": true, "min": true, "next": true, "oct": true,
	"open": true, "ord": true, "pow": true, "print": true,
	"property": true, "range": true, "repr": true, "reversed": true,
	"round": true, "setattr": true, "sorted": true, "staticmethod": true,
	"sum": true, "super": true, "vars": true, "zip": true,
	// Constants
	"NotImplemented": true, "Ellipsis": true, "__import__": true,
}

// tokenizePython lexes src as Python source and returns colored regions.
func tokenizePython(src string) []region {
	b2r := byteToRuneIndex(src)
	tokens := lexPython(src)

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

// --- Python lexer ---

const (
	tokKeyword = iota
	tokComment
	tokString
	tokNumber
	tokBuiltin
)

type pyToken struct {
	start, end int // byte offsets
	kind       int
}

// isValidPrefix reports whether s is a valid Python string prefix.
func isValidPrefix(s string) bool {
	switch strings.ToLower(s) {
	case "r", "u", "f", "b", "rb", "br", "rf", "fr":
		return true
	}
	return false
}

func isDigit(c byte) bool      { return c >= '0' && c <= '9' }
func isHexDigit(c byte) bool   { return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') }
func isIdentStart(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' }
func isIdentChar(c byte) bool  { return isIdentStart(c) || isDigit(c) }

// lexPython tokenizes Python source and returns tokens for
// keywords, comments, strings, numbers, and builtins.
func lexPython(src string) []pyToken {
	var tokens []pyToken
	i := 0
	n := len(src)

	for i < n {
		c := src[i]

		// Whitespace — skip.
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}

		// Comment: # to end of line.
		if c == '#' {
			start := i
			for i < n && src[i] != '\n' {
				i++
			}
			tokens = append(tokens, pyToken{start, i, tokComment})
			continue
		}

		// Bare string (no prefix).
		if c == '\'' || c == '"' {
			start := i
			i = scanString(src, i)
			tokens = append(tokens, pyToken{start, i, tokString})
			continue
		}

		// Number.
		if isDigit(c) || (c == '.' && i+1 < n && isDigit(src[i+1])) {
			start := i
			i = scanNumber(src, i)
			tokens = append(tokens, pyToken{start, i, tokNumber})
			continue
		}

		// Identifier, keyword, builtin, or string prefix.
		if isIdentStart(c) {
			start := i
			for i < n && isIdentChar(src[i]) {
				i++
			}
			word := src[start:i]

			// Check if this identifier is a string prefix.
			if i < n && (src[i] == '\'' || src[i] == '"') && isValidPrefix(word) {
				i = scanString(src, i)
				tokens = append(tokens, pyToken{start, i, tokString})
				continue
			}

			if pyKeywords[word] {
				tokens = append(tokens, pyToken{start, i, tokKeyword})
			} else if pyBuiltins[word] {
				tokens = append(tokens, pyToken{start, i, tokBuiltin})
			}
			continue
		}

		// Everything else: operators, punctuation, etc.
		i++
	}

	return tokens
}

// scanString scans a quoted string starting at src[i] (which must be
// ' or "). It handles triple-quoted and single-quoted strings with
// backslash escapes. Returns the byte offset past the closing quote.
func scanString(src string, i int) int {
	n := len(src)
	quote := src[i]
	i++

	// Triple-quoted string?
	if i+1 < n && src[i] == quote && src[i+1] == quote {
		i += 2 // skip two more quotes
		for i < n {
			if src[i] == '\\' && i+1 < n {
				i += 2
				continue
			}
			if i+2 < n && src[i] == quote && src[i+1] == quote && src[i+2] == quote {
				return i + 3
			}
			i++
		}
		return i // unterminated
	}

	// Single-line string.
	for i < n {
		if src[i] == '\\' && i+1 < n {
			i += 2
			continue
		}
		if src[i] == quote {
			return i + 1
		}
		if src[i] == '\n' {
			return i // unterminated at newline
		}
		i++
	}
	return i
}

// scanNumber scans a numeric literal starting at src[i].
// Handles int, float, hex, octal, binary, and complex (j suffix).
func scanNumber(src string, i int) int {
	n := len(src)

	// Leading dot: .5, .5e10, etc.
	if src[i] == '.' {
		i++
		for i < n && (isDigit(src[i]) || src[i] == '_') {
			i++
		}
		i = scanExponent(src, i)
		i = scanComplexSuffix(src, i)
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
			return i
		case 'o', 'O':
			i += 2
			for i < n && ((src[i] >= '0' && src[i] <= '7') || src[i] == '_') {
				i++
			}
			return i
		case 'b', 'B':
			i += 2
			for i < n && (src[i] == '0' || src[i] == '1' || src[i] == '_') {
				i++
			}
			return i
		}
	}

	// Decimal digits.
	for i < n && (isDigit(src[i]) || src[i] == '_') {
		i++
	}

	// Fractional part.
	if i < n && src[i] == '.' {
		i++
		for i < n && (isDigit(src[i]) || src[i] == '_') {
			i++
		}
	}

	i = scanExponent(src, i)
	i = scanComplexSuffix(src, i)
	return i
}

func scanExponent(src string, i int) int {
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

func scanComplexSuffix(src string, i int) int {
	if i < len(src) && (src[i] == 'j' || src[i] == 'J') {
		i++
	}
	return i
}
