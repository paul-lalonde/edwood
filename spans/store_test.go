package spans

import (
	"testing"

	"github.com/rjkroege/edwood/frame"
)

// colored is a non-plain Style used as a test fixture. Kind is the
// only authoritative discriminator for IsPlain in Slice A; Fg is
// left nil to keep the test independent of draw.Image construction.
var colored = frame.Style{Kind: frame.KindColored}

func TestStore_EmptyByDefault(t *testing.T) {
	s := newStoreWithLen(0)
	if !s.Empty() {
		t.Errorf("fresh store: Empty() = false, want true")
	}
	if got := s.Snapshot(); len(got) != 0 {
		t.Errorf("fresh store: Snapshot() = %v, want empty", got)
	}
	if got := s.GetStyleRuns(0, 0); len(got) != 0 {
		t.Errorf("GetStyleRuns(0,0) on empty store = %v, want empty", got)
	}
}

func TestStore_PlainCoverageIsEmpty(t *testing.T) {
	// A store seeded with a plain run covering the buffer is
	// still Empty() — Empty reports whether any non-plain region
	// exists, not whether any region exists.
	s := newStoreWithLen(10)
	if !s.Empty() {
		t.Errorf("plain-covered store: Empty() = false, want true")
	}
}

func TestStore_SetRegionMakesNonEmpty(t *testing.T) {
	s := newStoreWithLen(10)
	s.SetRegion(2, 4, colored)
	if s.Empty() {
		t.Errorf("after SetRegion(colored): Empty() = true, want false")
	}
}

func TestStore_ClearRegionRestoresEmpty(t *testing.T) {
	s := newStoreWithLen(10)
	s.SetRegion(0, 10, colored)
	s.ClearRegion(0, 10)
	if !s.Empty() {
		t.Errorf("after ClearRegion(full): Empty() = false, want true")
	}
}

func TestStore_SetRegionPlainIsClear(t *testing.T) {
	// SetRegion with a plain Style is equivalent to ClearRegion.
	s := newStoreWithLen(10)
	s.SetRegion(2, 6, colored)
	s.SetRegion(2, 6, frame.Style{}) // plain
	if !s.Empty() {
		t.Errorf("SetRegion(plain) over colored: Empty() = false, want true")
	}
}

func TestStore_GetStyleRuns_SingleRegion(t *testing.T) {
	s := newStoreWithLen(10)
	s.SetRegion(2, 4, colored)
	got := s.GetStyleRuns(0, 10)

	// Expected: plain[0,2), colored[2,4), plain[4,10).
	want := []frame.StyleRun{
		{Len: 2, Style: frame.Style{}},
		{Len: 2, Style: colored},
		{Len: 6, Style: frame.Style{}},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d runs, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("run[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestStore_GetStyleRuns_MultipleRegions(t *testing.T) {
	s := newStoreWithLen(20)
	s.SetRegion(2, 5, colored)
	s.SetRegion(10, 15, colored)
	got := s.GetStyleRuns(0, 20)

	// Sum-of-Lens invariant.
	sum := 0
	for _, r := range got {
		sum += r.Len
	}
	if sum != 20 {
		t.Errorf("sum of run.Len = %d, want 20: %+v", sum, got)
	}

	// At least two non-plain runs present.
	nonPlain := 0
	for _, r := range got {
		if !r.Style.IsPlain() {
			nonPlain++
		}
	}
	if nonPlain != 2 {
		t.Errorf("got %d non-plain runs, want 2: %+v", nonPlain, got)
	}
}

func TestStore_GetStyleRuns_FullCoverageInvariant(t *testing.T) {
	s := newStoreWithLen(50)
	s.SetRegion(5, 15, colored)
	s.SetRegion(25, 30, colored)

	cases := []struct{ p0, p1 int }{
		{0, 50},  // entire buffer
		{0, 10},  // covers part of first region
		{10, 20}, // crosses into a gap
		{5, 30},  // covers first region exactly + into second
		{7, 12},  // entirely within first colored region
		{20, 25}, // entirely within a plain gap
	}
	for _, c := range cases {
		got := s.GetStyleRuns(c.p0, c.p1)
		want := c.p1 - c.p0
		sum := 0
		for _, r := range got {
			if r.Len == 0 {
				t.Errorf("GetStyleRuns(%d,%d) emitted zero-Len run: %+v", c.p0, c.p1, got)
			}
			sum += r.Len
		}
		if sum != want {
			t.Errorf("GetStyleRuns(%d,%d): sum=%d, want %d: %+v", c.p0, c.p1, sum, want, got)
		}
	}
}

func TestStore_GetStyleRuns_EmptyQueryReturnsEmpty(t *testing.T) {
	s := newStoreWithLen(10)
	s.SetRegion(0, 10, colored)
	if got := s.GetStyleRuns(5, 5); len(got) != 0 {
		t.Errorf("GetStyleRuns(5,5) = %+v, want empty", got)
	}
}

func TestStore_SetRegion_OverlapWins(t *testing.T) {
	// Two SetRegions over overlapping ranges: second wins.
	s := newStoreWithLen(10)
	a := frame.Style{Kind: frame.KindColored}
	// Use a different style to detect which wins. We can't make
	// two distinct non-plain styles with just KindColored and
	// nil Fg/Bg, so introduce a Kind that's different.
	b := frame.Style{Kind: frame.KindColored | 1<<4}

	s.SetRegion(0, 6, a)
	s.SetRegion(3, 9, b)

	got := s.GetStyleRuns(0, 10)
	// Expected: a[0,3), b[3,9), plain[9,10).
	want := []frame.StyleRun{
		{Len: 3, Style: a},
		{Len: 6, Style: b},
		{Len: 1, Style: frame.Style{}},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d runs, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("run[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestStore_ClearRegion_PartialSplitsRun(t *testing.T) {
	// A colored region [0, 10) with a clear in the middle [4, 6)
	// should produce three runs: colored[0,4), plain[4,6),
	// colored[6,10).
	s := newStoreWithLen(10)
	s.SetRegion(0, 10, colored)
	s.ClearRegion(4, 6)

	got := s.GetStyleRuns(0, 10)
	want := []frame.StyleRun{
		{Len: 4, Style: colored},
		{Len: 2, Style: frame.Style{}},
		{Len: 4, Style: colored},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d runs, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("run[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestStore_Snapshot_SortedFullCoverage(t *testing.T) {
	s := newStoreWithLen(10)
	s.SetRegion(2, 5, colored)
	got := s.Snapshot()

	// Snapshot returns the full coverage; Starts must be
	// monotonically increasing and contiguous.
	if len(got) == 0 {
		t.Fatalf("Snapshot() empty, want non-empty for populated store")
	}
	prev := -1
	for i, r := range got {
		if r.Start <= prev {
			t.Errorf("region[%d].Start = %d, not strictly increasing (prev=%d)", i, r.Start, prev)
		}
		prev = r.Start
		if r.Length <= 0 {
			t.Errorf("region[%d].Length = %d, want > 0", i, r.Length)
		}
	}
	// Total length matches the buffer.
	sum := 0
	for _, r := range got {
		sum += r.Length
	}
	if sum != 10 {
		t.Errorf("Snapshot total length = %d, want 10", sum)
	}
}

func TestStore_EmptyQueryWithoutContent(t *testing.T) {
	s := newStoreWithLen(0)
	if got := s.GetStyleRuns(0, 0); len(got) != 0 {
		t.Errorf("GetStyleRuns(0,0) on len-0 store = %+v, want empty", got)
	}
}
