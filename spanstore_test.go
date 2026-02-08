package main

import (
	"image/color"
	"testing"
)

// Test styles used throughout. Letters A–D are distinct non-default styles.
var (
	styleDefault = StyleAttrs{}
	styleA       = StyleAttrs{Fg: color.RGBA{R: 255, A: 255}, Bold: true}
	styleB       = StyleAttrs{Bg: color.RGBA{G: 255, A: 255}, Italic: true}
	styleC       = StyleAttrs{Fg: color.RGBA{B: 255, A: 255}}
	styleD       = StyleAttrs{Fg: color.RGBA{R: 128, G: 128, A: 255}, Bold: true, Italic: true}
)

// --- helpers ---

// buildStore creates a SpanStore pre-loaded with the given runs.
// It uses RegionUpdate on an appropriately-sized default store so that
// the gap buffer internals are exercised from the start.
func buildStore(runs []StyleRun) *SpanStore {
	s := NewSpanStore()
	total := 0
	for _, r := range runs {
		total += r.Len
	}
	if total == 0 {
		return s
	}
	// First insert a default run covering the total length.
	s.Insert(0, total)
	// Then region-update to set the desired runs.
	if len(runs) == 1 && runs[0].Style.Equal(styleDefault) {
		return s // already correct
	}
	s.RegionUpdate(0, runs)
	return s
}

// expectRuns asserts that s.Runs() matches expected in order.
func expectRuns(t *testing.T, label string, s *SpanStore, expected []StyleRun) {
	t.Helper()
	got := s.Runs()
	if len(got) != len(expected) {
		t.Errorf("%s: got %d runs, want %d\n  got:  %+v\n  want: %+v", label, len(got), len(expected), got, expected)
		return
	}
	for i := range expected {
		if got[i].Len != expected[i].Len || !got[i].Style.Equal(expected[i].Style) {
			t.Errorf("%s: run[%d] got {Len:%d, Style:%+v}, want {Len:%d, Style:%+v}",
				label, i, got[i].Len, got[i].Style, expected[i].Len, expected[i].Style)
		}
	}
}

// expectTotalLen asserts TotalLen.
func expectTotalLen(t *testing.T, label string, s *SpanStore, want int) {
	t.Helper()
	if got := s.TotalLen(); got != want {
		t.Errorf("%s: TotalLen = %d, want %d", label, got, want)
	}
}

// expectNumRuns asserts NumRuns.
func expectNumRuns(t *testing.T, label string, s *SpanStore, want int) {
	t.Helper()
	if got := s.NumRuns(); got != want {
		t.Errorf("%s: NumRuns = %d, want %d", label, got, want)
	}
}

// =========================================================================
// Empty Store (tests 1–4)
// =========================================================================

func TestSpanStore_EmptyStore(t *testing.T) {
	s := NewSpanStore()

	// Test 1: NewSpanStore has TotalLen 0, NumRuns 0
	expectTotalLen(t, "#1", s, 0)
	expectNumRuns(t, "#1", s, 0)

	// Test 2: ForEachRun on empty store — fn never called
	called := false
	s.ForEachRun(func(r StyleRun) { called = true })
	if called {
		t.Error("#2: ForEachRun should not call fn on empty store")
	}

	// Test 3: Runs on empty store returns empty slice
	runs := s.Runs()
	if len(runs) != 0 {
		t.Errorf("#3: Runs() = %v, want empty", runs)
	}

	// Test 4: Clear on empty store — no panic
	s.Clear()
	expectTotalLen(t, "#4", s, 0)
	expectNumRuns(t, "#4", s, 0)
}

// =========================================================================
// Insert (tests 5–11)
// =========================================================================

func TestSpanStore_InsertIntoEmpty(t *testing.T) {
	// Test 5: Insert into empty → [{5,default}], TotalLen=5
	s := NewSpanStore()
	s.Insert(0, 5)
	expectRuns(t, "#5", s, []StyleRun{{Len: 5, Style: styleDefault}})
	expectTotalLen(t, "#5", s, 5)
}

