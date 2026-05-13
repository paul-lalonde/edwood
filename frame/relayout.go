package frame

import "image"

// relayoutFrom walks f.box[nb0:] in a single forward pass,
// populating each box's X, Y, LineH, and LineA fields. Per the
// per-box-Y architecture (frame-rendering-spec §5.2), every
// layout walk reads these fields rather than re-deriving pt
// from scratch — but R2 only writes them. R3 migrates the walk
// callers.
//
// nb0 must name the START of a line (newline boundary or a
// soft-wrap boundary). relayoutFrom does NOT walk back to find
// a line start; callers that mutate mid-line must pass an
// already-line-aligned nb0. Insert / Delete / SetStyleRange
// pass 0 (full relayout) — cheap because the box list is
// small. A future optimization can pass a tighter nb0 once R3
// is in.
//
// Pre-R4 every line has constant height (defaultfontheight),
// so the "two-phase per line" pass collapses: phase A's
// computed lineH/lineA are both defaultfontheight on every
// line. The structure stays because R4 will set Style.Scale
// → scaled font → larger boxHeight → real per-line max.
func (f *frameimpl) relayoutFrom(nb0 int) {
	if nb0 < 0 {
		nb0 = 0
	}
	if nb0 >= len(f.box) {
		return
	}

	// Seed pt at the start of box[nb0]. nb0==0 → rect.Min;
	// otherwise the previous box's position + its width
	// determines where box[nb0] starts.
	var pt image.Point
	if nb0 == 0 {
		pt = f.rect.Min
	} else {
		prev := f.box[nb0-1]
		pt = image.Pt(prev.X+prev.Wid, prev.Y)
		if prev.Bc == '\n' {
			pt.X = f.rect.Min.X
			pt.Y += prev.LineH
		}
	}

	nb := nb0
	for nb < len(f.box) {
		// Phase A: find the line's box range and compute its
		// max height / ascent.
		lineStart := nb
		lineStartX := pt.X
		lineStartY := pt.Y
		lineH := f.defaultfontheight
		lineA := f.defaultfontheight // Ascent stand-in (R5)
		for nb < len(f.box) {
			b := f.box[nb]
			// Wrap decision mirrors cklinewrap0: content
			// box wraps when its Wid doesn't fit at pt.X;
			// special box wraps when Minwid doesn't fit.
			var wrap bool
			if b.Nrune < 0 {
				wrap = int(b.Minwid) > f.rect.Max.X-pt.X
			} else {
				wrap = b.Wid > f.rect.Max.X-pt.X
			}
			if wrap && nb > lineStart {
				// b moves to the next line. Close the
				// current line; b stays for the next
				// iteration of the outer for.
				break
			}
			// b stays on this line.
			f.updateLineMaxes(b, &lineH, &lineA)
			pt.X += b.Wid
			nb++
			if b.Nrune < 0 && b.Bc == '\n' {
				// Hard wrap: newline is the line's last
				// box.
				break
			}
		}

		// Phase B: write X/Y/LineH/LineA over the line's
		// box range.
		x := lineStartX
		for i := lineStart; i < nb; i++ {
			b := f.box[i]
			b.X = x
			b.Y = lineStartY
			b.LineH = lineH
			b.LineA = lineA
			x += b.Wid
		}

		// Advance pt to the next line's top.
		pt = image.Pt(f.rect.Min.X, lineStartY+lineH)

		// Off-screen guard: stop once we've passed
		// rect.Max.Y. Out-of-view boxes keep stale fields;
		// the visible region (which is what walks care
		// about) is current.
		if pt.Y >= f.rect.Max.Y {
			break
		}
	}
}

// updateLineMaxes folds box b's height and ascent into the
// running line maximums. Special boxes (Nrune<0) contribute
// only the default font height. Content boxes use the height
// of the font fontFor would return for the box's Style — so
// a KindScale box contributes the scaled font's height, and a
// plain box contributes defaultfontheight.
//
// Until R5 the line's ascent equals its height (Ascent stand-
// in); R5 adds true baseline-aligned glyph paint.
func (f *frameimpl) updateLineMaxes(b *frbox, lineH, lineA *int) {
	h := f.defaultfontheight
	if b.Nrune >= 0 {
		h = f.fontFor(b.Style).Height()
	}
	a := h
	if h > *lineH {
		*lineH = h
	}
	if a > *lineA {
		*lineA = a
	}
}
