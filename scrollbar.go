package main

import (
	"fmt"
	"image"
	"time"

	"github.com/rjkroege/edwood/draw"
)

// MinThumbHeightPx is the minimum on-screen height of the scrollbar
// thumb. Below this the thumb becomes hard to grab on hi-DPI displays
// and visually disappears against the track. Chosen empirically to be
// reliably grabbable; not tied to font height because the scrollbar
// must remain usable in extremely large documents where a strictly
// proportional thumb height would be sub-pixel.
const MinThumbHeightPx = 10

// largeDocThreshold is the document size beyond which thumb math is
// downscaled by 10 bits to avoid integer overflow in the
// h*p/tot multiplication. Mirrors the guard in the legacy scrl.go.
const largeDocThreshold = 1 << 20

// ScrollModel is the mode-specific adapter that the Scrollbar widget
// calls to ask "where is the document, and where should it move
// next?"
//
// All quantities are in document-space pixels: the document is
// treated as a vertical strip of total height TotalPx, of which
// ViewPx pixels are visible starting at OriginPx from the document
// top. Modes that operate in non-pixel units (e.g. text mode in
// runes) may return any unit triple from Geometry as long as the
// three values share the same unit; the widget uses them only to
// size and position the thumb proportionally.
//
// The widget never sees runes, lines, or any mode-specific concept.
// Three operations, all expressed in viewport-relative pixels.
type ScrollModel interface {
	// Geometry returns the current scroll state.
	//   totalPx:  total document height (>= viewPx).
	//   viewPx:   viewport height.
	//   originPx: offset from document top to viewport top.
	Geometry() (totalPx, viewPx, originPx int)

	// DragTopToPixel implements B1: the line currently at the top of
	// the viewport must end up at viewport pixel clickY. Equivalent
	// to subtracting clickY pixels from origin (clamped to
	// [0, totalPx-viewPx]).
	DragTopToPixel(clickY int)

	// DragPixelToTop implements B3: the line currently at viewport
	// pixel clickY must end up at the top of the viewport.
	// Equivalent to adding clickY pixels to origin (clamped).
	DragPixelToTop(clickY int)

	// JumpToFraction implements B2: set origin so its position
	// within [0, totalPx-viewPx] is at fraction f in [0, 1].
	JumpToFraction(f float64)
}

// Scrollbar renders an acme-style vertical scrollbar and (in a future
// commit) handles click-and-hold mouse interaction. It delegates all
// document arithmetic to a ScrollModel so the same widget can serve
// both text mode (rune-based) and rich-text mode (pixel-based with
// sub-line offsets).
//
// The widget is bound to a single display and model at construction.
// Track and thumb colors are also fixed at construction; both modes
// pass global.textcolors[ColBord] and [ColBack] so visuals are
// uniform across modes.
type Scrollbar struct {
	display draw.Display
	model   ScrollModel
	track   draw.Image // border / track background color
	thumb   draw.Image // thumb fill color

	rect   image.Rectangle // current scrollbar rectangle on the screen
	lastSR image.Rectangle // last drawn thumb rectangle (dirty cache)
	tmp    draw.Image      // off-screen scratch image, lazily allocated
}

// NewScrollbar constructs a Scrollbar bound to the given display and
// model. Callers must call SetRect before Draw.
func NewScrollbar(display draw.Display, model ScrollModel, track, thumb draw.Image) *Scrollbar {
	return &Scrollbar{
		display: display,
		model:   model,
		track:   track,
		thumb:   thumb,
	}
}

// SetRect updates the scrollbar's on-screen rectangle. Subsequent
// Draw calls render into this region. Invalidates the dirty cache so
// the next Draw definitely repaints.
func (s *Scrollbar) SetRect(r image.Rectangle) {
	s.rect = r
	s.lastSR = image.Rectangle{}
}

// Draw renders the scrollbar (track, thumb, edge) into its current
// rect on the display's screen image. Cheap: skips the blit if the
// thumb rectangle is identical to the last drawn one.
func (s *Scrollbar) Draw() {
	if s.rect.Empty() {
		return
	}
	totalPx, viewPx, originPx := s.model.Geometry()
	thumbRect := computeThumbRect(s.rect, totalPx, viewPx, originPx)
	if thumbRect.Eq(s.lastSR) {
		return
	}
	s.lastSR = thumbRect
	s.ensureScratch()
	s.renderThumb(thumbRect)
}

