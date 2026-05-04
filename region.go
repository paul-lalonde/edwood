package main

// Region represents a scoped layout region in the body — a
// contiguous rune range with a kind (code, blockquote,
// listitem, table) and optional parameters. Regions form a
// tree: a Parent is the enclosing region, or nil for
// top-level regions; Children are the directly-nested
// regions inside this one.
//
// Phase 3 round 5 introduces the type with the simplest
// kind ("code", v1 only). Rounds 6-8 extend the kind
// vocabulary; the type shape is stable from round 5.
type Region struct {
	// Start, End are body rune offsets, half-open [Start, End).
	// A rune at position End is NOT inside the region.
	Start, End int
	// Kind is the region's layout-mode discriminator. v1
	// recognized values: "code". Future rounds add
	// "blockquote", "listitem", "table". The wire-format
	// counterpart is `begin region <kind>` on the spans
	// protocol.
	Kind string
	// Params are key=value parameters attached to the
	// region (e.g., {"lang": "go"} for a fenced code
	// block). Unknown params are preserved (forward-compat);
	// the consumer interprets known ones at render time.
	Params map[string]string
	// Parent is the directly enclosing region, or nil for
	// top-level regions. Children is the list of directly
	// nested regions inside this one. The tree is
	// constructed by RegionStore.Add based on offset
	// containment.
	Parent   *Region
	Children []*Region
}

