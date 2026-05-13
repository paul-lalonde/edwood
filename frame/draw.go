package frame

import (
	"image"

	"github.com/rjkroege/edwood/draw"
)

// drawtext paints every box in f.box starting at pt. The pt
// accumulator walk is intentional: drawtext is only ever called
// on a child frame (nframe) built by bxscan, whose boxes are
// not run through relayoutFrom. The child frame inherits f.rect
// but its boxes occupy a sub-region beginning at the caller-
// supplied pt. The legacy cklinewrap+advance accumulator
// produces the right per-box position from that pt — read
// paths on the parent frame (repaintBoxRange) use box.X / box.Y
// directly because those boxes ARE relayouted.
func (f *frameimpl) drawtext(pt image.Point, text draw.Image, back draw.Image) {
	for _, b := range f.box {
		pt = f.cklinewrap(pt, b)
		f.paintBox(b, pt, text, back, false)
		pt.X += b.Wid
	}
}

// paintBox paints one content box at pt. text and back are the
// frame-default text and background colors; per-box Style
// overrides (Fg/Bg when KindColored is set) take precedence.
//
// If clearBg is true, the box's background rect is always
// painted before the glyph (using Style.Bg if set, else back).
// If clearBg is false, the rect is painted only when the box
// carries an explicit Style.Bg override — drawtext takes this
// path because fillNonGlyphAreas already painted the run-wide
// default background.
//
// paintBox is the *single* place in the frame package that
// resolves a content box's font (via fontFor), resolves its
// effective fg/bg, paints background, paints glyphs, and applies
// per-box decorations (KindHidden suppression here, KindHRule in
// row B4.2, future KindUnderline). Adding a new decoration is a
// one-site edit and lands on every paint path (initial draw and
// re-style repaint) automatically.
//
// Special boxes (Nrune < 0) and the noredraw mode produce no
// output here; callers still walk pt past them.
func (f *frameimpl) paintBox(b *frbox, pt image.Point, text, back draw.Image, clearBg bool) {
	if f.noredraw || b.Nrune < 0 {
		return
	}
	fg := text
	bg := back
	hasBgOverride := false
	if b.Style.Kind&KindColored != 0 {
		if b.Style.Fg != nil {
			fg = b.Style.Fg
		}
		if b.Style.Bg != nil {
			bg = b.Style.Bg
			hasBgOverride = true
		}
	}
	if clearBg || hasBgOverride {
		rect := image.Rect(pt.X, pt.Y, pt.X+b.Wid, pt.Y+f.defaultfontheight)
		f.background.Draw(rect, bg, nil, image.Point{})
	}
	if b.Style.Kind&KindHidden == 0 {
		f.background.Bytes(pt, fg, image.Point{}, f.fontFor(b.Style), b.Ptr)
	}
	// KindHRule decoration: draw a 1-pixel horizontal line across
	// the box's rect at the row's vertical center, in the box's
	// effective foreground color. The glyphs are still painted
	// above so the marker characters remain visible (the
	// "markers stay visible" stance shared by every other v1
	// directive).
	if b.Style.Kind&KindHRule != 0 {
		ymid := pt.Y + f.defaultfontheight/2
		rect := image.Rect(pt.X, ymid, pt.X+b.Wid, ymid+1)
		f.background.Draw(rect, fg, nil, image.Point{})
	}
	// Debug overlay: outline the painted box's line-extent rect
	// in Medblue. Drawn last so the outline sits on top of glyphs
	// and any decoration.
	if f.showBoxOutlines && f.boxOutlineColor != nil {
		f.DrawOutlineRect(image.Rect(pt.X, pt.Y, pt.X+b.Wid, pt.Y+f.defaultfontheight), f.boxOutlineColor)
	}
}