// ensureScratch lazily allocates s.tmp on first Draw and reallocates
// it when the screen has grown beyond its current height. The legacy
// scrl.go relied on a global ScrlResize call from acme.go on the
// resize event; the widget self-heals per Draw instead so that
// callers don't have to remember.
func (s *Scrollbar) ensureScratch() {
	needed := s.display.ScreenImage().R().Max.Y
	if s.tmp != nil && s.tmp.R().Max.Y >= needed {
		return
	}
	if s.tmp != nil {
		_ = s.tmp.Free()
	}
	s.tmp = s.allocTmp()
}

// renderThumb paints track + thumb + 1-pixel right edge into the
// scratch buffer and blits the result to the screen image. The edge
// matches the legacy look (scrl.go:94-95).
func (s *Scrollbar) renderThumb(thumbRect image.Rectangle) {
	local := s.rect.Sub(s.rect.Min)
	localThumb := thumbRect.Sub(s.rect.Min)
	s.tmp.Draw(local, s.track, nil, image.Point{})
	s.tmp.Draw(localThumb, s.thumb, nil, image.Point{})
	edge := localThumb
	edge.Min.X = edge.Max.X - 1
	if edge.Min.X < edge.Max.X {
		s.tmp.Draw(edge, s.track, nil, image.Point{})
	}
	s.display.ScreenImage().Draw(s.rect, s.tmp, nil, image.Point{X: 0, Y: local.Min.Y})
}

// allocTmp allocates a scratch image wide enough for any reasonable
// scrollbar (32px) and tall enough for the full screen, so that
// resizes don't force reallocation. Mirrors ScrlResize in scrl.go.
func (s *Scrollbar) allocTmp() draw.Image {
	r := image.Rect(0, 0, 32, s.display.ScreenImage().R().Max.Y)
	img, err := s.display.AllocImage(r, s.display.ScreenImage().Pix(), false, draw.Nofill)
	if err != nil {
		panic(fmt.Sprintf("scrollbar: alloc scratch: %v", err))
	}
	return img
}

// computeThumbRect returns the thumb rectangle within the given
// track for a model with the given geometry. Pure function; no side
// effects.
//
// The track is the on-screen scrollbar rectangle; the returned rect
// is also in screen coordinates and lies fully within the track.
//
// The thumb height is clamped to MinThumbHeightPx, and never extends
// past either edge of the track. For totalPx > largeDocThreshold the
// arithmetic is downscaled by 10 bits to avoid overflow in
// h*p/tot multiplication on multi-megabyte documents.
func computeThumbRect(track image.Rectangle, totalPx, viewPx, originPx int) image.Rectangle {
	q := track
	if totalPx == 0 {
		return q
	}
	h := q.Dy()
	p0, p1, tot := originPx, originPx+viewPx, totalPx
	// Overflow guard for large documents.
	if tot > largeDocThreshold {
		tot >>= 10
		p0 >>= 10
		p1 >>= 10
	}
	if p0 > 0 {
		q.Min.Y = track.Min.Y + h*p0/tot
	}
	if p1 < tot {
		q.Max.Y = track.Max.Y - h*(tot-p1)/tot
	}
	return clampThumbHeight(q, track)
}

// clampThumbHeight enforces MinThumbHeightPx on the thumb rectangle.
// If shorter, it extends down; if there isn't room within the track,
// it pins to the bottom edge. Matches the legacy logic
// (scrl.go:48-53) but with the new minimum height.
func clampThumbHeight(q, track image.Rectangle) image.Rectangle {
	if q.Max.Y-q.Min.Y >= MinThumbHeightPx {
		return q
	}
	if q.Max.Y+MinThumbHeightPx <= track.Max.Y {
		q.Max.Y = q.Min.Y + MinThumbHeightPx
	} else {
		q.Min.Y = q.Max.Y - MinThumbHeightPx
	}
	return q
}

