package frame

import (
	"fmt"
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
	defer f.lk.Unlock()
	if allPlain(styles) {
		return f.insertimpl(r, p0)
	}
	return f.insertbyteimpl([]byte(string(r)), p0, expandStyles(styles, len(r)))
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
	defer f.lk.Unlock()

	if p0 < 0 || p0 > p1 || p1 > f.nchars {
		panic(fmt.Sprintf("frame.SetStyleRange: out-of-range p0=%d p1=%d nchars=%d", p0, p1, f.nchars))
	}
	validateStyleRunsLen(styles, p1-p0)

	runeStyles := expandStyles(styles, p1-p0)

	// Split at the [p0, p1) boundaries so the affected runes
	// occupy a contiguous box range.
	nb0 := f.findbox(0, 0, p0)
	nb1 := f.findbox(nb0, p0, p1)

	// Walk boxes, applying styles. When the style changes mid-box,
	// splitbox is called and nb1 grows.
	runeIdx := 0
	nb := nb0
	for nb < nb1 {
		b := f.box[nb]
		boxRunes := nrune(b)
		if boxRunes <= 0 {
			// Special box (tab or newline): exactly one rune.
			b.Style = runeStyles[runeIdx]
			runeIdx++
			nb++
			continue
		}
		// Compute run of identical style within this box.
		curStyle := runeStyles[runeIdx]
		n := 1
		for n < boxRunes && runeStyles[runeIdx+n] == curStyle {
			n++
		}
		if n == boxRunes {
			b.Style = curStyle
			runeIdx += boxRunes
			nb++
			continue
		}
		f.splitbox(nb, n)
		f.box[nb].Style = curStyle
		runeIdx += n
		nb++
		nb1++
	}

	// Repaint the affected box range.
	if f.background != nil {
		col := f.cols[ColBack]
		tcol := f.cols[ColText]
		pt := f.ptofcharptb(p0, f.rect.Min, 0)
		f.repaintBoxRange(pt, nb0, nb1, tcol, col)
	}

	// Merge adjacent same-Style boxes within the affected range.
	// clean's per-pair guard already requires equal Style.
	cleanEnd := nb1 + 1
	if cleanEnd > len(f.box) {
		cleanEnd = len(f.box)
	}
	f.clean(f.ptofcharptb(p0, f.rect.Min, 0), nb0, cleanEnd)
}
