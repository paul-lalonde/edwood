package mdrender

import (
	"image"
	"image/color"

	edwooddraw "github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/rich"
)

// HRuleColor is the gray color used for horizontal rule lines.
// Moved from rich/frame.go in Phase 1.3 of the markdown-
// externalization plan.
var HRuleColor = color.RGBA{R: 180, G: 180, B: 180, A: 255}

// paintHorizontalRules walks the wrapped frame's layout lines and
// draws a 1px horizontal line for each line containing an
// HRule-styled box. Verbatim transplant of the logic that used to
// live in rich.Frame's paintPhaseHorizontalRules; line layout and
// box-style inspection are unchanged. Drawing target is the
// display's screen image at offset frame.Rect().Min, on top of the
// frame's already-blitted output.
func (r *Renderer) paintHorizontalRules() {
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
		for _, pb := range line.Boxes {
			if pb.Box.Style.HRule {
				r.drawHorizontalRuleTo(target, line, offset, frameWidth, frameHeight)
				break // one rule per line
			}
		}
	}
}

// drawHorizontalRuleTo draws a single 1px-tall rule for one line,
// vertically centered. Verbatim transplant of the same-named
// helper that used to live on rich.frameImpl.
func (r *Renderer) drawHorizontalRuleTo(target edwooddraw.Image, line rich.Line, offset image.Point, frameWidth, frameHeight int) {
	ruleImg := r.allocColorImage(HRuleColor)
	if ruleImg == nil {
		return
	}

	lineThickness := 1
	centerY := offset.Y + line.Y + line.Height/2

	ruleRect := image.Rect(
		offset.X,
		centerY,
		offset.X+frameWidth,
		centerY+lineThickness,
	)

	clipRect := image.Rect(offset.X, offset.Y, offset.X+frameWidth, offset.Y+frameHeight)
	ruleRect = ruleRect.Intersect(clipRect)
	if ruleRect.Empty() {
		return
	}

	target.Draw(ruleRect, ruleImg, nil, image.ZP)
}
