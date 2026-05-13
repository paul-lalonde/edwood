package frame

import (
	"image"
	"sync"

	"github.com/rjkroege/edwood/draw"
)

// TODO(rjk): Make this into a struct of colours?
const (
	ColBack = iota
	ColHigh
	ColBord
	ColText
	ColHText
	NumColours

	frtickw = 3
)

// SelectScrollUpdater are those frame.Frame methods offered to
// frame.Select callbacks.
type SelectScrollUpdater interface {
	// GetFrameFillStatus returns a snapshot of the capacity of the frame.
	GetFrameFillStatus() FrameFillStatus

	// Charofpt returns the index of the closest rune whose image's upper
	// left corner is up and to the left of pt.
	Charofpt(pt image.Point) int

	// DefaultFontHeight returns the height of the Frame's default font.
	// TODO(rjk): Reconsider this for Frames containing many styles.
	DefaultFontHeight() int

	// Delete deletes from the Frame the text between p0 and p1; p1 points at
	// the first rune beyond the deletion. Returns the number of whole lines
	// removed.
	//
	// Delete will clear a selection or tick if present but not put it back.
	Delete(int, int) int

	// Insert inserts r into Frame f starting at rune index p0.
	// If a NUL (0) character is inserted, chaos will ensue. Tabs
	// and newlines are handled by the library, but all other characters,
	// including control characters, are just displayed. For example,
	// backspaces are printed; to erase a character, use Delete.
	//
	// Insert will remove the selection or tick  if present but update selection offsets.
	Insert([]rune, int) bool
	InsertByte([]byte, int) bool

	// InsertWithStyle inserts r at p0 with per-rune styling. If
	// styles is nil or every StyleRun in styles is Style.IsPlain(),
	// the implementation takes the fast path identical to Insert.
	// Otherwise each StyleRun applies to that many consecutive
	// runes; the sum of Lens must equal len(r) (panics on
	// mismatch). See style.go for the Style/StyleRun shape.
	InsertWithStyle([]rune, int, []StyleRun) bool

	IsLastLineFull() bool
	Rect() image.Rectangle

	// TextOccupiedHeight returns the height of the region in the frame
	// occupied by boxes (which in the future could be of varying height)
	// that is closest to the height of rectangle r such that only unclipped
	// boxes fit in the returned height. If r.Dy() exeeds the total height of
	// the current boxes, then returns the height of current set of boxes.
	TextOccupiedHeight(r image.Rectangle) int
}

