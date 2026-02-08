package main

import "image/color"

// StyleAttrs holds concrete styling for a span of text.
// Zero value means "default" (no explicit styling).
type StyleAttrs struct {
	Fg     color.Color // nil = default foreground
	Bg     color.Color // nil = default background
	Bold   bool
	Italic bool
	Hidden bool // reserved for future use
}

// colorEqual compares two color.Color values, handling nil.
func colorEqual(a, b color.Color) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	r1, g1, b1, a1 := a.RGBA()
	r2, g2, b2, a2 := b.RGBA()
	return r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2
}

// Equal reports whether a and b have identical styling.
func (a StyleAttrs) Equal(b StyleAttrs) bool {
	return colorEqual(a.Fg, b.Fg) &&
		colorEqual(a.Bg, b.Bg) &&
		a.Bold == b.Bold &&
		a.Italic == b.Italic &&
		a.Hidden == b.Hidden
}

// StyleRun is a contiguous range of runes sharing the same style.
type StyleRun struct {
	Len   int // number of runes (must be >= 0)
	Style StyleAttrs
}

// SpanStore manages styled runs for a window's body text using a gap buffer.
type SpanStore struct {
	runs     []StyleRun // storage array with gap
	gap0     int        // start of gap (first unused index)
	gap1     int        // end of gap (first used index after gap)
	totalLen int        // cached sum of all run lengths
}

const minGapCapacity = 32

// NewSpanStore creates an empty SpanStore.
func NewSpanStore() *SpanStore {
	runs := make([]StyleRun, minGapCapacity)
	return &SpanStore{
		runs: runs,
		gap0: 0,
		gap1: len(runs),
	}
}

// TotalLen returns the total number of runes covered by all runs.
func (s *SpanStore) TotalLen() int {
	return s.totalLen
}

// NumRuns returns the number of active runs (excluding the gap).
func (s *SpanStore) NumRuns() int {
	return s.gap0 + (len(s.runs) - s.gap1)
}

// Clear removes all runs and resets TotalLen to 0.
func (s *SpanStore) Clear() {
	s.gap0 = 0
	s.gap1 = len(s.runs)
	s.totalLen = 0
}

// ForEachRun calls fn for each run in order.
func (s *SpanStore) ForEachRun(fn func(StyleRun)) {
	for i := 0; i < s.gap0; i++ {
		fn(s.runs[i])
	}
	for i := s.gap1; i < len(s.runs); i++ {
		fn(s.runs[i])
	}
}

// Runs returns all runs as a new slice.
func (s *SpanStore) Runs() []StyleRun {
	n := s.NumRuns()
	if n == 0 {
		return nil
	}
	result := make([]StyleRun, 0, n)
	s.ForEachRun(func(r StyleRun) {
		result = append(result, r)
	})
	return result
}

// physicalIndex converts a logical index to a physical index in the runs slice.
func (s *SpanStore) physicalIndex(logical int) int {
	if logical < s.gap0 {
		return logical
	}
	return logical + (s.gap1 - s.gap0)
}

// getRun returns the run at the given logical index.
func (s *SpanStore) getRun(logical int) StyleRun {
	return s.runs[s.physicalIndex(logical)]
}

// setRun sets the run at the given logical index.
func (s *SpanStore) setRun(logical int, r StyleRun) {
	s.runs[s.physicalIndex(logical)] = r
}

// moveGapTo repositions the gap so that gap0 == logicalIdx.
func (s *SpanStore) moveGapTo(logicalIdx int) {
	if logicalIdx == s.gap0 {
		return
	}
	if logicalIdx < s.gap0 {
		// Move runs [logicalIdx, gap0) rightward to end at gap1.
		count := s.gap0 - logicalIdx
		copy(s.runs[s.gap1-count:s.gap1], s.runs[logicalIdx:s.gap0])
		s.gap1 -= count
		s.gap0 = logicalIdx
	} else {
		// logicalIdx > gap0: move runs from after the gap leftward.
		count := logicalIdx - s.gap0
		copy(s.runs[s.gap0:s.gap0+count], s.runs[s.gap1:s.gap1+count])
		s.gap0 += count
		s.gap1 += count
	}
}

// gapSize returns the number of unused slots in the gap.
func (s *SpanStore) gapSize() int {
	return s.gap1 - s.gap0
}