func TestSpanStore_InsertAtStart(t *testing.T) {
	// Test 6: [{5,A}] → Insert(0,3) → [{8,A}], TotalLen=8
	s := buildStore([]StyleRun{{Len: 5, Style: styleA}})
	s.Insert(0, 3)
	expectRuns(t, "#6", s, []StyleRun{{Len: 8, Style: styleA}})
	expectTotalLen(t, "#6", s, 8)
}

func TestSpanStore_InsertAtEnd(t *testing.T) {
	// Test 7: [{5,A}] → Insert(5,3) → [{8,A}], TotalLen=8
	s := buildStore([]StyleRun{{Len: 5, Style: styleA}})
	s.Insert(5, 3)
	expectRuns(t, "#7", s, []StyleRun{{Len: 8, Style: styleA}})
	expectTotalLen(t, "#7", s, 8)
}

func TestSpanStore_InsertMidRun(t *testing.T) {
	// Test 8: [{10,A}] → Insert(5,3) → [{13,A}], TotalLen=13
	s := buildStore([]StyleRun{{Len: 10, Style: styleA}})
	s.Insert(5, 3)
	expectRuns(t, "#8", s, []StyleRun{{Len: 13, Style: styleA}})
	expectTotalLen(t, "#8", s, 13)
}

func TestSpanStore_InsertAtRunBoundary(t *testing.T) {
	// Test 9: [{5,A},{5,B}] → Insert(5,3) → [{8,A},{5,B}], TotalLen=13
	// Insert at boundary extends the preceding run.
	s := buildStore([]StyleRun{{Len: 5, Style: styleA}, {Len: 5, Style: styleB}})
	s.Insert(5, 3)
	expectRuns(t, "#9", s, []StyleRun{{Len: 8, Style: styleA}, {Len: 5, Style: styleB}})
	expectTotalLen(t, "#9", s, 13)
}

func TestSpanStore_InsertAtStartMultiRun(t *testing.T) {
	// Test 10: [{5,A},{5,B}] → Insert(0,3) → [{8,A},{5,B}], TotalLen=13
	s := buildStore([]StyleRun{{Len: 5, Style: styleA}, {Len: 5, Style: styleB}})
	s.Insert(0, 3)
	expectRuns(t, "#10", s, []StyleRun{{Len: 8, Style: styleA}, {Len: 5, Style: styleB}})
	expectTotalLen(t, "#10", s, 13)
}

func TestSpanStore_InsertInSecondRun(t *testing.T) {
	// Test 11: [{5,A},{5,B}] → Insert(7,2) → [{5,A},{7,B}], TotalLen=12
	s := buildStore([]StyleRun{{Len: 5, Style: styleA}, {Len: 5, Style: styleB}})
	s.Insert(7, 2)
	expectRuns(t, "#11", s, []StyleRun{{Len: 5, Style: styleA}, {Len: 7, Style: styleB}})
	expectTotalLen(t, "#11", s, 12)
}

// =========================================================================
// Delete (tests 12–21)
// =========================================================================

func TestSpanStore_DeleteWithinOneRun(t *testing.T) {
	// Test 12: [{10,A}] → Delete(3,4) → [{6,A}], TotalLen=6
	s := buildStore([]StyleRun{{Len: 10, Style: styleA}})
	s.Delete(3, 4)
	expectRuns(t, "#12", s, []StyleRun{{Len: 6, Style: styleA}})
	expectTotalLen(t, "#12", s, 6)
}

func TestSpanStore_DeleteEntireSingleRun(t *testing.T) {
	// Test 13: [{10,A}] → Delete(0,10) → [], TotalLen=0
	s := buildStore([]StyleRun{{Len: 10, Style: styleA}})
	s.Delete(0, 10)
	expectRuns(t, "#13", s, []StyleRun{})
	expectTotalLen(t, "#13", s, 0)
}

func TestSpanStore_DeleteFromStart(t *testing.T) {
	// Test 14: [{5,A},{5,B}] → Delete(0,3) → [{2,A},{5,B}], TotalLen=7
	s := buildStore([]StyleRun{{Len: 5, Style: styleA}, {Len: 5, Style: styleB}})
	s.Delete(0, 3)
	expectRuns(t, "#14", s, []StyleRun{{Len: 2, Style: styleA}, {Len: 5, Style: styleB}})
	expectTotalLen(t, "#14", s, 7)
}

