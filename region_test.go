package main

import "testing"

// =========================================================================
// Region equality
// =========================================================================

// TestRegion_DataEqual: Equal compares Start, End, Kind, Params
// — NOT tree structure (Parent / Children). Two regions with the
// same data fields but different positions in the tree are
// "equal" in the value-comparison sense.
func TestRegion_DataEqual(t *testing.T) {
	a := &Region{Start: 5, End: 10, Kind: "code"}
	b := &Region{Start: 5, End: 10, Kind: "code"}
	if !a.Equal(b) {
		t.Error("two regions with identical data fields should be equal")
	}
}

func TestRegion_DataNotEqual(t *testing.T) {
	a := &Region{Start: 5, End: 10, Kind: "code"}
	b := &Region{Start: 5, End: 11, Kind: "code"}
	if a.Equal(b) {
		t.Error("regions with different End should not be equal")
	}
	c := &Region{Start: 5, End: 10, Kind: "blockquote"}
	if a.Equal(c) {
		t.Error("regions with different Kind should not be equal")
	}
}

func TestRegion_ParamsEqual(t *testing.T) {
	a := &Region{Start: 0, End: 10, Kind: "code", Params: map[string]string{"lang": "go"}}
	b := &Region{Start: 0, End: 10, Kind: "code", Params: map[string]string{"lang": "go"}}
	if !a.Equal(b) {
		t.Error("regions with identical params should be equal")
	}
	c := &Region{Start: 0, End: 10, Kind: "code", Params: map[string]string{"lang": "python"}}
	if a.Equal(c) {
		t.Error("regions with different params should not be equal")
	}
	d := &Region{Start: 0, End: 10, Kind: "code"}
	if a.Equal(d) {
		t.Error("region with params should not equal one without")
	}
}

// =========================================================================
// RegionStore — empty
// =========================================================================

func TestRegionStore_NewIsEmpty(t *testing.T) {
	s := NewRegionStore()
	if s == nil {
		t.Fatal("NewRegionStore returned nil")
	}
	if got := s.Roots(); len(got) != 0 {
		t.Errorf("new store has %d roots, want 0", len(got))
	}
	if got := s.EnclosingAt(0); got != nil {
		t.Errorf("EnclosingAt on empty store returned %v, want nil", got)
	}
}

// =========================================================================
// RegionStore — Add (single region)
// =========================================================================

func TestRegionStore_AddSingleTopLevel(t *testing.T) {
	s := NewRegionStore()
	r := &Region{Start: 5, End: 10, Kind: "code"}
	s.Add(r)

	roots := s.Roots()
	if len(roots) != 1 {
		t.Fatalf("got %d roots, want 1", len(roots))
	}
	if roots[0] != r {
		t.Error("root is not the added region")
	}
	if r.Parent != nil {
		t.Error("top-level region should have nil Parent")
	}
}

// =========================================================================
// RegionStore — Add (nested case 1: child added after parent)
// =========================================================================

// TestRegionStore_AddNestedAfterParent: existing region [0,20)
// followed by a new region [5,10) — new region becomes a
// child.
func TestRegionStore_AddNestedAfterParent(t *testing.T) {
	s := NewRegionStore()
	parent := &Region{Start: 0, End: 20, Kind: "blockquote"}
	child := &Region{Start: 5, End: 10, Kind: "code"}
	s.Add(parent)
	s.Add(child)

	if len(s.Roots()) != 1 {
		t.Fatalf("got %d roots, want 1 (parent)", len(s.Roots()))
	}
	if len(parent.Children) != 1 {
		t.Fatalf("parent has %d children, want 1", len(parent.Children))
	}
	if parent.Children[0] != child {
		t.Error("parent's child is not the added region")
	}
	if child.Parent != parent {
		t.Error("child's Parent should point at parent")
	}
}

// =========================================================================
// RegionStore — Add (nested case 2: parent added after child)
// =========================================================================

