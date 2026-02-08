package main

import (
	"image"
	"image/color"
	"testing"

	"github.com/rjkroege/edwood/edwoodtest"
	"github.com/rjkroege/edwood/file"
)

// makeStyledTextWindow creates a window with body text, a SpanStore, and
// the body Text registered as an observer on the backing file. This allows
// tests to exercise the Text.Inserted/Text.Deleted observer callbacks
// that adjust spans when the buffer is edited.
func makeStyledTextWindow(t *testing.T, bodyText string, spans []StyleRun) *Window {
	t.Helper()

	display := edwoodtest.NewDisplay(image.Rectangle{})
	global.configureGlobals(display)

	f := file.MakeObservableEditableBuffer("", []rune(bodyText))

	w := NewWindow().initHeadless(nil)
	w.display = display
	w.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    f,
		what:    Body,
		eq0:     ^0,
	}
	w.body.w = w
	w.col = &Column{safe: true}

	// Register the body Text as a buffer observer so that file mutations
	// trigger Text.Inserted / Text.Deleted.
	f.AddObserver(&w.body)

	// Set up SpanStore with given spans.
	w.spanStore = NewSpanStore()
	total := 0
	for _, r := range spans {
		total += r.Len
	}
	if total > 0 {
		w.spanStore.Insert(0, total)
		w.spanStore.RegionUpdate(0, spans)
	}
	w.styledMode = true

	return w
}

// =========================================================================
// Integration test 1: Sequential typing in styled mode
// =========================================================================

func TestStyledMode_SequentialTyping(t *testing.T) {
	// Window with "helloworld" (10 runes), spans [5,red] [5,blue].
	// Simulate typing 3 chars at position 3 (within the red run).
	// Expected: [8,red] [5,blue], TotalLen=13.
	red := StyleAttrs{Fg: color.RGBA{R: 0xff, A: 0xff}}
	blue := StyleAttrs{Fg: color.RGBA{B: 0xff, A: 0xff}}

	w := makeStyledTextWindow(t, "helloworld", []StyleRun{
		{Len: 5, Style: red},
		{Len: 5, Style: blue},
	})

	// Verify preconditions.
	expectRuns(t, "pre", w.spanStore, []StyleRun{
		{Len: 5, Style: red},
		{Len: 5, Style: blue},
	})
	expectTotalLen(t, "pre", w.spanStore, 10)

	// Simulate typing 3 characters at position 3 (one at a time, as
	// sequential typing would). Each InsertAt triggers the observer.
	w.body.file.Mark(1)
	w.body.file.InsertAt(3, []rune("x"))
	w.body.file.InsertAt(4, []rune("y"))
	w.body.file.InsertAt(5, []rune("z"))

	// Verify spans adjusted: the red run (which contained pos 3) extends by 3.
	expectRuns(t, "after typing", w.spanStore, []StyleRun{
		{Len: 8, Style: red},
		{Len: 5, Style: blue},
	})
	expectTotalLen(t, "after typing", w.spanStore, 13)

	// Verify buffer content is correct.
	if got := w.body.file.String(); got != "helxyzloworld" {
		t.Errorf("buffer = %q, want %q", got, "helxyzloworld")
	}
}

// =========================================================================
// Integration test 2: Cut operation (delete range)
// =========================================================================

func TestStyledMode_CutOperation(t *testing.T) {
	// Window with "helloworld" (10 runes), spans [5,red] [5,blue].
	// Delete range [2, 7) — removes "llowo" which spans both runs.
	// Expected: [2,red] [3,blue], TotalLen=5.
	red := StyleAttrs{Fg: color.RGBA{R: 0xff, A: 0xff}}
	blue := StyleAttrs{Fg: color.RGBA{B: 0xff, A: 0xff}}

	w := makeStyledTextWindow(t, "helloworld", []StyleRun{
		{Len: 5, Style: red},
		{Len: 5, Style: blue},
	})

	w.body.file.Mark(1)
	w.body.file.DeleteAt(2, 7)

	// After deleting [2,7): red shrinks from 5 to 2, blue shrinks from 5 to 3.
	expectRuns(t, "after cut", w.spanStore, []StyleRun{
		{Len: 2, Style: red},
		{Len: 3, Style: blue},
	})
	expectTotalLen(t, "after cut", w.spanStore, 5)

	if got := w.body.file.String(); got != "herld" {
		t.Errorf("buffer = %q, want %q", got, "herld")
	}
}

// =========================================================================
// Integration test 3: Paste operation (insert at point)
// =========================================================================

