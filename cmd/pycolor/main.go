// Pycolor syntax-colors Python source files in edwood using the spans file.
//
// Usage: middle-click "pycolor" in a window containing Python source.
//
// Pycolor reads the window body, lexes it as Python source, and writes
// span definitions to the window's spans file. Edwood renders the
// styled text through its rich.Frame engine.
//
// After the initial coloring, pycolor watches for edit events and
// re-colors with a short debounce delay. It exits when the window
// is closed.
//
// The $winid environment variable (set automatically by edwood for
// B2 commands) identifies the target window.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"9fans.net/go/acme"
	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
)

// Color scheme (matches gocolor).
const (
	colorKeyword   = "#0000cc" // blue
	colorString    = "#008000" // green
	colorComment   = "#808080" // gray
	colorNumber    = "#cc6600" // orange
	colorBuiltin   = "#008080" // teal
	colorHighlight = "#f0f4ff" // very light blue background for matching occurrences
)

// Python keywords (3.12+).
var keywords = map[string]bool{
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
var builtins = map[string]bool{
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

type span struct {
	offset int
	length int
	color  string
	bg     string // highlight background; empty = default
	bold   bool
}

func main() {
	id, err := getWinID()
	if err != nil {
		fatal(err)
	}

	win, err := acme.Open(id, nil)
	if err != nil {
		fatal(fmt.Errorf("open window: %w", err))
	}

	win.OpenEvent()
	win.Ctl("menu")

	fsys, err := client.MountService("acme")
	if err != nil {
		fatal(fmt.Errorf("mount acme: %w", err))
	}

	lastBody := recolor(win, fsys, id, nil)
	eventLoop(win, fsys, id, lastBody)
}

// recolor reads the body, tokenizes it, and writes span definitions.
// If highlights is non-empty, match backgrounds are merged into the
// syntax spans. Returns the body text for caching.
func recolor(win *acme.Win, fsys *client.Fsys, id int, highlights [][2]int) string {
	body, err := win.ReadAll("body")
	if err != nil {
		warn(fmt.Errorf("read body: %w", err))
		return ""
	}
	if len(body) == 0 {
		return ""
	}

	src := string(body)
	spans := colorize(src)
	if len(spans) == 0 {
		return src
	}

	if len(highlights) > 0 {
		spans = applyHighlights(spans, highlights)
	}

	if err := writeSpans(fsys, id, spans); err != nil {
		warn(err)
	}
	return src
}

// eventLoop watches for edit and selection events, re-coloring with debouncing.
// It exits when the window is closed (event channel closed).
func eventLoop(win *acme.Win, fsys *client.Fsys, id int, lastBody string) {
	events := win.EventChan()
	var editTimer <-chan time.Time
	var selTimer <-chan time.Time
	var lastSel string
	var lastSelQ0, lastSelQ1 int
	var highlights [][2]int

	for {
		select {
		case e, ok := <-events:
			if !ok {
				return
			}
			switch e.C2 {
			case 'I', 'D':
				// Body edit — clear cached state and schedule re-coloring.
				lastBody = ""
				lastSel = ""
				highlights = nil
				editTimer = time.After(300 * time.Millisecond)
			case 'S':
				// Body selection changed.
				if lastBody == "" {
					break
				}
				runes := []rune(lastBody)
				q0, q1 := e.Q0, e.Q1
				if q0 < 0 || q1 > len(runes) || q0 > q1 {
					break
				}
				sel := string(runes[q0:q1])
				if utf8.RuneCountInString(sel) >= 2 {
					if sel != lastSel || q0 != lastSelQ0 || q1 != lastSelQ1 {
						lastSel = sel
						lastSelQ0 = q0
						lastSelQ1 = q1
						highlights = findMatches(lastBody, sel, q0, q1)
						selTimer = time.After(100 * time.Millisecond)
					}
				} else if lastSel != "" {
					// Selection cleared or too short — remove highlights.
					lastSel = ""
					highlights = nil
					selTimer = time.After(100 * time.Millisecond)
				}
			case 'x', 'X', 'l', 'L':
				win.WriteEvent(e)
			}
		case <-editTimer:
			editTimer = nil
			lastBody = recolor(win, fsys, id, nil)
		case <-selTimer:
			selTimer = nil
			recolor(win, fsys, id, highlights)
		}
	}
}

// findMatches finds all rune-offset occurrences of sel in body,
// excluding the selection itself at [selQ0, selQ1).
func findMatches(body, sel string, selQ0, selQ1 int) [][2]int {
	runes := []rune(body)
	selRunes := []rune(sel)
	selLen := len(selRunes)
	var matches [][2]int

	for i := 0; i <= len(runes)-selLen; i++ {
		match := true
		for j := 0; j < selLen; j++ {
			if runes[i+j] != selRunes[j] {
				match = false
				break
			}
		}
		if match {
			mq0, mq1 := i, i+selLen
			if mq0 == selQ0 && mq1 == selQ1 {
				continue // skip the selection itself
			}
			matches = append(matches, [2]int{mq0, mq1})
		}
	}
	return matches
}

// applyHighlights merges highlight backgrounds into syntax spans.
// Both spans and highlights must be sorted by offset.
func applyHighlights(spans []span, highlights [][2]int) []span {
	if len(highlights) == 0 {
		return spans
	}

	var result []span
	hi := 0 // index into highlights

	for _, s := range spans {
		sStart := s.offset
		sEnd := s.offset + s.length

		// Advance past highlights that end before this span.
		for hi < len(highlights) && highlights[hi][1] <= sStart {
			hi++
		}

		cursor := sStart
		for h := hi; h < len(highlights) && highlights[h][0] < sEnd; h++ {
			hStart := highlights[h][0]
			hEnd := highlights[h][1]

			// Clamp to span boundaries.
			if hStart < sStart {
				hStart = sStart
			}
			if hEnd > sEnd {
				hEnd = sEnd
			}

			// Segment before highlight.
			if hStart > cursor {
				result = append(result, span{cursor, hStart - cursor, s.color, s.bg, s.bold})
			}
			// Highlighted segment.
			result = append(result, span{hStart, hEnd - hStart, s.color, colorHighlight, s.bold})
			cursor = hEnd
		}
		// Remaining segment after last highlight in this span.
		if cursor < sEnd {
			result = append(result, span{cursor, sEnd - cursor, s.color, s.bg, s.bold})
		}
	}
	return result
}

func getWinID() (int, error) {
	s := os.Getenv("winid")
	if s == "" {
		return 0, fmt.Errorf("$winid not set")
	}
	return strconv.Atoi(s)
}

func writeSpans(fsys *client.Fsys, id int, spans []span) error {
	fid, err := fsys.Open(fmt.Sprintf("%d/spans", id), plan9.OWRITE)
	if err != nil {
		return fmt.Errorf("open spans: %w", err)
	}
	defer fid.Close()

	const maxChunk = 4000

	var buf strings.Builder
	for _, s := range spans {
		var line string
		switch {
		case s.bg != "" && s.bold:
			line = fmt.Sprintf("%d %d %s %s bold\n", s.offset, s.length, s.color, s.bg)
		case s.bg != "":
			line = fmt.Sprintf("%d %d %s %s\n", s.offset, s.length, s.color, s.bg)
		case s.bold:
			line = fmt.Sprintf("%d %d %s bold\n", s.offset, s.length, s.color)
		default:
			line = fmt.Sprintf("%d %d %s\n", s.offset, s.length, s.color)
		}

		if buf.Len()+len(line) > maxChunk && buf.Len() > 0 {
			if _, err := fid.Write([]byte(buf.String())); err != nil {
				return fmt.Errorf("write spans: %w", err)
			}
			buf.Reset()
		}
		buf.WriteString(line)
	}

	if buf.Len() > 0 {
		if _, err := fid.Write([]byte(buf.String())); err != nil {
			return fmt.Errorf("write spans: %w", err)
		}
	}

	return nil
}

// colorize lexes src as Python source and returns contiguous spans
// covering the entire text.
func colorize(src string) []span {
	b2r := byteToRuneIndex(src)
	totalRunes := utf8.RuneCountInString(src)

	tokens := lexPython(src)

	type region struct {
		runeStart, runeEnd int
		color              string
		bold               bool
	}
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

	var spans []span
	cursor := 0
	for _, r := range regions {
		if r.runeStart > cursor {
			spans = append(spans, span{cursor, r.runeStart - cursor, "-", "", false})
		}
		spans = append(spans, span{r.runeStart, r.runeEnd - r.runeStart, r.color, "", r.bold})
		cursor = r.runeEnd
	}
	if cursor < totalRunes {
		spans = append(spans, span{cursor, totalRunes - cursor, "-", "", false})
	}

	return spans
}

func byteToRuneIndex(s string) []int {
	idx := make([]int, len(s)+1)
	ri := 0
	for bi := range s {
		idx[bi] = ri
		ri++
	}
	idx[len(s)] = ri
	return idx
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

			if keywords[word] {
				tokens = append(tokens, pyToken{start, i, tokKeyword})
			} else if builtins[word] {
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

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "pycolor: %v\n", err)
	os.Exit(1)
}

func warn(err error) {
	fmt.Fprintf(os.Stderr, "pycolor: %v\n", err)
}