// TestRegionStore_AddContainingAfterChild: existing region
// [5,10) followed by a new region [0,20) — existing region
// becomes a child of the new one.
func TestRegionStore_AddContainingAfterChild(t *testing.T) {
	s := NewRegionStore()
	child := &Region{Start: 5, End: 10, Kind: "code"}
	parent := &Region{Start: 0, End: 20, Kind: "blockquote"}
	s.Add(child)
	s.Add(parent)

	if len(s.Roots()) != 1 {
		t.Fatalf("got %d roots, want 1 (parent)", len(s.Roots()))
	}
	if s.Roots()[0] != parent {
		t.Error("root should be parent")
	}
	if len(parent.Children) != 1 {
		t.Fatalf("parent has %d children, want 1", len(parent.Children))
	}
	if parent.Children[0] != child {
		t.Error("parent's child should be the previously-added region")
	}
	if child.Parent != parent {
		t.Error("child's Parent should now point at parent")
	}
}

// TestRegionStore_AddTwoSiblings: two non-overlapping regions
// at the top level remain siblings (both roots).
func TestRegionStore_AddTwoSiblings(t *testing.T) {
	s := NewRegionStore()
	a := &Region{Start: 0, End: 10, Kind: "code"}
	b := &Region{Start: 20, End: 30, Kind: "code"}
	s.Add(a)
	s.Add(b)

	if len(s.Roots()) != 2 {
		t.Fatalf("got %d roots, want 2", len(s.Roots()))
	}
}

// TestRegionStore_AddDeeplyNested: three-level nesting (round
// 6+ scenario synthesized for round-5 store coverage).
func TestRegionStore_AddDeeplyNested(t *testing.T) {
	s := NewRegionStore()
	outer := &Region{Start: 0, End: 100, Kind: "blockquote"}
	middle := &Region{Start: 10, End: 80, Kind: "listitem"}
	inner := &Region{Start: 20, End: 50, Kind: "code"}
	s.Add(outer)
	s.Add(middle)
	s.Add(inner)

	if len(s.Roots()) != 1 {
		t.Fatalf("got %d roots, want 1", len(s.Roots()))
	}
	if outer.Children[0] != middle {
		t.Error("outer's child should be middle")
	}
	if middle.Children[0] != inner {
		t.Error("middle's child should be inner")
	}
	if inner.Parent != middle {
		t.Error("inner.Parent should be middle")
	}
	if middle.Parent != outer {
		t.Error("middle.Parent should be outer")
	}
	if outer.Parent != nil {
		t.Error("outer.Parent should be nil")
	}
}

// =========================================================================
// RegionStore — EnclosingAt
// =========================================================================

func TestRegionStore_EnclosingAtTopLevel(t *testing.T) {
	s := NewRegionStore()
	r := &Region{Start: 5, End: 10, Kind: "code"}
	s.Add(r)

	if got := s.EnclosingAt(7); got != r {
		t.Errorf("EnclosingAt(7) = %v, want %v", got, r)
	}
	if got := s.EnclosingAt(5); got != r {
		t.Errorf("EnclosingAt(5) (start) = %v, want %v", got, r)
	}
}

func TestRegionStore_EnclosingAtBoundaries(t *testing.T) {
	s := NewRegionStore()
	r := &Region{Start: 5, End: 10, Kind: "code"}
	s.Add(r)

	// [Start, End) is half-open: End is NOT inside.
	if got := s.EnclosingAt(10); got != nil {
		t.Errorf("EnclosingAt(10) (exact End) = %v, want nil", got)
	}
	if got := s.EnclosingAt(4); got != nil {
		t.Errorf("EnclosingAt(4) (before Start) = %v, want nil", got)
	}
	if got := s.EnclosingAt(11); got != nil {
		t.Errorf("EnclosingAt(11) (after End) = %v, want nil", got)
	}
}

func TestRegionStore_EnclosingAtFindsDeepest(t *testing.T) {
	s := NewRegionStore()
	outer := &Region{Start: 0, End: 100, Kind: "blockquote"}
	inner := &Region{Start: 20, End: 50, Kind: "code"}
	s.Add(outer)
	s.Add(inner)

	// Inside both → return inner (deepest).
	if got := s.EnclosingAt(30); got != inner {
		t.Errorf("EnclosingAt(30) = %v, want inner", got)
	}
	// Inside outer only.
	if got := s.EnclosingAt(60); got != outer {
		t.Errorf("EnclosingAt(60) = %v, want outer", got)
	}
}

