// Edcolor syntax-colors source files in edwood using the spans file.
//
// Usage: set up fileHooks in exec.go to map extensions to "edcolor".
// Edcolor is invoked automatically when a matching file is opened.
//
// Edcolor reads the window tag to determine the filename, selects
// the appropriate lexer based on the file extension, reads the window
// body, lexes it, and writes span definitions to the window's spans
// file. Edwood renders the styled text through its rich.Frame engine.
//
// After the initial coloring, edcolor watches for edit events and
// re-colors with a short debounce delay. It exits when the window
// is closed or when the file extension has no registered lexer.
//
// The $winid environment variable (set automatically by edwood for
// B2 commands) identifies the target window.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
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

// lexers maps file extensions to tokenizer functions.
// Each tokenizer takes source text and returns colored regions.
var lexers = map[string]func(string) []region{
	".go":  tokenizeGo,
	".py":  tokenizePython,
	".rs":  tokenizeRust,
	".tex": tokenizeLatex,
	".sty": tokenizeLatex,
	".cls": tokenizeLatex,
}

type span struct {
	offset int
	length int
	color  string
	bg     string // highlight background; empty = default
	bold   bool
}

type region struct {
	runeStart, runeEnd int
	color              string
	bold               bool
}

const version = "edcolor v0.1.0"

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

	// Read the tag to determine the filename and extension.
	tokenize, ext := lexerForWindow(win)
	if tokenize == nil {
		// No lexer for this file type — exit silently.
		return
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

	lastBody := recolor(win, fsys, id, tokenize, nil)
	eventLoop(win, fsys, id, tokenize, lastBody, ext)
}

// lexerForWindow reads the window tag and returns the appropriate
// tokenizer for the file extension, or nil if none matches.
// It also returns the lowercased file extension.
func lexerForWindow(win *acme.Win) (func(string) []region, string) {
	tag, err := win.ReadAll("tag")
	if err != nil {
		return nil, ""
	}
	// The tag starts with the filename, followed by a space and
	// the rest of the tag line.
	name := string(tag)
	if i := strings.IndexByte(name, ' '); i >= 0 {
		name = name[:i]
	}
	ext := strings.ToLower(filepath.Ext(name))
	return lexers[ext], ext
}

// recolor reads the body, tokenizes it, and writes span definitions.
// If highlights is non-empty, match backgrounds are merged into the
// syntax spans. Returns the body text for caching.
func recolor(win *acme.Win, fsys *client.Fsys, id int, tokenize func(string) []region, highlights [][2]int) string {
	body, err := win.ReadAll("body")
	if err != nil {
		warn(fmt.Errorf("read body: %w", err))
		return ""
	}
	if len(body) == 0 {
		return ""
	}

	src := string(body)
	spans := colorize(src, tokenize)
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

// indentExts lists file extensions that support brace-aware auto-indent.
var indentExts = map[string]bool{
	".go": true,
	".rs": true,
}

// eventLoop watches for edit and selection events, re-coloring with debouncing.
// It exits when the window is closed (event channel closed).
func eventLoop(win *acme.Win, fsys *client.Fsys, id int, tokenize func(string) []region, lastBody string, ext string) {
	events := win.EventChan()
	var editTimer <-chan time.Time
	var selTimer <-chan time.Time
	var lastSel string
	var lastSelQ0, lastSelQ1 int
	var highlights [][2]int
	autoIndent := indentExts[ext]

	for {
		select {
		case e, ok := <-events:
			if !ok {
				return
			}
			switch e.C2 {
			case 'I':
				if autoIndent && e.C1 == 'K' {
					handleAutoIndent(win, e)
				}
				// Body edit — clear cached state and schedule re-coloring.
				lastBody = ""
				lastSel = ""
				highlights = nil
				editTimer = time.After(300 * time.Millisecond)
			case 'D':
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
			case 'x', 'X':
				// Intercept "Indent" for file types that support auto-indent.
				if indentExts[ext] && strings.TrimRight(string(e.Text), "\n") == "Indent" {
					autoIndent = !autoIndent
					break
				}
				win.WriteEvent(e)
			case 'l', 'L':
				win.WriteEvent(e)
			}
		case <-editTimer:
			editTimer = nil
			lastBody = recolor(win, fsys, id, tokenize, nil)
		case <-selTimer:
			selTimer = nil
			recolor(win, fsys, id, tokenize, highlights)
		}
	}
}

// handleAutoIndent processes keyboard insert events for brace-aware
// auto-indentation. On newline, it inserts the previous line's
// indentation (plus one tab after '{'). On '}', it removes one tab
// of indentation from the current line.
func handleAutoIndent(win *acme.Win, e *acme.Event) {
	text := string(e.Text)

	if !strings.HasSuffix(text, "\n") && text != "}" {
		return
	}

	body, err := win.ReadAll("body")
	if err != nil {
		return
	}
	runes := []rune(string(body))

	if strings.HasSuffix(text, "\n") {
		// Newline: insert indentation after the newline.
		// e.Q0 is where the newline was inserted, e.Q1 is after it.
		insertPos := e.Q1
		level, afterBrace := computeIndent(runes, e.Q0)
		if afterBrace {
			level++
		}
		if level > 0 {
			indent := strings.Repeat("\t", level)
			win.Addr("#%d", insertPos)
			win.Write("data", []byte(indent))
		}
	} else if text == "}" {
		// Close brace: remove one tab of indentation before the '}'.
		// e.Q1 is after the '}', so '}' is at e.Q1-1.
		bracePos := e.Q1 - 1
		// Walk backward from bracePos to find start of line.
		lineStart := bracePos
		for lineStart > 0 && runes[lineStart-1] != '\n' {
			lineStart--
		}
		// Check if everything between lineStart and bracePos is whitespace
		// and contains at least one tab.
		allWhitespace := true
		tabPos := -1
		for i := lineStart; i < bracePos; i++ {
			if runes[i] == '\t' {
				tabPos = i
			} else if runes[i] != ' ' {
				allWhitespace = false
				break
			}
		}
		if allWhitespace && tabPos >= 0 {
			// Delete the last tab before the brace.
			win.Addr("#%d,#%d", tabPos, tabPos+1)
			win.Write("data", nil)
		}
	}
}

// computeIndent examines the line ending at pos (the position where a
// newline was inserted) and returns the indentation level (number of
// leading tabs) and whether the last non-whitespace character is '{'.
func computeIndent(body []rune, pos int) (level int, afterBrace bool) {
	// Find start of the line containing pos.
	lineStart := pos
	for lineStart > 0 && body[lineStart-1] != '\n' {
		lineStart--
	}

	// Count leading tabs.
	i := lineStart
	for i < pos && body[i] == '\t' {
		level++
		i++
	}
	// Also skip spaces (mixed indent).
	for i < pos && body[i] == ' ' {
		i++
	}

	// Find last non-whitespace character on the line.
	last := pos - 1
	for last >= lineStart && (body[last] == ' ' || body[last] == '\t' || body[last] == '\r') {
		last--
	}
	if last >= lineStart && body[last] == '{' {
		afterBrace = true
	}

	return
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

// colorize tokenizes src using the given tokenizer and returns
// contiguous spans covering the entire text, with colors for
// syntactic elements and default color ("-") for everything else.
func colorize(src string, tokenize func(string) []region) []span {
	totalRunes := utf8.RuneCountInString(src)
	regions := tokenize(src)

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

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "edcolor: %v\n", err)
	os.Exit(1)
}

func warn(err error) {
	fmt.Fprintf(os.Stderr, "edcolor: %v\n", err)
}
