// dirthumb renders directory-listing thumbnails into a window's
// spans file.
//
// Sibling of cmd/md2spans: invoked from edwood (typically as a
// B2 command in the window tag, with $winid set), reads the
// window's body as a directory listing, and emits a `b` (image
// box) directive with placement=below for each line whose entry
// is a recognized image filename. The source filename text
// stays visible; the image is painted on a ghost line below it.
//
// dirthumb watches the body for edits and re-renders on each
// change (debounced 200 ms). With -once it renders the current
// body and exits.
//
// dirthumb does NOT touch edwood code; it is purely a 9P client
// that uses the spans-protocol surface that md2spans already
// exercises. Image filename extensions: .png .jpg .jpeg .gif
// .webp.
package main

import (
	"flag"
	"fmt"
	"io"
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

const usage = `usage: dirthumb [-h] [-once]

Reads a directory listing from the window identified by $winid
(set by edwood when dirthumb is launched as a B2 command),
identifies image filenames by extension, and writes spans-protocol
output that paints a thumbnail below each image entry's line.

By default dirthumb watches the body for edits and re-renders
on each change (debounced 200 ms). With -once it renders the
current body and exits.

  -h        print this help and exit
  -once     render once and exit; do not watch for edits
`

// editDebounce is the delay between a body edit and the next
// re-render. Matches md2spans's 200 ms.
var editDebounce = 200 * time.Millisecond

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

// run is the testable core of main. Return values: 0 success,
// 1 runtime/environment error, 2 invocation error.
func run(argv []string, getenv func(string) string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("dirthumb", flag.ContinueOnError)
	fs.SetOutput(stderr)
	help := fs.Bool("h", false, "print help and exit")
	once := fs.Bool("once", false, "render once and exit")
	if err := fs.Parse(argv); err != nil {
		fmt.Fprint(stderr, usage)
		return 2
	}
	if *help {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if fs.NArg() != 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}

	winidStr := getenv("winid")
	if winidStr == "" {
		fmt.Fprintf(stderr, "dirthumb: $winid is not set; launch from edwood as a B2 command\n")
		return 1
	}
	winid, err := strconv.Atoi(winidStr)
	if err != nil {
		fmt.Fprintf(stderr, "dirthumb: $winid is not an integer (%q): %v\n", winidStr, err)
		return 1
	}

	if err := attachAndRender(winid, *once, stderr); err != nil {
		fmt.Fprintf(stderr, "dirthumb: %v\n", err)
		return 1
	}
	return 0
}

// bodyReader is the renderOnce contract for fetching the
// source. *acme.Win satisfies this via its ReadAll method.
type bodyReader interface {
	ReadAll(file string) ([]byte, error)
}

// spansOpener is the writeSpans contract for opening the
// window's spans file.
type spansOpener interface {
	OpenSpans(winid int) (io.WriteCloser, error)
}

// fsysOpener is the production spansOpener: a thin wrapper
// over *client.Fsys that opens "<winid>/spans" for write.
type fsysOpener struct {
	fsys *client.Fsys
}

func (o fsysOpener) OpenSpans(winid int) (io.WriteCloser, error) {
	return o.fsys.Open(fmt.Sprintf("%d/spans", winid), plan9.OWRITE)
}

// attachAndRender opens the window, mounts the acme service,
// renders once, and (unless once is true) watches the body
// for edits with debounce.
func attachAndRender(winid int, once bool, stderr io.Writer) error {
	win, err := acme.Open(winid, nil)
	if err != nil {
		return fmt.Errorf("open window %d: %w", winid, err)
	}
	defer win.CloseFiles()

	if err := win.Fprintf("tag", " Plain"); err != nil {
		fmt.Fprintf(stderr, "dirthumb: append Plain to tag: %v\n", err)
	}

	fsys, err := client.MountService("acme")
	if err != nil {
		return fmt.Errorf("mount acme: %w", err)
	}
	opener := fsysOpener{fsys: fsys}

	if err := renderOnce(win, opener, winid); err != nil {
		fmt.Fprintf(stderr, "dirthumb: render: %v\n", err)
	}

	if once {
		return nil
	}

	if err := win.OpenEvent(); err != nil {
		fmt.Fprintf(stderr, "dirthumb: open event: %v\n", err)
	}
	if err := win.Ctl("menu"); err != nil {
		fmt.Fprintf(stderr, "dirthumb: ctl menu: %v\n", err)
	}

	watchEdits(win, opener, winid, stderr)
	return nil
}

// renderOnce reads the body and the window name (from the tag),
// scans the body as a directory listing, and writes the
// spans-protocol output to the window's spans file.
func renderOnce(reader bodyReader, opener spansOpener, winid int) error {
	body, err := reader.ReadAll("body")
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	dirPath, err := readDirPath(reader)
	if err != nil {
		return fmt.Errorf("resolve directory path: %w", err)
	}
	src := string(body)
	spans := scanDirectory(src, dirPath)
	totalRunes := utf8.RuneCountInString(src)
	return writeSpans(opener, winid, spans, totalRunes)
}

// readDirPath extracts the window's directory path from its
// tag. The tag's first whitespace-terminated token is the
// window name (e.g., "/Users/paul/dev/edwood/" for a
// directory window). If the name does not end in "/", we
// return its parent directory — dirthumb is intended for
// directory windows but degrades gracefully on a file window.
func readDirPath(reader bodyReader) (string, error) {
	tag, err := reader.ReadAll("tag")
	if err != nil {
		return "", fmt.Errorf("read tag: %w", err)
	}
	tagStr := string(tag)
	idx := strings.IndexAny(tagStr, " \t\n")
	var name string
	if idx < 0 {
		name = tagStr
	} else {
		name = tagStr[:idx]
	}
	if name == "" {
		return "", fmt.Errorf("window tag has no name")
	}
	if strings.HasSuffix(name, "/") {
		return name, nil
	}
	return filepath.Dir(name) + "/", nil
}

// watchEdits is the body-edit watch loop with debounce.
// Mirrors md2spans/main.go:watchEdits.
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
				win.WriteEvent(e)
			}
		case <-editTimer.C:
			if err := renderOnce(win, opener, winid); err != nil {
				fmt.Fprintf(stderr, "dirthumb: render after edit: %v\n", err)
			}
		}
	}
}

