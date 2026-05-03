package rich

import (
	"image"
	"image/color"
	"strings"
	"unicode/utf8"

	"9fans.net/go/draw"
	edwooddraw "github.com/rjkroege/edwood/draw"
	xdraw "golang.org/x/image/draw"
)

const (
	// frtickw is the tick (cursor) width in unscaled pixels, matching frame/frame.go.
	frtickw = 3
)

// Option is a functional option for configuring a Frame.
type Option func(*frameImpl)

// Frame renders styled text content with selection support.
type Frame interface {
	// Initialization
	// Init applies options to a freshly-constructed frame. The
	// frame's rectangle is left at its zero value; callers must
	// drive geometry via SetRect afterward (typically a wrapper
	// like RichText that knows the parent layout's coordinate
	// system). Phase 1.4 of the markdown-externalization plan
	// removed the rect from Init's signature to settle the
	// architect-review finding P1-6 (geometry ownership
	// ambiguity) on side D — RichText / wrapper owns geometry,
	// frame is passive.
	Init(opts ...Option)
	Clear()

	// Content
	SetContent(c Content)

	// Geometry
	Rect() image.Rectangle
	SetRect(r image.Rectangle)   // Update the frame's rectangle
	Ptofchar(p int) image.Point  // Character position → screen point
	Charofpt(pt image.Point) int // Screen point → character position

	// Selection
	Select(mc *draw.Mousectl, m *draw.Mouse) (p0, p1 int)
	SelectWithChord(mc *draw.Mousectl, m *draw.Mouse) (p0, p1 int, chordButtons int)
	SelectWithColor(mc *draw.Mousectl, m *draw.Mouse, col edwooddraw.Image) (p0, p1 int)
	SelectWithChordAndColor(mc *draw.Mousectl, m *draw.Mouse, col edwooddraw.Image) (p0, p1 int, chordButtons int)
	SetSelection(p0, p1 int)
	GetSelection() (p0, p1 int)

	// Scrolling
	SetOrigin(org int)
	GetOrigin() int
	SetOriginYOffset(pixels int)
	GetOriginYOffset() int
	// SetScrollSnap configures snap behavior for subsequent layouts.
	// Callers (typically scroll handlers) set this immediately
	// before calling SetOrigin/SetOriginYOffset to record the
	// user's intent. Edge cases (origin at file top, tall origin
	// line) override this inside layoutFromOrigin.
	SetScrollSnap(s ScrollSnap)
	// ScrollSnap returns the currently configured snap behavior.
	// Note that file-top and tall-line overrides are applied
	// inside layoutFromOrigin and are not reflected here — this
	// returns the caller-set preference, not the effective snap.
	ScrollSnap() ScrollSnap
	MaxLines() int
	VisibleLines() int
	TotalLines() int          // Total number of layout lines in the content
	LineStartRunes() []int    // Rune offset at the start of each visual line
	LinePixelHeights() []int  // Pixel height of each visual line (accounts for images)
	LinePixelYs() []int       // Rendered Y of each visual line, with inter-line gaps + scrollbar adjustments
	TotalDocumentHeight() int // Total rendered height including all inter-line gaps (paragraph, heading, scrollbar)
	// LayoutLines returns the laid-out lines from the current
	// origin (a fresh clone, mutable by the caller). Empty content
	// returns nil. Transitional accessor consumed by rich/mdrender
	// for post-paint decoration; goes away in Phase 4 of the
	// markdown-externalization work.
	LayoutLines() []Line

	// Rendering
	Redraw()

	// Content queries
	ImageURLAt(pos int) string // Returns image URL at position, or "" if not an image

	// ExpandAtPos returns the expanded selection range for double-click.
	// If pos is inside a code block (Block && Code), returns the full
	// code block rune range. Otherwise returns word boundaries.
	ExpandAtPos(pos int) (q0, q1 int)

	// ExpandWordAtPos returns word boundaries at pos (alphanumeric + underscore).
	// Unlike ExpandAtPos, it never expands to full code blocks or inline code spans.
	ExpandWordAtPos(pos int) (q0, q1 int)

	// Font metrics
	DefaultFontHeight() int // Height of the default font

	// Horizontal scrollbar hit testing
	HScrollBarAt(pt image.Point) (regionIndex int, ok bool)

	// HScrollBarRect returns the screen-coordinate rectangle of the
	// horizontal scrollbar for the given block region. Returns the zero
	// rectangle if the region has no scrollbar.
	HScrollBarRect(regionIndex int) image.Rectangle

	// Horizontal scrollbar click handling (acme three-button semantics)
	HScrollClick(button int, pt image.Point, regionIndex int)

	// PointInBlockRegion checks if a screen point falls within any
	// horizontally-scrollable block region (the content area, not just
	// the scrollbar). Returns the region index and true if hit.
	PointInBlockRegion(pt image.Point) (regionIndex int, ok bool)

	// HScrollWheel adjusts the horizontal scroll offset for the given
	// block region by delta pixels (positive = scroll right, negative = left).
	// The resulting offset is clamped to [0, maxScrollable].
	HScrollWheel(delta int, regionIndex int)

	// HasSlideBreakBetween returns true if there is a slide break (---\n---)
	// between rune positions a and b.
	HasSlideBreakBetween(a, b int) bool

	// SnapOriginToSlideStart takes a target origin (rune position) and returns
	// an adjusted origin that aligns to the start of the slide if the target
	// falls within a slide region. Returns the original origin if not in a slide.
	SnapOriginToSlideStart(targetOrigin int) int

	// Status
	Full() bool // True if frame is at capacity
}

// frameImpl is the concrete implementation of Frame.
//
// Fields are organized into named substructs that are embedded so
// existing call sites can continue to use the promoted field names
// (`f.tickImage`, `f.hscrollOrigins`, etc.). The grouping makes the
// orthogonal subsystems visible at the type level: vertical scroll,
// selection, cursor tick, horizontal-block scroll, and layout cache
// each own their state, so adding a new field has an obvious home.
//
// Top-level fields are those that don't fit a clear subsystem yet
// (display/font handles, content, scratch image, image cache,
// callback, color cache, tab width).
type frameImpl struct {
	rect           image.Rectangle
	display        edwooddraw.Display
	background     edwooddraw.Image // background image for filling
	textColor      edwooddraw.Image // text color image for rendering
	selectionColor edwooddraw.Image // selection highlight color
	font           edwooddraw.Font  // font for text rendering
	content        Content

	// Font variants for styled text
	boldFont       edwooddraw.Font
	italicFont     edwooddraw.Font
	boldItalicFont edwooddraw.Font
	codeFont       edwooddraw.Font // monospace font for code spans

	// Scaled fonts for headings (key is scale factor: 2.0 for H1, 1.5 for H2, etc.)
	scaledFonts map[float64]edwooddraw.Font

	// Scratch image for clipped rendering - all drawing goes here first,
	// then blitted to screen. This ensures text doesn't overflow frame bounds.
	scratchImage edwooddraw.Image
	scratchRect  image.Rectangle // size of current scratch image

	// Image cache for loading images during layout
	imageCache *ImageCache

	// Base path for resolving relative image paths (e.g., the markdown file path)
	basePath string

	// Callback invoked when an async image load completes. Runs on an
	// unspecified goroutine; callers that need main-goroutine execution
	// must marshal through the row lock or a channel.
	onImageLoaded func(path string)

	// Tab width in characters (default 4 when zero)
	maxtabChars int

	vScrollState
	selectionState
	tickState
	hScrollState
	layoutCache

	// Color image cache. Plan 9 image handles are scarce server-side
	// resources, not just memory; allocColorImage was previously
	// hitting display.AllocImage on every call (per styled span, per
	// redraw, per keystroke), leaking handles for the lifetime of a
	// styled window. Keyed by the packed RGBA byte representation so
	// equal colors expressed as different color.Color implementations
	// (e.g. RGBA vs NRGBA) share an entry. Cache size is bounded by
	// the number of unique colors a document uses; lazily initialized.
	colorCache map[edwooddraw.Color]edwooddraw.Image
}

// vScrollState groups vertical-scroll position state. The combination
// (origin, originYOffset, scrollSnap) defines what's at the top of
// the viewport: which document rune (origin), how many pixels into
// that line's vertical extent the viewport top is (originYOffset, for
// sub-line scrolling on tall lines like images), and how the layout
// should snap when the viewport doesn't perfectly tile the document.
type vScrollState struct {
	origin        int
	originYOffset int        // pixel offset within the origin line (for sub-line scrolling)
	scrollSnap    ScrollSnap // current snap preference; default SnapTop
}

// selectionState groups selection state. p0/p1 are the selection
// range (rune offsets, p0 <= p1); sweepColor is a transient
// per-drag override of selectionColor used by the B2/B3 colored
// sweeps and cleared after the drag completes.
type selectionState struct {
	p0, p1     int
	sweepColor edwooddraw.Image
}

// tickState groups insertion-cursor (tick) rendering state. The tick
// image is pre-rendered (transparent + opaque pattern) and re-init'd
// when the cursor's height changes; tickScale tracks display DPI.
type tickState struct {
	tickImage  edwooddraw.Image
	tickScale  int
	tickHeight int
}

// hScrollState groups horizontal-block-scroll state. Edwood's rich
// frames have per-block-region horizontal scrollbars (each non-
// wrapping block — code block etc. — has its own thumb), so the
// state is keyed by ordinal block index and accompanied by metadata
// from the last layout pass for hit-testing scrollbar clicks.
type hScrollState struct {
	// Per-block scroll origins. Index is ordinal of block region
	// (0th, 1st, ...); value is pixel offset from the block's left.
	hscrollOrigins []int

	// Total non-wrapping blocks seen on the last layout pass.
	// Used to detect when blocks are added or removed.
	hscrollBlockCount int

	// Offset from visible-region index to global hscrollOrigins
	// index. Equal to the number of block regions entirely above
	// the viewport.
	hscrollRegionOffset int

	// Cached adjusted block regions from the last layout pass,
	// used for hit-testing horizontal scrollbar clicks.
	hscrollRegions []AdjustedBlockRegion

	// Horizontal scrollbar colors (passed from RichText so they
	// match the vertical scrollbar).
	hscrollBg    edwooddraw.Image
	hscrollThumb edwooddraw.Image

	// Horizontal scrollbar height in pixels. Defaults to
	// DefaultHScrollHeight at NewFrame time; RichText.Init overrides
	// via WithHScrollHeight so main's Scrollwid is the single source
	// of truth in production.
	hscrollHeight int
}

// layoutCache stores the result of contentToBoxes + layoutBoxes so
// repeated Redraw / TotalLines / etc. calls reuse the expensive
// line-breaking computation when content and frame width are
// unchanged. Invalidated by SetContent and by SetRect when the width
// changes (height-only changes leave the cache valid).
type layoutCache struct {
	cachedBaseLines []Line
	layoutDirty     bool
	cachedWidth     int
}

// maxtabPixels returns the tab width in pixels.
// Defaults to 4 characters if maxtabChars is unset (zero).
func (f *frameImpl) maxtabPixels() int {
	chars := f.maxtabChars
	if chars <= 0 {
		chars = 4
	}
	return chars * f.font.StringWidth("0")
}

// NewFrame creates a new Frame.
// DefaultHScrollHeight is the rich package's standalone fallback for
// the horizontal scrollbar height in pixels. It exists so the rich
// package can be tested in isolation without depending on the main
// package; production code (RichText.Init) overrides this via
// WithHScrollHeight, passing main's Scrollwid through. Match the value
// to main.Scrollwid (currently 12) until the option is wired through.
const DefaultHScrollHeight = 12

func NewFrame() Frame {
	return &frameImpl{
		hScrollState: hScrollState{hscrollHeight: DefaultHScrollHeight},
	}
}

// Init applies the supplied options to the frame. The rect is left
// at its zero value; callers drive geometry via SetRect. See the
// Frame interface godoc for the rationale.
func (f *frameImpl) Init(opts ...Option) {
	for _, opt := range opts {
		opt(f)
	}
}

// Clear resets the frame.
func (f *frameImpl) Clear() {
	f.content = nil
	f.origin = 0
	f.originYOffset = 0
	f.p0 = 0
	f.p1 = 0
}

