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
//
// At Phase 2.1, run only handles arg parsing and $winid lookup.
// The acme-attach + watch-loop logic is added in row 2.5; for
// now the placeholder path returns 0 immediately after
// validating $winid.
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

	// Phase 2.1 scaffold: $winid validated, args parsed. The
	// acme-attach / read-body / write-spans pipeline lands in
	// rows 2.2-2.4 (parser + emit) and row 2.5 (watch loop).
	// Suppress unused-variable warnings now that we're returning
	// early.
	_ = winid
	_ = once
	return 0
}
