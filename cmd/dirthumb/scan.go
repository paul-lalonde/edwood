package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// imageExts is the set of filename extensions dirthumb
// recognizes as image files. Lowercased; lookup compares
// the lowercased trailing extension. Matches md2spans's
// inline-image extension set (parser.go round 4).
var imageExts = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
}

// defaultThumbWidth is the pixel width used for thumbnail
// boxes. Matches the user's example `width=100px`. Height
// is left to scale proportionally from the natural image.
//
// dirthumb encodes the width in the payload (`width=N`)
// and leaves the wire WIDTH/HEIGHT slots at 0, mirroring
// md2spans's emission pattern. The renderer's
// applyImagePayload reads `width=N` from the payload and
// sets ImageWidth, which drives proportional height
// scaling in imageBoxDimensions.
const defaultThumbWidth = 100

// scanDirectory tokenizes the body as a columnar directory
// listing (whitespace-separated, with `\<rune>` as an
// escape for whitespace inside filenames) and emits a
// SpanBox for each token whose unescaped value is an image
// filename. dirPath is the absolute directory the listing
// represents — image filenames are resolved against it to
// produce the box's `image:<absolute-path>` payload.
//
// The Span covers the token's on-screen runes (including
// any `\` escape characters); the payload uses the
// unescaped name. placement=below leaves the source text
// visible above the rendered thumbnail.
func scanDirectory(body string, dirPath string) []Span {
	var spans []Span
	for _, tok := range tokenize(body) {
		if !isImageName(tok.name) {
			continue
		}
		spans = append(spans, Span{
			Kind:         SpanBox,
			Offset:       tok.runeStart,
			Length:       tok.runeLen,
			BoxWidth:     0,
			BoxHeight:    0,
			BoxPlacement: "below",
			BoxPayload:   fmt.Sprintf("image:%s width=%d",
				filepath.Join(dirPath, tok.name),
				defaultThumbWidth),
		})
	}
	return spans
}

// token represents one whitespace-bounded entry in the
// directory listing. runeStart/runeLen describe the
// on-screen position (including escape backslashes); name
// is the unescaped filename (the literal on-disk name).
type token struct {
	runeStart int
	runeLen   int
	name      string
}

// tokenize walks body rune-by-rune and emits tokens
// separated by ASCII whitespace (space, tab, newline,
// carriage return). Inside a token, `\` escapes the next
// rune: `\ ` becomes a literal space in `name` (and adds
// 2 to the token's runeLen). A trailing `\` at EOF or
// before a whitespace stop is treated as a literal `\`
// for the on-screen length but dropped from the unescaped
// name (matching plan9's `quote(3)`-style convention,
// where a stray trailing `\` is degenerate input — we
// don't classify such tokens as images anyway since the
// extension check fails).
//
// Tokens are emitted in order; empty runs (consecutive
// whitespace) produce no token.
func tokenize(body string) []token {
	var out []token
	var cur strings.Builder
	tokStart := -1
	runeIdx := 0
	flush := func(endRunes int) {
		if tokStart < 0 {
			return
		}
		out = append(out, token{
			runeStart: tokStart,
			runeLen:   endRunes - tokStart,
			name:      cur.String(),
		})
		tokStart = -1
		cur.Reset()
	}
	runes := []rune(body)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if isWS(r) {
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

// isWS reports whether r is one of the whitespace runes
// that terminate a directory-listing token. Matches
// strings.Fields's default split rules restricted to the
// ASCII set acme uses for column padding.
func isWS(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r':
		return true
	}
	return false
}

// isImageName reports whether name (the unescaped
// filename) names a recognized image. Returns false for
// directory entries (trailing `/`) and for names whose
// extension is not in imageExts.
func isImageName(name string) bool {
	if name == "" || strings.HasSuffix(name, "/") {
		return false
	}
	return imageExts[strings.ToLower(filepath.Ext(name))]
}
