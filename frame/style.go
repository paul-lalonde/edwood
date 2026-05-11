package frame

import (
	"github.com/rjkroege/edwood/draw"
)

// StyleRun is a contiguous run of Len runes that share a Style.
// A slice of StyleRuns whose Lens sum to K applies to K runes.
type StyleRun struct {
	Len   int
	Style Style
}

// Style is the per-run attribute bundle the frame consumes during
// layout and rendering. The field set is the Slice A subset
// (coloring only); Slices B and C grow the struct with font and
// replaced-element fields. IsZero() is kept in sync with the
// current field set.
type Style struct {
	Fg draw.Image
	Bg draw.Image
}

// IsZero reports whether s is the default style. Callers use this
// to detect plain text and take the fast path.
func (s Style) IsZero() bool {
	return s.Fg == nil && s.Bg == nil
}

// ReplacedKind classifies a replaced element (image, code block,
// table, fixed box). Declared in Slice A so the type is available
// for forward references; consumed in Slice C.
type ReplacedKind int

const (
	ReplacedNone ReplacedKind = iota
	ReplacedImage
	ReplacedCodeBlock
	ReplacedTable
	ReplacedFixedBox
)