func TestRegionStore_EnclosingAtMultipleSiblings(t *testing.T) {
	s := NewRegionStore()
	a := &Region{Start: 0, End: 10, Kind: "code"}
	b := &Region{Start: 20, End: 30, Kind: "code"}
	s.Add(a)
	s.Add(b)

	if got := s.EnclosingAt(5); got != a {
		t.Errorf("EnclosingAt(5) = %v, want a", got)
	}
	if got := s.EnclosingAt(25); got != b {
		t.Errorf("EnclosingAt(25) = %v, want b", got)
	}
	if got := s.EnclosingAt(15); got != nil {
		t.Errorf("EnclosingAt(15) (in gap) = %v, want nil", got)
	}
}

// =========================================================================
// RegionStore — Clear
// =========================================================================

func TestRegionStore_Clear(t *testing.T) {
	s := NewRegionStore()
	s.Add(&Region{Start: 0, End: 10, Kind: "code"})
	s.Add(&Region{Start: 20, End: 30, Kind: "code"})
	s.Clear()

	if got := s.Roots(); len(got) != 0 {
		t.Errorf("after Clear, got %d roots, want 0", len(got))
	}
	if got := s.EnclosingAt(5); got != nil {
		t.Errorf("after Clear, EnclosingAt(5) = %v, want nil", got)
	}
}

// =========================================================================
// RegionStore — BoundariesIn (Phase 3 round 6)
// =========================================================================

func TestRegionStore_BoundariesIn_Empty(t *testing.T) {
	s := NewRegionStore()
	if got := s.BoundariesIn(0, 100); len(got) != 0 {
		t.Errorf("empty store: BoundariesIn = %v, want empty", got)
	}
}

func TestRegionStore_BoundariesIn_SingleRegion(t *testing.T) {
	s := NewRegionStore()
	s.Add(&Region{Start: 5, End: 15, Kind: "code"})

	// Range that contains the whole region: both boundaries.
	if got := s.BoundariesIn(0, 20); !intsEqual(got, []int{5, 15}) {
		t.Errorf("contains region: got %v, want [5, 15]", got)
	}
	// Range that contains only the Start boundary.
	if got := s.BoundariesIn(0, 10); !intsEqual(got, []int{5}) {
		t.Errorf("Start only: got %v, want [5]", got)
	}
	// Range that contains only the End boundary.
	if got := s.BoundariesIn(10, 20); !intsEqual(got, []int{15}) {
		t.Errorf("End only: got %v, want [15]", got)
	}
	// Range that misses both: none.
	if got := s.BoundariesIn(20, 30); len(got) != 0 {
		t.Errorf("disjoint: got %v, want empty", got)
	}
}

func TestRegionStore_BoundariesIn_Nested(t *testing.T) {
	s := NewRegionStore()
	s.Add(&Region{Start: 0, End: 100, Kind: "blockquote"})
	s.Add(&Region{Start: 20, End: 50, Kind: "code"})

	// Range that spans both: 4 boundaries (outer Start at 0
	// excluded — exclusive; outer End at 100 excluded too).
	got := s.BoundariesIn(0, 100)
	want := []int{20, 50}
	if !intsEqual(got, want) {
		t.Errorf("nested: got %v, want %v", got, want)
	}
	// Wider range catches the outer's boundaries too.
	got = s.BoundariesIn(-5, 105)
	want = []int{0, 20, 50, 100}
	if !intsEqual(got, want) {
		t.Errorf("wider: got %v, want %v", got, want)
	}
}

func TestRegionStore_BoundariesIn_DedupesSharedOffsets(t *testing.T) {
	s := NewRegionStore()
	// Two regions that share a boundary at 10 (one ends, other begins).
	s.Add(&Region{Start: 0, End: 10, Kind: "code"})
	s.Add(&Region{Start: 10, End: 20, Kind: "code"})
	got := s.BoundariesIn(-1, 21)
	want := []int{0, 10, 20}
	if !intsEqual(got, want) {
		t.Errorf("shared boundary: got %v, want %v", got, want)
	}
}

func intsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// =========================================================================
// RegionStore — Insert (body edit shifts offsets)
// =========================================================================

