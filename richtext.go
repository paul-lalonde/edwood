package main

import (
	"image"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/rich"
	"github.com/rjkroege/edwood/rich/mdrender"
)

// Note: scrollbar dimensions use Scrollwid and Scrollgap from dat.go
// with display.ScaleSize() for proper high-DPI support

// RichText is a component that combines a rich.Frame with a scrollbar.
// It manages the layout of the scrollbar area and the text frame area.
type RichText struct {
	// Cached rectangles from last Render() call, used for hit-testing.
	// The canonical rectangle is body.all - these are derived at render time.
	lastRect       image.Rectangle // Full area including scrollbar (cached)
	lastScrollRect image.Rectangle // Scrollbar area (cached)

	display draw.Display
	frame   rich.Frame
	// renderer wraps frame for the markdown-aware paint phases that
	// no longer live in rich.Frame (Phase 1 of the markdown-
	// externalization work). Constructed in Init alongside the
	// frame; non-nil for the lifetime of RichText. Drives all
	// frame redraws so blockquote / future markdown decorations
	// land on top of the frame's paint pass.
	renderer *mdrender.Renderer
	content  rich.Content

	// Options stored for frame initialization
	background     draw.Image
	textColor      draw.Image
	selectionColor draw.Image

	// Font variants for styled text
	boldFont       draw.Font
	italicFont     draw.Font
	boldItalicFont draw.Font
	codeFont       draw.Font

	// Scaled fonts for headings (key is scale factor)
	scaledFonts map[float64]draw.Font

	// Scrollbar colors
	scrollBg    draw.Image // Scrollbar background color
	scrollThumb draw.Image // Scrollbar thumb color

	// Image cache for loading images in markdown
	imageCache *rich.ImageCache

	// Base path for resolving relative image paths (e.g., the markdown file path)
	basePath string

	// Callback invoked when an async image load completes
	onImageLoaded func(path string)

	// Tab width in characters (forwarded to rich.WithMaxTab)
	maxtabChars int

	// Shift content up so last visible line is fully visible
	defaultScrollSnap rich.ScrollSnap // forwarded to frame on Init

	// scrollbar is the shared Scrollbar widget that owns drawing,
	// dirty caching, and the click-and-hold latch loop. Constructed
	// in Init once the bg/thumb colors have been resolved from
	// options. Driven by a richScrollModel adapter (see
	// richtext_scroll.go).
	scrollbar *Scrollbar
}

// NewRichText creates a new RichText component.
func NewRichText() *RichText {
	return &RichText{}
}

