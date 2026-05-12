package main

import (
	"image"
	"testing"

	"github.com/rjkroege/edwood/edwoodtest"
	"github.com/rjkroege/edwood/file"
	"github.com/rjkroege/edwood/frame"
	"github.com/rjkroege/edwood/spans"
)

// A5.2 — QWspans wiring. The xfid write payload is parsed via
// spans.ParseAll, colors are resolved through the window's
// display, and SetRegion / ClearRegion are applied to
// w.body.spans. Tests exercise writeSpansToStore (the testable
// helper) directly so we don't need to build a fake Xfid.

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

	if err := writeSpansToStore(w, "s 0 5 fg=#ff0000"); err != nil {
		t.Fatalf("writeSpansToStore: %v", err)
	}

	// Verify via GetStyleRuns: the first 5 runes should carry a
	// KindColored style with a non-nil Fg.
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

func TestA52_WriteSpansToStore_ClearDirective(t *testing.T) {
	w := setupWindowForSpansWriteTest(t)

	// First set, then clear.
	if err := writeSpansToStore(w, "s 0 5 fg=#ff0000\nc 0 5"); err != nil {
		t.Fatalf("writeSpansToStore: %v", err)
	}
	if !w.body.spans.Empty() {
		t.Errorf("after set+clear, spans should be Empty(); Snapshot=%+v", w.body.spans.Snapshot())
	}
}

func TestA52_WriteSpansToStore_MultiDirective(t *testing.T) {
	w := setupWindowForSpansWriteTest(t)
	payload := "s 0 5 fg=#ff0000\ns 6 5 fg=#00ff00\n"
	if err := writeSpansToStore(w, payload); err != nil {
		t.Fatalf("writeSpansToStore: %v", err)
	}
	// Two colored regions expected.
	runs := w.body.spans.GetStyleRuns(0, 11)
	colored := 0
	for _, r := range runs {
		if !r.Style.IsPlain() {
			colored++
		}
	}
	if colored != 2 {
		t.Errorf("got %d colored runs, want 2: %+v", colored, runs)
	}
}

func TestA52_WriteSpansToStore_BadDirectiveErrors(t *testing.T) {
	w := setupWindowForSpansWriteTest(t)
	if err := writeSpansToStore(w, "b 0 1 image w=400 h=300 ref=/x"); err == nil {
		t.Errorf("expected error for `b` directive (Slice C only), got nil")
	}
}

func TestA52_WriteSpansToStore_NilSpansErrors(t *testing.T) {
	w := setupWindowForSpansWriteTest(t)
	w.body.spans = nil
	if err := writeSpansToStore(w, "s 0 5 fg=#ff0000"); err == nil {
		t.Errorf("expected error when body.spans is nil, got nil")
	}
}

func TestA53_Integration_WriteSpansPropagatesToFrame(t *testing.T) {
	// End-to-end check for Slice A's producer-driven update path:
	//   spans-file write
	//      → writeSpansToStore (A5.2)
	//      → spans.Store.SetRegion (A3.1)
	//      → Observe callback registered by attachSpans (A4.4)
	//      → frame.SetStyleRange (A2.2 via recordingFrame).
	w := setupWindowForSpansWriteTest(t)
	rf := newRecordingFrame()
	rf.nchars = w.body.file.Nr() // model the whole buffer as visible
	w.body.fr = rf
	w.body.what = Body
	w.body.org = 0
	// No need to re-attach: the A4.4 Observe callback closes
	// over t (the *Text), so setting t.fr above is picked up
	// when the callback runs.

	if err := writeSpansToStore(w, "s 2 4 fg=#ff0000"); err != nil {
		t.Fatalf("writeSpansToStore: %v", err)
	}

	if rf.setStyleRangeCalls != 1 {
		t.Fatalf("SetStyleRange calls = %d, want 1 (producer-driven update should repaint)", rf.setStyleRangeCalls)
	}
	if rf.lastStyleRangeP0 != 2 || rf.lastStyleRangeP1 != 6 {
		t.Errorf("SetStyleRange args = (%d,%d), want (2,6)", rf.lastStyleRangeP0, rf.lastStyleRangeP1)
	}
	// The styled run inside the affected range carries the new color.
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

func TestA52_WriteSpansToStore_BgOnly(t *testing.T) {
	w := setupWindowForSpansWriteTest(t)
	if err := writeSpansToStore(w, "s 2 4 bg=#0000ff"); err != nil {
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
		t.Errorf("Fg = %v, want nil (only bg= specified)", runs[0].Style.Fg)
	}
}
