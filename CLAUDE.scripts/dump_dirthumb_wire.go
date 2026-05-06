// dump_dirthumb_wire prints the exact wire bytes dirthumb would
// write for the given directory. Run with `go run ...` and
// pass the directory path as the only arg. Output: the `c\n`
// clear plus all chunks, joined.
//
// Used to verify that dirthumb's emit is producing what we
// expect for a real on-disk directory's listing.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// One-off script: copy a minimal scanner here so we don't need
// to reach into cmd/dirthumb's package-private types.

var imageExts = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
}

type span struct {
	offset, length int
	payload        string
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: dump_dirthumb_wire <directory>")
		os.Exit(2)
	}
	dir := os.Args[1]
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}
	body := lsLikeAcme(dir)
	fmt.Printf("=== body (%d bytes) ===\n%s\n", len(body), body)

	spans := scan(body, dir)
	fmt.Printf("=== %d image spans ===\n", len(spans))
	for _, s := range spans {
		fmt.Printf("  offset=%d length=%d payload=%q\n", s.offset, s.length, s.payload)
	}

	totalRunes := len([]rune(body))
	wire := formatSpans(spans, totalRunes)
	fmt.Printf("=== wire output (%d bytes) ===\nc\n%s", len(wire)+2, wire)
}

func lsLikeAcme(dir string) string {
	out, err := exec.Command("ls", dir).Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ls:", err)
		os.Exit(1)
	}
	return string(out)
}

func scan(body, dir string) []span {
	var out []span
	runes := []rune(body)
	tokStart := -1
	var cur strings.Builder
	runeIdx := 0
	flush := func(end int) {
		if tokStart < 0 {
			return
		}
		name := cur.String()
		if isImage(name) {
			out = append(out, span{
				offset:  tokStart,
				length:  end - tokStart,
				payload: "image:" + filepath.Join(dir, name),
			})
		}
		tokStart = -1
		cur.Reset()
	}
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			flush(runeIdx)
			runeIdx++
			continue
		}
		if tokStart < 0 {
			tokStart = runeIdx
		}
		if r == '\\' && i+1 < len(runes) {
			cur.WriteRune(runes[i+1])
			i++
			runeIdx += 2
			continue
		}
		cur.WriteRune(r)
		runeIdx++
	}
	flush(runeIdx)
	return out
}

func isImage(name string) bool {
	if name == "" || strings.HasSuffix(name, "/") {
		return false
	}
	return imageExts[strings.ToLower(filepath.Ext(name))]
}

func formatSpans(spans []span, total int) string {
	var b strings.Builder
	cursor := 0
	for _, s := range spans {
		if s.offset > cursor {
			fmt.Fprintf(&b, "s %d %d -\n", cursor, s.offset-cursor)
		}
		fmt.Fprintf(&b, "b %d %d 100 0 - - placement=below %s\n", s.offset, s.length, s.payload)
		cursor = s.offset + s.length
	}
	if cursor < total {
		fmt.Fprintf(&b, "s %d %d -\n", cursor, total-cursor)
	}
	return b.String()
}