// Init initializes the RichText component with the given display, font, and options.
// The rectangle is not provided at init time - use Render(rect) to draw into a specific area.
// This allows the rectangle to be provided dynamically (e.g., from body.all).
func (rt *RichText) Init(display draw.Display, font draw.Font, opts ...RichTextOption) {
	rt.display = display

	// Apply options
	for _, opt := range opts {
		opt(rt)
	}

	// Create the frame (but don't init with a rectangle yet)
	rt.frame = rich.NewFrame()

	// Build frame options
	frameOpts := []rich.Option{
		rich.WithDisplay(display),
		rich.WithFont(font),
		// Pass main's Scrollwid through so rich's per-block-region
		// scrollbar geometry agrees with main's vertical scrollbar
		// width. Without this, rich falls back to its own
		// DefaultHScrollHeight constant, which is set to 12 to
		// match Scrollwid but is not compile-time linked to it.
		rich.WithHScrollHeight(Scrollwid),
	}
	if rt.background != nil {
		frameOpts = append(frameOpts, rich.WithBackground(rt.background))
	}
	if rt.textColor != nil {
		frameOpts = append(frameOpts, rich.WithTextColor(rt.textColor))
	}
	if rt.boldFont != nil {
		frameOpts = append(frameOpts, rich.WithBoldFont(rt.boldFont))
	}
	if rt.italicFont != nil {
		frameOpts = append(frameOpts, rich.WithItalicFont(rt.italicFont))
	}
	if rt.boldItalicFont != nil {
		frameOpts = append(frameOpts, rich.WithBoldItalicFont(rt.boldItalicFont))
	}
	if rt.codeFont != nil {
		frameOpts = append(frameOpts, rich.WithCodeFont(rt.codeFont))
	}
	for scale, f := range rt.scaledFonts {
		frameOpts = append(frameOpts, rich.WithScaledFont(scale, f))
	}
	if rt.imageCache != nil {
		frameOpts = append(frameOpts, rich.WithImageCache(rt.imageCache))
	}
	if rt.basePath != "" {
		frameOpts = append(frameOpts, rich.WithBasePath(rt.basePath))
	}
	if rt.onImageLoaded != nil {
		frameOpts = append(frameOpts, rich.WithOnImageLoaded(rt.onImageLoaded))
	}
	if rt.selectionColor != nil {
		frameOpts = append(frameOpts, rich.WithSelectionColor(rt.selectionColor))
	}
	if rt.scrollBg != nil && rt.scrollThumb != nil {
		frameOpts = append(frameOpts, rich.WithHScrollColors(rt.scrollBg, rt.scrollThumb))
	}
	if rt.maxtabChars > 0 {
		frameOpts = append(frameOpts, rich.WithMaxTab(rt.maxtabChars))
	}
	if rt.defaultScrollSnap != rich.SnapTop {
		frameOpts = append(frameOpts, rich.WithDefaultScrollSnap(rt.defaultScrollSnap))
	}

	// Initialize frame with empty rectangle - will be set on first Render() call
	rt.frame.Init(image.Rectangle{}, frameOpts...)

	// Wrap the frame in a markdown-aware Renderer. As of Phase 1.2
	// of the markdown-externalization plan, blockquote-bar painting
	// lives on the wrapper rather than in rich.Frame. Subsequent
	// rows move horizontal rules and slide-break handling here too.
	// The wrapper is unconditionally constructed: styled mode never
	// produces blockquote-styled content (per the spans-protocol
	// invariant) so the wrapper's extra paint pass is a no-op there.
	rt.renderer = mdrender.New(rt.frame, display)

	// Construct the shared Scrollbar widget driven by the rich-text
	// adapter. SetRect is called by Render() each time the scroll
	// rectangle is recomputed; the widget's own dirty-cache
	// invalidates on rect change.
	if rt.scrollBg != nil && rt.scrollThumb != nil {
		rt.scrollbar = NewScrollbar(display, &richScrollModel{rt: rt}, rt.scrollBg, rt.scrollThumb)
	}
}

// All returns the full rectangle area of the RichText component.
func (rt *RichText) All() image.Rectangle {
	return rt.lastRect
}

// Frame returns the underlying rich.Frame.
func (rt *RichText) Frame() rich.Frame {
	return rt.frame
}

// Display returns the display.
func (rt *RichText) Display() draw.Display {
	return rt.display
}

// ScrollRect returns the scrollbar rectangle.
func (rt *RichText) ScrollRect() image.Rectangle {
	return rt.lastScrollRect
}

// SetContent sets the content to display.
func (rt *RichText) SetContent(c rich.Content) {
	rt.content = c
	if rt.frame != nil {
		rt.frame.SetContent(c)
	}
}

// Content returns the current content.
func (rt *RichText) Content() rich.Content {
	return rt.content
}

// Selection returns the current selection range.
func (rt *RichText) Selection() (p0, p1 int) {
	if rt.frame == nil {
		return 0, 0
	}
	return rt.frame.GetSelection()
}

// SetSelection sets the selection range.
func (rt *RichText) SetSelection(p0, p1 int) {
	if rt.frame != nil {
		rt.frame.SetSelection(p0, p1)
	}
}

// Origin returns the current scroll origin.
func (rt *RichText) Origin() int {
	if rt.frame == nil {
		return 0
	}
	return rt.frame.GetOrigin()
}

// SetOrigin sets the scroll origin.
// This resets the pixel offset within the origin line to 0.
func (rt *RichText) SetOrigin(org int) {
	if rt.frame != nil {
		rt.frame.SetOrigin(org)
	}
}

// SetOriginYOffset sets the pixel offset within the origin line.
func (rt *RichText) SetOriginYOffset(pixels int) {
	if rt.frame != nil {
		rt.frame.SetOriginYOffset(pixels)
	}
}

// GetOriginYOffset returns the pixel offset within the origin line.
func (rt *RichText) GetOriginYOffset() int {
	if rt.frame == nil {
		return 0
	}
	return rt.frame.GetOriginYOffset()
}

