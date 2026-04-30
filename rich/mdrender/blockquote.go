package mdrender

import (
	"image"
	"image/color"

	edwooddraw "github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/rich"
)

// BlockquoteBorderColor is the color of the blockquote vertical
// border bar. Moved from rich/frame.go in Phase 1.2 of the
// markdown-externalization plan.
var BlockquoteBorderColor = color.RGBA{R: 200, G: 200, B: 200, A: 255}

// BlockquoteBorderWidth is the width in pixels of the blockquote
// vertical bar.
const BlockquoteBorderWidth = 2

// paintBlockquoteBorders walks the wrapped frame's layout lines and
// draws left-edge vertical bars for blockquote-depth-styled
// content. Verbatim transplant of the logic that used to live in
// rich.Frame's paintPhaseBlockquoteBorders, with two adjustments:
//
//   - Lines come from frame.LayoutLines() (Phase 1.2 added that
//     accessor specifically for this).
//   - The target is the display's screen image, with offset
//     frame.Rect().Min — so we draw on top of the frame's already-
//     blitted output rather than into the frame's scratch image.
//
// Color allocation goes through the Renderer's per-instance color
// cache (allocColorImage below) to avoid the same per-redraw
// AllocImage leak that allocColorImage on rich.frameImpl was
// recently fixed for.
func (r *Renderer) paintBlockquoteBorders() {
	lines := r.frame.LayoutLines()
	if len(lines) == 0 {
		return
	}
	frameRect := r.frame.Rect()
	frameWidth := frameRect.Dx()
	frameHeight := frameRect.Dy()
	if frameWidth <= 0 || frameHeight <= 0 {
		return
	}
	offset := frameRect.Min
	target := r.display.ScreenImage()

	for _, line := range lines {
		if line.Y >= frameHeight {
			break
		}
		r.drawBlockquoteBorders(target, line, offset, frameWidth, frameHeight)
	}
}

// drawBlockquoteBorders draws vertical left border bars for one
// line. Each nesting level gets a 2px vertical bar at the left
// edge of its indent zone. Verbatim transplant of the same-named
// helper that used to live on rich.frameImpl.
func (r *Renderer) drawBlockquoteBorders(target edwooddraw.Image, line rich.Line, offset image.Point, frameWidth, frameHeight int) {
	depth := 0
	for _, pb := range line.Boxes {
		if pb.Box.Style.Blockquote && pb.Box.Style.BlockquoteDepth > depth {
			depth = pb.Box.Style.BlockquoteDepth
		}
	}
	if depth == 0 {
		return
	}

	borderImg := r.allocColorImage(BlockquoteBorderColor)
	if borderImg == nil {
		return
	}

	clipRect := image.Rect(offset.X, offset.Y, offset.X+frameWidth, offset.Y+frameHeight)

	for level := 1; level <= depth; level++ {
		barX := offset.X + (level-1)*rich.ListIndentWidth + 2
		barRect := image.Rect(
			barX,
			offset.Y+line.Y,
			barX+BlockquoteBorderWidth,
			offset.Y+line.Y+line.Height,
		)
		barRect = barRect.Intersect(clipRect)
		if barRect.Empty() {
			continue
		}
		target.Draw(barRect, borderImg, nil, image.ZP)
	}
}

// allocColorImage returns a 1x1 replicated image for the given
// color, caching by packed RGBA so repeated calls reuse the same
// handle. Mirrors the equivalent helper on rich.frameImpl
// (introduced for the same leak-prevention reason).
//
// Cache lifetime is the Renderer's lifetime; entries bounded by
// the number of unique colors mdrender uses (currently one:
// BlockquoteBorderColor).
func (r *Renderer) allocColorImage(c color.Color) edwooddraw.Image {
	if r.display == nil {
		return nil
	}
	rr, gg, bb, aa := c.RGBA()
	key := edwooddraw.Color(uint32(rr>>8)<<24 | uint32(gg>>8)<<16 | uint32(bb>>8)<<8 | uint32(aa>>8))
	if img, ok := r.colorCache[key]; ok {
		return img
	}
	img, err := r.display.AllocImage(image.Rect(0, 0, 1, 1), r.display.ScreenImage().Pix(), true, key)
	if err != nil {
		return nil
	}
	if r.colorCache == nil {
		r.colorCache = make(map[edwooddraw.Color]edwooddraw.Image)
	}
	r.colorCache[key] = img
	return img
}
