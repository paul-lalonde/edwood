package mdrender

import (
	"image"
	"testing"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/edwoodtest"
	"github.com/rjkroege/edwood/rich"
)

// newTestFrameOnDisplay builds a minimal rich.Frame ready to
// render, using a caller-supplied display so the test can pass
// that same display to mdrender.New(...).
func newTestFrameOnDisplay(t *testing.T, display draw.Display, rect image.Rectangle) rich.Frame {
	t.Helper()
	font := edwoodtest.NewFont(10, 14)
	bg, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0xFFFFFFFF)
	fg, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0x000000FF)
	f := rich.NewFrame()
	f.Init(rect,
		rich.WithDisplay(display),
		rich.WithFont(font),
		rich.WithBackground(bg),
		rich.WithTextColor(fg),
	)
	f.SetContent(rich.Plain("hello world"))
	return f
}

// TestNewReturnsNonNilForValidArgs covers R1 (positive path):
// New with non-nil frame and display returns a non-nil *Renderer
// that holds the supplied frame.
func TestNewReturnsNonNilForValidArgs(t *testing.T) {
	rect := image.Rect(0, 0, 200, 200)
	d := edwoodtest.NewDisplay(rect)
	f := newTestFrameOnDisplay(t, d, rect)
	r := New(f, d)
	if r == nil {
		t.Fatal("New returned nil for valid args")
	}
	if r.Frame() != f {
		t.Errorf("Renderer.Frame() = %p, want supplied frame %p", r.Frame(), f)
	}
}

// TestNewPanicsOnNilFrame covers R1 (negative path, frame):
// New(nil, display) panics rather than constructing a silently-
// broken Renderer.
func TestNewPanicsOnNilFrame(t *testing.T) {
	d := edwoodtest.NewDisplay(image.Rect(0, 0, 200, 200))
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("New(nil, display) did not panic")
		}
		assertNonEmptyPanic(t, r, "frame")
	}()
	_ = New(nil, d)
}

// TestNewPanicsOnNilDisplay covers R1 (negative path, display):
// New(frame, nil) panics. Required for the wrapper to draw
// decorations on top of the frame's blitted output.
func TestNewPanicsOnNilDisplay(t *testing.T) {
	rect := image.Rect(0, 0, 200, 200)
	d := edwoodtest.NewDisplay(rect)
	f := newTestFrameOnDisplay(t, d, rect)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("New(frame, nil) did not panic")
		}
		assertNonEmptyPanic(t, r, "display")
	}()
	_ = New(f, nil)
}

// assertNonEmptyPanic checks that a recovered panic carries an
// informative string or error message, with bonus credit if the
// message names the offending argument.
func assertNonEmptyPanic(t *testing.T, recovered interface{}, expectedMention string) {
	t.Helper()
	var msg string
	switch v := recovered.(type) {
	case string:
		msg = v
	case error:
		msg = v.Error()
	default:
		t.Errorf("panic value not a string or error: %T %v", recovered, recovered)
		return
	}
	if len(msg) < 5 {
		t.Errorf("panic message too short or empty: %q", msg)
	}
}

// TestRedrawIsPassThrough covers R2: calling Redraw on a wrapped
// frame produces the same draw operations as calling Redraw on the
// frame directly. At Phase 1.1 the wrapper has no paint phases of
// its own, so this equivalence must hold for any content the frame
// can render.
func TestRedrawIsPassThrough(t *testing.T) {
	// Build two independent frame+display pairs with identical
	// content so a difference in draw ops can only come from the
	// wrapping itself, not from shared cache state or display
	// state contamination.
	rect := image.Rect(0, 0, 200, 200)
	d1 := edwoodtest.NewDisplay(rect)
	d2 := edwoodtest.NewDisplay(rect)
	font := edwoodtest.NewFont(10, 14)

	build := func(d draw.Display) rich.Frame {
		bg, _ := d.AllocImage(image.Rect(0, 0, 1, 1), d.ScreenImage().Pix(), true, 0xFFFFFFFF)
		fg, _ := d.AllocImage(image.Rect(0, 0, 1, 1), d.ScreenImage().Pix(), true, 0x000000FF)
		ff := rich.NewFrame()
		ff.Init(rect,
			rich.WithDisplay(d),
			rich.WithFont(font),
			rich.WithBackground(bg),
			rich.WithTextColor(fg),
		)
		ff.SetContent(rich.Plain("hello world"))
		return ff
	}

	frameDirect := build(d1)
	frameWrapped := build(d2)

	d1.(edwoodtest.GettableDrawOps).Clear()
	d2.(edwoodtest.GettableDrawOps).Clear()

	frameDirect.Redraw()
	New(frameWrapped, d2).Redraw()

	opsDirect := d1.(edwoodtest.GettableDrawOps).DrawOps()
	opsWrapped := d2.(edwoodtest.GettableDrawOps).DrawOps()

	if len(opsDirect) != len(opsWrapped) {
		t.Fatalf("Redraw op count differs: direct=%d, wrapped=%d\ndirect=%v\nwrapped=%v",
			len(opsDirect), len(opsWrapped), opsDirect, opsWrapped)
	}
	for i := range opsDirect {
		if opsDirect[i] != opsWrapped[i] {
			t.Errorf("Redraw op %d differs:\n  direct  = %q\n  wrapped = %q", i, opsDirect[i], opsWrapped[i])
		}
	}
}

// TestFrameAccessorReturnsWrappedFrame covers R3: the Frame() getter
// returns the same frame instance supplied to New, so callers in the
// transition can drive the frame directly when wrapper-side methods
// don't yet exist.
func TestFrameAccessorReturnsWrappedFrame(t *testing.T) {
	rect := image.Rect(0, 0, 200, 200)
	d := edwoodtest.NewDisplay(rect)
	f := newTestFrameOnDisplay(t, d, rect)
	r := New(f, d)
	if got := r.Frame(); got != f {
		t.Errorf("Frame() = %p, want %p", got, f)
	}
}