// Redraw redraws the RichText component using the last rendered rectangle.
func (rt *RichText) Redraw() {
	// Draw scrollbar first (behind frame).
	if rt.scrollbar != nil {
		rt.scrollbar.Draw()
	}

	// Draw the frame content via the markdown-aware Renderer so
	// blockquote / future markdown decorations land on top of the
	// frame's paint pass. Renderer is constructed in Init and is
	// non-nil for the lifetime of RichText.
	if rt.renderer != nil {
		rt.renderer.Redraw()
	} else if rt.frame != nil {
		// Defensive fallback (should not occur in production —
		// renderer is always constructed alongside frame).
		rt.frame.Redraw()
	}
}

// Render draws the rich text component into the given rectangle.
// This computes scrollbar and frame areas from r at render time,
// allowing the rectangle to be provided dynamically (e.g., from body.all).
func (rt *RichText) Render(r image.Rectangle) {
	rt.lastRect = r
	if r.Dx() <= 0 || r.Dy() <= 0 {
		return
	}

	// Compute scrollbar rectangle (left side)
	scrollWid := rt.display.ScaleSize(Scrollwid)
	scrollGap := rt.display.ScaleSize(Scrollgap)

	rt.lastScrollRect = image.Rect(
		r.Min.X,
		r.Min.Y,
		r.Min.X+scrollWid,
		r.Max.Y,
	)

	// Compute gap rectangle (between scrollbar and frame)
	gapRect := image.Rect(
		r.Min.X+scrollWid,
		r.Min.Y,
		r.Min.X+scrollWid+scrollGap,
		r.Max.Y,
	)

	// Compute frame rectangle (right of scrollbar with gap)
	frameRect := image.Rect(
		r.Min.X+scrollWid+scrollGap,
		r.Min.Y,
		r.Max.X,
		r.Max.Y,
	)

	// Update frame geometry if changed
	if rt.frame != nil && rt.frame.Rect() != frameRect {
		rt.frame.SetRect(frameRect)
	}

	// Draw scrollbar via the shared widget. The widget's SetRect is
	// the canonical way to invalidate its dirty cache, defending
	// against the rare path where Render is called after a frame
	// redraw that clobbered the scrollbar pixels (the same contract
	// that text mode relies on — see scrollbar.go SetRect godoc).
	if rt.scrollbar != nil {
		rt.scrollbar.SetRect(rt.lastScrollRect)
		rt.scrollbar.Draw()
	}

	// Fill the gap with the frame background color
	if rt.display != nil && rt.background != nil {
		screen := rt.display.ScreenImage()
		screen.Draw(gapRect, rt.background, rt.background, image.ZP)
	}

	// Draw frame content via the markdown-aware Renderer (see
	// Redraw godoc for context).
	if rt.renderer != nil {
		rt.renderer.Redraw()
	} else if rt.frame != nil {
		rt.frame.Redraw()
	}
}


// ScrollClick is the legacy public scrollbar-click API. It synthesizes
// a click in scrollbar-relative coordinates and delegates to the
// shared richScrollModel adapter (which is what the unified Scrollbar
// widget calls). Kept around because external callers (and tests)
// still use it; new code should drive the model directly through
// rt.scrollbar.HandleClick.
func (rt *RichText) ScrollClick(button int, pt image.Point) int {
	scrollRect := rt.lastScrollRect
	if scrollRect.Empty() {
		return rt.Origin()
	}
	m := &richScrollModel{rt: rt}
	return m.scrollByClickY(button, pt.Y-scrollRect.Min.Y)
}

// scrThumbRect returns the current thumb rectangle by recomputing it
// from the model's geometry. Test-only accessor: the widget's own
// dirty-cache (lastDrawnThumb) reflects the last *painted* state and
// can lag behind a SetOrigin that wasn't followed by Redraw, so
// tests prefer this fresh recomputation.
func (rt *RichText) scrThumbRect() image.Rectangle {
	if rt.scrollbar == nil {
		return image.Rectangle{}
	}
	totalPx, viewPx, originPx := rt.scrollbar.model.Geometry()
	return computeThumbRect(rt.lastScrollRect, totalPx, viewPx, originPx)
}