// TestRegionStore_InsertAtStart: a region [10, 20). Inserting
// 5 runes at body position 0 shifts the region to [15, 25).
func TestRegionStore_InsertAtStart(t *testing.T) {
	s := NewRegionStore()
	r := &Region{Start: 10, End: 20, Kind: "code"}
	s.Add(r)
	s.Insert(0, 5)
	if r.Start != 15 || r.End != 25 {
		t.Errorf("after Insert(0, 5): [%d, %d), want [15, 25)", r.Start, r.End)
	}
}

// TestRegionStore_InsertBeforeRegion: insert at the boundary
// (pos == Start) is treated as before — region's Start
// shifts. Mirrors the spanStore convention: text inserted
// AT the start of a styled run is taken as part of that run
// would be, but for regions the simpler convention is
// "inserts at or before Start shift Start". Tested
// explicitly to pin the contract.
func TestRegionStore_InsertBeforeRegion(t *testing.T) {
	s := NewRegionStore()
	r := &Region{Start: 10, End: 20, Kind: "code"}
	s.Add(r)
	s.Insert(10, 3)
	if r.Start != 13 || r.End != 23 {
		t.Errorf("after Insert(10, 3) at Start: [%d, %d), want [13, 23)", r.Start, r.End)
	}
}

// TestRegionStore_InsertInsideRegion: insert at a body
// position within the region's range grows the region's End.
func TestRegionStore_InsertInsideRegion(t *testing.T) {
	s := NewRegionStore()
	r := &Region{Start: 10, End: 20, Kind: "code"}
	s.Add(r)
	s.Insert(15, 4)
	if r.Start != 10 || r.End != 24 {
		t.Errorf("after Insert(15, 4) inside: [%d, %d), want [10, 24)", r.Start, r.End)
	}
}

// TestRegionStore_InsertAtRegionEnd: pin the boundary
// behavior — insert at exactly pos == r.End is treated as
// "after the region": the region is untouched (mirrors the
// half-open contract [Start, End)).
func TestRegionStore_InsertAtRegionEnd(t *testing.T) {
	s := NewRegionStore()
	r := &Region{Start: 10, End: 20, Kind: "code"}
	s.Add(r)
	s.Insert(20, 5)
	if r.Start != 10 || r.End != 20 {
		t.Errorf("after Insert(20, 5) at End: [%d, %d), want [10, 20) (untouched)", r.Start, r.End)
	}
}

// TestRegionStore_InsertAfterRegion: insert after End leaves
// the region untouched.
func TestRegionStore_InsertAfterRegion(t *testing.T) {
	s := NewRegionStore()
	r := &Region{Start: 10, End: 20, Kind: "code"}
	s.Add(r)
	s.Insert(25, 5)
	if r.Start != 10 || r.End != 20 {
		t.Errorf("after Insert(25, 5) after End: [%d, %d), want [10, 20)", r.Start, r.End)
	}
}

// TestRegionStore_InsertChildShiftsWithParent: nested
// regions stay nested when their containing range shifts.
func TestRegionStore_InsertChildShiftsWithParent(t *testing.T) {
	s := NewRegionStore()
	parent := &Region{Start: 0, End: 100, Kind: "blockquote"}
	child := &Region{Start: 20, End: 50, Kind: "code"}
	s.Add(parent)
	s.Add(child)

	s.Insert(0, 5)
	if parent.Start != 5 || parent.End != 105 {
		t.Errorf("parent: [%d, %d), want [5, 105)", parent.Start, parent.End)
	}
	if child.Start != 25 || child.End != 55 {
		t.Errorf("child: [%d, %d), want [25, 55)", child.Start, child.End)
	}
	if child.Parent != parent {
		t.Error("child's Parent pointer should still be parent")
	}
}

// =========================================================================
// RegionStore — Delete (body edit shifts/clips/drops regions)
// =========================================================================

// TestRegionStore_DeleteAfterRegion: delete after End leaves
// the region untouched.
func TestRegionStore_DeleteAfterRegion(t *testing.T) {
	s := NewRegionStore()
	r := &Region{Start: 10, End: 20, Kind: "code"}
	s.Add(r)
	s.Delete(25, 5)
	if r.Start != 10 || r.End != 20 {
		t.Errorf("after Delete(25, 5) after End: [%d, %d), want [10, 20)", r.Start, r.End)
	}
	if len(s.Roots()) != 1 {
		t.Errorf("region should still be in store; got %d roots", len(s.Roots()))
	}
}