// fontFor picks the right font variant for a styled run. Falls
// back to the base font when the requested variant hasn't been
// configured (so styling degrades gracefully on installations
// that don't have bold/italic/code font files).
//
// KindCodeFamily takes precedence over weight/italic — family is
// a stronger choice than weight, and md2spans v1 doesn't combine
// code with bold or italic. If the code variant isn't configured
// we still fall through to the weight/italic lookup before
// reaching the base font, so a producer that requests
// KindCodeFamily|KindBold still gets a bold variant when only
// fontBold is available.
func (f *frameimpl) fontFor(s Style) draw.Font {
	if s.Kind&KindCodeFamily != 0 && f.fontCode != nil {
		return f.fontCode
	}
	bold := s.Kind&KindBold != 0
	italic := s.Kind&KindItalic != 0
	switch {
	case bold && italic && f.fontBoldItalic != nil:
		return f.fontBoldItalic
	case bold && italic && f.fontBold != nil:
		return f.fontBold
	case bold && italic && f.fontItalic != nil:
		return f.fontItalic
	case bold && f.fontBold != nil:
		return f.fontBold
	case italic && f.fontItalic != nil:
		return f.fontItalic
	}
	return f.font
}

// repaintBoxRange repaints boxes [nb0, nb1) starting at pt. Each
// box's background rect is always cleared before the glyph is
// drawn, so the function is safe to call over a styled range
// that previously rendered with a different style. Used by
// SetStyleRange; the upstream Insert path uses drawtext, which
// relies on fillNonGlyphAreas having already painted the
// run-wide background.
func (f *frameimpl) repaintBoxRange(pt image.Point, nb0, nb1 int, text draw.Image, back draw.Image) {
	if nb0 < 0 {
		nb0 = 0
	}
	if nb1 > len(f.box) {
		nb1 = len(f.box)
	}
	// B2.2 R3: pt arg unused; boxes know their position via
	// relayoutFrom. See drawtext comment.
	_ = pt
	for nb := nb0; nb < nb1; nb++ {
		b := f.box[nb]
		f.paintBox(b, image.Pt(b.X, b.Y), text, back, true)
	}
}

// drawBox is a helpful debugging utility that wraps each box with a
// rectangle to show its extent.
func (f *frameimpl) drawBox(r image.Rectangle, col, back draw.Image, qt image.Point) {
	f.background.Draw(r, col, nil, qt)
	r = r.Inset(1)
	f.background.Draw(r, back, nil, qt)
}

// DrawSel draws or clears a selection highlight over glyphs in the range
// [p0, p1). highlighted is true if the selection highlight is to be set
// to on. If p0 == p1, draws or removes the text insertion mark (i.e.
// "the tick") instead.
// TODO(rjk): pt is the position of p0. Consider computing that internally.
func (f *frameimpl) DrawSel(pt image.Point, p0, p1 int, highlighted bool) {
	// log.Printf("Frame.DrawSel start pt=%v p0=%d p1=%d highlighted=%v\n", pt, p0, p1, highlighted)
	// defer log.Println("Frame.DrawSel end")
	f.lk.Lock()
	defer f.lk.Unlock()
	f.drawselimpl(pt, p0, p1, highlighted)
}

func (f *frameimpl) drawselimpl(pt image.Point, p0, p1 int, highlighted bool) {
	// log.Println("Frame DrawSel Start", p0, p1, highlighted, f.sp0, f.sp1, f.ticked)
	// defer log.Println("Frame DrawSel End",  f.sp0, f.sp1, f.ticked)
	if p0 > p1 {
		panic("Drawsel0: p0 and p1 must be ordered")
	}

	// TODO(rjk): one of ticked and highlighton seems sometimes redundant.
	if f.ticked {
		f.Tick(f.ptofcharptb(f.sp0, f.rect.Min, 0), false)
	}

	if f.sp0 != f.sp1 && f.highlighton {
		// Clear the selection so that subsequent code can
		// update correctly.
		back := f.cols[ColBack]
		text := f.cols[ColText]
		f.drawsel0(f.ptofcharptb(f.sp0, f.rect.Min, 0), f.sp0, f.sp1, back, text)

		// Avoid multiple draws.
		f.highlighton = false
	}

	// We've already done everything necessary above if not
	// highlighting so simply return.
	if !highlighted {
		// This has to be updated here so that select can
		// correctly update the selection during a drag loop.
		f.sp0 = p0
		f.sp1 = p1
		return
	}

	// If we should just show the tick, do that and return.
	if p0 == p1 {
		f.Tick(pt, highlighted)
		f.display.Flush() // To show the tick.
		f.sp0 = p0
		f.sp1 = p1
		return
	}

	// Need to use the highlight colour.
	back := f.cols[ColHigh]
	text := f.cols[ColHText]

	f.drawsel0(pt, p0, p1, back, text)
	f.sp0 = p0
	f.sp1 = p1
	f.highlighton = true
}