// lineAtDocY returns (lineIndex, offsetWithinLine) for the line whose
// rendered Y range contains docY (round-down: largest lineYs[i] <=
// docY). If docY falls in a gap *between* lines, the next line down
// is selected (offset 0). Used by B3 (drag-this-line-to-top) and B2
// (jump-to-fraction) — both want the click to land at or just below
// docY in document space. Y values are in document-rendered space
// matching frame.LinePixelYs().
func lineAtDocY(docY int, lineYs, lineHeights []int) (lineIdx, offset int) {
	if len(lineYs) == 0 {
		return 0, 0
	}
	// Find the largest i with lineYs[i] <= docY.
	lineIdx = 0
	for i, y := range lineYs {
		if y > docY {
			break
		}
		lineIdx = i
	}
	// If docY is in the gap below lineIdx (past the line's content),
	// advance to the next line so the new viewport top lands on a
	// line boundary rather than in dead space.
	if lineIdx+1 < len(lineYs) {
		lineEnd := lineYs[lineIdx] + lineHeights[lineIdx]
		if docY >= lineEnd {
			lineIdx++
			return lineIdx, 0
		}
	}
	offset = docY - lineYs[lineIdx]
	if offset < 0 {
		offset = 0
	}
	return lineIdx, offset
}

// lineAtDocYRoundUp returns the smallest line index whose Y is >=
// docY (round-up). Used by B1 (drag-top-line-down-to-click-position):
// the new viewport top is the *earliest* line whose start is at or
// after the requested target, so the previous top ends up at
// viewport-Y <= clickY in the new view.
//
// The asymmetry with lineAtDocY (round-down) is what makes B3+B1
// round-trips exact regardless of clickY. B3 snaps down by some
// residual R; B1 rounds up across the SAME R, landing back on the
// original line.
func lineAtDocYRoundUp(docY int, lineYs []int) (lineIdx, offset int) {
	if len(lineYs) == 0 {
		return 0, 0
	}
	for i, y := range lineYs {
		if y >= docY {
			return i, 0
		}
	}
	// docY is past all line starts; clamp to the last line.
	return len(lineYs) - 1, 0
}


// RichTextOption is a functional option for configuring RichText.
type RichTextOption func(*RichText)

// WithRichTextBackground sets the background image for the rich text component.
func WithRichTextBackground(bg draw.Image) RichTextOption {
	return func(rt *RichText) {
		rt.background = bg
	}
}

// WithRichTextColor sets the text color image for the rich text component.
func WithRichTextColor(c draw.Image) RichTextOption {
	return func(rt *RichText) {
		rt.textColor = c
	}
}

// WithScrollbarColors sets the scrollbar background and thumb colors.
func WithScrollbarColors(bg, thumb draw.Image) RichTextOption {
	return func(rt *RichText) {
		rt.scrollBg = bg
		rt.scrollThumb = thumb
	}
}

// WithRichTextBoldFont sets the bold font variant for the RichText frame.
func WithRichTextBoldFont(f draw.Font) RichTextOption {
	return func(rt *RichText) {
		rt.boldFont = f
	}
}

// WithRichTextItalicFont sets the italic font variant for the RichText frame.
func WithRichTextItalicFont(f draw.Font) RichTextOption {
	return func(rt *RichText) {
		rt.italicFont = f
	}
}

// WithRichTextBoldItalicFont sets the bold-italic font variant for the RichText frame.
func WithRichTextBoldItalicFont(f draw.Font) RichTextOption {
	return func(rt *RichText) {
		rt.boldItalicFont = f
	}
}

// WithRichTextCodeFont sets the monospace font for code spans and code blocks.
func WithRichTextCodeFont(f draw.Font) RichTextOption {
	return func(rt *RichText) {
		rt.codeFont = f
	}
}

// WithRichTextScaledFont sets a scaled font for a specific scale factor (e.g., 2.0 for H1).
func WithRichTextScaledFont(scale float64, f draw.Font) RichTextOption {
	return func(rt *RichText) {
		if rt.scaledFonts == nil {
			rt.scaledFonts = make(map[float64]draw.Font)
		}
		rt.scaledFonts[scale] = f
	}
}

// WithRichTextImageCache sets the image cache for loading images in markdown content.
// The cache is passed through to the underlying Frame for use during layout.
func WithRichTextImageCache(cache *rich.ImageCache) RichTextOption {
	return func(rt *RichText) {
		rt.imageCache = cache
	}
}

// WithRichTextBasePath sets the base path for resolving relative image paths.
// This should be the path to the source file (e.g., markdown file) containing image references.
// When combined with WithRichTextImageCache, relative paths will be resolved relative to this path.
func WithRichTextBasePath(path string) RichTextOption {
	return func(rt *RichText) {
		rt.basePath = path
	}
}

