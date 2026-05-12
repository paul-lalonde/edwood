package main

import (
	"image"
	"testing"

	"github.com/rjkroege/edwood/edwoodtest"
	"github.com/rjkroege/edwood/file"
	"github.com/rjkroege/edwood/frame"
	"github.com/rjkroege/edwood/spans"
)

// A5.2/A5.3 — QWspans wiring. Wire format and apply rules
// follow the published spans-protocol spec (Slice A subset).

func setupWindowForSpansWriteTest(t *testing.T) *Window {
	t.Helper()
	display := edwoodtest.NewDisplay(image.Rectangle{})
	global.configureGlobals(display)
	w := NewWindow().initHeadless(nil)
	w.display = display
	// Replace the body buffer with one pre-loaded with content
	// (avoids the tag-observer chain that InsertAt would trigger).
	buf := file.MakeObservableEditableBuffer("test", []rune("hello world"))
	w.body.file = buf
	w.body.attachSpans(spans.NewStore(buf))
	return w
}

func TestA52_WriteSpansToStore_SetDirectiveAppliesStyle(t *testing.T) {
	w := setupWindowForSpansWriteTest(t)

	if err := writeSpansToStore(w, "s 0 5 #ff0000"); err != nil {
		t.Fatalf("writeSpansToStore: %v", err)
	}

	runs := w.body.spans.GetStyleRuns(0, 11)
	if len(runs) == 0 {
		t.Fatalf("no runs returned")
	}
	if runs[0].Style.Kind&frame.KindColored == 0 {
		t.Errorf("runs[0].Style.Kind = %v, missing KindColored", runs[0].Style.Kind)
	}
	if runs[0].Style.Fg == nil {
		t.Errorf("runs[0].Style.Fg = nil, want non-nil (#ff0000 resolved)")
	}
}

func TestA52_WriteSpansToStore_ClearWipesAll(t *testing.T) {
	w := setupWindowForSpansWriteTest(t)
	// Style some content first.
	if err := writeSpansToStore(w, "s 0 5 #ff0000"); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	if w.body.spans.Empty() {
		t.Fatalf("spans should be non-empty after the seed write")
	}

	// Per protocol: `c` (no args) clears all spans.
	if err := writeSpansToStore(w, "c"); err != nil {
		t.Fatalf("writeSpansToStore(c): %v", err)
	}
	if !w.body.spans.Empty() {
		t.Errorf("after c: spans should be Empty(); Snapshot=%+v", w.body.spans.Snapshot())
	}
}

func TestA52_WriteSpansToStore_MultiDirective_Contiguous(t *testing.T) {
	w := setupWindowForSpansWriteTest(t)
	// Contiguous: 0..5 (red) immediately followed by 5..8 (default).
	payload := "s 0 5 #ff0000\ns 5 3 -\n"
	if err := writeSpansToStore(w, payload); err != nil {
		t.Fatalf("writeSpansToStore: %v", err)
	}
	runs := w.body.spans.GetStyleRuns(0, 11)
	// Expect one colored run for [0,5) and plain elsewhere.
	colored := 0
	for _, r := range runs {
		if !r.Style.IsPlain() {
			colored++
		}
	}
	if colored != 1 {
		t.Errorf("got %d colored runs, want 1: %+v", colored, runs)
	}
}

func TestA52_WriteSpansToStore_NonContiguousErrors(t *testing.T) {
	w := setupWindowForSpansWriteTest(t)
	if err := writeSpansToStore(w, "s 0 5 #ff0000\ns 7 3 -"); err == nil {
		t.Errorf("expected contiguity error, got nil")
	}
}

func TestA52_WriteSpansToStore_AcceptsKnownFlagsSilently(t *testing.T) {
	// Prior `edcolor` emits `s ... <fg> bold` and similar
	// flag-tagged lines. Slice A's parser accepts these silently
	// (ignoring the flag); the directive still applies the color.
	w := setupWindowForSpansWriteTest(t)
	if err := writeSpansToStore(w, "s 0 5 #ff0000 bold"); err != nil {
		t.Fatalf("writeSpansToStore: %v", err)
	}
	runs := w.body.spans.GetStyleRuns(0, 5)
	if len(runs) == 0 {
		t.Fatalf("no runs returned")
	}
	if runs[0].Style.Kind&frame.KindColored == 0 || runs[0].Style.Fg == nil {
		t.Errorf("color must still apply when a known flag is present; got %+v", runs[0])
	}
}