func TestSpanStore_DeleteFromEnd(t *testing.T) {
	// Test 15: [{5,A},{5,B}] → Delete(7,3) → [{5,A},{2,B}], TotalLen=7
	s := buildStore([]StyleRun{{Len: 5, Style: styleA}, {Len: 5, Style: styleB}})
	s.Delete(7, 3)
	expectRuns(t, "#15", s, []StyleRun{{Len: 5, Style: styleA}, {Len: 2, Style: styleB}})
	expectTotalLen(t, "#15", s, 7)
}

func TestSpanStore_DeleteExactRun(t *testing.T) {
	// Test 16: [{5,A},{5,B},{5,C}] → Delete(5,5) → [{5,A},{5,C}], TotalLen=10
	s := buildStore([]StyleRun{
		{Len: 5, Style: styleA},
		{Len: 5, Style: styleB},
		{Len: 5, Style: styleC},
	})
	s.Delete(5, 5)
	expectRuns(t, "#16", s, []StyleRun{{Len: 5, Style: styleA}, {Len: 5, Style: styleC}})
	expectTotalLen(t, "#16", s, 10)
}

func TestSpanStore_DeleteSpanningTwoRuns(t *testing.T) {
	// Test 17: [{5,A},{5,B}] → Delete(3,4) → [{3,A},{3,B}], TotalLen=6
	s := buildStore([]StyleRun{{Len: 5, Style: styleA}, {Len: 5, Style: styleB}})
	s.Delete(3, 4)
	expectRuns(t, "#17", s, []StyleRun{{Len: 3, Style: styleA}, {Len: 3, Style: styleB}})
	expectTotalLen(t, "#17", s, 6)
}

func TestSpanStore_DeleteMiddleMerge(t *testing.T) {
	// Test 18: [{5,A},{5,B},{5,A}] → Delete(5,5) → [{10,A}], TotalLen=10
	// Deleting the middle B run merges the two A runs.
	s := buildStore([]StyleRun{
		{Len: 5, Style: styleA},
		{Len: 5, Style: styleB},
		{Len: 5, Style: styleA},
	})
	s.Delete(5, 5)
	expectRuns(t, "#18", s, []StyleRun{{Len: 10, Style: styleA}})
	expectTotalLen(t, "#18", s, 10)
}

func TestSpanStore_DeleteEntireStore(t *testing.T) {
	// Test 19: [{5,A},{5,B}] → Delete(0,10) → [], TotalLen=0
	s := buildStore([]StyleRun{{Len: 5, Style: styleA}, {Len: 5, Style: styleB}})
	s.Delete(0, 10)
	expectRuns(t, "#19", s, []StyleRun{})
	expectTotalLen(t, "#19", s, 0)
}

func TestSpanStore_DeleteShrinksRunToZero(t *testing.T) {
	// Test 20: [{3,A},{2,B},{5,C}] → Delete(3,2) → [{3,A},{5,C}], TotalLen=8
	s := buildStore([]StyleRun{
		{Len: 3, Style: styleA},
		{Len: 2, Style: styleB},
		{Len: 5, Style: styleC},
	})
	s.Delete(3, 2)
	expectRuns(t, "#20", s, []StyleRun{{Len: 3, Style: styleA}, {Len: 5, Style: styleC}})
	expectTotalLen(t, "#20", s, 8)
}

func TestSpanStore_DeleteSpanningMultiplePartialEdges(t *testing.T) {
	// Test 21: [{5,A},{5,B},{5,C},{5,D}] → Delete(3,14) → [{3,A},{3,D}], TotalLen=6
	s := buildStore([]StyleRun{
		{Len: 5, Style: styleA},
		{Len: 5, Style: styleB},
		{Len: 5, Style: styleC},
		{Len: 5, Style: styleD},
	})
	s.Delete(3, 14)
	expectRuns(t, "#21", s, []StyleRun{{Len: 3, Style: styleA}, {Len: 3, Style: styleD}})
	expectTotalLen(t, "#21", s, 6)
}

