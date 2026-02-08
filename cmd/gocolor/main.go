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
	colorKeyword = "#0000cc" // blue
	colorString  = "#008000" // green
	colorComment = "#808080" // gray
	colorNumber  = "#cc6600" // orange
	colorBuiltin = "#008080" // teal
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

	recolor(win, fsys, id)
	eventLoop(win, fsys, id)
}

// recolor reads the body, tokenizes it, and writes span definitions.
func recolor(win *acme.Win, fsys *client.Fsys, id int) {
	body, err := win.ReadAll("body")
	if err != nil {
		warn(fmt.Errorf("read body: %w", err))
		return
	}
	if len(body) == 0 {
		return
	}

	spans := colorize(string(body))
	if len(spans) == 0 {
		return
	}

	if err := writeSpans(fsys, id, spans); err != nil {
		warn(err)
	}
}

// eventLoop watches for edit events and re-colors with debouncing.
// It exits when the window is closed (event channel closed).
func eventLoop(win *acme.Win, fsys *client.Fsys, id int) {
	events := win.EventChan()
	var timer <-chan time.Time

	for {
		select {
		case e, ok := <-events:
			if !ok {
				return
			}
			switch e.C2 {
			case 'I', 'D':
				// Body edit — schedule re-coloring after debounce.
				timer = time.After(300 * time.Millisecond)
			case 'x', 'X', 'l', 'L':
				win.WriteEvent(e)
			}
		case <-timer:
			timer = nil
			recolor(win, fsys, id)
		}
	}
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
		if s.bold {
			line = fmt.Sprintf("%d %d %s bold\n", s.offset, s.length, s.color)
		} else {
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
			spans = append(spans, span{cursor, r.runeStart - cursor, "-", false})
		}
		spans = append(spans, span{r.runeStart, r.runeEnd - r.runeStart, r.color, r.bold})
		cursor = r.runeEnd
	}
	if cursor < totalRunes {
		spans = append(spans, span{cursor, totalRunes - cursor, "-", false})
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
