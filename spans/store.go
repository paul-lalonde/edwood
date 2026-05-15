// Package spans maintains per-rune style data for one
// file.ObservableEditableBuffer. The Store observes the buffer
// (A3.2) so that style offsets stay aligned through Insert and
// Delete edits. Callers query GetStyleRuns to get the styling
// for a visible window of runes.
//
// The internal representation is a sorted slice of Regions that
// fully covers the buffer's rune range [0, totalLen) with no
// gaps. Plain regions are stored explicitly alongside non-plain
// ones; Empty() returns true when every region is plain (the
// fast-path enabler for unstyled buffers).
package spans

import (
	"fmt"
	"sort"

	"github.com/rjkroege/edwood/file"
	"github.com/rjkroege/edwood/frame"
)

// Region is one entry in the store. Together with the rest of
// the store's regions it covers the full rune range of the
// buffer with no gaps and no overlaps. Length is always > 0.
type Region struct {
	Start  int
	Length int
	Style  frame.Style
}

// Store maintains per-rune styling for one buffer.
type Store interface {
	// Empty reports whether any non-plain region exists.
	Empty() bool

	// GetStyleRuns returns a slice of StyleRuns covering the
	// rune range [p0, p1). Sum of Len equals p1-p0; no
	// zero-Len runs. Adjacent runs with identical Style may or
	// may not be coalesced.
	GetStyleRuns(p0, p1 int) []frame.StyleRun

	// SetRegion replaces (or creates) styling on the range
	// [p0, p1) with s. SetRegion with a plain s is equivalent
	// to ClearRegion. Triggers all Observe callbacks with
	// (p0, p1).
	SetRegion(p0, p1 int, s frame.Style)

	// ClearRegion restores the runes in [p0, p1) to plain style.
	// Triggers all Observe callbacks with (p0, p1).
	ClearRegion(p0, p1 int)

	// Observe registers fn for style-only updates — calls to
	// SetRegion or ClearRegion. fn receives the rune range that
	// was re-styled. Buffer-driven shifts (Inserted / Deleted)
	// are bookkeeping and do NOT fire fn.
	Observe(fn func(p0, p1 int))

	// Batch runs fn with notifications suspended. Calls to
	// SetRegion / ClearRegion inside fn update the regions but
	// do not fire Observe callbacks. After fn returns, a single
	// Observe call covers the union of all ranges touched during
	// the batch. Used by the xfid spans-write path so a payload
	// containing `c` (clear all) + `s ...` directives produces
	// one paint of the final state, not a paint of the cleared
	// state followed by a paint of the styled state.
	Batch(fn func())

	// Snapshot returns a copy of the current regions, sorted
	// by Start and covering the buffer's full rune range.
	Snapshot() []Region
}

// NewStore creates a Store. If buf is non-nil and already
// contains runes, the store is seeded with a single plain region
// covering them and registers itself on buf's observer chain so
// that Inserted/Deleted edits keep the store's offsets aligned.
func NewStore(buf *file.ObservableEditableBuffer) Store {
	s := &store{buf: buf}
	if buf != nil {
		if n := buf.Nr(); n > 0 {
			s.regions = []Region{{Start: 0, Length: n, Style: frame.Style{}}}
			s.totalLen = n
		}
		buf.AddObserver(s)
	}
	return s
}

// newStoreWithLen creates a store seeded with a plain run of
// length n. Package-internal helper for tests that don't carry a
// real buffer.
func newStoreWithLen(n int) *store {
	s := &store{}
	if n > 0 {
		s.regions = []Region{{Start: 0, Length: n, Style: frame.Style{}}}
		s.totalLen = n
	}
	return s
}

type store struct {
	buf       *file.ObservableEditableBuffer
	regions   []Region
	totalLen  int
	observers []func(p0, p1 int)

	// Batch state. When batchDepth > 0, notify() does not fire
	// observers; instead it widens batchP0/batchP1 to cover the
	// passed range. When the outermost Batch() exits, a single
	// notify fires with the accumulated range.
	batchDepth int
	batchP0    int
	batchP1    int
}

// Observe registers fn for style-only update callbacks. See the
// Store interface for the contract.
func (s *store) Observe(fn func(p0, p1 int)) {
	if fn == nil {
		return
	}
	s.observers = append(s.observers, fn)
}