// =========================================================================
// RegionUpdate (tests 22–31)
// =========================================================================

func TestSpanStore_ReplaceEntireStore(t *testing.T) {
	// Test 22: [{10,A}] → RU(0, [{10,B}]) → [{10,B}], TotalLen=10
	s := buildStore([]StyleRun{{Len: 10, Style: styleA}})
	s.RegionUpdate(0, []StyleRun{{Len: 10, Style: styleB}})
	expectRuns(t, "#22", s, []StyleRun{{Len: 10, Style: styleB}})
	expectTotalLen(t, "#22", s, 10)
}

func TestSpanStore_ReplaceAtStart(t *testing.T) {
	// Test 23: [{10,A}] → RU(0, [{5,B}]) → [{5,B},{5,A}], TotalLen=10
	s := buildStore([]StyleRun{{Len: 10, Style: styleA}})
	s.RegionUpdate(0, []StyleRun{{Len: 5, Style: styleB}})
	expectRuns(t, "#23", s, []StyleRun{{Len: 5, Style: styleB}, {Len: 5, Style: styleA}})
	expectTotalLen(t, "#23", s, 10)
}

func TestSpanStore_ReplaceAtEnd(t *testing.T) {
	// Test 24: [{10,A}] → RU(5, [{5,B}]) → [{5,A},{5,B}], TotalLen=10
	s := buildStore([]StyleRun{{Len: 10, Style: styleA}})
	s.RegionUpdate(5, []StyleRun{{Len: 5, Style: styleB}})
	expectRuns(t, "#24", s, []StyleRun{{Len: 5, Style: styleA}, {Len: 5, Style: styleB}})
	expectTotalLen(t, "#24", s, 10)
}

func TestSpanStore_ReplaceMiddle(t *testing.T) {
	// Test 25: [{10,A}] → RU(3, [{4,B}]) → [{3,A},{4,B},{3,A}], TotalLen=10
	s := buildStore([]StyleRun{{Len: 10, Style: styleA}})
	s.RegionUpdate(3, []StyleRun{{Len: 4, Style: styleB}})
	expectRuns(t, "#25", s, []StyleRun{
		{Len: 3, Style: styleA},
		{Len: 4, Style: styleB},
		{Len: 3, Style: styleA},
	})
	expectTotalLen(t, "#25", s, 10)
}

func TestSpanStore_ReplaceSpanningRuns(t *testing.T) {
	// Test 26: [{5,A},{5,B}] → RU(3, [{4,C}]) → [{3,A},{4,C},{3,B}], TotalLen=10
	s := buildStore([]StyleRun{{Len: 5, Style: styleA}, {Len: 5, Style: styleB}})
	s.RegionUpdate(3, []StyleRun{{Len: 4, Style: styleC}})
	expectRuns(t, "#26", s, []StyleRun{
		{Len: 3, Style: styleA},
		{Len: 4, Style: styleC},
		{Len: 3, Style: styleB},
	})
	expectTotalLen(t, "#26", s, 10)
}

func TestSpanStore_ReplaceWithMergeLeft(t *testing.T) {
	// Test 27: [{5,A},{5,B}] → RU(5, [{5,A}]) → [{10,A}], TotalLen=10
	// The new run has the same style as the preceding run → merge.
	s := buildStore([]StyleRun{{Len: 5, Style: styleA}, {Len: 5, Style: styleB}})
	s.RegionUpdate(5, []StyleRun{{Len: 5, Style: styleA}})
	expectRuns(t, "#27", s, []StyleRun{{Len: 10, Style: styleA}})
	expectTotalLen(t, "#27", s, 10)
}