// Frame is the public interface to a frame of text. Unlike the C implementation,
// new Frame instances should be created with NewFrame.
type Frame interface {
	SelectScrollUpdater

	// Maxtab sets the maximum size of a tab in pixels.
	Maxtab(m int)

	// GetMaxtab returns the current maximum size of a tab in pixels.
	GetMaxtab() int

	// Init prepares the Frame for the display of text in rectangle r.
	// Frame f will reuse previously set font, colours, tab width and
	// destination image for drawing unless these are overridden with
	// one or more instances of the OptColors, OptBackground
	// OptFont or OptMaxTab option settings.
	//
	// The background (OptBackground setter) may be null to allow
	// calling the other routines to maintain the model in, for example,
	// an obscured window.
	//
	// Changing the background or font will force the tick to be
	// recreated.
	Init(image.Rectangle, ...OptionClosure)

	// Clear frees the internal structures associated with f, permitting
	// another Init or SetRects on the Frame. It does not clear the
	// associated display. If f is to be deallocated, the associated Font and
	// Image must be freed separately. The resize argument should be non-zero
	// if the frame is to be redrawn with a different font; otherwise the
	// frame will maintain some data structures associated with the font.
	//
	// To resize a Frame, use Clear and Init and then Insert to recreate the
	// display. If a Frame is being moved but not resized, that is, if the
	// shape of its containing rectangle is unchanged, it is sufficient to
	// use Draw to copy the containing rectangle from the old to the new
	// location and then call SetRects to establish the new geometry.
	Clear(bool)

	// Ptofchar returns the location of the upper left corner of the p'th
	// rune, starting from 0, in the receiver Frame. If the Frame holds
	// fewer than p runes, Ptofchar returns the location of the upper right
	// corner of the last character in the Frame
	Ptofchar(int) image.Point

	// Redraw redraws the background of the Frame where the Frame is inside
	// enclosing. Frame is responsible for drawing all of the pixels inside
	// enclosing though may fill less than enclosing with text. (In particular,
	// a margin may be added and the rectangle occupied by text is always
	// a multiple of the fixed line height.)
	// TODO(rjk): Modify this function to redraw the text as well and stop having
	// the drawing of text strings be a side-effect of Insert, Delete, etc.
	// TODO(rjk): Draw text to the bottom of enclosing as opposed to filling the
	// bottom partial text row with blank.
	//
	// Note: this function is not part of the documented libframe entrypoints and
	// was not invoked from Edwood code. Consequently, I am repurposing the name.
	// Future changes will have this function able to clear the Frame and draw the
	// entire box model.
	Redraw(enclosing image.Rectangle)

	// GetSelectionExtent returns the rune offsets of the selection maintained by
	// the Frame.
	GetSelectionExtent() (int, int)

	// Select takes ownership of the mouse channel to update the selection
	// so long as a button is down in downevent. Selection stops when the
	// staring point buttondown is altered. getmorelines is a callback provided
	// by the caller to provide n additional lines on demand to the specified frame.
	// The implementation of the callback must use the Frame instance provided
	// in place of the one that Select is invoked on.
	//
	// Select returns the selection range in the Frame.
	Select(*draw.Mousectl, *draw.Mouse, func(SelectScrollUpdater, int)) (int, int)

	// SelectOpt makes a selection in the same fashion as Select but does it in a
	// temporary way with the specified text colours fg, bg.
	SelectOpt(*draw.Mousectl, *draw.Mouse, func(SelectScrollUpdater, int), draw.Image, draw.Image) (int, int)

	// DrawSel repaints a section of the frame, delimited by rune
	// positions p0 and p1, either with plain background or entirely
	// highlighted, according to the flag highlighted, managing the tick
	// appropriately. The point pt0 is the geometrical location of p0 on the
	// screen; like all of the selection-helper routines' Point arguments, it
	// must be a value generated by Ptofchar.
	//
	// Clarification of semantics: the point of this routine is to redraw the
	// state of the Frame with selection p0, p1. In particular, this requires
	// updating f.p0 and f.p1 so that other entry points (e.g. Insert) can (transparently) remove
	// a pre-existing selection.
	//
	// Note that the original C code does not remove the pre-existing selection where
	// this code does draw the selection to the p0, p1. I (rjk) believe that this is a better
	// API.
	//
	// DrawSel does the minimum work needed to clear a highlight and (in particular)
	// multiple calls to DrawSel with highlighted false will be cheap.
	// TODO(rjk): DrawSel does more drawing work than necessary.
	DrawSel(image.Point, int, int, bool)

	// SetStyleRange re-styles the runes already in the frame at
	// rune offsets [p0, p1) using styles. Sum of StyleRun.Lens
	// must equal p1-p0; panics on mismatch or on out-of-range
	// arguments. SetStyleRange does not move sp0/sp1 of the
	// selection. The affected region is repainted synchronously;
	// the caller is responsible for display.Flush().
	SetStyleRange(p0, p1 int, styles []StyleRun)

	// SetOriginYOffset clips the top of the frame's first visible
	// line by yPx pixels. Meaningful only when the first visible
	// rune is a tall replaced element; in Slice A this is a stub
	// — Set is a no-op and Get always returns 0. Real behavior
	// arrives in Slice C alongside replaced-element rendering.
	SetOriginYOffset(yPx int)
	GetOriginYOffset() int

	// ── debug overlays ──────────────────────────────────────

	// ToggleBoxOutlines flips the per-paint box-outline overlay.
	// When on, paintBox draws a 1-pixel Purpleblue rectangle around
	// every painted box's rect immediately after its glyph. The
	// "Box" tag-bar command toggles this. Returns the new state.
	ToggleBoxOutlines() bool

	// SetAfterPaintHook registers a callback fired once per
	// public paint-causing call (Insert / InsertByte /
	// InsertWithStyle / SetStyleRange) after the box model has
	// been updated and the frame lock has been released. The
	// hook may freely call back into Frame methods. nil clears.
	// The "Spans" tag-bar command uses this to overlay span
	// boundaries on top of frame paint.
	SetAfterPaintHook(fn func())

	// DrawOutlineRect draws a 1-pixel outline of r in color col.
	// No fill. Used by debug overlays (the "Spans" hook outlines
	// each non-plain region).
	DrawOutlineRect(r image.Rectangle, col draw.Image)

	// LineHAt returns the line height (in pixels) of the box
	// containing rune offset p. Used by Text.paintSpansOverlay
	// to size span outlines correctly on variable-height lines
	// — heading lines are taller than body lines, so the
	// overlay rect needs the per-line height, not the frame's
	// default font height. Returns DefaultFontHeight for an
	// out-of-range p or an empty box list.
	LineHAt(p int) int
}

