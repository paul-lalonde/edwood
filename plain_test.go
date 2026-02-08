package main

import (
	"image"
	"image/color"
	"testing"

	"github.com/rjkroege/edwood/edwoodtest"
	"github.com/rjkroege/edwood/file"
)

// makePlainTestWindow creates a headless window with body text for testing
// plaincmd. The window starts in plain mode with no spans.
func makePlainTestWindow(t *testing.T, bodyText string) *Window {
	t.Helper()

	display := edwoodtest.NewDisplay(image.Rectangle{})
	global.configureGlobals(display)

	w := NewWindow().initHeadless(nil)
	w.display = display
	w.body = Text{
		display: display,
		fr:      &MockFrame{},
		file:    file.MakeObservableEditableBuffer("", []rune(bodyText)),
	}
	w.body.w = w
	return w
}

// TestPlaincmd_StyledToPlain verifies that Plain toggles a styled window
// back to plain mode, preserving the span store.
func TestPlaincmd_StyledToPlain(t *testing.T) {
	w := makePlainTestWindow(t, "hello")

	// Set up spans and enter styled mode.
	w.spanStore = NewSpanStore()
	w.spanStore.RegionUpdate(0, []StyleRun{
		{Len: 5, Style: StyleAttrs{Fg: color.RGBA{R: 0xff, A: 0xff}}},
	})
	w.initStyledMode()
	if !w.IsStyledMode() {
		t.Fatal("precondition: should be in styled mode")
	}

	// Nil display to skip redraw (no full display in test).
	w.display = nil

	plaincmd(&w.body, nil, nil, false, false, "")

	if w.IsStyledMode() {
		t.Error("should have exited styled mode")
	}
	// Spans must be preserved.
	if w.spanStore == nil || w.spanStore.TotalLen() == 0 {
		t.Error("span store should be preserved after toggling to plain")
	}
}

// TestPlaincmd_PlainToStyledWithSpans verifies that Plain toggles a plain
// window with existing spans back to styled mode.
func TestPlaincmd_PlainToStyledWithSpans(t *testing.T) {
	w := makePlainTestWindow(t, "hello")

	// Set up spans but stay in plain mode (simulating a previous toggle-off).
	w.spanStore = NewSpanStore()
	w.spanStore.RegionUpdate(0, []StyleRun{
		{Len: 5, Style: StyleAttrs{Fg: color.RGBA{B: 0xff, A: 0xff}}},
	})

	if w.IsStyledMode() {
		t.Fatal("precondition: should be in plain mode")
	}

	plaincmd(&w.body, nil, nil, false, false, "")

	if !w.IsStyledMode() {
		t.Error("should have entered styled mode")
	}
}

// TestPlaincmd_NoopWhenNoSpans verifies that Plain is a no-op when the
// window has no span data.
func TestPlaincmd_NoopWhenNoSpans(t *testing.T) {
	w := makePlainTestWindow(t, "hello")

	// No spanStore at all.
	plaincmd(&w.body, nil, nil, false, false, "")

	if w.IsStyledMode() {
		t.Error("should remain in plain mode when no spans exist")
	}

	// With an empty span store (TotalLen == 0).
	w.spanStore = NewSpanStore()
	plaincmd(&w.body, nil, nil, false, false, "")

	if w.IsStyledMode() {
		t.Error("should remain in plain mode when span store is empty")
	}
}

// TestPlaincmd_SpansPreservedAfterToggle verifies that spans are preserved
// and continue to adjust after toggling to plain mode.
func TestPlaincmd_SpansPreservedAfterToggle(t *testing.T) {
	w := makePlainTestWindow(t, "hello world")

	red := color.RGBA{R: 0xff, A: 0xff}

	// Set up spans and enter styled mode.
	w.spanStore = NewSpanStore()
	w.spanStore.RegionUpdate(0, []StyleRun{
		{Len: 5, Style: StyleAttrs{Fg: red}},
		{Len: 6, Style: StyleAttrs{}},
	})
	w.initStyledMode()
	if !w.IsStyledMode() {
		t.Fatal("precondition: should be in styled mode")
	}

	// Toggle to plain.
	w.display = nil
	plaincmd(&w.body, nil, nil, false, false, "")

	if w.IsStyledMode() {
		t.Fatal("should be in plain mode after toggle")
	}

	// Verify spans are still present.
	if w.spanStore == nil {
		t.Fatal("spanStore should not be nil after toggle to plain")
	}
	if w.spanStore.TotalLen() != 11 {
		t.Errorf("TotalLen = %d, want 11", w.spanStore.TotalLen())
	}

	// Verify span content is preserved.
	runs := w.spanStore.Runs()
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(runs))
	}
	if runs[0].Len != 5 {
		t.Errorf("run[0].Len = %d, want 5", runs[0].Len)
	}
	if !runs[0].Style.Equal(StyleAttrs{Fg: red}) {
		t.Error("run[0] style should be red fg")
	}
}

// TestPlaincmd_ReRenderAfterToggleBack verifies that toggling back to
// styled mode re-renders correctly from preserved spans.
func TestPlaincmd_ReRenderAfterToggleBack(t *testing.T) {
	w := makePlainTestWindow(t, "hello")

	red := color.RGBA{R: 0xff, A: 0xff}

	// Set up spans and enter styled mode.
	w.spanStore = NewSpanStore()
	w.spanStore.RegionUpdate(0, []StyleRun{
		{Len: 5, Style: StyleAttrs{Fg: red}},
	})
	w.initStyledMode()
	if !w.IsStyledMode() {
		t.Fatal("precondition: should be in styled mode")
	}

	// Toggle to plain.
	w.display = nil
	plaincmd(&w.body, nil, nil, false, false, "")
	if w.IsStyledMode() {
		t.Fatal("should be in plain mode after first toggle")
	}

	// Toggle back to styled. Re-set display so initStyledMode can init.
	w.display = edwoodtest.NewDisplay(image.Rectangle{})
	plaincmd(&w.body, nil, nil, false, false, "")

	if !w.IsStyledMode() {
		t.Fatal("should be in styled mode after second toggle")
	}

	// Verify styled content is built from preserved spans.
	content := w.buildStyledContent()
	if len(content) != 1 {
		t.Fatalf("got %d spans, want 1", len(content))
	}
	if content[0].Text != "hello" {
		t.Errorf("span text = %q, want %q", content[0].Text, "hello")
	}
	if content[0].Style.Fg == nil {
		t.Fatal("span Fg is nil, want red")
	}
	r, _, _, _ := content[0].Style.Fg.RGBA()
	wr, _, _, _ := red.RGBA()
	if r != wr {
		t.Errorf("span Fg red component = %d, want %d", r, wr)
	}
}

// TestPlaincmd_NoopInPreviewMode verifies that Plain is a no-op when the
// window is in preview mode.
func TestPlaincmd_NoopInPreviewMode(t *testing.T) {
	w := makePlainTestWindow(t, "hello")

	// Simulate preview mode.
	w.previewMode = true

	plaincmd(&w.body, nil, nil, false, false, "")

	if !w.IsPreviewMode() {
		t.Error("should still be in preview mode")
	}
	if w.IsStyledMode() {
		t.Error("should not have entered styled mode")
	}
}

// TestPlaincmd_NilText verifies that plaincmd handles nil inputs gracefully.
func TestPlaincmd_NilText(t *testing.T) {
	// Should not panic with nil text.
	plaincmd(nil, nil, nil, false, false, "")

	// Should not panic with text that has nil window.
	et := &Text{}
	plaincmd(et, nil, nil, false, false, "")
}