func TestSpanStore_ReplaceWithMergeRight(t *testing.T) {
	// Test 28: [{5,A},{5,B}] → RU(0, [{5,B}]) → [{10,B}], TotalLen=10
	// The new run has the same style as the following run → merge.
	s := buildStore([]StyleRun{{Len: 5, Style: styleA}, {Len: 5, Style: styleB}})
	s.RegionUpdate(0, []StyleRun{{Len: 5, Style: styleB}})
	expectRuns(t, "#28", s, []StyleRun{{Len: 10, Style: styleB}})
	expectTotalLen(t, "#28", s, 10)
}

func TestSpanStore_ReplaceWithMultiRuns(t *testing.T) {
	// Test 29: [{10,A}] → RU(0, [{3,B},{4,C},{3,D}]) → [{3,B},{4,C},{3,D}], TotalLen=10
	s := buildStore([]StyleRun{{Len: 10, Style: styleA}})
	s.RegionUpdate(0, []StyleRun{
		{Len: 3, Style: styleB},
		{Len: 4, Style: styleC},
		{Len: 3, Style: styleD},
	})
	expectRuns(t, "#29", s, []StyleRun{
		{Len: 3, Style: styleB},
		{Len: 4, Style: styleC},
		{Len: 3, Style: styleD},
	})
	expectTotalLen(t, "#29", s, 10)
}

func TestSpanStore_ReplaceAlignedBoundaries(t *testing.T) {
	// Test 30: [{5,A},{5,B},{5,C}] → RU(5, [{5,D}]) → [{5,A},{5,D},{5,C}], TotalLen=15
	s := buildStore([]StyleRun{
		{Len: 5, Style: styleA},
		{Len: 5, Style: styleB},
		{Len: 5, Style: styleC},
	})
	s.RegionUpdate(5, []StyleRun{{Len: 5, Style: styleD}})
	expectRuns(t, "#30", s, []StyleRun{
		{Len: 5, Style: styleA},
		{Len: 5, Style: styleD},
		{Len: 5, Style: styleC},
	})
	expectTotalLen(t, "#30", s, 15)
}

func TestSpanStore_ReplaceAtStartMergeRight(t *testing.T) {
	// Test 31: [{5,A},{5,B}] → RU(0, [{5,B}]) → [{10,B}], TotalLen=10
	// Same as test 28 but listed separately in the matrix.
	s := buildStore([]StyleRun{{Len: 5, Style: styleA}, {Len: 5, Style: styleB}})
	s.RegionUpdate(0, []StyleRun{{Len: 5, Style: styleB}})
	expectRuns(t, "#31", s, []StyleRun{{Len: 10, Style: styleB}})
	expectTotalLen(t, "#31", s, 10)
}

// =========================================================================
// ForEachRun (tests 32–34)
// =========================================================================

func TestSpanStore_ForEachRunSingle(t *testing.T) {
	// Test 32: Single run — fn called once with {10,A}.
	s := buildStore([]StyleRun{{Len: 10, Style: styleA}})
	var collected []StyleRun
	s.ForEachRun(func(r StyleRun) { collected = append(collected, r) })
	if len(collected) != 1 {
		t.Fatalf("#32: expected 1 call, got %d", len(collected))
	}
	if collected[0].Len != 10 || !collected[0].Style.Equal(styleA) {
		t.Errorf("#32: got %+v, want {Len:10, Style:A}", collected[0])
	}
}

func TestSpanStore_ForEachRunMultiple(t *testing.T) {
	// Test 33: Multiple runs — fn called 3 times in order.
	s := buildStore([]StyleRun{
		{Len: 5, Style: styleA},
		{Len: 3, Style: styleB},
		{Len: 7, Style: styleC},
	})
	var collected []StyleRun
	s.ForEachRun(func(r StyleRun) { collected = append(collected, r) })
	expected := []StyleRun{
		{Len: 5, Style: styleA},
		{Len: 3, Style: styleB},
		{Len: 7, Style: styleC},
	}
	if len(collected) != len(expected) {
		t.Fatalf("#33: expected %d calls, got %d", len(expected), len(collected))
	}
	for i, e := range expected {
		if collected[i].Len != e.Len || !collected[i].Style.Equal(e.Style) {
			t.Errorf("#33: run[%d] got %+v, want %+v", i, collected[i], e)
		}
	}
}

