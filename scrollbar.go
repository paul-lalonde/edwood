package main

import (
	"fmt"
	"image"

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
	if s.tmp == nil {
		s.tmp = s.allocTmp()
	}
	s.renderThumb(thumbRect)
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
