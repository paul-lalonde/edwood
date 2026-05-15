package frame

import "image"

// lineSummary describes one visible line of text. It is the
// canonical answer to "which line is rune p on?", "how tall is
// line k?", and "what's the bottom Y of content?". Per
// frame-layout-design §2.2, the table is fully derived from
// f.box and rebuilt by relayoutFrom in the same pass that fills
// per-box fields. Per-box LineH/LineA mirror these values so
// existing per-box readers keep working without an extra hop.
//
// FirstRune is the rune-coordinate identity of the line — the
// sum of nrune(b) over f.box[:FirstBox]. It is stable across
// box-list index shifts caused by inserts/deletes in earlier
// lines, and is the key §3.5's diffLines uses to match
// pre-mutation to post-mutation lines.
type lineSummary struct {
	FirstBox  int // index into f.box of the first box on this line
	FirstRune int // sum of nrune() over f.box[:FirstBox]
	TopY      int // line's top Y (== f.box[FirstBox].Y)
	LineH     int // line's height in pixels (== f.box[FirstBox].LineH)
	LineA     int // line's max ascent (== f.box[FirstBox].LineA)
}

// relayoutFrom walks f.box[nb0:] in a single forward pass,
// populating each box's X, Y, LineH, and LineA fields and
// rebuilding the f.lines summary table for the affected
// suffix. Per the per-box-Y architecture (frame-rendering-spec
// §5.2, frame-layout-design §3), every layout walk reads these
// fields rather than re-deriving pt from scratch.
//
// nb0 must name the START of a line (newline boundary or a
// soft-wrap boundary). relayoutFrom does NOT walk back to find
// a line start; callers that mutate mid-line must pass an
// already-line-aligned nb0. Insert / Delete / SetStyleRange
// pass 0 (full relayout) — cheap because the box list is
// small.
//
// Per frame-layout-design §3.3, relayoutFrom is also the
// single site that performs:
//   - eager splitbox of content boxes whose Wid > rect.Dx()
//     (long-word fallback), and
//   - eager coalesce (inverse splitbox) of adjacent same-style
//     same-category content boxes whose combined Wid fits on
//     the current line.
//
// Both happen inline during phase A and are bounded by the
// total rune count: splits strictly shrink the trailing
// piece, merges strictly shrink len(f.box).
func (f *frameimpl) relayoutFrom(nb0 int) {
	if nb0 < 0 {
		nb0 = 0
	}
	if nb0 > len(f.box) {
		return
	}

	// Truncate f.lines at the line containing nb0. For nb0 == 0
	// this is empty; for nb0 > 0 we drop entries whose FirstBox
	// >= nb0 (assumed line-aligned).
	k := 0
	for k < len(f.lines) && f.lines[k].FirstBox < nb0 {
		k++
	}
	f.lines = f.lines[:k]

	if nb0 == len(f.box) {
		return
	}

	// firstRune accumulates the rune offset of the next line
	// start as we walk forward. Seed from f.box[:nb0].
	firstRune := 0
	for i := 0; i < nb0; i++ {
		firstRune += nrune(f.box[i])
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

	// I-LAYOUT-4: lastlinefull is derived from the line table.
	// Set at every return path including the empty-content case.
	defer func() {
		f.lastlinefull = false
		if n := len(f.lines); n > 0 {
			last := f.lines[n-1]
			if last.TopY+last.LineH >= f.rect.Max.Y {
				f.lastlinefull = true
			}
		}
	}()

	nb := nb0
	for nb < len(f.box) {
		// Phase A: find the line's box range and compute its
		// max height / ascent.
		lineStart := nb
		lineStartX := pt.X
		lineStartY := pt.Y
		lineFirstRune := firstRune
		lineH := f.defaultfontheight
		lineA := f.font.Ascent()
		for nb < len(f.box) {
			// Eager coalesce (§3.3): while box[nb] and
			// box[nb+1] are mergeable, splice them. Each
			// iteration strictly shrinks len(f.box).
			for f.coalesceAt(nb, pt.X) {
				f.mergebox(nb)
			}

			b := f.box[nb]

			// Eager split (§3.3, case 3): at a line start, if
			// the box can't fit on any line (Wid > rect.Dx()),
			// break it. canfit returns the max-fitting
			// rune-prefix; fall back to k=1 if even one rune
			// won't fit at this pt (mid-rune split is a B5.4
			// follow-up).
			if nb == lineStart && b.Nrune > 0 && b.Wid > f.rect.Dx() {
				kfit, _ := f.canfit(pt, b)
				if kfit <= 0 {
					kfit = 1
				}
				if kfit < b.Nrune {
					f.splitbox(nb, kfit)
					b = f.box[nb] // leading piece
				}
			}

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
			// B2.3 R4: tab Wid is position-dependent
			// (tabstop alignment). Recompute now that
			// pt.X is fixed for this iteration. Was
			// previously done by the _draw accumulator;
			// folding it here lets the line table
			// describe the final geometry directly.
			if b.Nrune < 0 && b.Bc == '\t' {
				b.Wid = f.newwid0(pt, b)
			}
			// b stays on this line.
			f.updateLineMaxes(b, &lineH, &lineA)
			pt.X += b.Wid
			firstRune += nrune(b)
			nb++
			if b.Nrune < 0 && b.Bc == '\n' {
				// Hard wrap: newline is the line's last
				// box.
				break
			}
		}

		// Phase B: write X/Y/LineH/LineA over the line's
		// box range and append the line-table entry.
		x := lineStartX
		for i := lineStart; i < nb; i++ {
			b := f.box[i]
			b.X = x
			b.Y = lineStartY
			b.LineH = lineH
			b.LineA = lineA
			x += b.Wid
		}
		if nb > lineStart {
			f.lines = append(f.lines, lineSummary{
				FirstBox:  lineStart,
				FirstRune: lineFirstRune,
				TopY:      lineStartY,
				LineH:     lineH,
				LineA:     lineA,
			})
		}

		// Advance pt to the next line's top. We continue past
		// rect.Max.Y rather than breaking — boxes past the
		// visible cutoff still need their X/Y/LineH refreshed
		// so layout-shift detection (contentBottomY in
		// SetStyleRange, etc.) sees the true post-mutation
		// extent. Paint walks (paintBox) bail when a box's Y
		// is outside the rect, so off-screen rows aren't
		// drawn — but their geometry is current.
		pt = image.Pt(f.rect.Min.X, lineStartY+lineH)
	}
}

// nextPositionAfterLast returns the screen point where the
// next character would land after the last box in f.box,
// derived from the line table. Replaces the _draw pt-
// accumulator walk in bxscan (B2.3 R4).
//
// For an empty frame, returns rect.Min. For a frame whose
// last box is a newline, returns the left margin at the
// next line's top. Otherwise returns (lastBox.X + lastBox.Wid,
// lastLine.TopY).
func (f *frameimpl) nextPositionAfterLast() image.Point {
	if len(f.box) == 0 || len(f.lines) == 0 {
		return f.rect.Min
	}
	last := f.lines[len(f.lines)-1]
	lastBox := f.box[len(f.box)-1]
	if lastBox.Nrune < 0 && lastBox.Bc == '\n' {
		return image.Pt(f.rect.Min.X, last.TopY+last.LineH)
	}
	return image.Pt(lastBox.X+lastBox.Wid, last.TopY)
}

// truncateOffscreen drops boxes/lines whose layout puts them
// at or past rect.Max.Y, adjusts f.nchars, and re-derives
// f.lastlinefull. Returns the post-truncation pt for "next
// character would land here," clamped to rect.Max.Y.
//
// Used by bxscan, which historically did this work via
// _draw's pt.Y == rect.Max.Y check. Per the new design
// (frame-layout-design §6.4), the staging frame's bxscan
// path is the only caller that wants bounded-frame
// semantics; the parent's relayoutFrom keeps off-screen
// content for correct shift detection.
func (f *frameimpl) truncateOffscreen() image.Point {
	// Find the first line whose TopY is at or past rect.Max.Y.
	// Everything from there is off-screen and gets dropped.
	truncFrom := -1
	for i, line := range f.lines {
		if line.TopY >= f.rect.Max.Y {
			truncFrom = i
			break
		}
	}
	if truncFrom >= 0 {
		firstBox := f.lines[truncFrom].FirstBox
		for _, b := range f.box[firstBox:] {
			f.nchars -= nrune(b)
		}
		f.box = f.box[:firstBox]
		// Re-relayout so f.lines is consistent with f.box and
		// the deferred lastlinefull update fires from the
		// truncated state. (Cheap — the loop runs at most
		// O(remaining lines) and never grows f.box again.)
		f.relayoutFrom(0)
	}
	pt := f.nextPositionAfterLast()
	if pt.Y > f.rect.Max.Y {
		pt.Y = f.rect.Max.Y
	}
	return pt
}

// coalesceAt reports whether box[nb] and box[nb+1] can be
// merged per frame-layout-design §3.3's eager-coalesce rule:
// both content boxes, same Style, same space/word category
// (preserves B5 word-wrap), combined Wid still fits at ptX.
func (f *frameimpl) coalesceAt(nb int, ptX int) bool {
	if nb+1 >= len(f.box) {
		return false
	}
	a, b := f.box[nb], f.box[nb+1]
	if a.Nrune <= 0 || b.Nrune <= 0 {
		return false
	}
	if a.Style != b.Style {
		return false
	}
	if isSpaceOnlyBox(a) != isSpaceOnlyBox(b) {
		return false
	}
	return ptX+a.Wid+b.Wid <= f.rect.Max.X
}

// updateLineMaxes folds box b's height and ascent into the
// running line maximums. Special boxes (Nrune<0) contribute
// only the default font's height/ascent. Content boxes use
// fontFor(b.Style).Height() and .Ascent() — so a KindScale
// box contributes the scaled font's metrics, driving the
// line's LineH and LineA from the dominant font on the line.
//
// LineA (max Ascent) is what paintBox uses to align glyphs at
// a shared baseline: each box paints at
//
//	Y + (LineA - fontAscent(box))
//
// (see frame-rendering-spec §3.3 / §6.3).
func (f *frameimpl) updateLineMaxes(b *frbox, lineH, lineA *int) {
	h := f.defaultfontheight
	a := f.font.Ascent()
	if b.Nrune >= 0 {
		ft := f.fontFor(b.Style)
		h = ft.Height()
		a = ft.Ascent()
	}
	if h > *lineH {
		*lineH = h
	}
	if a > *lineA {
		*lineA = a
	}
}