func TestSpanStore_ForEachRunAfterGapMove(t *testing.T) {
	// Test 34: After gap move — build then Insert at distant position,
	// iteration still correct.
	s := buildStore([]StyleRun{
		{Len: 5, Style: styleA},
		{Len: 5, Style: styleB},
		{Len: 5, Style: styleC},
	})
	// Insert near the end to force a gap move away from the initial position.
	s.Insert(12, 3)
	// Expected: [{5,A},{5,B},{8,C}] (insert mid-run in C at offset 12 = 5+5+2 into C)
	expected := []StyleRun{
		{Len: 5, Style: styleA},
		{Len: 5, Style: styleB},
		{Len: 8, Style: styleC},
	}
	var collected []StyleRun
	s.ForEachRun(func(r StyleRun) { collected = append(collected, r) })
	if len(collected) != len(expected) {
		t.Fatalf("#34: expected %d runs, got %d: %+v", len(expected), len(collected), collected)
	}
	for i, e := range expected {
		if collected[i].Len != e.Len || !collected[i].Style.Equal(e.Style) {
			t.Errorf("#34: run[%d] got %+v, want %+v", i, collected[i], e)
		}
	}
}

// =========================================================================
// Clear (tests 35–36)
// =========================================================================

func TestSpanStore_ClearNonEmpty(t *testing.T) {
	// Test 35: Clear non-empty → TotalLen=0, NumRuns=0
	s := buildStore([]StyleRun{{Len: 5, Style: styleA}, {Len: 5, Style: styleB}})
	s.Clear()
	expectTotalLen(t, "#35", s, 0)
	expectNumRuns(t, "#35", s, 0)
}

func TestSpanStore_ClearThenInsert(t *testing.T) {
	// Test 36: [{5,A}] → Clear → Insert(0,3) → [{3,default}], TotalLen=3
	s := buildStore([]StyleRun{{Len: 5, Style: styleA}})
	s.Clear()
	s.Insert(0, 3)
	expectRuns(t, "#36", s, []StyleRun{{Len: 3, Style: styleDefault}})
	expectTotalLen(t, "#36", s, 3)
}

// =========================================================================
// Zero-Length Spans (test 37)
// =========================================================================

func TestSpanStore_ZeroLengthInRegionUpdate(t *testing.T) {
	// Test 37: Zero-length runs in RegionUpdate should be dropped.
	// [{10,A}] → RU(5, [{0,B},{5,A}]) → the 0-length B run is dropped,
	// leaving [{10,A}] since the 5,A merges with the preceding 5,A.
	s := buildStore([]StyleRun{{Len: 10, Style: styleA}})
	s.RegionUpdate(5, []StyleRun{{Len: 0, Style: styleB}, {Len: 5, Style: styleA}})
	// After dropping the zero-length run and merging adjacent A runs:
	expectRuns(t, "#37", s, []StyleRun{{Len: 10, Style: styleA}})
	expectTotalLen(t, "#37", s, 10)
}

// =========================================================================
// TotalLen Consistency (tests 38–41)
// =========================================================================

func TestSpanStore_TotalLenAfterInsertSequence(t *testing.T) {
	// Test 38: Insert 5, Insert 3, Insert 2 → TotalLen == 10
	s := NewSpanStore()
	s.Insert(0, 5)
	s.Insert(2, 3)
	s.Insert(6, 2)
	expectTotalLen(t, "#38", s, 10)
}

func TestSpanStore_TotalLenAfterDeleteSequence(t *testing.T) {
	// Test 39: [{10,A}] → Delete(0,3) → Delete(0,2) → TotalLen == 5
	s := buildStore([]StyleRun{{Len: 10, Style: styleA}})
	s.Delete(0, 3)
	expectTotalLen(t, "#39a", s, 7)
	s.Delete(0, 2)
	expectTotalLen(t, "#39b", s, 5)
}

func TestSpanStore_TotalLenAfterRegionUpdate(t *testing.T) {
	// Test 40: [{10,A}] → RU(0,[{5,B},{5,C}]) → TotalLen == 10
	s := buildStore([]StyleRun{{Len: 10, Style: styleA}})
	s.RegionUpdate(0, []StyleRun{{Len: 5, Style: styleB}, {Len: 5, Style: styleC}})
	expectTotalLen(t, "#40", s, 10)
}

