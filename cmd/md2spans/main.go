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
	"unicode/utf8"

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
// re-render. v1 uses 200 ms to match the "fast feedback" feel of
// preview mode without overloading on rapid typing. (cmd/edcolor
// uses 300 ms; the divergence is intentional — markdown
// rendering is cheaper than syntax tokenization for typical
// document sizes.)
//
// Exposed as a var (not a const) so tests can override it for
// fast watch-loop testing without sleeping for 200 ms.
var editDebounce = 200 * time.Millisecond

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

// run is the testable core of main. argv is the program's
// arguments (without the program name). getenv is the env-getter
// (os.Getenv in production, a closure in tests). stdout receives
// the help text on -h (POSIX convention); stderr receives error
// messages and usage on bad-args.
//
// Return values: 0 success, 1 runtime/environment error, 2
// invocation error (bad args).
func run(argv []string, getenv func(string) string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("md2spans", flag.ContinueOnError)
	fs.SetOutput(stderr)
	help := fs.Bool("h", false, "print help and exit")
	once := fs.Bool("once", false, "render once and exit")
	if err := fs.Parse(argv); err != nil {
		fmt.Fprint(stderr, usage)
		return 2
	}
	if *help {
		// POSIX convention: -h goes to stdout. Errors go to
		// stderr (the bad-args branch below).
		fmt.Fprint(stdout, usage)
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

// bodyReader is the renderOnce contract for fetching the source
// to render. *acme.Win satisfies this via its ReadAll method.
// Tests inject a fake to feed canned bodies without 9P.
type bodyReader interface {
	ReadAll(file string) ([]byte, error)
}

// spansOpener is the writeSpans contract for opening the
// window's spans file. *client.Fsys's Open returns a
// *client.Fid which satisfies io.WriteCloser; this interface
// lets tests inject a fake that captures writes without 9P.
type spansOpener interface {
	OpenSpans(winid int) (io.WriteCloser, error)
}

// fsysOpener is the production spansOpener: a thin wrapper over
// *client.Fsys that opens "<winid>/spans" for write.
type fsysOpener struct {
	fsys *client.Fsys
}

func (o fsysOpener) OpenSpans(winid int) (io.WriteCloser, error) {
	return o.fsys.Open(fmt.Sprintf("%d/spans", winid), plan9.OWRITE)
}

// attachAndRender opens the window, mounts the acme service for
// the spans file, renders once, and (unless once is true)
// watches the body for edits with debounce, re-rendering on each.
//
// The *acme.Win is released via win.CloseFiles before return.
// *client.Fsys has no explicit close in 9fans.net/go/plan9/client;
// its underlying connection lives until the process exits.
func attachAndRender(winid int, once bool, stderr io.Writer) error {
	win, err := acme.Open(winid, nil)
	if err != nil {
		return fmt.Errorf("open window %d: %w", winid, err)
	}
	defer win.CloseFiles()

	fsys, err := client.MountService("acme")
	if err != nil {
		return fmt.Errorf("mount acme: %w", err)
	}
	opener := fsysOpener{fsys: fsys}

	if err := renderOnce(win, opener, winid); err != nil {
		fmt.Fprintf(stderr, "md2spans: render: %v\n", err)
		// Continue to watch loop anyway; transient errors shouldn't
		// take down the watcher.
	}

	if once {
		return nil
	}

	// Open the event file and re-enable the menu so user
	// commands (Undo / Redo / Put) stay in the tag — same
	// pattern as cmd/edcolor. Errors here are unusual but
	// non-fatal: log and proceed.
	if err := win.OpenEvent(); err != nil {
		fmt.Fprintf(stderr, "md2spans: open event: %v\n", err)
	}
	if err := win.Ctl("menu"); err != nil {
		fmt.Fprintf(stderr, "md2spans: ctl menu: %v\n", err)
	}

	watchEdits(win, opener, winid, stderr)
	return nil
}

// renderOnce reads the body, parses it, and writes the
// spans-protocol output to the window's spans file.
//
// Race note: the body can change between ReadAll and the spans
// write. Spans are computed against the pre-edit body; if the
// user typed during the render, the styled offsets will be
// off-by-N runes for the post-edit body. The next debounced
// edit triggers another render that corrects the displacement.
// v1 accepts the transient mismatch; an interlock here would
// require coordination with edwood's event stream that doesn't
// fit the v1 architecture.
func renderOnce(reader bodyReader, opener spansOpener, winid int) error {
	body, err := reader.ReadAll("body")
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	src := string(body)
	spans := Parse(src)
	totalRunes := utf8.RuneCountInString(src)
	return writeSpans(opener, winid, spans, totalRunes)
}

// watchEdits is the v1 body-edit watch loop. Mirrors
// cmd/edcolor's eventLoop simplified to just body edits + a
// debounce. Selection events, command interception, and
// auto-indent are NOT handled by md2spans v1.
//
// The loop exits when the window's event channel closes (the
// window was deleted).
//
// Implementation note: editTimer is a *time.Timer whose channel
// (`timerC`) is consumed when the timer fires. Each new edit
// calls Reset on the existing timer rather than allocating a
// new one — the previous time.After-based implementation
// allocated a fresh runtime timer for every edit, leaving the
// prior one to expire and be GC'd. Reset reuses the timer's
// underlying state.
func watchEdits(win *acme.Win, opener spansOpener, winid int, stderr io.Writer) {
	events := win.EventChan()
	editTimer := newStoppedTimer()
	defer editTimer.Stop()

	for {
		select {
		case e, ok := <-events:
			if !ok {
				return
			}
			switch e.C2 {
			case 'I', 'D':
				resetTimer(editTimer, editDebounce)
			case 'x', 'X', 'l', 'L':
				// Pass user-issued commands through unchanged
				// so edwood handles them normally.
				win.WriteEvent(e)
			}
		case <-editTimer.C:
			if err := renderOnce(win, opener, winid); err != nil {
				fmt.Fprintf(stderr, "md2spans: render after edit: %v\n", err)
			}
		}
	}
}

// newStoppedTimer returns a *time.Timer that has not been
// armed; the channel C will not fire until the caller calls
// Reset. Used to set up the debounce timer cleanly without
// a sentinel "first edit" branch in the watch loop.
func newStoppedTimer() *time.Timer {
	t := time.NewTimer(time.Hour)
	if !t.Stop() {
		<-t.C
	}
	return t
}

// resetTimer arms t to fire after d, draining any pending fire
// from a previous arming first. Stop+drain pattern per the
// time.Timer docs: without it, a previous fire that hasn't been
// consumed yet would race the Reset and produce a spurious tick.
func resetTimer(t *time.Timer, d time.Duration) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	t.Reset(d)
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
func writeSpans(opener spansOpener, winid int, spans []Span, totalRunes int) error {
	fid, err := opener.OpenSpans(winid)
	if err != nil {
		return fmt.Errorf("open spans: %w", err)
	}
	defer fid.Close()

	if _, err := fid.Write([]byte("c\n")); err != nil {
		return fmt.Errorf("write clear: %w", err)
	}

	return writeChunked(fid, FormatSpans(spans, totalRunes))
}

// maxChunk caps each Twrite to stay under the typical 9P msize
// (8192). Splitting at newline boundaries means each chunk is a
// complete set of span directives that parseSpanMessage parses
// independently.
const maxChunk = 4000

// writeChunked writes payload to fid in chunks of at most
// maxChunk bytes, each ending at a newline so parseSpanMessage
// receives complete directive sets per Twrite. Returns an error
// if the payload contains a single line longer than maxChunk
// (which v1's emitter never produces, but is a guarded
// invariant).
func writeChunked(fid io.Writer, payload string) error {
	for len(payload) > 0 {
		end := len(payload)
		if end > maxChunk {
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
