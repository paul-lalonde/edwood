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