// SetContent sets the content to display.
func (f *frameImpl) SetContent(c Content) {
	f.content = c
	f.layoutDirty = true
}

// Rect returns the frame's rectangle.
func (f *frameImpl) Rect() image.Rectangle {
	return f.rect
}

// SetRect updates the frame's rectangle.
// This is used when the frame needs to be resized without full re-initialization.
// Subsequent calls to layout-dependent methods (TotalLines, Redraw, etc.)
// will use the new rectangle dimensions.
func (f *frameImpl) SetRect(r image.Rectangle) {
	if r.Dx() != f.rect.Dx() {
		f.layoutDirty = true
	}
	f.rect = r
}

// Ptofchar maps a character position to a screen point.
// The position p is a rune offset into the content.
// Returns the screen point where that character would be drawn.
func (f *frameImpl) Ptofchar(p int) image.Point {
	if p <= 0 {
		return f.rect.Min
	}

	// Use layoutFromOrigin to get viewport-relative lines and the origin rune offset.
	// p is a content-absolute rune position; we subtract originRune to get a
	// viewport-relative position for searching through the visible lines.
	lines, originRune := f.layoutFromOrigin()
	if len(lines) == 0 {
		return f.rect.Min
	}

	// Adjust p to be relative to the origin
	p -= originRune
	if p <= 0 {
		return f.rect.Min
	}

	// Walk through positioned boxes counting runes
	runeCount := 0
	for _, line := range lines {
		for _, pb := range line.Boxes {
			boxRunes := pb.Box.Nrune
			if pb.Box.IsNewline() || pb.Box.IsTab() {
				// Special characters count as 1 rune
				boxRunes = 1
			}

			// Check if position p is within this box
			if runeCount+boxRunes > p {
				// p is within this box, calculate offset within the box
				runeOffset := p - runeCount

				// For newline/tab, just return the start position
				if pb.Box.IsNewline() || pb.Box.IsTab() {
					return image.Point{
						X: f.rect.Min.X + pb.X,
						Y: f.rect.Min.Y + line.Y,
					}
				}

				// For text, measure the width of the first runeOffset runes
				text := pb.Box.Text
				byteOffset := 0
				for i := 0; i < runeOffset && byteOffset < len(text); i++ {
					_, size := utf8.DecodeRune(text[byteOffset:])
					byteOffset += size
				}
				partialWidth := f.fontForStyle(pb.Box.Style).BytesWidth(text[:byteOffset])

				return image.Point{
					X: f.rect.Min.X + pb.X + partialWidth,
					Y: f.rect.Min.Y + line.Y,
				}
			}

			runeCount += boxRunes
		}
	}

	// Position is past end of content - return position after last character
	if len(lines) > 0 {
		lastLine := lines[len(lines)-1]
		// Calculate X position at end of last line
		endX := 0
		for _, pb := range lastLine.Boxes {
			if pb.Box.IsNewline() {
				// After a newline, position is at start of next line
				return image.Point{
					X: f.rect.Min.X,
					Y: f.rect.Min.Y + lastLine.Y + lastLine.Height,
				}
			}
			endX = pb.X + pb.Box.Wid
		}
		return image.Point{
			X: f.rect.Min.X + endX,
			Y: f.rect.Min.Y + lastLine.Y,
		}
	}

	return f.rect.Min
}

// Charofpt maps a screen point to a character position.
// The point is in screen coordinates. Returns the rune offset
// of the character at that position.
func (f *frameImpl) Charofpt(pt image.Point) int {
	// Use layoutFromOrigin to get viewport-relative lines and the origin rune offset.
	// After scrolling, click coordinates are viewport-relative but layoutBoxes()
	// returns document-absolute Y positions. layoutFromOrigin() adjusts Y to start
	// from 0 at the first visible line.
	lines, originRune := f.layoutFromOrigin()
	if len(lines) == 0 {
		return originRune
	}

	// Convert point to frame-relative coordinates
	relX := pt.X - f.rect.Min.X
	relY := pt.Y - f.rect.Min.Y

	// Handle points above or to the left of frame
	if relX < 0 {
		relX = 0
	}
	if relY < 0 {
		relY = 0
	}

	// Find which line the point is on
	lineIdx := 0
	for i, line := range lines {
		// Check if point is within this line's Y range
		lineTop := line.Y
		lineBottom := line.Y + line.Height
		if relY >= lineTop && relY < lineBottom {
			lineIdx = i
			break
		}
		// If we're past this line, keep updating lineIdx
		if relY >= lineTop {
			lineIdx = i
		}
	}

	// Count runes up to the target line (viewport-relative)
	runeCount := 0
	for i := 0; i < lineIdx; i++ {
		for _, pb := range lines[i].Boxes {
			if pb.Box.IsNewline() || pb.Box.IsTab() {
				runeCount++
			} else {
				runeCount += pb.Box.Nrune
			}
		}
	}

	// Now find the position within the target line
	targetLine := lines[lineIdx]
	for _, pb := range targetLine.Boxes {
		boxStart := pb.X
		boxEnd := pb.X + pb.Box.Wid

		// Handle newline boxes (width 0, but still represent a character)
		if pb.Box.IsNewline() {
			// Point at or after the newline position returns the newline's position
			// We return here because we've found the position
			if relX >= boxStart {
				return originRune + runeCount
			}
			continue
		}

		// Handle tab boxes
		if pb.Box.IsTab() {
			if relX >= boxEnd {
				// Point is past this tab
				runeCount++
				continue
			}
			if relX >= boxStart {
				// Point is within the tab
				return originRune + runeCount
			}
			// Point is before this box
			return originRune + runeCount
		}

		// Handle text boxes
		if relX >= boxEnd {
			// Point is past this box
			runeCount += pb.Box.Nrune
			continue
		}

		if relX >= boxStart {
			// Point is within this box - find which character
			localX := relX - boxStart
			return originRune + runeCount + f.runeAtX(pb.Box.Text, pb.Box.Style, localX)
		}

		// Point is before this box (shouldn't normally happen
		// since boxes are laid out left to right)
		return originRune + runeCount
	}

	// Point is past all content on this line
	return originRune + runeCount
}

// runeAtX finds which rune in text corresponds to pixel offset x.
// Returns the rune index (0-based) within the text.
func (f *frameImpl) runeAtX(text []byte, style Style, x int) int {
	font := f.fontForStyle(style)
	cumWidth := 0
	runeIdx := 0

	for i := 0; i < len(text); {
		_, runeLen := utf8.DecodeRune(text[i:])
		runeWidth := font.BytesWidth(text[i : i+runeLen])

		// Check if x falls within this rune
		// We use midpoint - if x is in the first half, return current index
		// if in second half, return next index
		if cumWidth+runeWidth > x {
			// x is within this rune's span
			midpoint := cumWidth + runeWidth/2
			if x < midpoint {
				return runeIdx
			}
			return runeIdx
		}

		cumWidth += runeWidth
		runeIdx++
		i += runeLen
	}

	// x is past all runes
	return runeIdx
}

// ImageURLAt returns the ImageURL if the given character position falls within
// an image box. Returns empty string if not an image.
func (f *frameImpl) ImageURLAt(pos int) string {
	boxes := contentToBoxes(f.content)
	if len(boxes) == 0 {
		return ""
	}

	// Walk through boxes counting runes until we find the one containing pos
	runeCount := 0
	for _, box := range boxes {
		var boxRunes int
		if box.IsNewline() || box.IsTab() {
			boxRunes = 1
		} else {
			boxRunes = box.Nrune
		}

		// Check if pos falls within this box
		if pos >= runeCount && pos < runeCount+boxRunes {
			if box.Style.Image && box.Style.ImageURL != "" {
				return box.Style.ImageURL
			}
			return ""
		}

		runeCount += boxRunes
	}

	return ""
}

// ExpandAtPos returns the expanded selection range for a double-click at pos.
// If pos is inside a code block (Block && Code), returns the rune range of the
// entire contiguous code block. Otherwise returns word boundaries (alphanumeric
// + underscore), matching acme's double-click behavior.
func (f *frameImpl) ExpandAtPos(pos int) (q0, q1 int) {
	q0, q1 = pos, pos

	// Walk spans to find which span contains pos and its rune offset.
	runeOffset := 0
	spanIdx := -1
	for i, s := range f.content {
		sLen := len([]rune(s.Text))
		if pos >= runeOffset && pos < runeOffset+sLen {
			spanIdx = i
			break
		}
		runeOffset += sLen
	}
	if spanIdx < 0 {
		return q0, q1
	}

	span := f.content[spanIdx]

	// If inside a fenced code block, select the entire contiguous block.
	if span.Style.Block && span.Style.Code {
		// Scan backward for contiguous Block && Code spans.
		blockStart := runeOffset
		for i := spanIdx - 1; i >= 0; i-- {
			s := f.content[i]
			if !(s.Style.Block && s.Style.Code) {
				break
			}
			blockStart -= len([]rune(s.Text))
		}
		// Scan forward for contiguous Block && Code spans.
		blockEnd := runeOffset
		for i := spanIdx; i < len(f.content); i++ {
			s := f.content[i]
			if !(s.Style.Block && s.Style.Code) {
				break
			}
			blockEnd += len([]rune(s.Text))
		}
		return blockStart, blockEnd
	}

	// If inside an inline code span, select the entire span.
	if span.Style.Code {
		return runeOffset, runeOffset + len([]rune(span.Text))
	}

	// Not in a code block: expand to word (alphanumeric + underscore).
	plain := f.content.Plain()
	q0 = pos
	for q0 > 0 && isExpandWordChar(plain[q0-1]) {
		q0--
	}
	q1 = pos
	for q1 < len(plain) && isExpandWordChar(plain[q1]) {
		q1++
	}
	return q0, q1
}

// ExpandWordAtPos returns word boundaries at pos (alphanumeric + underscore).
// Unlike ExpandAtPos, it never expands to full code blocks or inline code spans.
func (f *frameImpl) ExpandWordAtPos(pos int) (q0, q1 int) {
	plain := f.content.Plain()
	q0, q1 = pos, pos
	for q0 > 0 && isExpandWordChar(plain[q0-1]) {
		q0--
	}
	for q1 < len(plain) && isExpandWordChar(plain[q1]) {
		q1++
	}
	return q0, q1
}

// isExpandWordChar returns true if the rune is part of a word for double-click
// expansion (alphanumeric or underscore).
func isExpandWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

// Select handles mouse selection.
// It takes the Mousectl for reading subsequent mouse events and the
// initial mouse-down event. It tracks the mouse drag and returns the
// selection range (p0, p1) where p0 <= p1. The frame's internal
// selection state is also updated.
//
// Teardown contract: this method blocks on `<-mc.C` until a mouse
// event arrives with all buttons released. There is no context,
// timeout, or display-close signal — the caller MUST guarantee that
// `mc.C` continues to deliver events for the lifetime of the call,
// including in error/teardown paths. If the display is destroyed
// while a Select is in flight (e.g. window torn down mid-drag) the
// goroutine will block on the channel read forever. This matches the
// acme/Plan 9 idiom: window teardown waits for the active drag to
// release before reaching the destruction code path. Same constraint
// applies to SelectWithChord and the *WithColor variants below.
func (f *frameImpl) Select(mc *draw.Mousectl, m *draw.Mouse) (p0, p1 int) {
	// Get the initial position from the mouse-down event
	anchor := f.Charofpt(m.Point)
	current := anchor

	// Read mouse events until button is released
	for {
		me := <-mc.C
		current = f.Charofpt(me.Point)

		// Update selection as we drag (for visual feedback)
		if anchor <= current {
			f.p0 = anchor
			f.p1 = current
		} else {
			f.p0 = current
			f.p1 = anchor
		}

		// Redraw to show updated selection during drag
		f.Redraw()

		// Flush the display to make selection visible immediately
		if f.display != nil {
			f.display.Flush()
		}

		// Check if button was released
		if me.Buttons == 0 {
			break
		}
	}

	// Return normalized selection (p0 <= p1)
	return f.p0, f.p1
}

