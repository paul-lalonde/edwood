package frame

import (
	"image"

	"github.com/rjkroege/edwood/draw"
)

// InitTick sets up the TickImage (e.g. cursor) at the base
// font's height. tick() resizes via initTickAtHeight when the
// caret lands on a tall (scaled) line.
// TODO(rjk): doesn't appear to need to be exposed publically.
func (f *frameimpl) InitTick() {
	if f.cols[ColBack] == nil || f.display == nil {
		return
	}
	f.tickscale = f.display.ScaleSize(1)
	f.initTickAtHeight(f.font.Height())
}

// initTickAtHeight allocates / reallocates tickimage and
// tickback at the given line height. B2.2 R6: the caret bar's
// vertical extent matches the line it's on, so a heading line
// gets a tall caret and body lines a normal one.
func (f *frameimpl) initTickAtHeight(height int) {
	if f.cols[ColBack] == nil || f.display == nil || height <= 0 {
		return
	}
	b := f.display.ScreenImage()

	if f.tickimage != nil {
		f.tickimage.Free()
		f.tickimage = nil
	}
	if f.tickback != nil {
		f.tickback.Free()
		f.tickback = nil
	}

	var err error
	f.tickimage, err = f.display.AllocImage(image.Rect(0, 0, f.tickscale*frtickw, height), b.Pix(), false, draw.Transparent)
	if err != nil {
		return
	}

	f.tickback, err = f.display.AllocImage(f.tickimage.R(), b.Pix(), false, draw.White)
	if err != nil {
		f.tickimage.Free()
		f.tickimage = nil
		return
	}
	f.tickback.Draw(f.tickback.R(), f.cols[ColBack], nil, image.Point{})

	f.tickimage.Draw(f.tickimage.R(), f.display.Transparent(), nil, image.Pt(0, 0))
	// vertical line
	f.tickimage.Draw(image.Rect(f.tickscale*(frtickw/2), 0, f.tickscale*(frtickw/2+1), height), f.display.Opaque(), nil, image.Pt(0, 0))
	// box on each end
	f.tickimage.Draw(image.Rect(0, 0, f.tickscale*frtickw, f.tickscale*frtickw), f.display.Opaque(), nil, image.Pt(0, 0))
	f.tickimage.Draw(image.Rect(0, height-f.tickscale*frtickw, f.tickscale*frtickw, height), f.display.Opaque(), nil, image.Pt(0, 0))
}

// lineHAtPt returns the LineH of the box whose stored line
// rect contains pt.Y. Used by tick to size the caret to the
// line. Falls back to defaultfontheight when no box matches
// (empty frame, pt past last box).
func (f *frameimpl) lineHAtPt(pt image.Point) int {
	for _, b := range f.box {
		if b.LineH <= 0 {
			continue
		}
		if pt.Y >= b.Y && pt.Y < b.Y+b.LineH {
			return b.LineH
		}
	}
	return f.defaultfontheight
}