// growGap ensures there are at least needed free slots in the gap.
func (s *SpanStore) growGap(needed int) {
	if s.gapSize() >= needed {
		return
	}
	oldLen := len(s.runs)
	// Double or add needed, whichever is larger.
	growth := oldLen
	if growth < needed {
		growth = needed
	}
	if growth < minGapCapacity {
		growth = minGapCapacity
	}
	newLen := oldLen + growth

	newRuns := make([]StyleRun, newLen)
	// Copy before-gap.
	copy(newRuns[:s.gap0], s.runs[:s.gap0])
	// Copy after-gap to the end of the new slice.
	afterCount := oldLen - s.gap1
	newGap1 := newLen - afterCount
	copy(newRuns[newGap1:], s.runs[s.gap1:])

	s.runs = newRuns
	s.gap1 = newGap1
}

// findRunAt locates which run contains rune position pos.
// Returns (logicalIndex, offsetWithinRun).
// When pos == TotalLen, returns (NumRuns(), 0).
func (s *SpanStore) findRunAt(pos int) (int, int) {
	n := s.NumRuns()
	accum := 0
	for i := 0; i < n; i++ {
		r := s.getRun(i)
		if pos < accum+r.Len {
			return i, pos - accum
		}
		accum += r.Len
	}
	return n, 0
}

// Insert adjusts runs when text is inserted at rune position pos.
func (s *SpanStore) Insert(pos, length int) {
	if length <= 0 {
		return
	}
	n := s.NumRuns()

	// Empty store: create a default run.
	if n == 0 {
		s.moveGapTo(0)
		s.growGap(1)
		s.runs[s.gap0] = StyleRun{Len: length, Style: StyleAttrs{}}
		s.gap0++
		s.totalLen = length
		return
	}

	// Insert at start: extend first run.
	if pos == 0 {
		r := s.getRun(0)
		r.Len += length
		s.setRun(0, r)
		s.totalLen += length
		return
	}

	// Insert at end: extend last run.
	if pos >= s.totalLen {
		r := s.getRun(n - 1)
		r.Len += length
		s.setRun(n-1, r)
		s.totalLen += length
		return
	}

	runIdx, offsetInRun := s.findRunAt(pos)

	if offsetInRun == 0 {
		// At run boundary: extend the preceding run.
		r := s.getRun(runIdx - 1)
		r.Len += length
		s.setRun(runIdx-1, r)
	} else {
		// Mid-run: extend the containing run.
		r := s.getRun(runIdx)
		r.Len += length
		s.setRun(runIdx, r)
	}
	s.totalLen += length
}

// Delete adjusts runs when runes in [pos, pos+length) are deleted.
func (s *SpanStore) Delete(pos, length int) {
	// Clamp.
	if pos+length > s.totalLen {
		length = s.totalLen - pos
	}
	if length <= 0 {
		return
	}

	startIdx, startOff := s.findRunAt(pos)
	endIdx, endOff := s.findRunAt(pos + length)

	if startIdx == endIdx {
		// Deletion within a single run.
		r := s.getRun(startIdx)
		r.Len -= length
		if r.Len == 0 {
			// Remove this run by moving the gap to it and absorbing it.
			s.moveGapTo(startIdx)
			s.gap1++
		} else {
			s.setRun(startIdx, r)
		}
		s.totalLen -= length
		return
	}

	// Deletion spans multiple runs.
	// Shrink the first run.
	firstRun := s.getRun(startIdx)
	firstRun.Len = startOff

	// Shrink the last run.
	var lastRun StyleRun
	if endIdx < s.NumRuns() {
		lastRun = s.getRun(endIdx)
		lastRun.Len -= endOff
	}

	// Move gap to startIdx+1 if first run survives, else startIdx.
	keepFirst := firstRun.Len > 0
	keepLast := endIdx < s.NumRuns() && lastRun.Len > 0

	if keepFirst {
		s.setRun(startIdx, firstRun)
	}

	// Determine the range of runs to remove: all fully-contained plus
	// zero-length edge runs.
	removeStart := startIdx
	if keepFirst {
		removeStart = startIdx + 1
	}
	removeEnd := endIdx // exclusive; endIdx run is handled separately
	if endIdx < s.NumRuns() {
		if keepLast {
			// Update the last run in place before we move the gap.
			s.setRun(endIdx, lastRun)
		} else {
			removeEnd = endIdx + 1
		}
	}

	// Remove runs [removeStart, removeEnd) by absorbing into the gap.
	if removeEnd > removeStart {
		s.moveGapTo(removeStart)
		// Absorb (removeEnd - removeStart) runs after the gap.
		s.gap1 += (removeEnd - removeStart)
	}

	// Merge adjacent runs with identical styles at the deletion boundary.
	s.totalLen -= length
	s.mergeAdjacent()
}

