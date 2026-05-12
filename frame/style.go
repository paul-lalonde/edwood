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
// layout and rendering. Kind is the bitmask discriminator; the
// data fields are meaningful iff their corresponding Kind bit is
// set.
//
// The field set is the Slice A subset (Kind, Fg, Bg). Slice B
// adds FontIdx and the bold/italic/underline/font-idx bits to
// Kind. Slice C adds the replaced-element fields, block-context
// bits, and HOffset.
type Style struct {
	Kind Kind

	// Meaningful iff Kind & KindColored != 0.
	Fg draw.Image
	Bg draw.Image
}

// Kind is a bitmask of active style attributes. KindPlain is the
// zero value and means "upstream defaults" — IsPlain() returns
// true for any Style whose Kind is KindPlain. Bit positions are
// stable across slices; later slices add the bits they need.
type Kind uint

const KindPlain Kind = 0

const (
	// Slice A
	KindColored Kind = 1 << iota // Fg / Bg meaningful

	// Slice B (typographic variation that doesn't change line
	// height). All three are bare flag tokens in the published
	// spans protocol.
	KindBold   // bold weight; renders with the frame's bold font
	KindItalic // italic angle; renders with the italic font
	KindHidden // glyph is not painted (frame still paints bg)

	// Future bits (KindFontIdx for FontIdx-driven font picking,
	// KindReplaced + Replaced* fields for Slice C, block-context
	// bits) take the next iota steps in this block.
)

// IsPlain reports whether s carries no styling — i.e., a frame
// asked to render this Style produces output identical to
// upstream's plain Insert. Equivalent to s.Kind == KindPlain.
// Callers use this to take the fast path.
func (s Style) IsPlain() bool { return s.Kind == KindPlain }

// ReplacedKind classifies a replaced element. The Replaced*
// fields (added in Slice C) are gated by Kind & KindReplaced;
// this enum names the subtype. Declared in Slice A so the type
// is available for forward references; consumed in Slice C.
type ReplacedKind int

const (
	ReplacedNone ReplacedKind = iota
	ReplacedImage
	ReplacedCodeBlock
	ReplacedTable
	ReplacedFixedBox
)