// TestRegionStore_DeleteBeforeRegion: delete strictly before
// the region shifts both Start and End down.
func TestRegionStore_DeleteBeforeRegion(t *testing.T) {
	s := NewRegionStore()
	r := &Region{Start: 10, End: 20, Kind: "code"}
	s.Add(r)
	s.Delete(0, 5)
	if r.Start != 5 || r.End != 15 {
		t.Errorf("after Delete(0, 5) before: [%d, %d), want [5, 15)", r.Start, r.End)
	}
}

// TestRegionStore_DeleteIntersectsBodyDropsRegion: v1
// conservative behavior — a delete that touches the
// region's body removes the region entirely. The next
// render rebuilds it from md2spans.
func TestRegionStore_DeleteIntersectsBodyDropsRegion(t *testing.T) {
	cases := []struct {
		name     string
		delPos   int
		delLen   int
	}{
		{"deleted middle", 12, 3},
		{"delete starts before, ends inside", 8, 5},
		{"delete starts inside, ends after", 15, 10},
		{"delete fully covers", 0, 30},
		{"delete starts at Start", 10, 3},
		{"delete ends at End-1", 17, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewRegionStore()
			r := &Region{Start: 10, End: 20, Kind: "code"}
			s.Add(r)
			s.Delete(tc.delPos, tc.delLen)
			if got := s.EnclosingAt(15); got != nil {
				t.Errorf("region should be dropped; EnclosingAt(15) = %v", got)
			}
		})
	}
}

// TestRegionStore_DeleteDropsChildKeepsParent: when a
// delete intersects a child but not the parent, the child
// is dropped and the parent shifts.
func TestRegionStore_DeleteDropsChildKeepsParent(t *testing.T) {
	s := NewRegionStore()
	parent := &Region{Start: 0, End: 100, Kind: "blockquote"}
	child := &Region{Start: 20, End: 50, Kind: "code"}
	s.Add(parent)
	s.Add(child)

	// Delete inside child.
	s.Delete(30, 5)

	// Parent shrinks (5 runes deleted from inside).
	if parent.End != 95 {
		t.Errorf("parent.End = %d, want 95 (shrunk)", parent.End)
	}
	// Child is dropped.
	if len(parent.Children) != 0 {
		t.Errorf("parent should have 0 children after delete dropped child; got %d", len(parent.Children))
	}
	// EnclosingAt at the former-child range now returns parent.
	if got := s.EnclosingAt(35); got != parent {
		t.Errorf("EnclosingAt(35) = %v, want parent (child was dropped)", got)
	}
}

// TestRegionStore_AddPanicsOnPartialOverlap pins the
// producer-bug guardrail added in Phase 3 round 6. Partial
// overlap (two regions that overlap without one containing
// the other) is illegal; Add panics rather than silently
// corrupting the forest.
func TestRegionStore_AddPanicsOnPartialOverlap(t *testing.T) {
	cases := []struct {
		name     string
		first    *Region
		second   *Region
	}{
		{
			"second-overlaps-end-of-first",
			&Region{Start: 0, End: 20, Kind: "code"},
			&Region{Start: 10, End: 30, Kind: "blockquote"},
		},
		{
			"second-overlaps-start-of-first",
			&Region{Start: 10, End: 30, Kind: "code"},
			&Region{Start: 0, End: 20, Kind: "blockquote"},
		},
		{
			"second-partially-overlaps-nested-child",
			&Region{Start: 0, End: 100, Kind: "blockquote"},
			// Add a child first, then a sibling that
			// partially overlaps the child.
			nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := NewRegionStore()
			s.Add(c.first)
			if c.second == nil {
				// Special case: nested partial overlap.
				child := &Region{Start: 10, End: 50, Kind: "code"}
				s.Add(child)
				bad := &Region{Start: 30, End: 70, Kind: "code"}
				defer func() {
					if r := recover(); r == nil {
						t.Fatalf("Add did not panic on nested partial overlap")
					}
				}()
				s.Add(bad)
				return
			}
			defer func() {
				if r := recover(); r == nil {
					t.Fatalf("Add did not panic on partial overlap")
				}
			}()
			s.Add(c.second)
		})
	}
}
