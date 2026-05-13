package main

import (
	"image"
	"testing"

	"github.com/rjkroege/edwood/edwoodtest"
)

// setupBodyForDebugCommand wires the minimal state both the
// "Box" and "Spans" commands need: a Window with a body that
// has a frame, a display, and a non-degenerate rect. The
// commands trigger Text.Redraw which dereferences t.display
// and uses t.fr.Rect(), so both must be valid.
func setupBodyForDebugCommand(t *testing.T) (*Window, *recordingFrame) {
	t.Helper()
	w, rf := setupBodyForInsertedTest(t)
	disp := edwoodtest.NewDisplay(image.Rect(0, 0, 200, 100))
	w.display = disp
	w.body.display = disp
	w.body.all = image.Rect(0, 0, 200, 100)
	w.body.nofill = true // skip fill loop in Text.Redraw under test
	return w, rf
}

// boxToggleFrame tracks ToggleBoxOutlines calls.
type boxToggleFrame struct {
	*recordingFrame
	toggles int
	state   bool
}

func (b *boxToggleFrame) ToggleBoxOutlines() bool {
	b.toggles++
	b.state = !b.state
	return b.state
}

// TestExecBox_TogglesFrameOutlines confirms that the "Box" tag
// command calls Frame.ToggleBoxOutlines exactly once on the
// window's body frame.
func TestExecBox_TogglesFrameOutlines(t *testing.T) {
	w, rf := setupBodyForDebugCommand(t)
	bf := &boxToggleFrame{recordingFrame: rf}
	w.body.fr = bf

	boxoutlines(&w.body, nil, nil, false, false, "")

	if bf.toggles != 1 {
		t.Errorf("expected 1 ToggleBoxOutlines call; got %d", bf.toggles)
	}
}

// TestExecBox_NilWindow_NoCrash confirms that calling boxoutlines
// with a nil et window does not panic.
func TestExecBox_NilWindow_NoCrash(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("boxoutlines panicked with nil et: %v", r)
		}
	}()
	boxoutlines(nil, nil, nil, false, false, "")
}

// TestExecSpans_TogglesTextOverlay confirms that the "Spans" tag
// command flips Text.showSpans.
func TestExecSpans_TogglesTextOverlay(t *testing.T) {
	w, rf := setupBodyForDebugCommand(t)
	h := &hookRecordingFrame{recordingFrame: rf}
	w.body.fr = h
	w.body.showSpans = false

	spansoverlay(&w.body, nil, nil, false, false, "")

	if !w.body.showSpans {
		t.Errorf("expected showSpans=true after Spans command; got false")
	}
}

// TestExecSpans_NilWindow_NoCrash confirms a nil et window is a
// no-op.
func TestExecSpans_NilWindow_NoCrash(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("spansoverlay panicked with nil et: %v", r)
		}
	}()
	spansoverlay(nil, nil, nil, false, false, "")
}