// WithRichTextOnImageLoaded sets a callback invoked when an asynchronous image
// load completes. The callback runs on an unspecified goroutine; callers must
// marshal to the main goroutine (e.g., via the row lock) before touching UI state.
func WithRichTextOnImageLoaded(fn func(path string)) RichTextOption {
	return func(rt *RichText) {
		rt.onImageLoaded = fn
	}
}

// WithRichTextSelectionColor sets the selection highlight color.
// This color is used to highlight selected text in the rich text frame.
func WithRichTextSelectionColor(c draw.Image) RichTextOption {
	return func(rt *RichText) {
		rt.selectionColor = c
	}
}

// WithRichTextMaxTab sets the tab width in characters for the rich text frame.
// This is forwarded to rich.WithMaxTab during Init.
func WithRichTextMaxTab(chars int) RichTextOption {
	return func(rt *RichText) {
		rt.maxtabChars = chars
	}
}

// WithRichTextDefaultScrollSnap configures the initial ScrollSnap
// preference for the underlying rich frame. Default is SnapTop;
// pass SnapBottom only if a freshly-displayed document should land
// bottom-anchored. Most callers should leave this at the default —
// scroll handlers switch to SnapBottom on B1 click, and
// SetOrigin/SetOriginYOffset reset to SnapTop, so the freshly-
// constructed default rarely matters in steady state.
func WithRichTextDefaultScrollSnap(s rich.ScrollSnap) RichTextOption {
	return func(rt *RichText) {
		rt.defaultScrollSnap = s
	}
}

// scrollWheelLines is the number of lines to scroll per mouse wheel event.
const scrollWheelLines = 3

// pixelToLineOffset converts an absolute pixel Y position (from document top)
// to a (lineIndex, pixelWithinLine) pair. Used for sub-line pixel scrolling.
func pixelToLineOffset(pixelY int, lineHeights []int) (lineIdx, offset int) {
	for i, h := range lineHeights {
		if pixelY < h {
			return i, pixelY
		}
		pixelY -= h
	}
	if len(lineHeights) > 0 {
		return len(lineHeights) - 1, 0
	}
	return 0, 0
}

// lineOffsetToPixel converts a (lineIndex, pixelWithinLine) pair to an absolute
// pixel Y position from the document top. Used for sub-line pixel scrolling.
func lineOffsetToPixel(lineIdx, offset int, lineHeights []int) int {
	total := 0
	for i := 0; i < lineIdx && i < len(lineHeights); i++ {
		total += lineHeights[i]
	}
	return total + offset
}

// findLineForOrigin returns the line index corresponding to the given rune origin.
func findLineForOrigin(origin int, lineStarts []int) int {
	line := 0
	for i, start := range lineStarts {
		if origin >= start {
			line = i
		} else {
			break
		}
	}
	return line
}

// snapOffset snaps the pixel offset to 0 for short lines (height <= 2*fontH),
// preserving line-granular scrolling for normal text while allowing sub-line
// scrolling only on tall elements (images, large code blocks).
func snapOffset(lineIdx, offset, fontH int, lineHeights []int) int {
	if lineIdx < len(lineHeights) && lineHeights[lineIdx] <= 2*fontH {
		return 0
	}
	return offset
}

// landWithinTallLine maps docY to a (line, sub-line offset) when docY
// falls inside a "tall" line — one taller than 2*fontH, the same
// threshold snapOffset uses to allow sub-line scrolling. Returns
// ok=false if docY is not inside any tall line, in which case the
// caller falls back to the normal line-granular mapping
// (lineAtDocYRoundUp for B1 / lineAtDocY for B3 and B2).
//
// This exists to fix B1 specifically: round-up snaps past the image
// to the line below, so without this override B1 cannot move the
// viewport top onto a tall image. For B3 and B2 the result is
// identical to lineAtDocY, since round-down already lands inside the
// tall line — so applying this universally is safe.
func landWithinTallLine(docY int, lineYs, lineHeights []int, fontH int) (lineIdx, offset int, ok bool) {
	for i, y := range lineYs {
		if i >= len(lineHeights) {
			break
		}
		h := lineHeights[i]
		if h <= 2*fontH {
			continue
		}
		if docY >= y && docY < y+h {
			return i, docY - y, true
		}
	}
	return 0, 0, false
}