// TODO(rjk): Consider calling this SetMaxtab?
func (f *frameimpl) Maxtab(m int) {
	f.lk.Lock()
	defer f.lk.Unlock()

	f.maxtab = m
}

func (f *frameimpl) GetMaxtab() int { return f.maxtab }

// ToggleBoxOutlines flips the box-outline debug overlay and
// returns the new state. Lazy-allocates the Purpleblue outline
// color on first enable.
func (f *frameimpl) ToggleBoxOutlines() bool {
	f.lk.Lock()
	defer f.lk.Unlock()
	f.showBoxOutlines = !f.showBoxOutlines
	if f.showBoxOutlines && f.boxOutlineColor == nil && f.display != nil {
		if img, err := f.display.AllocImage(image.Rect(0, 0, 1, 1), f.display.ScreenImage().Pix(), true, draw.Purpleblue); err == nil {
			f.boxOutlineColor = img
		}
	}
	return f.showBoxOutlines
}

// SetAfterPaintHook registers fn as the per-paint callback.
// Public paint-causing entry points (Insert / InsertByte /
// InsertWithStyle / SetStyleRange) fire fn after releasing
// f.lk, so the hook may freely call back into Frame methods.
// nil clears.
func (f *frameimpl) SetAfterPaintHook(fn func()) {
	f.lk.Lock()
	defer f.lk.Unlock()
	f.afterPaintHook = fn
}

// LineHAt walks f.box, finds the box containing rune p, and
// returns its LineH. Falls back to defaultfontheight when p is
// out of range, the box list is empty, or the matching box's
// LineH is unset (zero). Pure reader of the post-relayout box
// model.
func (f *frameimpl) LineHAt(p int) int {
	f.lk.Lock()
	defer f.lk.Unlock()
	if p < 0 {
		p = 0
	}
	for _, b := range f.box {
		l := nrune(b)
		if p < l {
			if b.LineH > 0 {
				return b.LineH
			}
			return f.defaultfontheight
		}
		p -= l
	}
	return f.defaultfontheight
}

// DrawOutlineRect draws a 1-pixel border at r in color col on
// the frame's background image. Clipped to f.rect so debug
// overlays never bleed outside the frame.
func (f *frameimpl) DrawOutlineRect(r image.Rectangle, col draw.Image) {
	if f.background == nil || col == nil {
		return
	}
	r = r.Intersect(f.rect)
	if r.Empty() {
		return
	}
	// Top, bottom, left, right.
	f.background.Draw(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), col, nil, image.Point{})
	f.background.Draw(image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), col, nil, image.Point{})
	f.background.Draw(image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), col, nil, image.Point{})
	f.background.Draw(image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y), col, nil, image.Point{})
}

// FrameFillStatus is a snapshot of the capacity of the Frame.
type FrameFillStatus struct {
	Nchars         int
	Nlines         int
	Maxlines       int
	MaxPixelHeight int
}

func (f *frameimpl) GetFrameFillStatus() FrameFillStatus {
	f.lk.Lock()
	defer f.lk.Unlock()
	return FrameFillStatus{
		Nchars:         f.nchars,
		Nlines:         f.nlines,
		Maxlines:       f.maxlines,
		MaxPixelHeight: f.maxlines * f.defaultfontheight,
	}
}

