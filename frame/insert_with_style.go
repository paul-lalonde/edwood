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