// SelectWithChord handles mouse selection with chord detection.
// Like Select, it tracks drag from the initial B1 mouse-down event,
// but also detects when additional buttons (B2, B3) are pressed during
// the drag. Returns the selection range and the button state at chord
// time (0 if no chord was detected, i.e. only B1 was held).
//
// Same teardown contract as Select: callers must keep `mc.C`
// delivering until all buttons release.
func (f *frameImpl) SelectWithChord(mc *draw.Mousectl, m *draw.Mouse) (p0, p1 int, chordButtons int) {
	anchor := f.Charofpt(m.Point)
	current := anchor
	initialButtons := m.Buttons

	for {
		me := <-mc.C
		current = f.Charofpt(me.Point)

		if anchor <= current {
			f.p0 = anchor
			f.p1 = current
		} else {
			f.p0 = current
			f.p1 = anchor
		}

		f.Redraw()

		if f.display != nil {
			f.display.Flush()
		}

		// Detect chord: additional buttons pressed beyond the initial button
		if me.Buttons != 0 && me.Buttons != initialButtons && chordButtons == 0 {
			chordButtons = me.Buttons
		}

		if me.Buttons == 0 {
			break
		}
	}

	return f.p0, f.p1, chordButtons
}

// SelectWithColor performs a mouse drag selection using a custom sweep color
// for the selection highlight during the drag. After the drag completes, the
// sweep color is cleared so subsequent redraws use the normal selectionColor.
// This matches normal Acme's SelectOpt behavior for B2 (red) and B3 (green) sweeps.
func (f *frameImpl) SelectWithColor(mc *draw.Mousectl, m *draw.Mouse, col edwooddraw.Image) (p0, p1 int) {
	f.sweepColor = col
	defer func() { f.sweepColor = nil }()
	return f.Select(mc, m)
}

// SelectWithChordAndColor performs a mouse drag selection with chord detection
// using a custom sweep color for the selection highlight during the drag.
// After the drag completes, the sweep color is cleared.
func (f *frameImpl) SelectWithChordAndColor(mc *draw.Mousectl, m *draw.Mouse, col edwooddraw.Image) (p0, p1 int, chordButtons int) {
	f.sweepColor = col
	defer func() { f.sweepColor = nil }()
	return f.SelectWithChord(mc, m)
}

// SetSelection sets the selection range.
func (f *frameImpl) SetSelection(p0, p1 int) {
	f.p0 = p0
	f.p1 = p1
}

// GetSelection returns the current selection range.
func (f *frameImpl) GetSelection() (p0, p1 int) {
	return f.p0, f.p1
}

// SetOrigin sets the scroll origin and resets the pixel offset and
// snap preference. Programmatic scrolls (Look, search, auto-scroll)
// call this and shouldn't inherit a stale SnapBottom from a prior
// B1; resetting to SnapTop here keeps the policy predictable.
// Scrollbar handlers that need a different snap call SetScrollSnap
// AFTER SetOrigin.
func (f *frameImpl) SetOrigin(org int) {
	f.origin = org
	f.originYOffset = 0
	f.scrollSnap = SnapTop
}

// GetOrigin returns the current scroll origin.
func (f *frameImpl) GetOrigin() int {
	return f.origin
}

// SetOriginYOffset sets the pixel offset within the origin line.
// This enables sub-line scrolling for tall elements (images, large
// code blocks). Resets snap preference to SnapTop, matching
// SetOrigin's semantics for programmatic scrolls.
func (f *frameImpl) SetOriginYOffset(pixels int) {
	f.originYOffset = pixels
	f.scrollSnap = SnapTop
}

// SetScrollSnap records the caller's snap preference for subsequent
// layouts. Should be called AFTER SetOrigin/SetOriginYOffset because
// those reset snap to SnapTop. Scrollbar click handlers set this to
// SnapBottom for B1 (revealing earlier content) and leave it at the
// SnapTop reset for B2 / B3 / programmatic scrolls.
func (f *frameImpl) SetScrollSnap(s ScrollSnap) {
	f.scrollSnap = s
}

// ScrollSnap returns the currently configured snap preference.
func (f *frameImpl) ScrollSnap() ScrollSnap {
	return f.scrollSnap
}

// GetOriginYOffset returns the pixel offset within the origin line.
func (f *frameImpl) GetOriginYOffset() int {
	return f.originYOffset
}

// MaxLines returns the maximum number of lines that can be displayed.
// This is based on the frame height divided by the font height.
func (f *frameImpl) MaxLines() int {
	if f.font == nil {
		return 0
	}
	fontHeight := f.font.Height()
	if fontHeight <= 0 {
		return 0
	}
	return f.rect.Dy() / fontHeight
}

// VisibleLines returns the number of lines currently visible.
// This accounts for the origin offset and line wrapping.
func (f *frameImpl) VisibleLines() int {
	if f.font == nil || f.content == nil {
		return 0
	}
	lines, _ := f.layoutFromOrigin()
	return len(lines)
}

// TotalLines returns the total number of layout lines in the content.
// This includes all lines after word wrapping, not just source newlines.
func (f *frameImpl) TotalLines() int {
	if f.font == nil || f.content == nil {
		return 0
	}
	lines := f.ensureBaseLayout()
	return len(lines)
}

// LineStartRunes returns the rune offset at the start of each visual line.
// This maps visual line indices to rune positions for scrolling.
func (f *frameImpl) LineStartRunes() []int {
	if f.font == nil || f.content == nil {
		return []int{0}
	}
	lines := f.ensureBaseLayout()
	if len(lines) == 0 {
		return []int{0}
	}

	// Walk through lines and calculate rune offset at start of each line
	lineStarts := make([]int, len(lines))
	runeCount := 0
	for i, line := range lines {
		lineStarts[i] = runeCount
		// Count runes in this line
		for _, pb := range line.Boxes {
			if pb.Box.IsNewline() || pb.Box.IsTab() {
				runeCount++
			} else {
				runeCount += pb.Box.Nrune
			}
		}
	}

	return lineStarts
}

// LinePixelHeights returns the pixel height of each visual line.
// For lines containing images, the height will be larger than the default font height.
// Note: these are raw line heights without inter-line gaps (paragraph spacing,
// heading spacing, scrollbar space). Use TotalDocumentHeight() for the full height.
func (f *frameImpl) LinePixelHeights() []int {
	if f.font == nil || f.content == nil {
		return nil
	}
	lines := f.ensureBaseLayout()
	if len(lines) == 0 {
		return nil
	}

	heights := make([]int, len(lines))
	for i, line := range lines {
		heights[i] = line.Height
	}
	return heights
}

// LinePixelYs returns the rendered Y position of each visual line in
// the current layout, accounting for inter-line gaps (paragraph
// spacing, heading spacing) and horizontal-scrollbar height
// adjustments. Y values are in document-absolute layout space (line
// 0 at Y=0). Use together with LinePixelHeights and LineStartRunes
// for screen-Y ↔ line mapping that respects gaps.
//
// Slides adjustments are NOT applied here because layoutFromOrigin
// applies those only to the viewport-visible subset.
func (f *frameImpl) LinePixelYs() []int {
	if f.font == nil || f.content == nil {
		return nil
	}
	lines := f.ensureBaseLayout()
	if len(lines) == 0 {
		return nil
	}
	// ensureBaseLayout returns a fresh clone we may mutate.
	frameWidth := f.rect.Dx()
	scrollbarHeight := f.hscrollHeight // matches layoutFromOrigin
	regions := findBlockRegions(lines)
	adjustLayoutForScrollbars(lines, regions, frameWidth, scrollbarHeight)

	ys := make([]int, len(lines))
	for i, line := range lines {
		ys[i] = line.Y
	}
	return ys
}

// LayoutLines returns the laid-out lines from the current origin.
// The returned slice is a fresh clone; callers may mutate Y /
// Height / ContentWidth without affecting the layout cache.
//
// Transitional accessor consumed by rich/mdrender for post-paint
// decoration. After Phase 4 of the markdown-externalization work,
// the wrapper goes away and so does this method.
//
// Empty content returns nil. Equivalent to layoutFromOrigin
// internally; the difference is just visibility (this method is
// part of the Frame interface; layoutFromOrigin is package-private).
func (f *frameImpl) LayoutLines() []Line {
	lines, _ := f.layoutFromOrigin()
	return lines
}

// HasSlideBreakBetween returns true if there is a slide break (---\n---)
// between rune positions a and b in the document.
func (f *frameImpl) HasSlideBreakBetween(a, b int) bool {
	if a > b {
		a, b = b, a
	}
	if f.font == nil || f.content == nil {
		return false
	}
	lines := f.ensureBaseLayout()
	if len(lines) == 0 {
		return false
	}

	slideRegions := findSlideRegions(lines)
	if len(slideRegions) == 0 {
		return false
	}

	// Build rune offset at start of each line.
	lineStarts := make([]int, len(lines))
	runeCount := 0
	for i, line := range lines {
		lineStarts[i] = runeCount
		for _, pb := range line.Boxes {
			if pb.Box.IsNewline() || pb.Box.IsTab() {
				runeCount++
			} else {
				runeCount += pb.Box.Nrune
			}
		}
	}

	for _, region := range slideRegions {
		breakRune := lineStarts[region.FirstHRuleLineIdx]
		if breakRune > a && breakRune < b {
			return true
		}
	}
	return false
}

// SnapOriginToSlideStart takes a target origin (rune position) and returns
// an adjusted origin that aligns to the start of the slide if the target
// falls within a slide region. A slide region spans from the line after
// the second HRule (SlideBreak) to the line before the next first HRule
// (or end of document). Returns the original origin if not in a slide.
func (f *frameImpl) SnapOriginToSlideStart(targetOrigin int) int {
	if f.font == nil || f.content == nil {
		return targetOrigin
	}
	lines := f.ensureBaseLayout()
	if len(lines) == 0 {
		return targetOrigin
	}

	// Find slide regions
	slideRegions := findSlideRegions(lines)
	if len(slideRegions) == 0 {
		return targetOrigin
	}

	// Build rune offset at start of each line
	lineStarts := make([]int, len(lines))
	runeCount := 0
	for i, line := range lines {
		lineStarts[i] = runeCount
		for _, pb := range line.Boxes {
			if pb.Box.IsNewline() || pb.Box.IsTab() {
				runeCount++
			} else {
				runeCount += pb.Box.Nrune
			}
		}
	}

	// For each slide region, check if targetOrigin falls within it.
	// A slide's content starts at the line after SecondHRuleLineIdx
	// and ends at the line before the next region's FirstHRuleLineIdx
	// (or end of document).
	for i, region := range slideRegions {
		slideStartLine := region.SecondHRuleLineIdx + 1
		if slideStartLine >= len(lineStarts) {
			continue
		}
		slideStartRune := lineStarts[slideStartLine]

		var slideEndRune int
		if i+1 < len(slideRegions) {
			slideEndLine := slideRegions[i+1].FirstHRuleLineIdx
			if slideEndLine < len(lineStarts) {
				slideEndRune = lineStarts[slideEndLine]
			} else {
				slideEndRune = runeCount
			}
		} else {
			slideEndRune = runeCount
		}

		if targetOrigin >= slideStartRune && targetOrigin < slideEndRune {
			return slideStartRune
		}
	}

	return targetOrigin
}

// TotalDocumentHeight returns the total rendered pixel height of the document,
// including all inter-line gaps from paragraph breaks, heading spacing, and
// horizontal scrollbar space. This is computed from the last line's Y position
// plus its height after all layout adjustments, so it matches what
// layoutFromOrigin produces for rendering.
func (f *frameImpl) TotalDocumentHeight() int {
	if f.font == nil || f.content == nil {
		return 0
	}
	lines := f.ensureBaseLayout()
	if len(lines) == 0 {
		return 0
	}
	// ensureBaseLayout returns a fresh clone we may mutate.
	frameWidth := f.rect.Dx()

	// Apply the same scrollbar adjustments that layoutFromOrigin uses,
	// so the total height accounts for horizontal scrollbar space.
	scrollbarHeight := f.hscrollHeight
	regions := findBlockRegions(lines)
	adjustLayoutForScrollbars(lines, regions, frameWidth, scrollbarHeight)

	// Apply slide fill adjustments so scrollbar range accounts for expanded slides.
	slideRegions := findSlideRegions(lines)
	adjustLayoutForSlides(lines, slideRegions, f.rect.Dy())

	last := lines[len(lines)-1]
	return last.Y + last.Height
}