// newStoppedTimer returns a *time.Timer that has not been
// armed; the channel C will not fire until the caller calls
// Reset.
func newStoppedTimer() *time.Timer {
	t := time.NewTimer(time.Hour)
	if !t.Stop() {
		<-t.C
	}
	return t
}

// resetTimer arms t to fire after d, draining any pending
// fire from a previous arming first.
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
// Issues two writes: a `c\n` clear (which also resets the
// window's styledSuppressed flag), then the chunked span
// directives.
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

// maxChunk caps each Twrite to stay under the typical 9P
// msize (8192). Splitting at newline boundaries means each
// chunk is a complete set of span directives.
const maxChunk = 4000

// writeChunked writes payload to fid in chunks of at most
// maxChunk bytes, each ending at a newline. dirthumb emits
// no region directives so chunking is simpler than
// md2spans's variant — every newline is a valid split.
func writeChunked(fid io.Writer, payload string) error {
	for len(payload) > 0 {
		end, err := nextChunkEnd(payload)
		if err != nil {
			return err
		}
		if _, err := fid.Write([]byte(payload[:end])); err != nil {
			return fmt.Errorf("write spans: %w", err)
		}
		payload = payload[end:]
	}
	return nil
}

// nextChunkEnd returns the byte position just past the end
// of the next chunk: the latest newline at-or-before
// maxChunk, or the next newline past maxChunk if none falls
// before it. Errors if the payload contains a single line
// longer than maxChunk with no newline anywhere.
func nextChunkEnd(payload string) (int, error) {
	if len(payload) <= maxChunk {
		return len(payload), nil
	}
	lastNL := -1
	for i := 0; i <= maxChunk && i < len(payload); i++ {
		if payload[i] == '\n' {
			lastNL = i
		}
	}
	if lastNL >= 0 {
		return lastNL + 1, nil
	}
	nextNL := strings.IndexByte(payload[maxChunk:], '\n')
	if nextNL < 0 {
		return 0, fmt.Errorf("dirthumb spans payload contains a line longer than %d bytes", maxChunk)
	}
	return maxChunk + nextNL + 1, nil
}