// Debounce timings for HandleClick's auto-repeat. Values match the
// legacy scrl.go (200ms initial, 80ms repeat). Defined as vars so
// future tests can override; production code never mutates them.
var (
	initialDebounceMs = 200
	repeatDebounceMs  = 80
)

// dispatch fires the appropriate ScrollModel method for the given
// button. clickY is in track-relative pixels (0 = top of track);
// trackHeight is the full track height. Pure with respect to mouse
// state and display side effects, so unit-testable.
func (s *Scrollbar) dispatch(button, clickY, trackHeight int) {
	switch button {
	case 1:
		s.model.DragTopToPixel(clickY)
	case 2:
		if trackHeight > 0 {
			s.model.JumpToFraction(float64(clickY) / float64(trackHeight))
		}
	case 3:
		s.model.DragPixelToTop(clickY)
	}
}

// HandleClick runs the click-and-hold latch loop for a single
// scrollbar button-press. Reads global mouse state until the button
// is released, re-firing dispatch on a 200ms-then-80ms debounce for
// B1/B3 (auto-repeat) and per-mousectl-event for B2 (live thumb
// drag). Mirrors the latch in the legacy scrl.go:101-166 with the
// only change being pixel-space dispatch (via the model) rather
// than rune-space.
//
// Timing-and-event-driven; verified manually rather than by unit
// test. See PLAN_unified-scrollbar.md §1.3 / §2.3 for the manual
// verification plan.
func (s *Scrollbar) HandleClick(button int) {
	rect := s.rect.Inset(1)
	if rect.Empty() {
		return
	}
	centerX := (rect.Min.X + rect.Max.X) / 2
	h := rect.Dy()
	first := true
	for {
		s.display.Flush()
		my := clampMouseY(rect)
		s.warpToCenter(centerX, my)
		s.dispatch(button, my-rect.Min.Y, h)
		s.Draw()
		if !s.waitForNextTick(button, &first) {
			break
		}
	}
	drainMouseEvents()
}

// clampMouseY returns the current mouse Y clamped to the rect's
// vertical extent. Matches scrl.go's clamping (Max inclusive).
func clampMouseY(rect image.Rectangle) int {
	my := global.mouse.Point.Y
	if my < rect.Min.Y {
		my = rect.Min.Y
	}
	if my >= rect.Max.Y {
		my = rect.Max.Y
	}
	return my
}

// warpToCenter pins the cursor to the scrollbar's centerline column
// at the given Y, absorbing the synthetic mouse event MoveTo
// generates. Matches scrl.go:122-125.
func (s *Scrollbar) warpToCenter(centerX, my int) {
	if global.mouse.Point.Eq(image.Pt(centerX, my)) {
		return
	}
	s.display.MoveTo(image.Pt(centerX, my))
	global.mousectl.Read()
}

// waitForNextTick implements the per-iteration delay and exit check.
// Returns false when the button has been released and the loop
// should exit. B2 reads per-event for live thumb drag; B1/B3 use
// 200ms initial, then 80ms repeating debounce.
func (s *Scrollbar) waitForNextTick(button int, first *bool) bool {
	if button == 2 {
		global.mousectl.Read()
	} else if *first {
		s.display.Flush()
		time.Sleep(time.Duration(initialDebounceMs) * time.Millisecond)
		global.mousectl.Mouse = <-global.mousectl.C
		*first = false
	} else {
		scrollbarSleep(repeatDebounceMs)
	}
	return global.mouse.Buttons&(1<<uint(button-1)) != 0
}

// drainMouseEvents reads pending events until all buttons are
// released. Matches scrl.go:163-165.
func drainMouseEvents() {
	for global.mouse.Buttons != 0 {
		global.mousectl.Read()
	}
}

// scrollbarSleep waits for dt milliseconds, returning early if a
// mouse event arrives. Mirrors ScrSleep in scrl.go and
// previewScrSleep in wind.go.
func scrollbarSleep(dt int) {
	if dt == 0 {
		return
	}
	timer := time.NewTimer(time.Duration(dt) * time.Millisecond)
	select {
	case <-timer.C:
	case <-global.mousectl.C:
		timer.Stop()
	}
}