// Redraw redraws the frame.
func (f *frameImpl) Redraw() {
	if f.display == nil || f.background == nil {
		return
	}
	if f.rect.Dx() <= 0 || f.rect.Dy() <= 0 {
		return
	}

	screen := f.display.ScreenImage()

	// Ensure scratch image exists and is the right size.
	// The scratch image is used to clip text rendering - we draw to it first,
	// then blit to the screen. This prevents text from overflowing frame bounds.
	scratch := f.ensureScratchImage()
	if scratch == nil {
		// Fallback: draw directly to screen (no clipping for text)
		scratch = screen
	}

	// Calculate the destination rectangle for drawing.
	// If using scratch image, we draw at origin (0,0) since scratch is frame-sized.
	// If drawing directly to screen, we draw at f.rect.Min.
	var drawRect image.Rectangle
	var drawOffset image.Point // offset to add when calculating screen coordinates
	if scratch != screen {
		// Drawing to scratch: use local coordinates (0,0 origin)
		drawRect = image.Rect(0, 0, f.rect.Dx(), f.rect.Dy())
		drawOffset = image.ZP
	} else {
		// Drawing directly to screen: use frame coordinates
		drawRect = f.rect
		drawOffset = f.rect.Min
	}

	// Fill with background color
	scratch.Draw(drawRect, f.background, nil, image.ZP)

	// Draw text (and selection) if we have content, font, and text color.
	// Selection highlight is drawn inside drawTextTo after background phases
	// so that code block and inline code backgrounds don't overwrite it.
	if f.content != nil && f.font != nil && f.textColor != nil {
		f.drawTextTo(scratch, drawOffset)
	}

	// Draw cursor tick when selection is a point (p0 == p1)
	if f.content != nil && f.font != nil && f.display != nil && f.p0 == f.p1 {
		f.drawTickTo(scratch, drawOffset)
	}

	// If we used a scratch image, blit it to the screen
	if scratch != screen {
		screen.Draw(f.rect, scratch, nil, image.ZP)
	}
}

// ensureScratchImage allocates or resizes the scratch image to match frame dimensions.
// Returns nil if allocation fails or dimensions are zero.
func (f *frameImpl) ensureScratchImage() edwooddraw.Image {
	if f.rect.Dx() <= 0 || f.rect.Dy() <= 0 {
		return nil
	}
	frameSize := image.Rect(0, 0, f.rect.Dx(), f.rect.Dy())

	// Check if we already have a correctly-sized scratch image
	if f.scratchImage != nil && f.scratchRect.Eq(frameSize) {
		return f.scratchImage
	}

	// Free old scratch image if it exists
	if f.scratchImage != nil {
		f.scratchImage.Free()
		f.scratchImage = nil
	}

	// Allocate new scratch image
	pix := f.display.ScreenImage().Pix()
	img, err := f.display.AllocImage(frameSize, pix, false, 0)
	if err != nil {
		return nil
	}

	f.scratchImage = img
	f.scratchRect = frameSize
	return f.scratchImage
}

// paintCtx bundles the per-paint state shared across drawTextTo's
// phase methods. Built once at the top of drawTextTo and passed by
// pointer to each phase. Held only for the duration of one paint;
// no goroutines or cross-paint reuse.
type paintCtx struct {
	target          edwooddraw.Image
	offset          image.Point
	lines           []Line
	frameWidth      int
	frameHeight     int
	adjustedRegions []AdjustedBlockRegion
	// lineRegion[i] is the index into adjustedRegions for line i, or
	// -1 if the line is not part of a scrollable block region.
	lineRegion []int
}

// regionFor returns the adjustedRegions index for a line, or -1.
func (c *paintCtx) regionFor(lineIdx int) int {
	if lineIdx < 0 || lineIdx >= len(c.lineRegion) {
		return -1
	}
	return c.lineRegion[lineIdx]
}

// leftIndentForLine returns the LeftIndent for a line in a scrollable
// block region, or 0 if the line is not in one.
func (c *paintCtx) leftIndentForLine(lineIdx int) int {
	ri := c.regionFor(lineIdx)
	if ri < 0 {
		return 0
	}
	return c.adjustedRegions[ri].LeftIndent
}

// hOffsetForLine returns the horizontal scroll offset for a line.
// Needs the frame because hscroll origins live there; otherwise this
// would belong on paintCtx.
func (f *frameImpl) hOffsetForLine(c *paintCtx, lineIdx int) int {
	ri := c.regionFor(lineIdx)
	if ri < 0 {
		return 0
	}
	return f.GetHScrollOrigin(ri)
}

// buildPaintCtx assembles the per-paint context. Returns nil if there
// is nothing to render (no lines).
func (f *frameImpl) buildPaintCtx(target edwooddraw.Image, offset image.Point) *paintCtx {
	lines, _ := f.layoutFromOrigin()
	if len(lines) == 0 {
		return nil
	}
	frameWidth := f.rect.Dx()
	// Lines already have correct Y values (including scrollbar height)
	// from layoutFromOrigin, so use the read-only metadata variant
	// rather than adjustLayoutForScrollbars which would double-shift.
	regions := findBlockRegions(lines)
	adjustedRegions := computeScrollbarMetadata(lines, regions, frameWidth, f.hscrollHeight)
	return &paintCtx{
		target:          target,
		offset:          offset,
		lines:           lines,
		frameWidth:      frameWidth,
		frameHeight:     f.rect.Dy(),
		adjustedRegions: adjustedRegions,
		lineRegion:      buildLineRegionIndex(len(lines), adjustedRegions),
	}
}

// buildLineRegionIndex returns a slice mapping each line index to its
// adjustedRegions index, or -1 if the line is not part of any block
// region. Pulled out of buildPaintCtx to keep that function compact.
func buildLineRegionIndex(numLines int, adjustedRegions []AdjustedBlockRegion) []int {
	idx := make([]int, numLines)
	for i := range idx {
		idx[i] = -1
	}
	for ri, ar := range adjustedRegions {
		for li := ar.StartLine; li < ar.EndLine; li++ {
			if li < numLines {
				idx[li] = ri
			}
		}
	}
	return idx
}

// drawTextTo renders the content boxes onto the target image.
// The offset parameter specifies where the frame's (0,0) maps to in
// the target. When drawing to a scratch image, offset is (0,0); when
// drawing directly to screen, offset is f.rect.Min.
//
// The render is split into ordered phases. Order is load-bearing:
// later phases assume earlier ones have laid down their pixels.
//
//  1. block backgrounds       — full-line, behind everything else.
//  2. box backgrounds         — inline code etc., behind text.
//     2b. selection highlight     — between backgrounds and text, so
//     backgrounds don't overdraw it.
//  4. text                    — on top of backgrounds.
//  5. images and fixed boxes  — at their layout positions.
//     5b. gutter repaint          — clips horizontal-scroll overflow
//     that crossed into the gutter.
//  6. horizontal scrollbars   — on top of everything.
//
// Markdown-specific decoration phases (blockquote borders,
// horizontal rules, slide-break fills) live in rich/mdrender and
// are applied by the wrapping mdrender.Renderer after this paint
// pass returns. See docs/designs/features/markdown-externalization.md.
func (f *frameImpl) drawTextTo(target edwooddraw.Image, offset image.Point) {
	c := f.buildPaintCtx(target, offset)
	if c == nil {
		return
	}
	// Cache the adjusted regions for hit-testing (HScrollBarAt).
	f.hscrollRegions = c.adjustedRegions

	f.paintPhaseBlockBackgrounds(c)
	f.paintPhaseBoxBackgrounds(c)
	f.paintPhaseSelectionHighlight(c)
	f.paintPhaseText(c)
	f.paintPhaseImagesAndFixedBoxes(c)
	f.paintPhaseGutterRepaint(c)
	f.paintPhaseHScrollbars(c)
}

// paintPhaseBlockBackgrounds draws full-line backgrounds for fenced
// code blocks (Phase 1). These are not shifted by horizontal scroll —
// the colored band remains full-width as the inner text scrolls
// underneath.
func (f *frameImpl) paintPhaseBlockBackgrounds(c *paintCtx) {
	for _, line := range c.lines {
		if line.Y >= c.frameHeight {
			break
		}
		for _, pb := range line.Boxes {
			if pb.Box.Style.Block && pb.Box.Style.Bg != nil {
				f.drawBlockBackgroundTo(c.target, line, c.offset, c.frameWidth, c.frameHeight)
				break // only once per line
			}
		}
	}
}

// paintPhaseBoxBackgrounds draws per-box backgrounds for inline-code
// and similar Bg-styled spans (Phase 2). Shifted by -hOffset inside
// scrollable block regions so backgrounds track the text.
func (f *frameImpl) paintPhaseBoxBackgrounds(c *paintCtx) {
	for lineIdx, line := range c.lines {
		if line.Y >= c.frameHeight {
			break
		}
		hOff := f.hOffsetForLine(c, lineIdx)
		for _, pb := range line.Boxes {
			if pb.Box.IsNewline() || pb.Box.IsTab() {
				continue
			}
			if len(pb.Box.Text) == 0 {
				continue
			}
			// Block-level Bg is handled in Phase 1.
			if pb.Box.Style.Bg != nil && !pb.Box.Style.Block {
				shiftedPB := pb
				shiftedPB.X -= hOff
				f.drawBoxBackgroundTo(c.target, shiftedPB, line, c.offset, c.frameWidth, c.frameHeight)
			}
		}
	}
}

// paintPhaseSelectionHighlight draws the selection rectangle (Phase
// 2b). Sits between Phases 2 and 4 so backgrounds don't overdraw the
// highlight and text appears on top of it.
func (f *frameImpl) paintPhaseSelectionHighlight(c *paintCtx) {
	if f.selectionColor != nil && f.p0 != f.p1 {
		f.drawSelectionTo(c.target, c.offset)
	}
}

// paintPhaseText renders glyphs (Phase 4). Skips boxes that other
// phases own (newlines, tabs, images, fixed boxes, horizontal rules).
// Text inside a scrollable block region is shifted by -hOffset.
func (f *frameImpl) paintPhaseText(c *paintCtx) {
	for lineIdx, line := range c.lines {
		if line.Y >= c.frameHeight {
			break
		}
		hOff := f.hOffsetForLine(c, lineIdx)
		for _, pb := range line.Boxes {
			if pb.Box.IsNewline() || pb.Box.IsTab() {
				continue
			}
			if (pb.Box.Style.Image && !pb.Box.Style.ImageBelow) || pb.Box.IsFixedBox() {
				continue // Phase 5 — but ImageBelow renders source text normally; the image paints below in paintLineImagesBelow.
			}
			if len(pb.Box.Text) == 0 {
				continue
			}
			// HRule-styled boxes used to be skipped here so that
			// the rich/mdrender wrapper's rule painter could draw
			// over a blank line. Phase 3 round 3 (April 2026)
			// removed the skip per user feedback: source markers
			// should remain visible alongside the rendered rule,
			// matching the "markup remains visible" stance of all
			// other md2spans-emitted markdown features. The rule
			// line is still drawn by rich/mdrender; the markers
			// (`---`/`***`/`___` from md2spans, or HRune chars
			// from the in-tree markdown path) now show through.
			pt := image.Point{
				X: c.offset.X + pb.X - hOff,
				Y: c.offset.Y + line.Y,
			}
			textColorImg := f.textColor
			if pb.Box.Style.Fg != nil {
				if colorImg := f.allocColorImage(pb.Box.Style.Fg); colorImg != nil {
					textColorImg = colorImg
				}
			}
			boxFont := f.fontForStyle(pb.Box.Style)
			c.target.Bytes(pt, textColorImg, image.ZP, boxFont, pb.Box.Text)
		}
	}
}

// paintPhaseImagesAndFixedBoxes renders inline images, image
// placeholders (loading / error), and fixed-rectangle boxes (Phase 5).
// Images within a scrollable block region are shifted by -hOffset;
// fixed boxes are not (they're full-width markers).
//
// ImageBelow boxes (Phase 3 round 4) are routed to a separate
// helper that paints them stacked below the line's text rather
// than at the line's top.
func (f *frameImpl) paintPhaseImagesAndFixedBoxes(c *paintCtx) {
	for lineIdx, line := range c.lines {
		if line.Y >= c.frameHeight {
			break
		}
		hOff := f.hOffsetForLine(c, lineIdx)
		// Two passes per line: first inline-replacing images and
		// fixed boxes (existing behavior), then ImageBelow boxes
		// which paint below the line's text.
		for _, pb := range line.Boxes {
			if pb.Box.IsFixedBox() && !pb.Box.Style.Image {
				pt := image.Point{X: c.offset.X + pb.X, Y: c.offset.Y + line.Y}
				f.drawFixedBox(c.target, pt, pb.Box, c.frameWidth, c.frameHeight, c.offset)
				continue
			}
			if pb.Box.Style.Image && !pb.Box.Style.ImageBelow {
				f.paintImageBox(c, line, pb, hOff)
			}
		}
		f.paintLineImagesBelow(c, line)
	}
}