func TestSpanStore_TotalLenAfterMixedOps(t *testing.T) {
	// Test 41: Insert, RegionUpdate, Delete, Insert → TotalLen consistent.
	s := NewSpanStore()
	s.Insert(0, 20)                                                                // 20
	s.RegionUpdate(0, []StyleRun{{Len: 10, Style: styleA}, {Len: 10, Style: styleB}}) // still 20
	expectTotalLen(t, "#41a", s, 20)

	s.Delete(5, 5) // 15
	expectTotalLen(t, "#41b", s, 15)

	s.Insert(10, 5) // 20
	expectTotalLen(t, "#41c", s, 20)

	// Verify runs are consistent: compute sum of run lengths.
	sum := 0
	s.ForEachRun(func(r StyleRun) { sum += r.Len })
	if sum != s.TotalLen() {
		t.Errorf("#41d: sum of run lengths (%d) != TotalLen (%d)", sum, s.TotalLen())
	}
}

// =========================================================================
// StyleAttrs Equality (tests 42–47)
// =========================================================================

func TestStyleAttrs_BothNilColors(t *testing.T) {
	// Test 42: Both nil colors → equal
	a := StyleAttrs{}
	b := StyleAttrs{}
	if !a.Equal(b) {
		t.Error("#42: expected equal for both nil colors")
	}
}

func TestStyleAttrs_OneNilOneNonNil(t *testing.T) {
	// Test 43: One nil, one non-nil → not equal
	a := StyleAttrs{Fg: color.RGBA{R: 255, A: 255}}
	b := StyleAttrs{}
	if a.Equal(b) {
		t.Error("#43: expected not equal (one nil Fg)")
	}
	// Also test Bg
	c := StyleAttrs{Bg: color.RGBA{G: 255, A: 255}}
	d := StyleAttrs{}
	if c.Equal(d) {
		t.Error("#43b: expected not equal (one nil Bg)")
	}
}

func TestStyleAttrs_SameRGBA(t *testing.T) {
	// Test 44: Same RGBA values → equal
	a := StyleAttrs{Fg: color.RGBA{R: 100, G: 200, B: 50, A: 255}}
	b := StyleAttrs{Fg: color.RGBA{R: 100, G: 200, B: 50, A: 255}}
	if !a.Equal(b) {
		t.Error("#44: expected equal for same RGBA values")
	}
}

func TestStyleAttrs_DifferentRGBA(t *testing.T) {
	// Test 45: Different RGBA values → not equal
	a := StyleAttrs{Fg: color.RGBA{R: 100, G: 200, B: 50, A: 255}}
	b := StyleAttrs{Fg: color.RGBA{R: 100, G: 201, B: 50, A: 255}}
	if a.Equal(b) {
		t.Error("#45: expected not equal for different RGBA values")
	}
}

func TestStyleAttrs_SameBoolFlags(t *testing.T) {
	// Test 46: Same bool flags → equal
	a := StyleAttrs{Bold: true, Italic: true, Hidden: false}
	b := StyleAttrs{Bold: true, Italic: true, Hidden: false}
	if !a.Equal(b) {
		t.Error("#46: expected equal for same bool flags")
	}
}

func TestStyleAttrs_DifferentBoolFlags(t *testing.T) {
	// Test 47: Different bool flags → not equal
	a := StyleAttrs{Bold: true}
	b := StyleAttrs{Bold: false}
	if a.Equal(b) {
		t.Error("#47a: expected not equal for different Bold")
	}

	c := StyleAttrs{Italic: true}
	d := StyleAttrs{Italic: false}
	if c.Equal(d) {
		t.Error("#47b: expected not equal for different Italic")
	}

	e := StyleAttrs{Hidden: true}
	f := StyleAttrs{Hidden: false}
	if e.Equal(f) {
		t.Error("#47c: expected not equal for different Hidden")
	}
}