func TestStyledMode_PasteOperation(t *testing.T) {
	// Window with "helloworld" (10 runes), spans [5,red] [5,blue].
	// Insert "abc" at position 5 (the boundary between red and blue).
	// Expected: [8,red] [5,blue], TotalLen=13.
	// (Insert at boundary extends the preceding run.)
	red := StyleAttrs{Fg: color.RGBA{R: 0xff, A: 0xff}}
	blue := StyleAttrs{Fg: color.RGBA{B: 0xff, A: 0xff}}

	w := makeStyledTextWindow(t, "helloworld", []StyleRun{
		{Len: 5, Style: red},
		{Len: 5, Style: blue},
	})

	w.body.file.Mark(1)
	w.body.file.InsertAt(5, []rune("abc"))

	expectRuns(t, "after paste", w.spanStore, []StyleRun{
		{Len: 8, Style: red},
		{Len: 5, Style: blue},
	})
	expectTotalLen(t, "after paste", w.spanStore, 13)

	if got := w.body.file.String(); got != "helloabcworld" {
		t.Errorf("buffer = %q, want %q", got, "helloabcworld")
	}
}

// =========================================================================
// Integration test 4: Undo reverses span adjustments
// =========================================================================

func TestStyledMode_UndoReversesSpans(t *testing.T) {
	// Window with "helloworld" (10 runes), spans [5,red] [5,blue].
	// Insert "abc" at position 3, then undo.
	// After insert: [8,red] [5,blue], TotalLen=13.
	// After undo:   [5,red] [5,blue], TotalLen=10 (original state restored).
	red := StyleAttrs{Fg: color.RGBA{R: 0xff, A: 0xff}}
	blue := StyleAttrs{Fg: color.RGBA{B: 0xff, A: 0xff}}

	w := makeStyledTextWindow(t, "helloworld", []StyleRun{
		{Len: 5, Style: red},
		{Len: 5, Style: blue},
	})

	// Mark an undo point, then insert.
	w.body.file.Mark(1)
	w.body.file.InsertAt(3, []rune("abc"))

	// Verify post-insert state.
	expectRuns(t, "after insert", w.spanStore, []StyleRun{
		{Len: 8, Style: red},
		{Len: 5, Style: blue},
	})
	expectTotalLen(t, "after insert", w.spanStore, 13)

	// Undo. The file's Undo replays as a Deleted observer callback,
	// which triggers the span store Delete.
	w.body.file.Mark(2)
	w.body.file.Undo(true)

	// After undo, spans should return to original state.
	expectRuns(t, "after undo", w.spanStore, []StyleRun{
		{Len: 5, Style: red},
		{Len: 5, Style: blue},
	})
	expectTotalLen(t, "after undo", w.spanStore, 10)

	if got := w.body.file.String(); got != "helloworld" {
		t.Errorf("buffer = %q, want %q", got, "helloworld")
	}
}

// =========================================================================
// Integration test 5: Undo reverses a deletion
// =========================================================================

func TestStyledMode_UndoReversesDelete(t *testing.T) {
	// Window with "helloworld" (10 runes), spans [5,red] [5,blue].
	// Delete range [2, 4) (removes "ll"), then undo.
	// After delete: [3,red] [5,blue], TotalLen=8.
	// After undo:   [5,red] [5,blue], TotalLen=10 (insert at 2 extends red).
	red := StyleAttrs{Fg: color.RGBA{R: 0xff, A: 0xff}}
	blue := StyleAttrs{Fg: color.RGBA{B: 0xff, A: 0xff}}

	w := makeStyledTextWindow(t, "helloworld", []StyleRun{
		{Len: 5, Style: red},
		{Len: 5, Style: blue},
	})

	w.body.file.Mark(1)
	w.body.file.DeleteAt(2, 4)

	// After delete: "heloworld", red shrinks from 5 to 3.
	expectRuns(t, "after delete", w.spanStore, []StyleRun{
		{Len: 3, Style: red},
		{Len: 5, Style: blue},
	})
	expectTotalLen(t, "after delete", w.spanStore, 8)

	// Undo the deletion. The file's Undo replays as an Inserted observer
	// callback, which triggers the span store Insert at position 2.
	w.body.file.Mark(2)
	w.body.file.Undo(true)

	// After undo: insert at position 2 extends the red run (pos 2 is mid-run).
	expectRuns(t, "after undo", w.spanStore, []StyleRun{
		{Len: 5, Style: red},
		{Len: 5, Style: blue},
	})
	expectTotalLen(t, "after undo", w.spanStore, 10)

	if got := w.body.file.String(); got != "helloworld" {
		t.Errorf("buffer = %q, want %q", got, "helloworld")
	}
}

// =========================================================================
// Integration test 6: Multiple observers — only styled window adjusts spans
// =========================================================================