// ScrollToPixelY scrolls to an absolute pixel Y position in the document.
// Converts the pixel position to a (line, offset) pair and sets both the
// origin rune and pixel offset. Returns the new rune origin.
func (rt *RichText) ScrollToPixelY(pixelY int) int {
	if rt.content == nil || rt.frame == nil {
		return 0
	}

	lineHeights := rt.frame.LinePixelHeights()
	lineStarts := rt.frame.LineStartRunes()
	if len(lineHeights) == 0 {
		return 0
	}

	// Clamp to valid range (total includes all inter-line gaps)
	totalPixelHeight := rt.frame.TotalDocumentHeight()
	frameHeight := rt.frame.Rect().Dy()

	maxPixelY := totalPixelHeight - frameHeight
	if maxPixelY < 0 {
		maxPixelY = 0
	}
	if pixelY < 0 {
		pixelY = 0
	}
	if pixelY > maxPixelY {
		pixelY = maxPixelY
	}

	newLine, newOffset := pixelToLineOffset(pixelY, lineHeights)

	// Snap to line boundary for short lines
	fontH := rt.frame.DefaultFontHeight()
	newOffset = snapOffset(newLine, newOffset, fontH, lineHeights)

	rt.frame.SetOrigin(lineStarts[newLine])
	// SetOrigin resets originYOffset, so set it after:
	rt.frame.SetOriginYOffset(newOffset)
	return lineStarts[newLine]
}

// ScrollWheel handles mouse scroll wheel events.
// If up is true, scroll up (show earlier content), otherwise scroll down.
// Returns the new origin after scrolling.
// Uses pixel-based scrolling so that tall elements (images, code blocks)
// can be scrolled through smoothly.
func (rt *RichText) ScrollWheel(up bool) int {
	// If no content or frame, return 0
	if rt.content == nil || rt.frame == nil {
		return 0
	}

	totalRunes := rt.content.Len()
	if totalRunes == 0 {
		return 0
	}

	// Get per-line pixel heights and line start runes
	lineHeights := rt.frame.LinePixelHeights()
	lineStarts := rt.frame.LineStartRunes()
	if len(lineHeights) == 0 {
		return 0
	}

	// Total document height includes all inter-line gaps (paragraphs, headings, scrollbars)
	totalPixelHeight := rt.frame.TotalDocumentHeight()
	frameHeight := rt.frame.Rect().Dy()

	// If all content fits, no scrolling needed
	if totalPixelHeight <= frameHeight {
		return 0
	}

	// Compute current absolute pixel position
	currentOrigin := rt.Origin()
	currentLine := findLineForOrigin(currentOrigin, lineStarts)
	currentPixelY := lineOffsetToPixel(currentLine, rt.GetOriginYOffset(), lineHeights)

	// Fixed pixel step per scroll event
	scrollStep := scrollWheelLines * rt.frame.DefaultFontHeight()

	var newPixelY int
	if up {
		newPixelY = currentPixelY - scrollStep
		if newPixelY < 0 {
			newPixelY = 0
		}
	} else {
		maxPixelY := totalPixelHeight - frameHeight
		if maxPixelY < 0 {
			maxPixelY = 0
		}
		newPixelY = currentPixelY + scrollStep
		if newPixelY > maxPixelY {
			newPixelY = maxPixelY
		}
		// Defensive: scroll-wheel-down must never scroll backward.
		// scrollClickAt may set an origin past maxPixelY (e.g. B3
		// click far down the scrollbar with the new no-upper-clamp
		// behavior); when the user then scrolls the wheel down, a
		// naive maxPixelY clamp would reduce newPixelY below
		// currentPixelY. Keep currentPixelY in that case — at end
		// of content there's nothing to advance to.
		if newPixelY < currentPixelY {
			newPixelY = currentPixelY
		}
	}

	newLine, newOffset := pixelToLineOffset(newPixelY, lineHeights)

	// Snap to line boundary on short lines (preserves line-based feel for text)
	fontH := rt.frame.DefaultFontHeight()
	newOffset = snapOffset(newLine, newOffset, fontH, lineHeights)

	rt.frame.SetOrigin(lineStarts[newLine])
	// SetOrigin resets originYOffset, so set it after:
	rt.frame.SetOriginYOffset(newOffset)
	return lineStarts[newLine]
}
