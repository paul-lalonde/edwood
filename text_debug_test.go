package main

import (
	"image"
	"testing"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/frame"
)

// hookRecordingFrame extends recordingFrame to capture the hook
// registered via SetAfterPaintHook. Tests assert presence /
// absence of the hook rather than calling it (the calling
// contract lives in the frame package and is tested there).
type hookRecordingFrame struct {
	*recordingFrame
	hook func()
}

func (h *hookRecordingFrame) SetAfterPaintHook(fn func()) { h.hook = fn }

// TestToggleSpansOverlay_FlipsState confirms the toggle returns
// alternating values on successive calls.
func TestToggleSpansOverlay_FlipsState(t *testing.T) {
	w, rf := setupBodyForInsertedTest(t)
	h := &hookRecordingFrame{recordingFrame: rf}
	w.body.fr = h

	if got := w.body.ToggleSpansOverlay(); got != true {
		t.Errorf("first toggle = %v, want true", got)
	}
	if got := w.body.ToggleSpansOverlay(); got != false {
		t.Errorf("second toggle = %v, want false", got)
	}
}

// TestToggleSpansOverlay_RegistersHookOnEnable confirms that
// enabling the overlay calls SetAfterPaintHook with a non-nil fn.
func TestToggleSpansOverlay_RegistersHookOnEnable(t *testing.T) {
	w, rf := setupBodyForInsertedTest(t)
	h := &hookRecordingFrame{recordingFrame: rf}
	w.body.fr = h

	w.body.ToggleSpansOverlay()

	if h.hook == nil {
		t.Errorf("expected SetAfterPaintHook(non-nil) after enable; got nil")
	}
}

// TestToggleSpansOverlay_ClearsHookOnDisable confirms that
// disabling the overlay calls SetAfterPaintHook(nil).
func TestToggleSpansOverlay_ClearsHookOnDisable(t *testing.T) {
	w, rf := setupBodyForInsertedTest(t)
	h := &hookRecordingFrame{recordingFrame: rf}
	w.body.fr = h

	w.body.ToggleSpansOverlay() // on
	w.body.ToggleSpansOverlay() // off

	if h.hook != nil {
		t.Errorf("expected SetAfterPaintHook(nil) after disable; got non-nil")
	}
}

// TestToggleSpansOverlay_NilFrame_NoCrash confirms that toggling
// on a Text with no frame doesn't panic. (Used in some test
// setups; defensive.)
func TestToggleSpansOverlay_NilFrame_NoCrash(t *testing.T) {
	w, _ := setupBodyForInsertedTest(t)
	w.body.fr = nil

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("ToggleSpansOverlay panicked with nil fr: %v", r)
		}
	}()
	w.body.ToggleSpansOverlay()
}

// TestPaintSpansOverlay_NilSpans_NoOp confirms the overlay
// paint function does nothing (no DrawOutlineRect calls) when
// the Text has no spans.
func TestPaintSpansOverlay_NilSpans_NoOp(t *testing.T) {
	w, rf := setupBodyForInsertedTest(t)
	rect := &outlineRecorder{recordingFrame: rf}
	w.body.fr = rect
	w.body.spans = nil

	w.body.paintSpansOverlay()

	if rect.outlineCalls != 0 {
		t.Errorf("expected 0 DrawOutlineRect calls with nil spans; got %d",
			rect.outlineCalls)
	}
}

// TestPaintSpansOverlay_NonPlainRegion_DrawsOneOutline confirms
// that a styled region produces exactly one DrawOutlineRect call.
func TestPaintSpansOverlay_NonPlainRegion_DrawsOneOutline(t *testing.T) {
	w, rf := setupBodyForInsertedTest(t)
	rect := &outlineRecorder{recordingFrame: rf}
	w.body.fr = rect

	// Inform spans of an 11-rune buffer, then style the first 5
	// runes. We bypass the buffer to avoid the tag-observer
	// chain in InsertAt; the spans store just needs to know
	// "buffer has N runes and [0,5) is non-plain" for the
	// overlay's iteration.
	updateSpansForInserted(w, 0, 11)
	w.body.spans.SetRegion(0, 5, frame.Style{Kind: frame.KindColored})

	w.body.paintSpansOverlay()

	if rect.outlineCalls != 1 {
		t.Errorf("expected 1 DrawOutlineRect call for 1 non-plain region; got %d",
			rect.outlineCalls)
	}
}

// TestPaintSpansOverlay_PlainOnly_NoOutlines confirms that a
// buffer with only plain regions produces no outlines.
func TestPaintSpansOverlay_PlainOnly_NoOutlines(t *testing.T) {
	w, rf := setupBodyForInsertedTest(t)
	rect := &outlineRecorder{recordingFrame: rf}
	w.body.fr = rect

	updateSpansForInserted(w, 0, 5)

	w.body.paintSpansOverlay()

	if rect.outlineCalls != 0 {
		t.Errorf("expected 0 outlines for plain-only spans; got %d",
			rect.outlineCalls)
	}
}