// mergeAdjacent scans all runs and merges any adjacent runs with equal styles.
func (s *SpanStore) mergeAdjacent() {
	n := s.NumRuns()
	if n < 2 {
		return
	}
	i := 0
	for i < s.NumRuns()-1 {
		cur := s.getRun(i)
		next := s.getRun(i + 1)
		if cur.Style.Equal(next.Style) {
			// Merge: extend current, remove next.
			cur.Len += next.Len
			s.setRun(i, cur)
			// Remove run at i+1.
			s.moveGapTo(i + 1)
			s.gap1++
		} else {
			i++
		}
	}
}

// removeZeroLengthRuns removes any runs with Len == 0.
func (s *SpanStore) removeZeroLengthRuns() {
	i := 0
	for i < s.NumRuns() {
		r := s.getRun(i)
		if r.Len == 0 {
			s.moveGapTo(i)
			s.gap1++
		} else {
			i++
		}
	}
}

// RegionUpdate replaces style information in [offset, offset+sum(newRuns.Len)).
func (s *SpanStore) RegionUpdate(offset int, newRuns []StyleRun) {
	// Compute new total length for the region.
	newTotalLen := 0
	for _, r := range newRuns {
		newTotalLen += r.Len
	}

	if newTotalLen == 0 && len(newRuns) == 0 {
		return
	}

	// Find start and end boundaries.
	startIdx, startOff := s.findRunAt(offset)
	endIdx, endOff := s.findRunAt(offset + newTotalLen)

	// Split at start boundary if needed.
	if startOff > 0 {
		origRun := s.getRun(startIdx)
		// Split into [0, startOff) kept, [startOff, origRun.Len) to be replaced.
		beforeRun := StyleRun{Len: startOff, Style: origRun.Style}
		afterRun := StyleRun{Len: origRun.Len - startOff, Style: origRun.Style}

		// Replace the original with the before part, insert after part.
		s.setRun(startIdx, beforeRun)
		// Insert afterRun at startIdx+1.
		s.moveGapTo(startIdx + 1)
		s.growGap(1)
		s.runs[s.gap0] = afterRun
		s.gap0++

		startIdx++ // now points to the first run fully inside the region

		// Recalculate endIdx since we inserted a run.
		endIdx, endOff = s.findRunAt(offset + newTotalLen)
	}

	// Split at end boundary if needed.
	if endOff > 0 && endIdx < s.NumRuns() {
		origRun := s.getRun(endIdx)
		insideRun := StyleRun{Len: endOff, Style: origRun.Style}
		afterRun := StyleRun{Len: origRun.Len - endOff, Style: origRun.Style}

		// Replace the original with inside part, insert after part.
		s.setRun(endIdx, insideRun)
		s.moveGapTo(endIdx + 1)
		s.growGap(1)
		s.runs[s.gap0] = afterRun
		s.gap0++

		endIdx++ // include the inside part in the replacement range
	}

	// Now remove runs [startIdx, endIdx) and insert newRuns.
	numToRemove := endIdx - startIdx
	s.moveGapTo(startIdx)
	s.gap1 += numToRemove

	// Filter out zero-length runs from newRuns.
	filtered := make([]StyleRun, 0, len(newRuns))
	for _, r := range newRuns {
		if r.Len > 0 {
			filtered = append(filtered, r)
		}
	}

	// Insert filtered runs.
	if len(filtered) > 0 {
		s.growGap(len(filtered))
		for _, r := range filtered {
			s.runs[s.gap0] = r
			s.gap0++
		}
	}

	// Remove any zero-length runs and merge adjacent.
	s.removeZeroLengthRuns()
	s.mergeAdjacent()

	// Recompute totalLen to ensure consistency.
	s.recomputeTotalLen()
}

// recomputeTotalLen recalculates totalLen from the actual runs.
func (s *SpanStore) recomputeTotalLen() {
	total := 0
	s.ForEachRun(func(r StyleRun) {
		total += r.Len
	})
	s.totalLen = total
}