// notify dispatches a style-only update to all observers. When
// a Batch is active, observers are NOT fired here — the range
// is accumulated for a single notification at batch end.
func (s *store) notify(p0, p1 int) {
	if s.batchDepth > 0 {
		if s.batchP0 == s.batchP1 {
			// First range in this batch.
			s.batchP0, s.batchP1 = p0, p1
		} else {
			if p0 < s.batchP0 {
				s.batchP0 = p0
			}
			if p1 > s.batchP1 {
				s.batchP1 = p1
			}
		}
		return
	}
	for _, fn := range s.observers {
		fn(p0, p1)
	}
}

// Batch implements Store.Batch. Nestable: only the outermost
// call fires the accumulated notification.
func (s *store) Batch(fn func()) {
	s.batchDepth++
	if s.batchDepth == 1 {
		s.batchP0, s.batchP1 = 0, 0
	}
	defer func() {
		s.batchDepth--
		if s.batchDepth == 0 && s.batchP0 < s.batchP1 {
			p0, p1 := s.batchP0, s.batchP1
			s.batchP0, s.batchP1 = 0, 0
			for _, fnObs := range s.observers {
				fnObs(p0, p1)
			}
		}
	}()
	fn()
}

func (s *store) Empty() bool {
	for i := range s.regions {
		if !s.regions[i].Style.IsPlain() {
			return false
		}
	}
	return true
}

func (s *store) GetStyleRuns(p0, p1 int) []frame.StyleRun {
	if p0 >= p1 {
		return nil
	}
	// First region whose end is past p0.
	i := sort.Search(len(s.regions), func(k int) bool {
		return s.regions[k].Start+s.regions[k].Length > p0
	})
	var out []frame.StyleRun
	for ; i < len(s.regions) && s.regions[i].Start < p1; i++ {
		r := s.regions[i]
		start := r.Start
		if start < p0 {
			start = p0
		}
		end := r.Start + r.Length
		if end > p1 {
			end = p1
		}
		out = append(out, frame.StyleRun{Len: end - start, Style: r.Style})
	}
	return out
}

func (s *store) SetRegion(p0, p1 int, style frame.Style) {
	if p0 >= p1 {
		return
	}
	if p0 < 0 || p1 > s.totalLen {
		panic(fmt.Sprintf("spans.SetRegion: out-of-range p0=%d p1=%d totalLen=%d", p0, p1, s.totalLen))
	}
	s.splitAt(p0)
	s.splitAt(p1)
	// Apply style to every region whose Start ∈ [p0, p1).
	i := sort.Search(len(s.regions), func(k int) bool {
		return s.regions[k].Start >= p0
	})
	for j := i; j < len(s.regions) && s.regions[j].Start < p1; j++ {
		s.regions[j].Style = style
	}
	s.coalesce()
	s.notify(p0, p1)
}

func (s *store) ClearRegion(p0, p1 int) {
	s.SetRegion(p0, p1, frame.Style{})
}

func (s *store) Snapshot() []Region {
	out := make([]Region, len(s.regions))
	copy(out, s.regions)
	return out
}

// splitAt splits the region containing rune offset p so that p
// becomes a region boundary. No-op when p is already on a
// boundary or out of range.
func (s *store) splitAt(p int) {
	if p <= 0 || p >= s.totalLen {
		return
	}
	i := sort.Search(len(s.regions), func(k int) bool {
		return s.regions[k].Start+s.regions[k].Length > p
	})
	if i >= len(s.regions) {
		return
	}
	r := s.regions[i]
	if r.Start == p {
		return
	}
	left := Region{Start: r.Start, Length: p - r.Start, Style: r.Style}
	right := Region{Start: p, Length: r.Length - left.Length, Style: r.Style}
	// Replace s.regions[i] with left, right.
	s.regions = append(s.regions[:i], append([]Region{left, right}, s.regions[i+1:]...)...)
}

// coalesce merges adjacent regions with identical Style across
// the whole slice. O(n) — fine for the expected region count.
func (s *store) coalesce() {
	if len(s.regions) < 2 {
		return
	}
	out := s.regions[:1]
	for i := 1; i < len(s.regions); i++ {
		last := &out[len(out)-1]
		if s.regions[i].Style == last.Style {
			last.Length += s.regions[i].Length
			continue
		}
		out = append(out, s.regions[i])
	}
	s.regions = out
}
