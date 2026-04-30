// md2spans renders markdown styling into a window's spans file.
//
// Sibling of cmd/edcolor: invoked from edwood (typically as a B2
// command in the window tag, with $winid set) and watches the
// window's body for edits, re-rendering on each change.
//
// v1 covers paragraphs, emphasis (italic / bold / bold-italic),
// and inline links (rendered as colored runs, URL dropped).
// Anything else (headings, lists, code blocks, etc.) renders as
// literal text with no styling. See cmd/md2spans/md2spans.design.md
// for the full v1 scope and the future Phase 3 round map.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"9fans.net/go/acme"
	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
)

const usage = `usage: md2spans [-h] [-once]

Reads markdown from the window identified by $winid (set by
edwood when md2spans is launched as a B2 command), parses it,
and writes spans-protocol output to the window's spans file.

By default md2spans watches the body for edits and re-renders
on each change (debounced 200 ms). With -once it renders the
current body and exits.

  -h        print this help and exit
  -once     render once and exit; do not watch for edits
`

// editDebounce is the delay between a body edit and the next
// re-render. Mirrors cmd/edcolor's 300 ms; v1 uses 200 ms to
// match the "fast feedback" feel of preview mode without
// overloading on rapid typing.
var editDebounce = 200 * time.Millisecond

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stderr))
}

// run is the testable core of main. argv is the program's
// arguments (without the program name). getenv is the env-getter
// (os.Getenv in production, a closure in tests). stderr is where
// usage and error messages are written.
//
// Return values: 0 success, 1 runtime/environment error, 2
// invocation error (bad args).
func run(argv []string, getenv func(string) string, stderr io.Writer) int {
	fs := flag.NewFlagSet("md2spans", flag.ContinueOnError)
	fs.SetOutput(stderr)
	help := fs.Bool("h", false, "print help and exit")
	once := fs.Bool("once", false, "render once and exit")
	if err := fs.Parse(argv); err != nil {
		fmt.Fprint(stderr, usage)
		return 2
	}
	if *help {
		fmt.Fprint(stderr, usage)
		return 0
	}
	if fs.NArg() != 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}

	winidStr := getenv("winid")
	if winidStr == "" {
		fmt.Fprintf(stderr, "md2spans: $winid is not set; launch from edwood as a B2 command\n")
		return 1
	}
	winid, err := strconv.Atoi(winidStr)
	if err != nil {
		fmt.Fprintf(stderr, "md2spans: $winid is not an integer (%q): %v\n", winidStr, err)
		return 1
	}

	if err := attachAndRender(winid, *once, stderr); err != nil {
		fmt.Fprintf(stderr, "md2spans: %v\n", err)
		return 1
	}
	return 0
}

// attachAndRender opens the window, mounts the acme service for
// the spans file, renders once, and (unless once is true)
// watches the body for edits with debounce, re-rendering on each.
func attachAndRender(winid int, once bool, stderr io.Writer) error {
	win, err := acme.Open(winid, nil)
	if err != nil {
		return fmt.Errorf("open window %d: %w", winid, err)
	}

	fsys, err := client.MountService("acme")
	if err != nil {
		return fmt.Errorf("mount acme: %w", err)
	}

	if err := renderOnce(win, fsys, winid); err != nil {
		fmt.Fprintf(stderr, "md2spans: render: %v\n", err)
		// Continue to watch loop anyway; transient errors shouldn't
		// take down the watcher.
	}

	if once {
		return nil
	}

	// Open the event file and re-enable the menu so user
	// commands (Undo / Redo / Put) stay in the tag — same
	// pattern as cmd/edcolor.
	win.OpenEvent()
	win.Ctl("menu")

	watchEdits(win, fsys, winid, stderr)
	return nil
}

// renderOnce reads the body, parses it, and writes the
// spans-protocol output to the window's spans file.
func renderOnce(win *acme.Win, fsys *client.Fsys, winid int) error {
	body, err := win.ReadAll("body")
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	spans := Parse(string(body))
	return writeSpans(fsys, winid, spans)
}

// watchEdits is the v1 body-edit watch loop. Mirrors
// cmd/edcolor's eventLoop simplified to just body edits + a
// debounce. Selection events, command interception, and
// auto-indent are NOT handled by md2spans v1.
//
// The loop exits when the window's event channel closes (the
// window was deleted).
func watchEdits(win *acme.Win, fsys *client.Fsys, winid int, stderr io.Writer) {
	events := win.EventChan()
	var editTimer <-chan time.Time
	for {
		select {
		case e, ok := <-events:
			if !ok {
				return
			}
			switch e.C2 {
			case 'I', 'D':
				editTimer = time.After(editDebounce)
			case 'x', 'X', 'l', 'L':
				// Pass user-issued commands through unchanged
				// so edwood handles them normally.
				win.WriteEvent(e)
			}
		case <-editTimer:
			editTimer = nil
			if err := renderOnce(win, fsys, winid); err != nil {
				fmt.Fprintf(stderr, "md2spans: render after edit: %v\n", err)
			}
		}
	}
}

// writeSpans replaces the window's spans with the supplied list.
// Issues TWO writes to the spans file:
//
//  1. A `c\n` (clear). The spans-protocol parser at
//     spanparse.go:parseSpanMessage requires that a `c` directive
//     be the only command in its write — it returns isClear=true
//     immediately on seeing one and ignores any following lines.
//     The clear ALSO resets the `styledSuppressed` flag on the
//     window (xfid.go:611), so a window the user previously took
//     out of styled mode (e.g. via the Markdown tag command) will
//     re-enter styled mode on the subsequent span write.
//
//  2. The `s` lines (one per Span), chunked under the 9P msize
//     limit at newline boundaries. parseSpanMessage parses these
//     in a single batch and the auto-switch to styled mode at
//     xfid.go:642 fires.
//
// If `spans` is empty, writeSpans only issues the clear. The
// window goes plain.
func writeSpans(fsys *client.Fsys, winid int, spans []Span) error {
	fid, err := fsys.Open(fmt.Sprintf("%d/spans", winid), plan9.OWRITE)
	if err != nil {
		return fmt.Errorf("open spans: %w", err)
	}
	defer fid.Close()

	if _, err := fid.Write([]byte("c\n")); err != nil {
		return fmt.Errorf("write clear: %w", err)
	}

	payload := FormatSpans(spans)
	const maxChunk = 4000
	for len(payload) > 0 {
		end := len(payload)
		if end > maxChunk {
			// Cap at maxChunk and back off to the last newline so
			// each write is a complete set of directives.
			end = maxChunk
			for end > 0 && payload[end-1] != '\n' {
				end--
			}
			if end == 0 {
				return fmt.Errorf("spans payload contains a line longer than %d bytes", maxChunk)
			}
		}
		if _, err := fid.Write([]byte(payload[:end])); err != nil {
			return fmt.Errorf("write spans: %w", err)
		}
		payload = payload[end:]
	}
	return nil
}