// TODO(rjk): This function is convoluted.
// drawsel0 is a lower-level routine, taking as arguments a background
// color back and text color text. It assumes that the tick is being
// handled (removed beforehand, replaced afterwards, as required) by its
// caller. The selection is delimited by character positions p0 and p1.
// The point pt0 is the geometrical location of p0 on the screen and must
// be a value generated by Ptofchar.
//
// TODO(rjk): Figure out if this is a true or false statement.
// Function does not mutate f.p0, f.p1 (well... actually, it does.)
func (f *frameimpl) drawsel0(pt image.Point, p0, p1 int, back draw.Image, text draw.Image) image.Point {
	// log.Println("Frame Drawsel0 Start", p0, p1,  f.P0, f.P1)
	// defer log.Println("Frame Drawsel0 End", f.P0, f.P1 )
	p := 0
	trim := false
	x := 0

	if p0 > p1 {
		panic("Drawsel0: p0 and p1 must be ordered")
	}

	nb := 0
	var w int
	for ; nb < len(f.box) && p < p1; nb++ {
		b := f.box[nb]
		nr := nrune(b)
		if p+nr <= p0 {
			// This box doesn't need to be modified.
			p += nr
			continue
		}
		if p >= p0 {
			// Fills in the end of the previous line with selection highlight when the line has
			// has been wrapped.
			qt := pt
			pt = f.cklinewrap(pt, b)
			if pt.Y > qt.Y {
				if qt.X > f.rect.Max.X {
					qt.X = f.rect.Max.X
				}
				//f.drawBox(image.Rect(qt.X, qt.Y, f.Rect.Max.X, pt.Y), text, back,qt)
				f.background.Draw(image.Rect(qt.X, qt.Y, f.rect.Max.X, pt.Y), back, nil, qt)
			}
		}
		ptr := b.Ptr
		if p < p0 {
			// beginning of region: advance into box
			ptr = ptr[runeindex(ptr, p0-p):]
			nr -= p0 - p
			p = p0
		}
		trim = false
		if p+nr > p1 {
			// end of region: trim box
			nr -= (p + nr) - p1
			trim = true
		}

		if b.Nrune < 0 || nr == b.Nrune {
			w = b.Wid
		} else {
			w = f.fontFor(b.Style).BytesWidth(ptr[0:runeindex(ptr, nr)])
		}
		x = pt.X + w
		if x > f.rect.Max.X {
			x = f.rect.Max.X
		}
		// When `back` is the frame's default ColBack the caller
		// is clearing the highlight, not painting a new one.
		// Honor each box's Style so styled text survives the
		// deselect; without this the symmetric flicker happens
		// (selecting then deselecting styled text would flash
		// to plain colors until the next redraw).
		bg, glyph := back, text
		if back == f.cols[ColBack] && b.Style.Kind&KindColored != 0 {
			if b.Style.Bg != nil {
				bg = b.Style.Bg
			}
			if b.Style.Fg != nil {
				glyph = b.Style.Fg
			}
		}
		f.background.Draw(image.Rect(pt.X, pt.Y, x, pt.Y+f.defaultfontheight), bg, nil, pt)
		// Pick the bold/italic font variant per the box's Style.
		// In highlight mode the glyph color is ColHText but the
		// font weight/angle still comes from the box's Style so
		// the highlight reflects the underlying styling.
		// KindHidden boxes skip the glyph paint in clear mode;
		// in highlight mode we still draw to keep the highlight
		// readable.
		if b.Nrune >= 0 {
			if back == f.cols[ColBack] && b.Style.Kind&KindHidden != 0 {
				// hidden + clearing → no glyph
			} else {
				f.background.Bytes(pt, glyph, image.Point{}, f.fontFor(b.Style), ptr[0:runeindex(ptr, nr)])
			}
		}
		pt.X += w
		p += nr
	}

	if p1 > p0 && nb > 0 && nb < len(f.box) && f.box[nb-1].Nrune > 0 && !trim {
		qt := pt
		pt = f.cklinewrap(pt, f.box[nb])
		if pt.Y > qt.Y {
			f.drawBox(image.Rect(qt.X, qt.Y, f.rect.Max.X, pt.Y), f.cols[ColHigh], back, qt)
			// f.Background.Draw(image.Rect(qt.X, qt.Y, f.Rect.Max.X, pt.Y), back, nil, qt)
		}
	}

	return pt
}

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
func (f *frameimpl) Redraw(enclosing image.Rectangle) {
	f.lk.Lock()
	defer f.lk.Unlock()
	// log.Printf("Redraw %v %v", f.Rect, enclosing)
	f.background.Draw(enclosing, f.cols[ColBack], nil, image.Point{})
}

