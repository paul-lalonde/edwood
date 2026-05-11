package spans

import (
	"testing"

	"github.com/rjkroege/edwood/file"
	"github.com/rjkroege/edwood/frame"
)

// runeOff makes an OffsetTuple for tests that don't care about
// byte offsets.
func runeOff(r int) file.OffsetTuple {
	return file.Ot(0, r)
}

// =====================================================================
// Inserted
// =====================================================================

func TestObserver_Inserted_EmptyStore(t *testing.T) {
	// Insert into a brand-new store with no buffer; observer
	// seeds a plain region.
	s := &store{}
	s.Inserted(runeOff(0), nil, 5)
	if s.totalLen != 5 {
		t.Errorf("totalLen = %d, want 5", s.totalLen)
	}
	if len(s.regions) != 1 || s.regions[0] != (Region{Start: 0, Length: 5, Style: frame.Style{}}) {
		t.Errorf("regions = %+v, want [{0 5 plain}]", s.regions)
	}
}

func TestObserver_Inserted_AtEndOfBuffer(t *testing.T) {
	// Trailing-edge of last region: extends it.
	s := newStoreWithLen(5)
	s.Inserted(runeOff(5), nil, 3)
	if s.totalLen != 8 {
		t.Errorf("totalLen = %d, want 8", s.totalLen)
	}
	if len(s.regions) != 1 || s.regions[0].Length != 8 {
		t.Errorf("regions = %+v, want one region length 8", s.regions)
	}
}

func TestObserver_Inserted_MidRegion(t *testing.T) {
	// q0 strictly inside a non-plain region: extend.
	s := newStoreWithLen(10)
	s.SetRegion(2, 7, colored)
	// Snapshot: plain[0,2), colored[2,7), plain[7,10).
	s.Inserted(runeOff(4), nil, 3)
	// Expect colored region to grow by 3.
	if s.totalLen != 13 {
		t.Errorf("totalLen = %d, want 13", s.totalLen)
	}
	got := s.Snapshot()
	// Expected: plain[0,2), colored[2,10), plain[10,13).
	wantStarts := []int{0, 2, 10}
	wantLens := []int{2, 8, 3}
	if len(got) != 3 {
		t.Fatalf("got %d regions, want 3: %+v", len(got), got)
	}
	for i := range got {
		if got[i].Start != wantStarts[i] || got[i].Length != wantLens[i] {
			t.Errorf("region[%d] = {Start:%d, Length:%d}, want {Start:%d, Length:%d}",
				i, got[i].Start, got[i].Length, wantStarts[i], wantLens[i])
		}
	}
	if got[1].Style != colored {
		t.Errorf("middle region Style = %+v, want colored", got[1].Style)
	}
}

func TestObserver_Inserted_LeadingEdge_WithPrevious(t *testing.T) {
	// q0 == regions[1].Start: previous (regions[0], plain) extends;
	// regions[1] (colored) shifts.
	s := newStoreWithLen(10)
	s.SetRegion(4, 8, colored)
	// Snapshot: plain[0,4), colored[4,8), plain[8,10).
	s.Inserted(runeOff(4), nil, 3)
	got := s.Snapshot()
	// Expected: plain[0,7), colored[7,11), plain[11,13).
	wantStarts := []int{0, 7, 11}
	wantLens := []int{7, 4, 2}
	if len(got) != 3 {
		t.Fatalf("got %d regions, want 3: %+v", len(got), got)
	}
	for i := range got {
		if got[i].Start != wantStarts[i] || got[i].Length != wantLens[i] {
			t.Errorf("region[%d] = {Start:%d, Length:%d}, want {Start:%d, Length:%d}",
				i, got[i].Start, got[i].Length, wantStarts[i], wantLens[i])
		}
	}
	if got[1].Style != colored {
		t.Errorf("colored region Style = %+v, want colored", got[1].Style)
	}
}

func TestObserver_Inserted_LeadingEdge_NoPrevious_PlainRegion(t *testing.T) {
	// q0 == 0 on a plain-headed buffer: extend region 0.
	s := newStoreWithLen(10)
	s.Inserted(runeOff(0), nil, 3)
	if s.totalLen != 13 {
		t.Errorf("totalLen = %d, want 13", s.totalLen)
	}
	if len(s.regions) != 1 || s.regions[0].Length != 13 {
		t.Errorf("regions = %+v, want single plain length 13", s.regions)
	}
}

func TestObserver_Inserted_LeadingEdge_NoPrevious_NonPlainRegion(t *testing.T) {
	// q0 == 0 with region 0 non-plain: prepend plain.
	s := newStoreWithLen(5)
	s.SetRegion(0, 5, colored)
	s.Inserted(runeOff(0), nil, 3)
	// Expected: plain[0,3), colored[3,8).
	got := s.Snapshot()
	if len(got) != 2 {
		t.Fatalf("got %d regions, want 2: %+v", len(got), got)
	}
	if got[0].Start != 0 || got[0].Length != 3 || !got[0].Style.IsPlain() {
		t.Errorf("region[0] = %+v, want plain {0,3}", got[0])
	}
	if got[1].Start != 3 || got[1].Length != 5 || got[1].Style != colored {
		t.Errorf("region[1] = %+v, want colored {3,5}", got[1])
	}
}

// =====================================================================
// Deleted
// =====================================================================