// outlineRecorder counts and captures DrawOutlineRect calls.
type outlineRecorder struct {
	*recordingFrame
	outlineCalls int
	rects        []image.Rectangle
}

func (o *outlineRecorder) DrawOutlineRect(r image.Rectangle, _ draw.Image) {
	o.outlineCalls++
	o.rects = append(o.rects, r)
}

// gridPtofcharFrame simulates a frame that lays out runes on a
// fixed-width / fixed-height grid: charsPerLine runes per visual
// line, charWidth pixels per rune, lineH pixels per line. Used to
// drive paintSpansOverlay through wrap-aware code paths without
// pulling in the real frame.
type gridPtofcharFrame struct {
	*outlineRecorder
	charsPerLine int
	charWidth    int
	lineH        int
	rectMaxX     int // = charsPerLine * charWidth
	nchars       int
}

func (g *gridPtofcharFrame) Ptofchar(i int) image.Point {
	return image.Point{
		X: (i % g.charsPerLine) * g.charWidth,
		Y: (i / g.charsPerLine) * g.lineH,
	}
}

func (g *gridPtofcharFrame) DefaultFontHeight() int { return g.lineH }

func (g *gridPtofcharFrame) LineYOffset(n int) int { return n * g.lineH }

func (g *gridPtofcharFrame) LineHAt(int) int { return g.lineH }

func (g *gridPtofcharFrame) GetFrameFillStatus() frame.FrameFillStatus {
	return frame.FrameFillStatus{Nchars: g.nchars, Maxlines: 1 << 20}
}

func (g *gridPtofcharFrame) Rect() image.Rectangle {
	return image.Rect(0, 0, g.rectMaxX, 1<<20)
}

// TestPaintSpansOverlay_MultiLineSpan_SplitsAtLineBreak —
// invariant I-12: a single styled region that crosses a visual
// line break renders as one outline per visual line, not one
// hull rect.
//
// Layout: 10 chars/line, 13 px wide, 20 px tall.
// Buffer: 25 runes. Style region: [0, 25) (whole buffer).
// Expect: 3 visual lines → 3 outline rects.
func TestPaintSpansOverlay_MultiLineSpan_SplitsAtLineBreak(t *testing.T) {
	w, rf := setupBodyForInsertedTest(t)
	rec := &outlineRecorder{recordingFrame: rf}
	g := &gridPtofcharFrame{
		outlineRecorder: rec,
		charsPerLine:    10,
		charWidth:       13,
		lineH:           20,
		rectMaxX:        130,
		nchars:          25,
	}
	w.body.fr = g

	updateSpansForInserted(w, 0, 25)
	w.body.spans.SetRegion(0, 25, frame.Style{Kind: frame.KindColored})

	w.body.paintSpansOverlay()

	if rec.outlineCalls != 3 {
		t.Fatalf("expected 3 outlines (one per visual line); got %d:\n%v",
			rec.outlineCalls, rec.rects)
	}

	// First line: [0..10) → (0, 0) to (rectMaxX=130, 20)
	want0 := image.Rect(0, 0, 130, 20)
	// Second line: [10..20) → (0, 20) to (rectMaxX=130, 40)
	want1 := image.Rect(0, 20, 130, 40)
	// Third line: [20..25) → (0, 40) to (5*13=65, 60)
	want2 := image.Rect(0, 40, 65, 60)

	if rec.rects[0] != want0 {
		t.Errorf("rect[0] = %v, want %v (line 1, wraps so right edge = rectMaxX)", rec.rects[0], want0)
	}
	if rec.rects[1] != want1 {
		t.Errorf("rect[1] = %v, want %v (line 2, wraps so right edge = rectMaxX)", rec.rects[1], want1)
	}
	if rec.rects[2] != want2 {
		t.Errorf("rect[2] = %v, want %v (final line, ends at Ptofchar(25).X)", rec.rects[2], want2)
	}
}

// TestPaintSpansOverlay_MidLineSpan_SingleRect — a styled span
// that fits on one visual line produces exactly one rect with
// left = Ptofchar(start).X, right = Ptofchar(end).X.
func TestPaintSpansOverlay_MidLineSpan_SingleRect(t *testing.T) {
	w, rf := setupBodyForInsertedTest(t)
	rec := &outlineRecorder{recordingFrame: rf}
	g := &gridPtofcharFrame{
		outlineRecorder: rec,
		charsPerLine:    10,
		charWidth:       13,
		lineH:           20,
		rectMaxX:        130,
		nchars:          10,
	}
	w.body.fr = g

	updateSpansForInserted(w, 0, 10)
	w.body.spans.SetRegion(3, 7, frame.Style{Kind: frame.KindColored})

	w.body.paintSpansOverlay()

	if rec.outlineCalls != 1 {
		t.Fatalf("expected 1 outline for single-line mid-line span; got %d:\n%v",
			rec.outlineCalls, rec.rects)
	}
	// Span [3, 7): start at X=39, end at X=91, on line 1 (Y=0).
	want := image.Rect(39, 0, 91, 20)
	if rec.rects[0] != want {
		t.Errorf("rect = %v, want %v", rec.rects[0], want)
	}
}
