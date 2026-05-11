package spans

import (
	"sort"

	"github.com/rjkroege/edwood/file"
	"github.com/rjkroege/edwood/frame"
)

// Compile-time check that *store satisfies file.BufferObserver.
var _ file.BufferObserver = (*store)(nil)

// Inserted handles buffer.Inserted notifications. It applies the
// §6.2 trailing/leading-edge rule under the dense full-coverage
// invariant and grows totalLen.
//
// Per-region behavior (let s = region.Start, e = region.Start+Length):
//   - q0 < s: region shifts forward by +nr.
//   - q0 == s (leading edge): the previous region's trailing edge
//     extends by +nr; this region shifts. At i==0 with plain
//     region 0, "extend region 0" is observably identical to
//     "prepend a plain region of length nr" because of coalescing.
//   - s < q0 <= e: region's interior or trailing edge extends.
//   - q0 > e: region is untouched in isolation (later shifts may
//     still apply transitively).
func (s *store) Inserted(q0 file.OffsetTuple, b []byte, nr int) {
	if nr <= 0 {
		return
	}
	pos := q0.R

	if s.totalLen == 0 {
		s.regions = []Region{{Start: 0, Length: nr, Style: frame.Style{}}}
		s.totalLen = nr
		return
	}

	if pos >= s.totalLen {
		// End-of-buffer insertion: trailing edge of last region.
		last := &s.regions[len(s.regions)-1]
		last.Length += nr
		s.totalLen += nr
		return
	}

	// pos < totalLen — find first region with End > pos.
	i := sort.Search(len(s.regions), func(k int) bool {
		return s.regions[k].Start+s.regions[k].Length > pos
	})

	if s.regions[i].Start == pos {
		// Leading-edge insertion at region i.
		if i == 0 {
			if s.regions[0].Style.IsPlain() {
				// Extending plain region 0 ≡ prepend-plain + coalesce.
				s.regions[0].Length += nr
				for k := 1; k < len(s.regions); k++ {
					s.regions[k].Start += nr
				}
			} else {
				// Prepend a new plain region; shift all others.
				prepended := []Region{{Start: 0, Length: nr, Style: frame.Style{}}}
				s.regions = append(prepended, s.regions...)
				for k := 1; k < len(s.regions); k++ {
					s.regions[k].Start += nr
				}
			}
		} else {
			// Previous region's trailing edge extends; subsequent
			// regions shift.
			s.regions[i-1].Length += nr
			for k := i; k < len(s.regions); k++ {
				s.regions[k].Start += nr
			}
		}
	} else {
		// Mid-region insertion: that region extends, subsequent
		// regions shift.
		s.regions[i].Length += nr
		for k := i + 1; k < len(s.regions); k++ {
			s.regions[k].Start += nr
		}
	}
	s.totalLen += nr
}

// Deleted handles buffer.Deleted notifications. It clips
// intersecting regions, drops fully-covered ones, and shifts
// regions past the deletion left by delLen.
func (s *store) Deleted(q0, q1 file.OffsetTuple) {
	a := q0.R
	b := q1.R
	if a >= b || a < 0 {
		return
	}
	if a > s.totalLen {
		return
	}
	if b > s.totalLen {
		b = s.totalLen
	}
	delLen := b - a
	if delLen == 0 {
		return
	}

	out := s.regions[:0]
	for _, r := range s.regions {
		rs := r.Start
		re := r.Start + r.Length
		switch {
		case re <= a:
			// entirely before deletion: untouched
			out = append(out, r)
		case rs >= b:
			// entirely after deletion: shift left
			r.Start -= delLen
			out = append(out, r)
		case rs >= a && re <= b:
			// entirely contained: drop
		case rs < a && re <= b:
			// straddles left edge: keep [rs, a)
			r.Length = a - rs
			out = append(out, r)
		case rs >= a && re > b:
			// straddles right edge: keep [b, re), shift to [a, ...)
			r.Start = a
			r.Length = re - b
			out = append(out, r)
		default:
			// wraps (rs < a && re > b): shrink by delLen
			r.Length -= delLen
			out = append(out, r)
		}
	}
	s.regions = out
	s.totalLen -= delLen
	s.coalesce()
}