// paintLineImagesBelow paints any ImageBelow-styled boxes on the
// given line, stacked top-to-bottom in box-emission order. Each
// is anchored at the line's left edge (X = c.offset.X) and at
// Y = line.Y + textHeight + sum(prior_below_image_heights), so
// the source `s` text on the same line stays visible above it.
// Phase 3 round 4.
func (f *frameImpl) paintLineImagesBelow(c *paintCtx, line Line) {
	textHeight := lineTextHeight(line, c.frameWidth)
	cumulativeBelow := 0
	for _, pb := range line.Boxes {
		if !pb.Box.Style.Image || !pb.Box.Style.ImageBelow {
			continue
		}
		_, imgHeight := imageBoxDimensions(&pb.Box, c.frameWidth)
		// Place the image at the line's left edge; the box's pb.X
		// (the rune-anchor's X within the line) only determines
		// stacking order, not horizontal position.
		shifted := pb
		shifted.X = 0
		// Synthesize a Line whose Y is at the image's draw row so
		// drawImageTo/paintImageBox compute the right destination
		// without needing a separate code path.
		anchored := line
		anchored.Y = line.Y + textHeight + cumulativeBelow
		f.paintImageBox(c, anchored, shifted, 0)
		cumulativeBelow += imgHeight
	}
}

// lineTextHeight returns the line's text-only height: total
// Height minus the sum of any ImageBelow heights on the line.
// Inline-replacing images contribute to text height via
// max(text, image) per the existing rule.
func lineTextHeight(line Line, frameWidth int) int {
	belowSum := 0
	for _, pb := range line.Boxes {
		if pb.Box.Style.Image && pb.Box.Style.ImageBelow {
			_, h := imageBoxDimensions(&pb.Box, frameWidth)
			belowSum += h
		}
	}
	if line.Height-belowSum < 0 {
		return 0
	}
	return line.Height - belowSum
}

// paintImageBox draws one image-styled box: a loading placeholder, an
// error placeholder, or the actual rendered image. Extracted from the
// Phase 5 loop so the orchestrator stays compact and the per-box
// branch logic is easy to follow.
func (f *frameImpl) paintImageBox(c *paintCtx, line Line, pb PositionedBox, hOff int) {
	pt := image.Point{X: c.offset.X + pb.X - hOff, Y: c.offset.Y + line.Y}
	switch {
	case pb.Box.ImageData != nil && pb.Box.ImageData.Loading:
		f.drawImageLoadingPlaceholder(c.target, pt, string(pb.Box.Text))
	case pb.Box.ImageData != nil && pb.Box.ImageData.Err != nil:
		f.drawImageErrorPlaceholder(c.target, pt, string(pb.Box.Text))
	case !pb.Box.IsImage():
		f.drawImageErrorPlaceholder(c.target, pt, string(pb.Box.Text))
	default:
		shiftedPB := pb
		shiftedPB.X -= hOff
		f.drawImageTo(c.target, shiftedPB, line, c.offset, c.frameWidth, c.frameHeight)
	}
}

// paintPhaseGutterRepaint clips horizontal-scroll overflow (Phase 5b).
// When a block region is horizontally scrolled, text may render to the
// LEFT of the block's LeftIndent. Repaint the gutter column
// [0, LeftIndent) with the frame background so that overflow is hidden.
func (f *frameImpl) paintPhaseGutterRepaint(c *paintCtx) {
	for lineIdx, line := range c.lines {
		if line.Y >= c.frameHeight {
			break
		}
		indent := c.leftIndentForLine(lineIdx)
		if indent <= 0 || f.hOffsetForLine(c, lineIdx) <= 0 {
			continue
		}
		gutterRect := image.Rect(
			c.offset.X,
			c.offset.Y+line.Y,
			c.offset.X+indent,
			c.offset.Y+line.Y+line.Height,
		)
		clipRect := image.Rect(c.offset.X, c.offset.Y, c.offset.X+c.frameWidth, c.offset.Y+c.frameHeight)
		gutterRect = gutterRect.Intersect(clipRect)
		if !gutterRect.Empty() {
			c.target.Draw(gutterRect, f.background, nil, image.ZP)
		}
	}
}

// paintPhaseHScrollbars draws horizontal scrollbars for overflowing
// block regions (Phase 6). Drawn last so it sits on top of everything.
func (f *frameImpl) paintPhaseHScrollbars(c *paintCtx) {
	f.drawHScrollbarsTo(c.target, c.offset, c.lines, c.adjustedRegions, c.frameWidth)
}

// drawBlockBackgroundTo draws a full-width background for a line.
// This is used for fenced code blocks where the background extends to the frame edge.
func (f *frameImpl) drawBlockBackgroundTo(target edwooddraw.Image, line Line, offset image.Point, frameWidth, frameHeight int) {
	// Find the background color and left indent from a block-styled box on this line
	var bgColor color.Color
	leftIndent := -1 // -1 means "not found yet"
	for _, pb := range line.Boxes {
		if pb.Box.Style.Block && pb.Box.Style.Bg != nil {
			bgColor = pb.Box.Style.Bg
			// Only use this box's X if it's not a newline. Newline boxes on
			// blank lines are positioned at X=0, but the background should
			// still respect the code block indent.
			if !pb.Box.IsNewline() {
				leftIndent = pb.X
			}
			break
		}
	}
	if bgColor == nil {
		return
	}

	// If we didn't find a valid indent (blank line with only a newline box),
	// compute the expected code block indent from font metrics.
	if leftIndent < 0 {
		leftIndent = f.computeCodeBlockIndent()
	}

	bgImg := f.allocColorImage(bgColor)
	if bgImg == nil {
		return
	}

	// Background from indent to right edge (not full-width)
	bgRect := image.Rect(
		offset.X+leftIndent,
		offset.Y+line.Y,
		offset.X+frameWidth,
		offset.Y+line.Y+line.Height,
	)

	// Clip to frame bounds (in target coordinates)
	clipRect := image.Rect(offset.X, offset.Y, offset.X+frameWidth, offset.Y+frameHeight)
	bgRect = bgRect.Intersect(clipRect)
	if bgRect.Empty() {
		return
	}

	target.Draw(bgRect, bgImg, nil, image.ZP)
}

// computeCodeBlockIndent returns the expected left indent for block elements,
// computed from font metrics (GutterIndentChars * M-width of the base font).
// Must use the base font to match the layout's gutterIndent calculation.
func (f *frameImpl) computeCodeBlockIndent() int {
	if f.font == nil {
		return CodeBlockIndent // Fallback to default constant
	}
	return CodeBlockIndentChars * f.font.BytesWidth([]byte("M"))
}

// drawBoxBackgroundTo draws the background color for a positioned box.
// This is used for inline code backgrounds and other text-width backgrounds.
func (f *frameImpl) drawBoxBackgroundTo(target edwooddraw.Image, pb PositionedBox, line Line, offset image.Point, frameWidth, frameHeight int) {
	bgImg := f.allocColorImage(pb.Box.Style.Bg)
	if bgImg == nil {
		return
	}

	// Calculate the background rectangle for this box
	// X: from box start to box start + box width
	// Y: from line top to line top + line height
	bgRect := image.Rect(
		offset.X+pb.X,
		offset.Y+line.Y,
		offset.X+pb.X+pb.Box.Wid,
		offset.Y+line.Y+line.Height,
	)

	// Clip to frame bounds (in target coordinates)
	clipRect := image.Rect(offset.X, offset.Y, offset.X+frameWidth, offset.Y+frameHeight)
	bgRect = bgRect.Intersect(clipRect)
	if bgRect.Empty() {
		return
	}

	target.Draw(bgRect, bgImg, nil, image.ZP)
}

// layoutFromOrigin returns the layout lines starting from the origin position.
// It skips content before the origin and adjusts Y coordinates so that the
// first visible content starts at Y=0.
// Returns the lines and the rune offset of the first visible content.
func (f *frameImpl) layoutFromOrigin() ([]Line, int) {
	// ensureBaseLayout returns a fresh clone, so we may mutate Y
	// freely without affecting the cache. Only one of the two
	// branches below executes per call, so the single clone suffices.
	allLines := f.ensureBaseLayout()
	if len(allLines) == 0 {
		return nil, 0
	}

	frameWidth := f.rect.Dx()

	// If origin is 0 and no pixel offset, just return the normal layout.
	if f.origin == 0 && f.originYOffset == 0 {
		regions := findBlockRegions(allLines)
		f.syncHScrollState(len(regions))
		f.hscrollRegionOffset = 0
		// Apply scrollbar height adjustments so all callers get correct Y.
		scrollbarHeight := f.hscrollHeight
		adjustLayoutForScrollbars(allLines, regions, frameWidth, scrollbarHeight)
		// Apply slide fill adjustments.
		slideRegions := findSlideRegions(allLines)
		adjustLayoutForSlides(allLines, slideRegions, f.rect.Dy())
		f.applyScrollSnap(allLines, 0, allLines)
		return allLines, 0
	}

	// Sync horizontal scroll state and apply scrollbar height adjustments
	// to ALL lines BEFORE computing originY. This ensures originY accounts
	// for scrollbar heights of blocks above the viewport.
	regions := findBlockRegions(allLines)
	f.syncHScrollState(len(regions))
	scrollbarHeight := f.hscrollHeight
	adjustLayoutForScrollbars(allLines, regions, frameWidth, scrollbarHeight)

	// Find which line contains the origin position.
	// Line Y values now include scrollbar heights.
	runeCount := 0
	startLineIdx := 0
	originY := 0

	for lineIdx, line := range allLines {
		lineStartRune := runeCount
		for _, pb := range line.Boxes {
			if pb.Box.IsNewline() || pb.Box.IsTab() {
				runeCount++
			} else {
				runeCount += pb.Box.Nrune
			}
		}

		// Check if origin is within or at the start of this line
		if f.origin >= lineStartRune && f.origin < runeCount {
			startLineIdx = lineIdx
			originY = line.Y
			break
		}
		// If we've passed the origin position, the origin was at the end of the previous line
		if f.origin < runeCount {
			startLineIdx = lineIdx
			originY = line.Y
			break
		}
		// Keep track of the last line in case origin is past all content
		startLineIdx = lineIdx
		originY = line.Y
	}

	// Clamp originYOffset: if it exceeds the origin line's height,
	// advance to subsequent lines. This handles content changes where
	// line heights shrink (e.g., after an async image load).
	yOffset := f.originYOffset
	for startLineIdx < len(allLines) && yOffset >= allLines[startLineIdx].Height {
		if startLineIdx+1 >= len(allLines) {
			// Can't advance past the last line; clamp to max offset.
			yOffset = allLines[startLineIdx].Height - 1
			if yOffset < 0 {
				yOffset = 0
			}
			break
		}
		yOffset -= allLines[startLineIdx].Height
		startLineIdx++
		originY = allLines[startLineIdx].Y
	}

	// Count block regions entirely above the viewport.
	offset := 0
	for _, r := range regions {
		if r.EndLine <= startLineIdx {
			offset++
		}
	}
	f.hscrollRegionOffset = offset

	// Extract lines from the origin line onwards and adjust Y coordinates.
	// Y values already include scrollbar heights from the adjustment above.
	// Apply originYOffset so the first visible line starts at Y = -yOffset.
	visibleLines := make([]Line, 0, len(allLines)-startLineIdx)
	for i := startLineIdx; i < len(allLines); i++ {
		line := allLines[i]
		adjustedLine := Line{
			Y:            line.Y - originY - yOffset,
			Height:       line.Height,
			ContentWidth: line.ContentWidth,
			Boxes:        line.Boxes,
		}
		visibleLines = append(visibleLines, adjustedLine)
	}

	// Apply slide fill adjustments (viewport-aware: only expands
	// when both HRules of a pair have Y >= 0).
	slideRegions := findSlideRegions(visibleLines)
	adjustLayoutForSlides(visibleLines, slideRegions, f.rect.Dy())
	f.applyScrollSnap(visibleLines, startLineIdx, allLines)

	return visibleLines, f.origin
}