func TestA52_WriteSpansToStore_BadDirectiveErrors(t *testing.T) {
	w := setupWindowForSpansWriteTest(t)
	if err := writeSpansToStore(w, "b 0 1 100 50 - - image:/x"); err == nil {
		t.Errorf("expected error for `b` directive (Slice C only), got nil")
	}
}

func TestA52_WriteSpansToStore_NilSpansErrors(t *testing.T) {
	w := setupWindowForSpansWriteTest(t)
	w.body.spans = nil
	if err := writeSpansToStore(w, "s 0 5 #ff0000"); err == nil {
		t.Errorf("expected error when body.spans is nil, got nil")
	}
}

func TestA52_WriteSpansToStore_BgOnly(t *testing.T) {
	// "Bg-only" in protocol terms: default fg + explicit bg.
	w := setupWindowForSpansWriteTest(t)
	if err := writeSpansToStore(w, "s 2 4 - #0000ff"); err != nil {
		t.Fatalf("writeSpansToStore: %v", err)
	}
	runs := w.body.spans.GetStyleRuns(2, 6)
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1: %+v", len(runs), runs)
	}
	if runs[0].Style.Kind&frame.KindColored == 0 {
		t.Errorf("Kind missing KindColored: %v", runs[0].Style.Kind)
	}
	if runs[0].Style.Bg == nil {
		t.Errorf("Bg = nil, want non-nil")
	}
	if runs[0].Style.Fg != nil {
		t.Errorf("Fg = %v, want nil (explicit `-`)", runs[0].Style.Fg)
	}
}

func TestA52_WriteSpansToStore_OutOfRangeClamped(t *testing.T) {
	// Body has 11 runes. Directive that exceeds the bound must
	// be clamped, not panicked.
	w := setupWindowForSpansWriteTest(t)
	if err := writeSpansToStore(w, "s 8 100 #ff0000"); err != nil {
		t.Errorf("expected silent clamp, got error: %v", err)
	}
}

func TestA52_WriteSpansToStore_OutOfRangeOffsetDropped(t *testing.T) {
	w := setupWindowForSpansWriteTest(t)
	// Offset >= Nr(): drop silently.
	if err := writeSpansToStore(w, "s 100 5 #ff0000"); err != nil {
		t.Errorf("expected silent drop, got error: %v", err)
	}
	if !w.body.spans.Empty() {
		t.Errorf("spans should still be Empty (directive should have been dropped)")
	}
}

// ===== A5.3 integration test =====

func TestA53_Integration_WriteSpansPropagatesToFrame(t *testing.T) {
	// End-to-end check: writeSpansToStore → SetRegion → A4.4
	// Observe callback → frame.SetStyleRange.
	w := setupWindowForSpansWriteTest(t)
	rf := newRecordingFrame()
	rf.nchars = w.body.file.Nr()
	w.body.fr = rf
	w.body.what = Body
	w.body.org = 0

	if err := writeSpansToStore(w, "s 2 4 #ff0000"); err != nil {
		t.Fatalf("writeSpansToStore: %v", err)
	}

	if rf.setStyleRangeCalls != 1 {
		t.Fatalf("SetStyleRange calls = %d, want 1", rf.setStyleRangeCalls)
	}
	if rf.lastStyleRangeP0 != 2 || rf.lastStyleRangeP1 != 6 {
		t.Errorf("SetStyleRange args = (%d,%d), want (2,6)", rf.lastStyleRangeP0, rf.lastStyleRangeP1)
	}
	gotColored := false
	for _, sr := range rf.lastStyleRangeStyles {
		if sr.Style.Kind&frame.KindColored != 0 && sr.Style.Fg != nil {
			gotColored = true
		}
	}
	if !gotColored {
		t.Errorf("no colored run in SetStyleRange styles: %+v", rf.lastStyleRangeStyles)
	}
}
