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