func TestObserver_Deleted_EntirelyContained(t *testing.T) {
	// Colored region [3,7) entirely inside the deletion [2,8).
	s := newStoreWithLen(10)
	s.SetRegion(3, 7, colored)
	s.Deleted(runeOff(2), runeOff(8))
	if s.totalLen != 4 {
		t.Errorf("totalLen = %d, want 4", s.totalLen)
	}
	if !s.Empty() {
		t.Errorf("after deleting the colored region: Empty() = false, want true")
	}
}

func TestObserver_Deleted_StraddlesLeftEdge(t *testing.T) {
	// Colored region [3,8). Deletion [5,7). Expect colored [3,5)
	// after shifting (re-delLen = 8-2 = 6, but only the right
	// edge of the region is clipped; the left half [3,5) stays).
	s := newStoreWithLen(10)
	s.SetRegion(3, 8, colored)
	s.Deleted(runeOff(5), runeOff(7))
	// New totalLen = 8. Regions: plain[0,3), colored[3,5),
	// plain[5,8) (the trailing plain shrunk by 2 after the shift).
	got := s.Snapshot()
	if s.totalLen != 8 {
		t.Errorf("totalLen = %d, want 8", s.totalLen)
	}
	// Find the colored region.
	var col Region
	for _, r := range got {
		if r.Style == colored {
			col = r
		}
	}
	if col.Length != 3 {
		t.Errorf("colored region Length = %d, want 3 (was [3,8), [5,7) deleted, region [3,5) remains... wait, [3,8) with [5,7) deleted → [3,5)+[7,8) → [3,5)+[5,6) after shift → contiguous [3,6) length 3)", col.Length)
	}
}

func TestObserver_Deleted_AfterRegionShifts(t *testing.T) {
	// Colored at [2,4). Delete [6,8). Expect colored still [2,4),
	// trailing plain shrunk by 2.
	s := newStoreWithLen(10)
	s.SetRegion(2, 4, colored)
	s.Deleted(runeOff(6), runeOff(8))
	if s.totalLen != 8 {
		t.Errorf("totalLen = %d, want 8", s.totalLen)
	}
	got := s.Snapshot()
	// colored region untouched at [2,4).
	var col Region
	for _, r := range got {
		if r.Style == colored {
			col = r
		}
	}
	if col.Start != 2 || col.Length != 2 {
		t.Errorf("colored region = {Start:%d, Length:%d}, want {2,2}", col.Start, col.Length)
	}
}

func TestObserver_Deleted_BeforeRegionShifts(t *testing.T) {
	// Colored at [6,8). Delete [2,4). Expect colored at [4,6).
	s := newStoreWithLen(10)
	s.SetRegion(6, 8, colored)
	s.Deleted(runeOff(2), runeOff(4))
	if s.totalLen != 8 {
		t.Errorf("totalLen = %d, want 8", s.totalLen)
	}
	got := s.Snapshot()
	var col Region
	for _, r := range got {
		if r.Style == colored {
			col = r
		}
	}
	if col.Start != 4 || col.Length != 2 {
		t.Errorf("colored region = {Start:%d, Length:%d}, want {4,2}", col.Start, col.Length)
	}
}

func TestObserver_Deleted_WrapsRegion(t *testing.T) {
	// Colored at [4,8). Delete [5,7). Region shrinks by 2.
	s := newStoreWithLen(10)
	s.SetRegion(4, 8, colored)
	s.Deleted(runeOff(5), runeOff(7))
	if s.totalLen != 8 {
		t.Errorf("totalLen = %d, want 8", s.totalLen)
	}
	got := s.Snapshot()
	var col Region
	for _, r := range got {
		if r.Style == colored {
			col = r
		}
	}
	if col.Start != 4 || col.Length != 2 {
		t.Errorf("colored region = {Start:%d, Length:%d}, want {4,2}", col.Start, col.Length)
	}
}

// =====================================================================
// Integration: NewStore wires up the observer on the real buffer
// =====================================================================

func TestObserver_Integration_BufferDrivesStore(t *testing.T) {
	buf := file.NewObservableEditableBuffer()
	s := NewStore(buf).(*store)

	// Buffer starts empty.
	if s.totalLen != 0 {
		t.Errorf("fresh store totalLen = %d, want 0", s.totalLen)
	}

	// Insert "hello" at offset 0. The buffer observer propagates
	// to the store.
	buf.InsertAt(0, []rune("hello"))
	if s.totalLen != 5 {
		t.Errorf("after Insert: totalLen = %d, want 5", s.totalLen)
	}

	// Style runes [1,4) — "ell".
	s.SetRegion(1, 4, colored)

	// Insert " " at rune offset 0. Per leading-edge rule, region 0
	// (plain) extends and the colored region shifts.
	buf.InsertAt(0, []rune(" "))
	if s.totalLen != 6 {
		t.Errorf("after second Insert: totalLen = %d, want 6", s.totalLen)
	}
	got := s.Snapshot()
	var col Region
	for _, r := range got {
		if r.Style == colored {
			col = r
		}
	}
	if col.Start != 2 || col.Length != 3 {
		t.Errorf("colored after Insert at 0: %+v, want {2,3}", col)
	}

	// Delete the first 2 runes — should remove the leading plain
	// and clip the start of the colored region.
	buf.DeleteAt(0, 2)
	if s.totalLen != 4 {
		t.Errorf("after Delete: totalLen = %d, want 4", s.totalLen)
	}
}
