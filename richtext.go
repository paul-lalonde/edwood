package main

import (
	"image"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/rich"
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
	content rich.Content

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

	// Initialize frame with empty rectangle - will be set on first Render() call
	rt.frame.Init(image.Rectangle{}, frameOpts...)
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
	// Draw scrollbar first (behind frame)
	rt.scrDraw()

	// Draw the frame content
	if rt.frame != nil {
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

	// Draw scrollbar
	rt.scrDraw()

	// Fill the gap with the frame background color
	if rt.display != nil && rt.background != nil {
		screen := rt.display.ScreenImage()
		screen.Draw(gapRect, rt.background, rt.background, image.ZP)
	}

	// Draw frame content
	if rt.frame != nil {
		rt.frame.Redraw()
	}
}

// scrDraw renders the scrollbar background and thumb using cached rectangles.
func (rt *RichText) scrDraw() {
	rt.scrDrawAt(rt.lastScrollRect)
}

// scrDrawAt renders the scrollbar at the given rectangle.
func (rt *RichText) scrDrawAt(scrollRect image.Rectangle) {
	if rt.display == nil {
		return
	}

	screen := rt.display.ScreenImage()

	// Draw scrollbar background
	if rt.scrollBg != nil {
		screen.Draw(scrollRect, rt.scrollBg, rt.scrollBg, image.ZP)
	}

	// Draw scrollbar thumb
	if rt.scrollThumb != nil {
		thumbRect := rt.scrThumbRectAt(scrollRect)
		screen.Draw(thumbRect, rt.scrollThumb, rt.scrollThumb, image.ZP)
	}
}

// ScrollClick handles a click on the scrollbar using cached rectangles.
// It takes the button number (1, 2, or 3) and the click point,
// calculates the new origin based on the button behavior, and returns it.
// Button 1 (left): scroll up (backward in content)
// Button 2 (middle): jump to absolute position
// Button 3 (right): scroll down (forward in content)
// The origin is also updated in the RichText component.
func (rt *RichText) ScrollClick(button int, pt image.Point) int {
	return rt.scrollClickAt(button, pt, rt.lastScrollRect)
}

// scrollClickAt handles a click on the scrollbar using a given scroll rectangle.
// Uses pixel heights so that lines containing images scroll correctly.
func (rt *RichText) scrollClickAt(button int, pt image.Point, scrollRect image.Rectangle) int {
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
	lineCount := len(lineHeights)
	if lineCount == 0 {
		return 0
	}

	// Compute total pixel height and frame height
	totalPixelHeight := 0
	for _, h := range lineHeights {
		totalPixelHeight += h
	}
	frameHeight := rt.frame.Rect().Dy()

	// If all content fits, no scrolling needed
	if totalPixelHeight <= frameHeight {
		return 0
	}

	// Calculate click position as a proportion of the scrollbar height
	scrollHeight := scrollRect.Dy()
	if scrollHeight <= 0 {
		return rt.Origin()
	}

	clickY := pt.Y - scrollRect.Min.Y
	if clickY < 0 {
		clickY = 0
	}
	if clickY > scrollHeight {
		clickY = scrollHeight
	}
	clickProportion := float64(clickY) / float64(scrollHeight)

	// Find current origin line and compute current pixel Y
	currentOrigin := rt.Origin()
	currentLine := findLineForOrigin(currentOrigin, lineStarts)
	currentPixelY := lineOffsetToPixel(currentLine, rt.GetOriginYOffset(), lineHeights)

	maxPixelY := totalPixelHeight - frameHeight
	if maxPixelY < 0 {
		maxPixelY = 0
	}

	fontH := rt.frame.DefaultFontHeight()
	var newPixelY int

	switch button {
	case 1:
		// Button 1 (left): scroll up by a screenful scaled by click position
		pixelsToMove := int(float64(frameHeight) * (1.0 - clickProportion))
		if pixelsToMove < 1 {
			pixelsToMove = 1
		}
		newPixelY = currentPixelY - pixelsToMove
		// Clamp to [0, maxPixelY]
		if newPixelY < 0 {
			newPixelY = 0
		}

	case 2:
		// Button 2 (middle): jump to absolute position in the document.
		// Acme convention: clicking at X% means "show from X% of the content."
		// B2 can scroll past the bounded range to show any line (matching
		// acme behavior where the last line can appear at the top of the
		// viewport with empty space below).
		newPixelY = int(float64(totalPixelHeight) * clickProportion)
		if newPixelY < 0 {
			newPixelY = 0
		}

	case 3:
		// Button 3 (right): scroll down by a screenful scaled by click position.
		pixelsToMove := int(float64(frameHeight) * clickProportion)
		if pixelsToMove < 1 {
			pixelsToMove = 1
		}
		newPixelY = currentPixelY + pixelsToMove
		if newPixelY > maxPixelY {
			newPixelY = maxPixelY
		}

	default:
		return rt.Origin()
	}

	newLine, newOffset := pixelToLineOffset(newPixelY, lineHeights)
	newOffset = snapOffset(newLine, newOffset, fontH, lineHeights)

	newOrigin := lineStarts[newLine]
	rt.frame.SetOrigin(newOrigin)
	rt.frame.SetOriginYOffset(newOffset)
	return newOrigin
}

// scrThumbRect returns the rectangle for the scrollbar thumb using cached rectangles.
func (rt *RichText) scrThumbRect() image.Rectangle {
	return rt.scrThumbRectAt(rt.lastScrollRect)
}

// scrThumbRectAt computes thumb position for a given scrollbar rectangle.
// The thumb position and size reflect the current scroll position and
// the proportion of visible content to total content, using pixel heights
// so that lines containing images are properly accounted for.
func (rt *RichText) scrThumbRectAt(scrollRect image.Rectangle) image.Rectangle {
	// If no content or frame, fill the whole scrollbar
	if rt.content == nil || rt.frame == nil {
		return scrollRect
	}

	totalRunes := rt.content.Len()
	if totalRunes == 0 {
		return scrollRect
	}

	// Get per-line pixel heights and line start runes
	lineHeights := rt.frame.LinePixelHeights()
	lineStarts := rt.frame.LineStartRunes()
	if len(lineHeights) == 0 {
		return scrollRect
	}

	// Compute total pixel height of all content
	totalPixelHeight := 0
	for _, h := range lineHeights {
		totalPixelHeight += h
	}

	frameHeight := rt.frame.Rect().Dy()
	scrollHeight := scrollRect.Dy()

	// If all content fits in the frame, fill the scrollbar
	if totalPixelHeight <= frameHeight {
		return scrollRect
	}

	// Thumb height: proportion of frame to total content, in pixels
	visibleProportion := float64(frameHeight) / float64(totalPixelHeight)
	if visibleProportion > 1.0 {
		visibleProportion = 1.0
	}

	thumbHeight := int(float64(scrollHeight) * visibleProportion)
	if thumbHeight < 10 {
		thumbHeight = 10
	}

	// Find which line the origin corresponds to
	origin := rt.frame.GetOrigin()
	originLine := 0
	for i, start := range lineStarts {
		if origin >= start {
			originLine = i
		} else {
			break
		}
	}

	// Compute pixel offset of the origin line, including sub-line offset
	originPixelY := 0
	for i := 0; i < originLine && i < len(lineHeights); i++ {
		originPixelY += lineHeights[i]
	}
	originPixelY += rt.GetOriginYOffset()

	// Position proportion based on pixel offset
	scrollablePixels := totalPixelHeight - frameHeight
	if scrollablePixels < 1 {
		scrollablePixels = 1
	}
	posProportion := float64(originPixelY) / float64(scrollablePixels)
	if posProportion > 1.0 {
		posProportion = 1.0
	}

	// Available space for thumb movement
	availableSpace := scrollHeight - thumbHeight

	// Thumb top position
	thumbTop := scrollRect.Min.Y + int(float64(availableSpace)*posProportion)

	return image.Rect(
		scrollRect.Min.X,
		thumbTop,
		scrollRect.Max.X,
		thumbTop+thumbHeight,
	)
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

	// Clamp to valid range
	totalPixelHeight := 0
	for _, h := range lineHeights {
		totalPixelHeight += h
	}
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

	// Compute total pixel height
	totalPixelHeight := 0
	for _, h := range lineHeights {
		totalPixelHeight += h
	}
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