func TestStyledMode_MultipleObservers(t *testing.T) {
	// Two Text views on the same file. Only the body Text with styled mode
	// should adjust the span store. A second observer (simulating a clone)
	// should NOT cause double adjustments.
	red := StyleAttrs{Fg: color.RGBA{R: 0xff, A: 0xff}}

	display := edwoodtest.NewDisplay(image.Rectangle{})
	global.configureGlobals(display)

	f := file.MakeObservableEditableBuffer("", []rune("hello"))

	// First window: styled mode.
	w1 := NewWindow().initHeadless(nil)
	w1.display = display
	w1.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    f,
		what:    Body,
		eq0:     ^0,
	}
	w1.body.w = w1
	w1.col = &Column{safe: true}
	f.AddObserver(&w1.body)

	w1.spanStore = NewSpanStore()
	w1.spanStore.Insert(0, 5)
	w1.spanStore.RegionUpdate(0, []StyleRun{{Len: 5, Style: red}})
	w1.styledMode = true

	// Second window: plain mode, sharing the same file.
	w2 := NewWindow().initHeadless(nil)
	w2.display = display
	w2.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    f,
		what:    Body,
		eq0:     ^0,
	}
	w2.body.w = w2
	w2.col = &Column{safe: true}
	f.AddObserver(&w2.body)

	// w2 is NOT in styled mode and has no spanStore.

	// Insert into the shared file. Both observers fire, but only w1
	// has styled mode + spanStore, so only w1's spans are adjusted.
	f.Mark(1)
	f.InsertAt(3, []rune("XY"))

	// w1's span store should be adjusted exactly once (5 + 2 = 7).
	expectRuns(t, "w1 after insert", w1.spanStore, []StyleRun{
		{Len: 7, Style: red},
	})
	expectTotalLen(t, "w1 after insert", w1.spanStore, 7)

	// w2 should have no span store.
	if w2.spanStore != nil {
		t.Error("w2 should not have a spanStore")
	}
}

// =========================================================================
// Integration test 7: Insert at position 0 through observer
// =========================================================================

func TestStyledMode_InsertAtStart(t *testing.T) {
	// Verify that inserting at position 0 through the observer extends
	// the first run.
	red := StyleAttrs{Fg: color.RGBA{R: 0xff, A: 0xff}}

	w := makeStyledTextWindow(t, "hello", []StyleRun{
		{Len: 5, Style: red},
	})

	w.body.file.Mark(1)
	w.body.file.InsertAt(0, []rune("abc"))

	expectRuns(t, "after insert at 0", w.spanStore, []StyleRun{
		{Len: 8, Style: red},
	})
	expectTotalLen(t, "after insert at 0", w.spanStore, 8)
}

// =========================================================================
// Integration test 8: Insert at end through observer
// =========================================================================

func TestStyledMode_InsertAtEnd(t *testing.T) {
	// Verify that inserting at the end through the observer extends
	// the last run.
	blue := StyleAttrs{Fg: color.RGBA{B: 0xff, A: 0xff}}

	w := makeStyledTextWindow(t, "hello", []StyleRun{
		{Len: 5, Style: blue},
	})

	w.body.file.Mark(1)
	w.body.file.InsertAt(5, []rune("XYZ"))

	expectRuns(t, "after insert at end", w.spanStore, []StyleRun{
		{Len: 8, Style: blue},
	})
	expectTotalLen(t, "after insert at end", w.spanStore, 8)
}

// =========================================================================
// Integration test 9: Delete entire run through observer
// =========================================================================

func TestStyledMode_DeleteEntireRun(t *testing.T) {
	// Window with [5,red] [5,blue] [5,green]. Delete the blue run entirely.
	red := StyleAttrs{Fg: color.RGBA{R: 0xff, A: 0xff}}
	blue := StyleAttrs{Fg: color.RGBA{B: 0xff, A: 0xff}}
	green := StyleAttrs{Fg: color.RGBA{G: 0xff, A: 0xff}}

	w := makeStyledTextWindow(t, "aaaaabbbbbccccc", []StyleRun{
		{Len: 5, Style: red},
		{Len: 5, Style: blue},
		{Len: 5, Style: green},
	})

	w.body.file.Mark(1)
	w.body.file.DeleteAt(5, 10)

	expectRuns(t, "after delete entire run", w.spanStore, []StyleRun{
		{Len: 5, Style: red},
		{Len: 5, Style: green},
	})
	expectTotalLen(t, "after delete entire run", w.spanStore, 10)
}

// =========================================================================
// Integration test 10: Non-styled-mode window ignores span adjustments
// =========================================================================

func TestNonStyledMode_NoSpanAdjustment(t *testing.T) {
	// A window that is NOT in styled mode should not adjust spans,
	// even if a spanStore exists.
	display := edwoodtest.NewDisplay(image.Rectangle{})
	global.configureGlobals(display)

	f := file.MakeObservableEditableBuffer("", []rune("hello"))

	w := NewWindow().initHeadless(nil)
	w.display = display
	w.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    f,
		what:    Body,
		eq0:     ^0,
	}
	w.body.w = w
	w.col = &Column{safe: true}
	f.AddObserver(&w.body)

	// spanStore exists but styledMode is false.
	w.spanStore = NewSpanStore()
	w.spanStore.Insert(0, 5)
	w.styledMode = false

	f.Mark(1)
	f.InsertAt(3, []rune("XY"))

	// Span store should NOT have been adjusted — still 5, not 7.
	expectTotalLen(t, "non-styled insert", w.spanStore, 5)
}