func (f *frameimpl) TextOccupiedHeight(r image.Rectangle) int {
	f.lk.Lock()
	defer f.lk.Unlock()

	return f.textoccupiedheightimpl(r)
}

func (f *frameimpl) textoccupiedheightimpl(r image.Rectangle) int {
	f.lk.Lock()
	defer f.lk.Unlock()

	// TODO(rjk): To support multiple different fonts at once in a Frame,
	// this will have to be extended to be the sum of the height of the boxes
	// less than r.Dy
	if r.Dy() > f.nlines*f.defaultfontheight {
		return f.nlines * f.defaultfontheight
	}
	return (r.Dy() / f.defaultfontheight) * f.defaultfontheight
}

func (f *frameimpl) IsLastLineFull() bool {
	f.lk.Lock()
	defer f.lk.Unlock()
	return f.lastlinefull
}

func (f *frameimpl) Rect() image.Rectangle {
	f.lk.Lock()
	defer f.lk.Unlock()
	return f.rect
}

// TODO(rjk): no need for this to have public fields.
// TODO(rjk): Could fold Minwid && Bc into Nrune.
type frbox struct {
	Wid    int    // In pixels. Fixed large size for layout box.
	Nrune  int    // Number of runes in Ptr or -1 for special layout boxes (tab, newline)
	Ptr    []byte // UTF-8 string in this box.
	Bc     rune   // The kind of special layout box: '\n' or '\t'
	Minwid byte
	// Style is the per-box attribute bundle. Box equality (via
	// reflect.DeepEqual) requires Style equality, so plain boxes
	// continue to compare equal across the upstream and styled
	// insert paths. See style.go for the field's semantics.
	Style Style

	// Per-box layout fields (frame-rendering-spec §2.1, B2.2).
	// Populated by (*frameimpl).relayoutFrom after any box-
	// model mutation. R1 added the fields with safe defaults;
	// R2 populates them from a single forward layout pass; R3
	// migrates walk callers to read these instead of accumulating
	// their own pt.
	//   - X: box's left-edge X (absolute screen coord; matches
	//     pt.X that the historical walks produce).
	//   - Y: top Y of the line containing this box.
	//   - LineH: height of the box's line in pixels. Until R4,
	//     always defaultfontheight.
	//   - LineA: ascent of the box's line. Until R5, equals
	//     LineH (Ascent stand-in).
	X     int
	Y     int
	LineH int
	LineA int
}

// Helpful code for debugging reentrancy.
// TODO(rjk): Remove when we really don't have bugs.
// type debugginglock struct {
// 	reallock sync.Mutex
// 	havelock bool
// }
//
// func (m *debugginglock) Lock() {
// 	if m.havelock {
// 		panic("attempt to reentrantly enter locked frameimpl")
// 	}
// 	m.reallock.Lock()
// 	m.havelock = true
// }
//
// func (m *debugginglock) Unlock() {
// 	m.havelock = false
// 	m.reallock.Unlock()
// }