// applyScrollSnap dispatches to the configured snap behavior with
// two edge-case overrides:
//
//   - At file top (origin=0, originYOffset=0) snap is forced to
//     SnapTop so the user can always reach the first line aligned
//     to the viewport top.
//   - When the origin line is taller than the frame (e.g. a large
//     image) snap is forced to SnapPixel so the user can scroll
//     within the line via originYOffset; line-level snapping has
//     no clean anchor in this regime.
//
// SnapTop and SnapPixel are no-ops at this layer — the layout
// already aligns the origin line's top to the viewport top, with
// originYOffset accounted for in the visible-lines Y values.
// SnapBottom calls applyBottomShift, which is the legacy
// applySnapBottomLine logic verbatim.
//
// See docs/designs/features/unified-scrollbar.md § "Scroll snap
// policy (rich mode)" for the full policy.
func (f *frameImpl) applyScrollSnap(visible []Line, startIdx int, all []Line) {
	if len(visible) == 0 {
		return
	}
	snap := f.scrollSnap
	switch {
	case f.origin == 0 && f.originYOffset == 0:
		snap = SnapTop
	case startIdx >= 0 && startIdx < len(all) && all[startIdx].Height > f.rect.Dy():
		snap = SnapPixel
	}
	if snap == SnapBottom {
		f.applyBottomShift(visible)
	}
}

// applyBottomShift shifts all line Y values up so that the last
// visible line ends exactly at the frame bottom. This makes the
// top line absorb the partial-line clipping instead of the bottom
// line. Identical to the legacy applySnapBottomLine; only the name
// changed (the snap-vs-not decision moved to applyScrollSnap).
func (f *frameImpl) applyBottomShift(lines []Line) {
	if len(lines) == 0 {
		return
	}
	frameHeight := f.rect.Dy()
	// Find the last line that starts within the viewport.
	lastIdx := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i].Y < frameHeight {
			lastIdx = i
			break
		}
	}
	if lastIdx < 0 {
		return
	}
	overflow := lines[lastIdx].Y + lines[lastIdx].Height - frameHeight
	if overflow <= 0 {
		return
	}
	for i := range lines {
		lines[i].Y -= overflow
	}
}

// drawSelectionTo renders the selection highlight rectangles.
// The selection spans from p0 to p1 (rune offsets).
// For multi-line selections, multiple rectangles are drawn.
func (f *frameImpl) drawSelectionTo(target edwooddraw.Image, offset image.Point) {
	// Use layoutFromOrigin to get viewport-relative lines and origin rune offset.
	// Selection positions (f.p0, f.p1) are content-absolute, so we subtract
	// originRune to compare against viewport-relative rune counting.
	lines, originRune := f.layoutFromOrigin()
	if len(lines) == 0 {
		return
	}

	frameWidth := f.rect.Dx()
	frameHeight := f.rect.Dy()

	p0, p1 := f.p0, f.p1
	if p0 > p1 {
		p0, p1 = p1, p0
	}
	// Adjust selection to viewport-relative rune positions
	p0 -= originRune
	p1 -= originRune

	// Walk through lines and boxes, tracking rune position
	runePos := 0
	for _, line := range lines {
		// Skip lines that start at or below the frame bottom
		if line.Y >= frameHeight {
			break
		}

		lineStartRune := runePos
		lineEndRune := lineStartRune

		// Calculate the end rune position for this line
		for _, pb := range line.Boxes {
			if pb.Box.IsNewline() || pb.Box.IsTab() {
				lineEndRune++
			} else {
				lineEndRune += pb.Box.Nrune
			}
		}

		// Check if this line overlaps with the selection
		if lineEndRune <= p0 || lineStartRune >= p1 {
			// No overlap with selection, skip this line
			runePos = lineEndRune
			continue
		}

		// This line has selected content - calculate the selection rectangle
		selStartX := -1 // Start of selection on this line (relative to line start)
		selEndX := 0    // End of selection on this line

		boxRunePos := lineStartRune
		for _, pb := range line.Boxes {
			boxRunes := pb.Box.Nrune
			if pb.Box.IsNewline() || pb.Box.IsTab() {
				boxRunes = 1
			}

			boxStartRune := boxRunePos
			boxEndRune := boxStartRune + boxRunes

			// Check if selection starts in or before this box (only set once)
			if selStartX < 0 {
				if p0 <= boxStartRune {
					// Selection starts at or before this box
					selStartX = pb.X
				} else if p0 > boxStartRune && p0 < boxEndRune {
					// Selection starts within this box
					if pb.Box.IsNewline() || pb.Box.IsTab() {
						selStartX = pb.X
					} else {
						// Calculate partial position within the box
						runeOffset := p0 - boxStartRune
						selStartX = pb.X + f.runeWidthInBox(&pb.Box, runeOffset)
					}
				}
			}

			// Check if selection ends in or after this box
			if p1 >= boxEndRune {
				// Selection extends past this box
				selEndX = pb.X + pb.Box.Wid
			} else if p1 > boxStartRune && p1 < boxEndRune {
				// Selection ends within this box
				if pb.Box.IsNewline() || pb.Box.IsTab() {
					selEndX = pb.X + pb.Box.Wid
				} else {
					// Calculate partial position within the box
					runeOffset := p1 - boxStartRune
					selEndX = pb.X + f.runeWidthInBox(&pb.Box, runeOffset)
				}
			}

			boxRunePos = boxEndRune
		}

		// If the line ends with a selected newline, extend highlight to right margin
		// so the user can see that the newline is included in the selection.
		if len(line.Boxes) > 0 && line.Boxes[len(line.Boxes)-1].Box.IsNewline() && p1 >= lineEndRune {
			selEndX = frameWidth
		}

		// If selStartX wasn't set, default to 0
		if selStartX < 0 {
			selStartX = 0
		}
		if selEndX > frameWidth {
			selEndX = frameWidth
		}

		// Draw the selection rectangle for this line
		if selEndX > selStartX {
			selRect := image.Rect(
				offset.X+selStartX,
				offset.Y+line.Y,
				offset.X+selEndX,
				offset.Y+line.Y+line.Height,
			)
			// Clip to frame bounds (in target coordinates)
			clipRect := image.Rect(offset.X, offset.Y, offset.X+frameWidth, offset.Y+frameHeight)
			selRect = selRect.Intersect(clipRect)
			if !selRect.Empty() {
				color := f.selectionColor
				if f.sweepColor != nil {
					color = f.sweepColor
				}
				target.Draw(selRect, color, nil, image.ZP)
			}
		}

		runePos = lineEndRune
	}
}

// runeWidthInBox calculates the pixel width of the first n runes in a text box.
func (f *frameImpl) runeWidthInBox(box *Box, n int) int {
	if n <= 0 {
		return 0
	}
	text := box.Text
	byteOffset := 0
	for i := 0; i < n && byteOffset < len(text); i++ {
		_, size := utf8.DecodeRune(text[byteOffset:])
		byteOffset += size
	}
	return f.fontForStyle(box.Style).BytesWidth(text[:byteOffset])
}

// allocColorImage returns a 1x1 replicated image for the given color,
// caching by packed RGBA so repeated calls (and equal colors expressed
// as different color.Color implementations) reuse the same handle.
//
// The cache lives for the frame's lifetime; entries are bounded by the
// number of unique colors the document uses. See colorCache field
// comment for the leak history this fixes.
func (f *frameImpl) allocColorImage(c color.Color) edwooddraw.Image {
	if f.display == nil {
		return nil
	}

	r, g, b, a := c.RGBA()
	// RGBA returns 0-65535; scale to 0-255 to match draw.Color packing.
	drawColor := edwooddraw.Color(uint32(r>>8)<<24 | uint32(g>>8)<<16 | uint32(b>>8)<<8 | uint32(a>>8))

	if img, ok := f.colorCache[drawColor]; ok {
		return img
	}
	img, err := f.display.AllocImage(image.Rect(0, 0, 1, 1), f.display.ScreenImage().Pix(), true, drawColor)
	if err != nil {
		return nil
	}
	if f.colorCache == nil {
		f.colorCache = make(map[edwooddraw.Color]edwooddraw.Image)
	}
	f.colorCache[drawColor] = img
	return img
}

// DefaultFontHeight returns the height of the default font.
func (f *frameImpl) DefaultFontHeight() int {
	if f.font != nil {
		return f.font.Height()
	}
	return 0
}

// initTick creates or recreates the tick image when the required height changes.
// The tick image is a transparent mask with an opaque vertical line and serif boxes,
// matching the pattern from frame/tick.go:InitTick().
func (f *frameImpl) initTick(height int) {
	if f.display == nil {
		return
	}
	if f.tickImage != nil && f.tickHeight == height {
		return
	}
	if f.tickImage != nil {
		f.tickImage.Free()
		f.tickImage = nil
	}

	scale := f.display.ScaleSize(1)
	f.tickScale = scale
	w := frtickw * scale

	b := f.display.ScreenImage()
	img, err := f.display.AllocImage(
		image.Rect(0, 0, w, height),
		b.Pix(), false, edwooddraw.Transparent)
	if err != nil {
		return
	}

	// Fill transparent
	img.Draw(img.R(), f.display.Transparent(), nil, image.ZP)
	// Vertical line in center
	img.Draw(image.Rect(scale*(frtickw/2), 0, scale*(frtickw/2+1), height),
		f.display.Opaque(), nil, image.ZP)
	// Top serif box
	img.Draw(image.Rect(0, 0, w, w),
		f.display.Opaque(), nil, image.ZP)
	// Bottom serif box
	img.Draw(image.Rect(0, height-w, w, height),
		f.display.Opaque(), nil, image.ZP)

	f.tickImage = img
	f.tickHeight = height
}

// boxHeight returns the height of a box in pixels.
// For text boxes, this is the font height for the box's style.
// For image boxes, this is the scaled image height (via imageBoxDimensions).
func (f *frameImpl) boxHeight(box Box) int {
	if box.Style.Image && box.IsImage() {
		_, h := imageBoxDimensions(&box, f.rect.Dx())
		if h > 0 {
			return h
		}
	}
	if box.IsFixedBox() {
		return box.Style.ImageHeight
	}
	return f.fontForStyle(box.Style).Height()
}

