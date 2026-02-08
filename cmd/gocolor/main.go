// Gocolor syntax-colors Go source files in edwood using the spans file.
//
// Usage: middle-click "gocolor" in a window containing Go source.
//
// Gocolor reads the window body, lexes it as Go source, and writes
// span definitions to the window's spans file. Edwood renders the
// styled text through its rich.Frame engine.
//
// After the initial coloring, gocolor watches for edit events and
// re-colors with a short debounce delay. It exits when the window
// is closed.
//
// The $winid environment variable (set automatically by edwood for
// B2 commands) identifies the target window.
package main

import (
	"flag"
	"fmt"
	"go/scanner"
	"go/token"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"9fans.net/go/acme"
	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
)

// Color scheme.
const (
	colorKeyword   = "#0000cc" // blue
	colorString    = "#008000" // green
	colorComment   = "#808080" // gray
	colorNumber    = "#cc6600" // orange
	colorBuiltin   = "#008080" // teal
	colorHighlight = "#f0f4ff" // very light blue background for matching occurrences
)

// Predeclared Go identifiers that get special coloring.
var builtins = map[string]bool{
	// Types
	"bool": true, "byte": true, "complex64": true, "complex128": true,
	"error": true, "float32": true, "float64": true, "int": true,
	"int8": true, "int16": true, "int32": true, "int64": true,
	"rune": true, "string": true, "uint": true, "uint8": true,
	"uint16": true, "uint32": true, "uint64": true, "uintptr": true,
	"any": true, "comparable": true,
	// Functions
	"append": true, "cap": true, "clear": true, "close": true,
	"complex": true, "copy": true, "delete": true, "imag": true,
	"len": true, "make": true, "max": true, "min": true,
	"new": true, "panic": true, "print": true, "println": true,
	"real": true, "recover": true,
	// Constants
	"true": true, "false": true, "nil": true, "iota": true,
}

type span struct {
	offset int
	length int
	color  string
	bg     string // highlight background; empty = default
	bold   bool
}

const version = "gocolor v0.2.0 (highlight-on-select)"

var verbose = flag.Bool("v", false, "print version and verbose output")

func main() {
	flag.Parse()
	if *verbose {
		fmt.Println(version)
	}
	id, err := getWinID()
	if err != nil {
		fatal(err)
	}

	win, err := acme.Open(id, nil)
	if err != nil {
		fatal(fmt.Errorf("open window: %w", err))
	}

	// Force the event file open now (EventChan opens it lazily).
	// This sets filemenu=false in edwood. We then re-enable it
	// with "menu" so Undo/Redo/Put stay in the tag.
	win.OpenEvent()
	win.Ctl("menu")

	// 9P filesystem for spans writing (needs manual chunking
	// to stay within message size limits).
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

// writeSpans writes span definitions to the window's spans file,
// chunking to stay within 9P message size limits. Each chunk is a
// self-contained set of contiguous span definitions.
func writeSpans(fsys *client.Fsys, id int, spans []span) error {
	fid, err := fsys.Open(fmt.Sprintf("%d/spans", id), plan9.OWRITE)
	if err != nil {
		return fmt.Errorf("open spans: %w", err)
	}
	defer fid.Close()

	// Keep each chunk well under the typical 9P msize (8192+).
	// Each chunk must contain complete lines so the server can
	// parse them as a valid region update.
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

// colorize lexes src as Go source and returns contiguous spans
// covering the entire text, with colors for syntactic elements
// and default color ("-") for everything else.
func colorize(src string) []span {
	b2r := byteToRuneIndex(src)
	totalRunes := utf8.RuneCountInString(src)

	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))

	var s scanner.Scanner
	// Suppress error printing; color what we can.
	s.Init(file, []byte(src), func(token.Position, string) {}, scanner.ScanComments)

	type region struct {
		runeStart, runeEnd int
		color              string
		bold               bool
	}
	var regions []region

	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}

		// Skip auto-inserted semicolons; they have no source text.
		if tok == token.SEMICOLON && lit == "\n" {
			continue
		}

		byteOff := int(pos) - file.Base()
		byteEnd := byteOff
		if lit != "" {
			byteEnd += len(lit)
		} else {
			byteEnd += len(tok.String())
		}

		color, bold := tokenStyle(tok, lit)
		if color == "" {
			continue
		}

		runeStart := b2r[byteOff]
		runeEnd := b2r[byteEnd]
		if runeEnd > runeStart {
			regions = append(regions, region{runeStart, runeEnd, color, bold})
		}
	}

	// Build contiguous spans covering the entire source.
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

// byteToRuneIndex builds a lookup table mapping byte offsets to rune
// offsets. Only entries at rune-start byte positions are valid.
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

// tokenStyle returns the color and bold flag for a Go token.
// An empty color means use default (skip coloring this token).
func tokenStyle(tok token.Token, lit string) (color string, bold bool) {
	switch {
	case tok.IsKeyword():
		return colorKeyword, true
	case tok == token.COMMENT:
		return colorComment, false
	case tok == token.STRING, tok == token.CHAR:
		return colorString, false
	case tok == token.INT, tok == token.FLOAT, tok == token.IMAG:
		return colorNumber, false
	case tok == token.IDENT && builtins[lit]:
		return colorBuiltin, false
	default:
		return "", false
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "gocolor: %v\n", err)
	os.Exit(1)
}

func warn(err error) {
	fmt.Fprintf(os.Stderr, "gocolor: %v\n", err)
}