func (f *frameimpl) tick(pt image.Point, ticked bool) {
	//	log.Println("_tick")
	if f.ticked == ticked || f.tickimage == nil || !pt.In(f.rect) {
		return
	}

	pt.X -= f.tickscale
	r := image.Rect(pt.X, pt.Y, pt.X+frtickw*f.tickscale, pt.Y+f.defaultfontheight)

	if r.Max.X > f.rect.Max.X {
		r.Max.X = f.rect.Max.X
	}

	if ticked {
		f.tickback.Draw(f.tickback.R(), f.background, nil, pt)
		f.background.Draw(r, f.display.Black(), f.tickimage, image.Point{}) // draws an alpha-blended box
	} else {
		// There is an issue with tick management
		f.background.Draw(r, f.tickback, nil, image.Point{})
	}
	f.ticked = ticked
}

// Tick draws (if up is non-zero) or removes (if up is zero) the tick
// at the screen position indicated by pt.
//
// Commentary: because this code ignores selections, it is conceivably
// undesirable to use it in the public API.
func (f *frameimpl) Tick(pt image.Point, ticked bool) {
	if f.tickscale != f.display.ScaleSize(1) {
		if f.ticked {
			f.tick(pt, false)
		}
		f.InitTick()
	}

	f.tick(pt, ticked)
}

func (f *frameimpl) _draw(pt image.Point) image.Point {
	// f.Logboxes("_draw -- start")
	for nb := 0; nb < len(f.box); nb++ {
		b := f.box[nb]
		if b == nil {
			f.Logboxes("-- Frame._draw has invalid box mode --")
			panic("-- Frame._draw has invalid box mode --")
		}
		pt = f.cklinewrap0(pt, b)
		if pt.Y == f.rect.Max.Y {
			f.lastlinefull = true
			f.nchars -= f.strlen(nb)
			f.delbox(nb, len(f.box)-1)
			break
		}

		if b.Nrune > 0 {
			n, fits := f.canfit(pt, b)
			if !fits {
				break
			}
			if n != b.Nrune {
				f.splitbox(nb, n)
				b = f.box[nb]
			}
			pt.X += b.Wid
		} else {
			if b.Bc == '\n' {
				pt.X = f.rect.Min.X
				pt.Y += f.defaultfontheight
			} else {
				pt.X += f.newwid(pt, b)
			}
		}
	}
	// f.Logboxes("_draw -- end")
	return pt
}

func (f *frameimpl) strlen(nb int) int {
	n := 0
	for _, b := range f.box[nb:] {
		n += nrune(b)
	}
	return n
}