// drawTickTo draws the cursor tick (insertion bar) on the target image when
// the selection is a point (p0 == p1). It walks the layout to find the cursor
// position, determines height from the tallest adjacent box, and draws the tick.
func (f *frameImpl) drawTickTo(target edwooddraw.Image, offset image.Point) {
	if f.display == nil || f.font == nil {
		return
	}

	lines, originRune := f.layoutFromOrigin()
	if len(lines) == 0 {
		return
	}

	cursorPos := f.p0 - originRune
	if cursorPos < 0 {
		return
	}

	// Walk lines and boxes to find the cursor position, its X coordinate,
	// and the heights of adjacent boxes.
	runeCount := 0
	for _, line := range lines {
		for i, pb := range line.Boxes {
			boxRunes := pb.Box.Nrune
			if pb.Box.IsNewline() || pb.Box.IsTab() {
				boxRunes = 1
			}

			// Check if cursor is at the start of this box
			if runeCount == cursorPos {
				x := pb.X

				// Adjacent heights: prev box (if any) and this box
				prevHeight := 0
				if i > 0 {
					prevHeight = f.boxHeight(line.Boxes[i-1].Box)
				}
				nextHeight := f.boxHeight(pb.Box)
				tickH := prevHeight
				if nextHeight > tickH {
					tickH = nextHeight
				}
				if tickH == 0 {
					tickH = f.font.Height()
				}

				f.initTick(tickH)
				if f.tickImage == nil {
					return
				}

				w := frtickw * f.tickScale
				r := image.Rect(
					offset.X+x, offset.Y+line.Y,
					offset.X+x+w, offset.Y+line.Y+tickH,
				)
				target.Draw(r, f.display.Black(), f.tickImage, image.ZP)
				return
			}

			// Check if cursor is within this box
			if runeCount+boxRunes > cursorPos {
				// Cursor is inside this box — compute X offset within the box
				runeOffset := cursorPos - runeCount
				var x int
				if pb.Box.IsNewline() || pb.Box.IsTab() {
					x = pb.X
				} else {
					byteOffset := 0
					text := pb.Box.Text
					for j := 0; j < runeOffset && byteOffset < len(text); j++ {
						_, size := utf8.DecodeRune(text[byteOffset:])
						byteOffset += size
					}
					x = pb.X + f.fontForStyle(pb.Box.Style).BytesWidth(text[:byteOffset])
				}

				// The cursor is within this box, so both adjacent boxes are this box
				tickH := f.boxHeight(pb.Box)
				if tickH == 0 {
					tickH = f.font.Height()
				}

				f.initTick(tickH)
				if f.tickImage == nil {
					return
				}

				w := frtickw * f.tickScale
				r := image.Rect(
					offset.X+x, offset.Y+line.Y,
					offset.X+x+w, offset.Y+line.Y+tickH,
				)
				target.Draw(r, f.display.Black(), f.tickImage, image.ZP)
				return
			}

			runeCount += boxRunes
		}
	}

	// Cursor is at end of content — use last box's height
	if len(lines) > 0 {
		lastLine := lines[len(lines)-1]
		// Compute X at end of last line
		endX := 0
		for _, pb := range lastLine.Boxes {
			if pb.Box.IsNewline() {
				endX = 0 // after newline, cursor is at start of next line
			} else {
				endX = pb.X + pb.Box.Wid
			}
		}

		tickH := f.font.Height()
		if len(lastLine.Boxes) > 0 {
			lastBox := lastLine.Boxes[len(lastLine.Boxes)-1].Box
			h := f.boxHeight(lastBox)
			if h > 0 {
				tickH = h
			}
		}

		f.initTick(tickH)
		if f.tickImage == nil {
			return
		}

		y := lastLine.Y
		// If last box was a newline, cursor goes on next line
		if len(lastLine.Boxes) > 0 && lastLine.Boxes[len(lastLine.Boxes)-1].Box.IsNewline() {
			y = lastLine.Y + lastLine.Height
			endX = 0
		}

		w := frtickw * f.tickScale
		r := image.Rect(
			offset.X+endX, offset.Y+y,
			offset.X+endX+w, offset.Y+y+tickH,
		)
		target.Draw(r, f.display.Black(), f.tickImage, image.ZP)
	}
}

// syncHScrollState updates the horizontal scroll origins slice after layout.
// If the block region count changed, the slice is reset to zero values.
// If the count is unchanged, existing scroll positions are preserved.
func (f *frameImpl) syncHScrollState(regionCount int) {
	if regionCount != f.hscrollBlockCount {
		f.hscrollOrigins = make([]int, regionCount)
		f.hscrollBlockCount = regionCount
	}
}

// SetHScrollOrigin sets the horizontal scroll offset for a block region by index.
// The regionIndex is viewport-local; hscrollRegionOffset is added to map it to
// the global hscrollOrigins slice. Out-of-range indices are ignored.
func (f *frameImpl) SetHScrollOrigin(regionIndex, pixelOffset int) {
	idx := regionIndex + f.hscrollRegionOffset
	if idx < 0 || idx >= len(f.hscrollOrigins) {
		return
	}
	f.hscrollOrigins[idx] = pixelOffset
}

// GetHScrollOrigin returns the horizontal scroll offset for a block region by index.
// The regionIndex is viewport-local; hscrollRegionOffset is added to map it to
// the global hscrollOrigins slice. Out-of-range indices return 0.
func (f *frameImpl) GetHScrollOrigin(regionIndex int) int {
	idx := regionIndex + f.hscrollRegionOffset
	if idx < 0 || idx >= len(f.hscrollOrigins) {
		return 0
	}
	return f.hscrollOrigins[idx]
}

// HScrollBarAt checks if the given screen point falls within any horizontal
// scrollbar rectangle. Returns the region index and true if hit, or (0, false)
// if the point is not on a scrollbar.
func (f *frameImpl) HScrollBarAt(pt image.Point) (regionIndex int, ok bool) {
	scrollbarHeight := f.hscrollHeight
	frameWidth := f.rect.Dx()

	// Convert screen point to frame-relative coordinates
	relX := pt.X - f.rect.Min.X
	relY := pt.Y - f.rect.Min.Y

	for i, ar := range f.hscrollRegions {
		if !ar.HasScrollbar {
			continue
		}
		// Scrollbar rectangle: [LeftIndent, frameWidth) x [ScrollbarY, ScrollbarY+scrollbarHeight)
		if relX >= ar.LeftIndent && relX < frameWidth &&
			relY >= ar.ScrollbarY && relY < ar.ScrollbarY+scrollbarHeight {
			return i, true
		}
	}
	return 0, false
}

// HScrollBarRect returns the screen-coordinate rectangle of the horizontal
// scrollbar for the given block region. Returns the zero rectangle if the
// region index is out of range or the region has no scrollbar.
func (f *frameImpl) HScrollBarRect(regionIndex int) image.Rectangle {
	scrollbarHeight := f.hscrollHeight
	if regionIndex < 0 || regionIndex >= len(f.hscrollRegions) {
		return image.Rectangle{}
	}
	ar := f.hscrollRegions[regionIndex]
	if !ar.HasScrollbar {
		return image.Rectangle{}
	}
	return image.Rect(
		f.rect.Min.X+ar.LeftIndent,
		f.rect.Min.Y+ar.ScrollbarY,
		f.rect.Max.X,
		f.rect.Min.Y+ar.ScrollbarY+scrollbarHeight,
	)
}

// HScrollClick handles a mouse click on a horizontal scrollbar with acme
// three-button semantics. button is 1, 2, or 3. pt is the screen-coordinate
// click point. regionIndex identifies which block region's scrollbar was clicked.
// B1 scrolls left (amount proportional to click X within scrollbar).
// B2 jumps to an absolute horizontal position.
// B3 scrolls right (amount proportional to click X within scrollbar).
// The resulting offset is clamped to [0, maxScrollable].
func (f *frameImpl) HScrollClick(button int, pt image.Point, regionIndex int) {
	if regionIndex < 0 || regionIndex >= len(f.hscrollRegions) {
		return
	}
	ar := f.hscrollRegions[regionIndex]
	if !ar.HasScrollbar {
		return
	}

	frameWidth := f.rect.Dx()
	maxScrollable := ar.MaxContentWidth - frameWidth
	if maxScrollable <= 0 {
		return
	}

	// Compute click X proportion within the scrollbar (0.0 = left edge, 1.0 = right edge).
	// The scrollbar starts at ar.LeftIndent, not at X=0.
	scrollbarWidth := frameWidth - ar.LeftIndent
	if scrollbarWidth <= 0 {
		return
	}
	relX := pt.X - f.rect.Min.X - ar.LeftIndent
	if relX < 0 {
		relX = 0
	}
	if relX > scrollbarWidth {
		relX = scrollbarWidth
	}
	clickProportion := float64(relX) / float64(scrollbarWidth)

	currentOffset := f.GetHScrollOrigin(regionIndex)
	var newOffset int

	switch button {
	case 1:
		// B1: scroll left by frameWidth scaled by (1 - clickProportion).
		// Clicking near the left edge scrolls more, near the right edge less.
		pixelsToMove := int(float64(frameWidth) * (1.0 - clickProportion))
		if pixelsToMove < 1 {
			pixelsToMove = 1
		}
		newOffset = currentOffset - pixelsToMove

	case 2:
		// B2: jump to absolute position proportional to click X.
		newOffset = int(float64(maxScrollable) * clickProportion)

	case 3:
		// B3: scroll right by frameWidth scaled by clickProportion.
		// Clicking near the right edge scrolls more, near the left edge less.
		pixelsToMove := int(float64(frameWidth) * clickProportion)
		if pixelsToMove < 1 {
			pixelsToMove = 1
		}
		newOffset = currentOffset + pixelsToMove

	default:
		return
	}

	// Clamp to [0, maxScrollable]
	if newOffset < 0 {
		newOffset = 0
	}
	if newOffset > maxScrollable {
		newOffset = maxScrollable
	}

	f.SetHScrollOrigin(regionIndex, newOffset)
}

// PointInBlockRegion checks if the given screen point falls within any
// horizontally-scrollable block region (the content area, including the
// scrollbar area). Returns the region index and true if hit, or (0, false)
// if the point is not within any scrollable block region.
func (f *frameImpl) PointInBlockRegion(pt image.Point) (regionIndex int, ok bool) {
	frameWidth := f.rect.Dx()

	// Convert screen point to frame-relative coordinates.
	relX := pt.X - f.rect.Min.X
	relY := pt.Y - f.rect.Min.Y

	for i, ar := range f.hscrollRegions {
		if !ar.HasScrollbar {
			continue
		}
		// Block region spans [LeftIndent, frameWidth) x [RegionTopY, ScrollbarY + scrollbarHeight).
		// The scrollbar is at the bottom; include it in the region.
		// The gutter to the left of LeftIndent is excluded so vertical swipes pass through.
		scrollbarHeight := f.hscrollHeight
		if relX >= ar.LeftIndent && relX < frameWidth &&
			relY >= ar.RegionTopY && relY < ar.ScrollbarY+scrollbarHeight {
			return i, true
		}
	}
	return 0, false
}

// HScrollWheel adjusts the horizontal scroll offset for the given block region
// by delta pixels. Positive delta scrolls right, negative scrolls left.
// The resulting offset is clamped to [0, maxScrollable].
func (f *frameImpl) HScrollWheel(delta int, regionIndex int) {
	if regionIndex < 0 || regionIndex >= len(f.hscrollRegions) {
		return
	}
	ar := f.hscrollRegions[regionIndex]
	if !ar.HasScrollbar {
		return
	}

	frameWidth := f.rect.Dx()
	maxScrollable := ar.MaxContentWidth - frameWidth
	if maxScrollable <= 0 {
		return
	}

	newOffset := f.GetHScrollOrigin(regionIndex) + delta

	// Clamp to [0, maxScrollable].
	if newOffset < 0 {
		newOffset = 0
	}
	if newOffset > maxScrollable {
		newOffset = maxScrollable
	}

	f.SetHScrollOrigin(regionIndex, newOffset)
}

// HScrollBgColor is the background color of horizontal scrollbars.
var HScrollBgColor = color.RGBA{R: 153, G: 153, B: 76, A: 255} // dark yellow-green, similar to acme scrollbar

// HScrollThumbColor is the thumb color of horizontal scrollbars.
var HScrollThumbColor = color.RGBA{R: 255, G: 255, B: 170, A: 255} // pale yellow (Paleyellow), matching acme scrollbar thumb

