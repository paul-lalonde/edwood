package frame

import (
	"fmt"
	"image"
)

// InsertWithStyle inserts r at rune offset p0 with per-rune
// styling provided by styles. Implements the §5.4 contract:
//
//   - If styles is nil OR every StyleRun in styles is IsPlain(),
//     the call delegates to insertimpl — observable behavior is
//     identical to Insert.
//   - Otherwise the sum of StyleRun.Lens must equal len(r); the
//     function panics on mismatch (developer error).
//   - On the styled path, boxes produced for the inserted runes
//     carry the producer's Style. drawtext honors box.Style at
//     render time.
func (f *frameimpl) InsertWithStyle(r []rune, p0 int, styles []StyleRun) bool {
	if styles != nil {
		validateStyleRunsLen(styles, len(r))
	}
	f.lk.Lock()
	var ret bool
	if allPlain(styles) {
		ret = f.insertimpl(r, p0)
	} else {
		ret = f.insertbyteimpl([]byte(string(r)), p0, expandStyles(styles, len(r)))
	}
	hook := f.afterPaintHook
	f.lk.Unlock()
	if hook != nil {
		hook()
	}
	return ret
}

// validateStyleRunsLen enforces §5.4's invariant that the styles
// slice covers exactly len(r) runes.
func validateStyleRunsLen(styles []StyleRun, totalLen int) {
	sum := 0
	for _, sr := range styles {
		sum += sr.Len
	}
	if sum != totalLen {
		panic(fmt.Sprintf("frame.InsertWithStyle: sum of StyleRun.Lens (%d) != len(runes) (%d)", sum, totalLen))
	}
}

// allPlain returns true when every StyleRun carries a plain Style
// — i.e. the input warrants the fast path.
func allPlain(styles []StyleRun) bool {
	if styles == nil {
		return true
	}
	for _, sr := range styles {
		if !sr.Style.IsPlain() {
			return false
		}
	}
	return true
}

// expandStyles flattens a StyleRun slice into a per-rune Style
// slice of length total. Caller has already validated that the
// run lengths sum to total.
func expandStyles(styles []StyleRun, total int) []Style {
	out := make([]Style, total)
	i := 0
	for _, sr := range styles {
		for j := 0; j < sr.Len; j++ {
			out[i] = sr.Style
			i++
		}
	}
	return out
}

// SetStyleRange re-styles runes already in the frame at rune
// offsets [p0, p1) using styles. See §5.4 of the design.
func (f *frameimpl) SetStyleRange(p0, p1 int, styles []StyleRun) {
	if p0 == p1 {
		return
	}
	f.lk.Lock()
	defer func() {
		hook := f.afterPaintHook
		f.lk.Unlock()
		if hook != nil {
			hook()
		}
	}()

	if p0 < 0 || p0 > p1 || p1 > f.nchars {
		panic(fmt.Sprintf("frame.SetStyleRange: out-of-range p0=%d p1=%d nchars=%d", p0, p1, f.nchars))
	}
	validateStyleRunsLen(styles, p1-p0)

	runeStyles := expandStyles(styles, p1-p0)

	// B2.2 R4.1: snapshot the Y just past the last box —
	// effectively the total content height. If a SetStyleRange
	// shifts any line vertically (LineH change on some
	// preceding line), this value moves. Colour swaps and
	// bold-at-constant-height leave it unchanged → keep the
	// narrow repaint.
	preBottomY := f.contentBottomY()

	// Split at the [p0, p1) boundaries so the affected runes
	// occupy a contiguous box range.
	nb0 := f.findbox(0, 0, p0)
	nb1 := f.findbox(nb0, p0, p1)

	// Walk boxes, applying styles. When the style changes mid-box,
	// splitbox is called and nb1 grows. Box.Wid is recomputed
	// against the new style's font variant so the box advances by
	// the width the painter will actually use — without this, the
	// first paint after a span lands clips the right edge of a
	// bold glyph (the next box's background starts too early).
	runeIdx := 0
	nb := nb0
	for nb < nb1 {
		b := f.box[nb]
		if b.Nrune < 0 {
			// Special box (tab or newline): occupies one rune
			// but its width is metric/tabstop-driven, not
			// font-glyph-derived. Update Style only.
			b.Style = runeStyles[runeIdx]
			runeIdx++
			nb++
			continue
		}
		boxRunes := b.Nrune
		// Compute run of identical style within this box.
		curStyle := runeStyles[runeIdx]
		n := 1
		for n < boxRunes && runeStyles[runeIdx+n] == curStyle {
			n++
		}
		if n == boxRunes {
			b.Style = curStyle
			b.Wid = f.boxWid(b)
			runeIdx += boxRunes
			nb++
			continue
		}
		f.splitbox(nb, n)
		f.box[nb].Style = curStyle
		f.box[nb].Wid = f.boxWid(f.box[nb])
		runeIdx += n
		nb++
		nb1++
	}

	// Merge adjacent same-Style boxes within the affected range,
	// then refresh per-box X/Y/LineH/LineA. Both run BEFORE
	// paint so the narrow repaint reads fresh box geometry.
	cleanEnd := nb1 + 1
	if cleanEnd > len(f.box) {
		cleanEnd = len(f.box)
	}
	f.clean(f.ptofcharptb(p0, f.rect.Min, 0), nb0, cleanEnd)
	f.relayoutFrom(0)

	// If the layout shifted vertically (preBottomY changed),
	// the visible old-position pixels no longer match new-
	// position geometry. Clear the body rect and repaint
	// everything. Otherwise the narrow repaint suffices.
	if f.background == nil {
		return
	}
	col := f.cols[ColBack]
	tcol := f.cols[ColText]
	if f.contentBottomY() != preBottomY {
		f.background.Draw(f.rect, col, nil, image.Point{})
		f.repaintBoxRange(f.rect.Min, 0, len(f.box), tcol, col)
	} else {
		f.repaintBoxRange(f.rect.Min, nb0, nb1, tcol, col)
	}

	// Preserve the user's selection highlight if it overlaps
	// the styled range. The repaint just painted over the
	// selection's pixels; without this re-paint the highlight
	// would visibly flicker off when a producer (edcolor,
	// md2spans, …) reacts to the S event by re-styling the
	// just-selected token.
	if f.highlighton && f.sp0 < f.sp1 {
		ov0, ov1 := f.sp0, f.sp1
		if ov0 < p0 {
			ov0 = p0
		}
		if ov1 > p1 {
			ov1 = p1
		}
		if ov0 < ov1 {
			hpt := f.ptofcharptb(ov0, f.rect.Min, 0)
			f.drawsel0(hpt, ov0, ov1, f.cols[ColHigh], f.cols[ColHText])
		}
	}
}

// contentBottomY returns the Y just past the last box —
// effectively the total content height as laid out. If the
// last box is on a line of height H starting at Y, returns
// Y+H. Empty frame returns rect.Min.Y. Used by SetStyleRange
// to detect a vertical shift (line-height change) that
// invalidates the existing pixel layout, so we know whether
// to trigger a full clear+repaint or stay on the parsimonious
// narrow path.
func (f *frameimpl) contentBottomY() int {
	if len(f.box) == 0 {
		return f.rect.Min.Y
	}
	last := f.box[len(f.box)-1]
	lineH := last.LineH
	if lineH == 0 {
		lineH = f.defaultfontheight
	}
	return last.Y + lineH
}