// TODO(rjk): It might make sense to group frameimpl into context (e.g.
// fonts, etc.) and the actual boxes. At any rate, it's worth thinking
// carefully about the data structures and how they should really be put
// together.
type frameimpl struct {
	lk sync.Mutex
	// lk debugginglock

	font       draw.Font
	display    draw.Display           // on which the frame is displayed
	background draw.Image             // on which the frame appears
	cols       [NumColours]draw.Image // background and text colours
	rect       image.Rectangle        // in which the text appears

	// Slice B font variants. Each is optional — when nil the
	// renderer falls back to the base font, so a plain (no-
	// variant) Init is unchanged from upstream. A box's
	// Style.Kind & (KindBold|KindItalic) selects which font to
	// use (see fontFor in draw.go).
	fontBold       draw.Font
	fontItalic     draw.Font
	fontBoldItalic draw.Font
	// fontCode is the monospace ("code") family variant. A
	// box with Style.Kind & KindCodeFamily renders with it; it
	// takes precedence over weight/italic variants (family is
	// a stronger choice than weight). nil falls back to the
	// base font, same as the other variants.
	fontCode draw.Font

	// fontByScale maps Style.Scale → draw.Font for KindScale
	// runs (B2.2 R4). md2spans emits scale=1.5 / 2.0 etc. on
	// headings; the consumer (Text/acme) loads matching scaled
	// fonts and installs them via OptScaleFonts. A missing key
	// (or nil map) means the run renders with the base font,
	// graceful degradation. Replacing the map on a subsequent
	// frame.Init (as the Font command path does) re-derives
	// line heights on next relayout.
	fontByScale map[float32]draw.Font

	defaultfontheight int // height of default font

	// Debug overlays toggled by tag-bar commands. showBoxOutlines
	// makes paintBox draw a 1-px Purpleblue rect around every
	// painted box; boxOutlineColor is lazily allocated on first
	// enable. afterPaintHook fires at the end of drawtext /
	// repaintBoxRange so the "Spans" tag command can overlay
	// span boundaries on top of the frame's normal paint.
	showBoxOutlines bool
	boxOutlineColor draw.Image
	afterPaintHook  func()

	box []*frbox // the boxes of text in this frame.

	sp0, sp1 int // bounds of a selection
	maxtab   int // max size of a tab (in pixels)
	nchars   int // number of runes in frame
	nlines   int // number of lines with text

	// TODO(rjk): figure out what to do about this for multiple line fonts.
	maxlines     int // total number of lines in frame
	lastlinefull bool
	modified     bool

	tickimage   draw.Image // typing tick
	tickback    draw.Image // image under tick
	ticked      bool       // Is the tick on.
	highlighton bool       // True if the highlight is painted.

	// Set this to true to indicate that the Frame should not emit drawing ops.
	// Use this if the Frame is being used "headless" to measure some text.
	noredraw  bool
	tickscale int // tick scaling factor
}

// NewFrame creates a new Frame with Font ft, background image b,
// colours cols, and of the size r. Additional options (extra)
// are applied after the default options, so callers can override
// (e.g., add bold/italic font variants via OptBoldFont,
// OptItalicFont, OptBoldItalicFont) without restating the
// defaults.
func NewFrame(r image.Rectangle, ft draw.Font, b draw.Image, cols [NumColours]draw.Image, extra ...OptionClosure) Frame {
	f := new(frameimpl)
	opts := []OptionClosure{
		OptColors(cols), OptFont(ft), OptBackground(b), OptMaxTab(8),
	}
	opts = append(opts, extra...)
	f.Init(r, opts...)
	return f
}

func (f *frameimpl) DefaultFontHeight() int {
	f.lk.Lock()
	defer f.lk.Unlock()
	return f.defaultfontheight
}

// TODO(rjk): This may do unnecessary work for some option settings.
// At some point, consider the code carefully.
func (f *frameimpl) Init(r image.Rectangle, opts ...OptionClosure) {
	f.lk.Lock()
	defer f.lk.Unlock()
	f.nchars = 0
	f.nlines = 0
	f.sp0 = 0
	f.sp1 = 0
	f.box = nil
	f.lastlinefull = false

	// Update additional options. The values are optional so that the frame
	// will re-use the existing values if new ones are not provided.
	ctx := f.Option(opts...)

	f.defaultfontheight = f.font.Height()
	f.display = f.background.Display()
	f.maxtab = ctx.computemaxtab(f.maxtab, f.font.StringWidth("0"))
	f.setrects(r)

	if ctx.updatetick || (f.tickimage == nil && f.cols[ColBack] != nil) {
		f.InitTick()
	}
}

// setrects initializes the geometry of the frame.
func (f *frameimpl) setrects(r image.Rectangle) {
	height := f.defaultfontheight
	f.rect = r
	f.rect.Max.Y -= (r.Max.Y - r.Min.Y) % height
	f.maxlines = (r.Max.Y - r.Min.Y) / height
}

func (f *frameimpl) Clear(freeall bool) {
	f.lk.Lock()
	defer f.lk.Unlock()
	f.box = make([]*frbox, 0, 25)
	if freeall {
		f.tickimage.Free()
		f.tickback.Free()
		f.tickimage = nil
		f.tickback = nil
	}
	f.ticked = false
}