// Equal reports whether two regions have the same data
// (Start, End, Kind, Params). Tree position (Parent /
// Children) is NOT compared — Equal is a value-equality
// helper for tests, not a structural one.
func (r *Region) Equal(other *Region) bool {
	if r == nil || other == nil {
		return r == other
	}
	if r.Start != other.Start || r.End != other.End || r.Kind != other.Kind {
		return false
	}
	if len(r.Params) != len(other.Params) {
		return false
	}
	for k, v := range r.Params {
		if w, ok := other.Params[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// contains reports whether r's range fully contains
// [start, end). A region contains itself only if (start,
// end) == (r.Start, r.End); equal-range regions are not
// considered nested.
func (r *Region) contains(start, end int) bool {
	if r.Start <= start && end <= r.End {
		// Equal-range counts as "contains" only if it's the
		// same range; callers that need strict-contains
		// (proper containment) check separately.
		return true
	}
	return false
}

// strictlyContains reports whether r's range fully contains
// [start, end) AND the ranges are not identical. Used by
// Add to decide tree placement: a strictly-containing
// existing region becomes the parent of the new one.
func (r *Region) strictlyContains(start, end int) bool {
	if !r.contains(start, end) {
		return false
	}
	return r.Start != start || r.End != end
}

// enclosingAt walks down through this region's children and
// returns the deepest region containing pos, or nil if pos
// is outside this region. [Start, End) half-open: pos == End
// is NOT inside.
func (r *Region) enclosingAt(pos int) *Region {
	if pos < r.Start || pos >= r.End {
		return nil
	}
	for _, c := range r.Children {
		if deeper := c.enclosingAt(pos); deeper != nil {
			return deeper
		}
	}
	return r
}

// RegionStore manages the set of regions for a window's
// body. Regions form a forest of trees (multiple top-level
// regions, each potentially with nested children). The
// store is the consumer-side counterpart to the
// spans-protocol's `begin region` / `end region`
// directives. Phase 3 round 5.
type RegionStore struct {
	roots []*Region
}

// NewRegionStore creates an empty RegionStore.
func NewRegionStore() *RegionStore {
	return &RegionStore{}
}

// Roots returns the top-level regions (those with no
// Parent). Returned slice is the store's internal storage;
// callers should not mutate it.
func (s *RegionStore) Roots() []*Region {
	return s.roots
}

// Clear removes all regions from the store.
func (s *RegionStore) Clear() {
	s.roots = nil
}

// Add inserts a region into the tree. Placement is by
// offset containment:
//
//   - If an existing region strictly contains r, find the
//     deepest such ancestor and add r as one of its
//     children. Any existing siblings under that ancestor
//     that are themselves strictly contained by r are moved
//     under r.
//   - Otherwise r is added at the top level. Any existing
//     top-level regions strictly contained by r are moved
//     under r.
//
// Preconditions: regions added to the store must not
// partially overlap any existing region. Either nested
// (one strictly contains the other) or disjoint
// (non-overlapping). Partial-overlap inputs are not
// validated and produce undefined tree shape — the
// producer (parser of begin/end directives) is responsible
// for emitting well-formed regions.
func (s *RegionStore) Add(r *Region) {
	parent := s.findContainer(r.Start, r.End, s.roots)
	var siblingPool *[]*Region
	if parent == nil {
		siblingPool = &s.roots
	} else {
		siblingPool = &parent.Children
	}

	// Re-parent any existing siblings that r strictly contains.
	kept := (*siblingPool)[:0]
	for _, sibling := range *siblingPool {
		if r.strictlyContains(sibling.Start, sibling.End) {
			sibling.Parent = r
			r.Children = append(r.Children, sibling)
		} else {
			kept = append(kept, sibling)
		}
	}
	*siblingPool = kept

	r.Parent = parent
	*siblingPool = append(*siblingPool, r)
}

// findContainer returns the deepest existing region in
// `candidates` (or their descendants) that strictly contains
// [start, end). nil if none.
func (s *RegionStore) findContainer(start, end int, candidates []*Region) *Region {
	for _, c := range candidates {
		if !c.strictlyContains(start, end) {
			continue
		}
		if deeper := s.findContainer(start, end, c.Children); deeper != nil {
			return deeper
		}
		return c
	}
	return nil
}

// EnclosingAt returns the deepest region containing pos, or
// nil if pos is outside any region. Uses [Start, End)
// half-open semantics.
func (s *RegionStore) EnclosingAt(pos int) *Region {
	for _, r := range s.roots {
		if got := r.enclosingAt(pos); got != nil {
			return got
		}
	}
	return nil
}

// BoundariesIn returns the sorted, deduplicated list of
// region boundary offsets STRICTLY between start and end
// (exclusive on both sides). Used by the bridge to split a
// styled run at region boundaries when the producer's runs
// don't natively align with them — the producer-
// responsibility note from round 5 said producers should
// emit separate s/b runs at boundaries (and md2spans for
// `code` does this naturally because the inside style
// differs), but for blockquote regions covering
// default-styled runs the spanStore coalesces and loses
// the boundaries. Phase 3 round 6.
func (s *RegionStore) BoundariesIn(start, end int) []int {
	if start >= end {
		return nil
	}
	seen := map[int]bool{}
	var collect func([]*Region)
	collect = func(rs []*Region) {
		for _, r := range rs {
			// Skip regions that don't overlap [start, end).
			if r.End <= start || r.Start >= end {
				continue
			}
			if r.Start > start && r.Start < end {
				seen[r.Start] = true
			}
			if r.End > start && r.End < end {
				seen[r.End] = true
			}
			collect(r.Children)
		}
	}
	collect(s.roots)
	out := make([]int, 0, len(seen))
	for b := range seen {
		out = append(out, b)
	}
	// Insertion sort (small N).
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// Insert shifts region offsets to account for `length` runes
// inserted at body position `pos`. The contract:
//
//   - For a region whose Start is at or after pos, both Start
//     and End shift by length (the whole region moved).
//   - For a region whose body strictly contains pos
//     (Start < pos < End), only End shifts (the region grew).
//   - For a region entirely before pos (End <= pos), nothing
//     changes.
//
// Mirrors the spanStore Insert pattern. The convention "insert
// AT Start shifts the region" matches the spanStore's
// extend-the-following-run rule.
func (s *RegionStore) Insert(pos, length int) {
	if length <= 0 {
		return
	}
	for _, r := range s.roots {
		r.applyInsert(pos, length)
	}
}

// applyInsert recursively applies an Insert(pos, length) to r
// and its descendants.
func (r *Region) applyInsert(pos, length int) {
	switch {
	case pos <= r.Start:
		r.Start += length
		r.End += length
	case pos < r.End:
		r.End += length
	}
	for _, c := range r.Children {
		c.applyInsert(pos, length)
	}
}

// Delete shifts / drops regions to account for `length` runes
// deleted at body position `pos`. v1 conservative rule:
//
//   - A region entirely before the deleted range shifts both
//     Start and End down by length.
//   - A region entirely after the deleted range is untouched.
//   - A region whose body is touched by the delete (any
//     overlap with [pos, pos+length)) is DROPPED, with one
//     exception: if a CHILD region fully contained the delete,
//     the parent only shrinks (End -= length) and the child
//     takes the hit. This treats the parent's "proper body"
//     (its range minus any descendants) as the unit that
//     determines drop-vs-shrink.
//
// Dropped regions take their entire subtree with them. Next
// render rebuilds them from the producer.
func (s *RegionStore) Delete(pos, length int) {
	if length <= 0 {
		return
	}
	var surviving []*Region
	for _, r := range s.roots {
		if r.applyDelete(pos, length) {
			surviving = append(surviving, r)
		}
	}
	s.roots = surviving
}

// applyDelete recursively applies a Delete(pos, length) to r.
// Returns true if r should be kept; false if r is dropped.
// Modifies r and r.Children in place when keeping.
func (r *Region) applyDelete(pos, length int) bool {
	delEnd := pos + length
	switch {
	case delEnd <= r.Start:
		// Delete fully before r: shift both endpoints.
		r.Start -= length
		r.End -= length
		r.Children = filterDelete(r.Children, pos, length)
		return true
	case pos >= r.End:
		// Delete fully after r: untouched (children too —
		// they're inside r).
		return true
	case pos < r.Start || delEnd > r.End:
		// Delete crosses a boundary of r: drop r entirely
		// (and its subtree by virtue of returning false).
		return false
	}
	// Delete is fully inside r's range (r.Start <= pos and
	// delEnd <= r.End). Check whether any child contained
	// the delete — if so, the parent just shrinks; if not,
	// the delete touched r's proper body and we drop r.
	childContained := false
	for _, c := range r.Children {
		if c.Start <= pos && delEnd <= c.End {
			childContained = true
			break
		}
	}
	if childContained {
		r.Children = filterDelete(r.Children, pos, length)
		r.End -= length
		return true
	}
	return false
}

// filterDelete recursively applies the delete to children
// and returns the surviving subset, with parent links
// preserved.
func filterDelete(children []*Region, pos, length int) []*Region {
	var surviving []*Region
	for _, c := range children {
		if c.applyDelete(pos, length) {
			surviving = append(surviving, c)
		}
	}
	return surviving
}