// drawHScrollbarsTo draws horizontal scrollbars for overflowing block regions.
// For each block region where MaxContentWidth > frameWidth, it draws a scrollbar
// background and thumb at the bottom of the block region. The scrollbar height
// is scrollbarHeight (Scrollwid = 12) pixels. Thumb width is proportional to
// the visible fraction of content, with a minimum of 10 pixels.
// Thumb position is proportional to hscrollOrigin for that region.
func (f *frameImpl) drawHScrollbarsTo(target edwooddraw.Image, offset image.Point, lines []Line, adjustedRegions []AdjustedBlockRegion, frameWidth int) {
	scrollbarHeight := f.hscrollHeight

	for i, ar := range adjustedRegions {
		if !ar.HasScrollbar {
			continue
		}

		maxContentWidth := ar.MaxContentWidth
		if maxContentWidth <= frameWidth {
			continue
		}

		// Scrollbar starts at the block's left indent, leaving a gutter
		// on the left for vertical scroll gestures.
		scrollbarLeft := ar.LeftIndent
		scrollbarWidth := frameWidth - scrollbarLeft
		if scrollbarWidth <= 0 {
			continue
		}

		// Draw scrollbar background at ScrollbarY
		// Use configured colors if available, otherwise fall back to defaults
		bgImg := f.hscrollBg
		if bgImg == nil {
			bgImg = f.allocColorImage(HScrollBgColor)
			if bgImg == nil {
				continue
			}
		}
		bgRect := image.Rect(
			offset.X+scrollbarLeft,
			offset.Y+ar.ScrollbarY,
			offset.X+frameWidth,
			offset.Y+ar.ScrollbarY+scrollbarHeight,
		)
		target.Draw(bgRect, bgImg, bgImg, image.ZP)

		// Compute thumb dimensions within the scrollbar width
		thumbWidth := (scrollbarWidth * scrollbarWidth) / maxContentWidth
		if thumbWidth < 10 {
			thumbWidth = 10
		}
		if thumbWidth > scrollbarWidth {
			thumbWidth = scrollbarWidth
		}

		// Compute thumb position within the scrollbar
		maxScrollable := maxContentWidth - frameWidth
		hOffset := f.GetHScrollOrigin(i)
		thumbLeft := 0
		if maxScrollable > 0 && hOffset > 0 {
			thumbLeft = (hOffset * (scrollbarWidth - thumbWidth)) / maxScrollable
		}
		if thumbLeft < 0 {
			thumbLeft = 0
		}
		if thumbLeft+thumbWidth > scrollbarWidth {
			thumbLeft = scrollbarWidth - thumbWidth
		}

		// Draw thumb
		// Use configured colors if available, otherwise fall back to defaults
		thumbImg := f.hscrollThumb
		if thumbImg == nil {
			thumbImg = f.allocColorImage(HScrollThumbColor)
			if thumbImg == nil {
				continue
			}
		}
		thumbRect := image.Rect(
			offset.X+scrollbarLeft+thumbLeft,
			offset.Y+ar.ScrollbarY,
			offset.X+scrollbarLeft+thumbLeft+thumbWidth,
			offset.Y+ar.ScrollbarY+scrollbarHeight,
		)
		target.Draw(thumbRect, thumbImg, thumbImg, image.ZP)
	}
}

// Full returns true if the frame is at capacity.
// A frame is full when more content is visible than can fit in the frame.
func (f *frameImpl) Full() bool {
	return f.VisibleLines() > f.MaxLines()
}

// fontHeightForStyle returns the font height for a given style.
// This is used by the layout algorithm to calculate line heights.
func (f *frameImpl) fontHeightForStyle(style Style) int {
	return f.fontForStyle(style).Height()
}

// fontForStyle returns the appropriate font for the given style.
// Falls back to the regular font if the variant is not available.
// When a style has a Scale != 1.0, the scaled font takes precedence
// since it provides the correct metrics for heading layout.
func (f *frameImpl) fontForStyle(style Style) edwooddraw.Font {
	// Check for scaled fonts first (for headings like H1, H2, H3)
	// Scale takes precedence because heading layout requires the correct metrics
	if style.Scale != 1.0 && f.scaledFonts != nil {
		if scaledFont, ok := f.scaledFonts[style.Scale]; ok {
			return scaledFont
		}
	}

	// Check for code font (monospace for inline code and code blocks)
	if style.Code && f.codeFont != nil {
		return f.codeFont
	}

	// Check for bold/italic variants for non-scaled text
	if style.Bold && style.Italic {
		if f.boldItalicFont != nil {
			return f.boldItalicFont
		}
	} else if style.Bold {
		if f.boldFont != nil {
			return f.boldFont
		}
	} else if style.Italic {
		if f.italicFont != nil {
			return f.italicFont
		}
	}
	return f.font
}

// layoutBoxes runs the layout algorithm on the given boxes.
// If an imageCache is set on the frame, it uses layoutWithCacheAndBasePath to load
// images and populate their ImageData. Otherwise, it uses the regular layout.
func (f *frameImpl) layoutBoxes(boxes []Box, frameWidth, maxtab int) []Line {
	if f.imageCache != nil {
		return layoutWithCacheAndBasePath(boxes, f.font, frameWidth, maxtab, f.fontHeightForStyle, f.fontForStyle, f.imageCache, f.basePath, f.onImageLoaded)
	}
	return layout(boxes, f.font, frameWidth, maxtab, f.fontHeightForStyle, f.fontForStyle)
}

// ensureBaseLayout returns the base layout lines (before any
// scrollbar / slide Y adjustments). The returned slice is ALWAYS a
// fresh clone — callers may mutate Line.Y / Height / ContentWidth
// without disturbing the cache. The expensive line-breaking
// computation is still cached internally and reused when content and
// frame width are unchanged; only the surface slice is freshly
// allocated. Tests that need to verify cache reuse should read
// f.cachedBaseLines directly.
//
// This replaces an earlier "cooperative" contract where callers had
// to remember to call cloneLines themselves before any mutation.
// That contract was enforced only by code review and a future edit
// that forgot the clone would silently corrupt the cache.
func (f *frameImpl) ensureBaseLayout() []Line {
	frameWidth := f.rect.Dx()
	if !f.layoutDirty && f.cachedBaseLines != nil && f.cachedWidth == frameWidth {
		return cloneLines(f.cachedBaseLines)
	}
	boxes := contentToBoxes(f.content)
	if len(boxes) == 0 {
		f.cachedBaseLines = nil
		f.cachedWidth = frameWidth
		f.layoutDirty = false
		return nil
	}
	maxtab := f.maxtabPixels()
	f.cachedBaseLines = f.layoutBoxes(boxes, frameWidth, maxtab)
	f.cachedWidth = frameWidth
	f.layoutDirty = false
	return cloneLines(f.cachedBaseLines)
}

// cloneLines returns a shallow copy of the Line slice. The shallow
// copy is sufficient because the *consumers* of base layout
// (adjustLayoutForScrollbars, adjustLayoutForSlides) only mutate the
// Y / Height / ContentWidth fields on each Line — not the
// PositionedBox slices, not anything reachable through them.
//
// IMMUTABILITY NOTE: this function's correctness depends on
// PositionedBox and Box being treated as immutable once layout
// emits them. If a future change adds a mutator on PositionedBox.X
// (e.g. baking horizontal-scroll offset into layout time instead of
// applying it at paint time), or any Box field, this clone is no
// longer sufficient and all base-layout consumers will silently
// share state. Update both this comment and the clone strategy
// together if that boundary moves.
func cloneLines(lines []Line) []Line {
	clone := make([]Line, len(lines))
	copy(clone, lines)
	return clone
}

// drawImageTo renders an image box to the target at the appropriate position.
// The image is clipped to the frame boundaries using Intersect.
func (f *frameImpl) drawImageTo(target edwooddraw.Image, pb PositionedBox, line Line, offset image.Point, frameWidth, frameHeight int) {
	if f.display == nil {
		return
	}

	cached := pb.Box.ImageData
	if cached == nil || cached.Data == nil || cached.Original == nil {
		return
	}

	// Calculate the scaled dimensions for the image
	scaledWidth, scaledHeight := imageBoxDimensions(&pb.Box, frameWidth)
	if scaledWidth == 0 || scaledHeight == 0 {
		return
	}

	// Calculate the destination rectangle
	dstX := offset.X + pb.X
	dstY := offset.Y + line.Y

	// Create destination rectangle for the image
	dstRect := image.Rect(dstX, dstY, dstX+scaledWidth, dstY+scaledHeight)

	// Clip to frame bounds
	clipRect := image.Rect(offset.X, offset.Y, offset.X+frameWidth, offset.Y+frameHeight)
	clippedDst := dstRect.Intersect(clipRect)
	if clippedDst.Empty() {
		return
	}

	// Determine which Go image to convert to Plan 9 format.
	// If the display size differs from the original, pre-scale the image
	// using bilinear interpolation before conversion.
	var goImg image.Image
	var imgWidth, imgHeight int

	if scaledWidth == cached.Width && scaledHeight == cached.Height {
		// No scaling needed, use original
		goImg = cached.Original
		imgWidth = cached.Width
		imgHeight = cached.Height
	} else {
		// Pre-scale the image in Go-land before converting to Plan 9 format
		scaled := image.NewRGBA(image.Rect(0, 0, scaledWidth, scaledHeight))
		xdraw.BiLinear.Scale(scaled, scaled.Bounds(), cached.Original, cached.Original.Bounds(), xdraw.Src, nil)
		goImg = scaled
		imgWidth = scaledWidth
		imgHeight = scaledHeight
	}

	// Convert the (possibly scaled) image to Plan 9 pixel data
	plan9Data, err := ConvertToPlan9(goImg)
	if err != nil {
		pt := image.Point{X: dstX, Y: dstY}
		f.drawImageErrorPlaceholder(target, pt, string(pb.Box.Text))
		return
	}

	// Allocate a Plan 9 image at the (possibly scaled) dimensions
	srcRect := image.Rect(0, 0, imgWidth, imgHeight)
	srcImg, err := f.display.AllocImage(srcRect, edwooddraw.RGBA32, false, 0)
	if err != nil {
		pt := image.Point{X: dstX, Y: dstY}
		f.drawImageErrorPlaceholder(target, pt, string(pb.Box.Text))
		return
	}
	defer srcImg.Free()

	// Load the pixel data into the source image
	_, err = srcImg.Load(srcRect, plan9Data)
	if err != nil {
		pt := image.Point{X: dstX, Y: dstY}
		f.drawImageErrorPlaceholder(target, pt, string(pb.Box.Text))
		return
	}

	// Calculate the source point for clipping
	srcPt := image.ZP
	if dstRect.Min.X < clippedDst.Min.X {
		srcPt.X = clippedDst.Min.X - dstRect.Min.X
	}
	if dstRect.Min.Y < clippedDst.Min.Y {
		srcPt.Y = clippedDst.Min.Y - dstRect.Min.Y
	}

	target.Draw(clippedDst, srcImg, nil, srcPt)
}

// LoadingGray is the muted gray color for loading image placeholders.
var LoadingGray = color.RGBA{R: 153, G: 153, B: 153, A: 255}

// drawImageLoadingPlaceholder renders a loading placeholder for images being fetched asynchronously.
// It displays "[Loading: alt]" in muted gray to distinguish from error placeholders (blue) and links.
func (f *frameImpl) drawImageLoadingPlaceholder(target edwooddraw.Image, pt image.Point, boxText string) {
	if f.font == nil || f.textColor == nil {
		return
	}

	// Replace "[Image:" prefix with "[Loading:" if present
	placeholder := boxText
	if strings.HasPrefix(placeholder, "[Image:") {
		placeholder = "[Loading:" + placeholder[len("[Image:"):]
	}

	grayColor := f.allocColorImage(LoadingGray)
	if grayColor == nil {
		grayColor = f.textColor
	}

	target.Bytes(pt, grayColor, image.ZP, f.font, []byte(placeholder))
}

// drawImageErrorPlaceholder renders an error placeholder for failed image loads.
// It displays the box's text (e.g. "[Image: alt]" or "[Image: alt <unsupported format>]")
// in blue (like a link) so it can be clicked to open the image path.
func (f *frameImpl) drawImageErrorPlaceholder(target edwooddraw.Image, pt image.Point, boxText string) {
	if f.font == nil || f.textColor == nil {
		return
	}

	placeholder := boxText

	// Use blue (like links) so users know it's clickable
	blueColor := f.allocColorImage(LinkBlue)
	if blueColor == nil {
		blueColor = f.textColor // Fall back to default text color
	}

	// Render the placeholder text
	target.Bytes(pt, blueColor, image.ZP, f.font, []byte(placeholder))
}

// drawFixedBox renders a fixed-dimension box as a colored rectangle.
// Used for spans-protocol box elements without an image payload.
func (f *frameImpl) drawFixedBox(target edwooddraw.Image, pt image.Point, box Box, frameWidth, frameHeight int, offset image.Point) {
	w := box.Style.ImageWidth
	h := box.Style.ImageHeight

	boxRect := image.Rect(pt.X, pt.Y, pt.X+w, pt.Y+h)
	clipRect := image.Rect(offset.X, offset.Y, offset.X+frameWidth, offset.Y+frameHeight)
	boxRect = boxRect.Intersect(clipRect)
	if boxRect.Empty() {
		return
	}

	// Draw background color.
	bgColor := box.Style.Bg
	if bgColor == nil {
		bgColor = color.RGBA{R: 200, G: 200, B: 200, A: 255} // light gray default
	}
	bgImg := f.allocColorImage(bgColor)
	if bgImg != nil {
		target.Draw(boxRect, bgImg, nil, image.ZP)
	}
}
