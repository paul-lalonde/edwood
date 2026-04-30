package mdrender

import (
	"image"
	"testing"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/edwoodtest"
	"github.com/rjkroege/edwood/rich"
)

// newTestFrame builds a minimal rich.Frame ready to render. Used by
// the tests below to make a Renderer to wrap.
func newTestFrame(t *testing.T) rich.Frame {
	t.Helper()
	rect := image.Rect(0, 0, 200, 200)
	display := edwoodtest.NewDisplay(rect)
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

// TestNewReturnsNonNilForValidFrame covers R1 (positive path):
// New with a non-nil frame returns a non-nil *Renderer that holds
// the supplied frame.
func TestNewReturnsNonNilForValidFrame(t *testing.T) {
	f := newTestFrame(t)
	r := New(f)
	if r == nil {
		t.Fatal("New returned nil for a valid frame")
	}
	if r.Frame() != f {
		t.Errorf("Renderer.Frame() = %p, want supplied frame %p", r.Frame(), f)
	}
}

// TestNewPanicsOnNilFrame covers R1 (negative path):
// New(nil) panics rather than constructing a silently-broken
// Renderer. The panic message must be informative.
func TestNewPanicsOnNilFrame(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("New(nil) did not panic")
		}
		msg, ok := r.(string)
		if !ok {
			// Allow error-typed panics too, but we want SOMETHING
			// readable about what went wrong.
			if e, eok := r.(error); eok {
				msg = e.Error()
			}
		}
		if len(msg) < 5 {
			t.Errorf("New(nil) panic message too short or empty: %q", msg)
		}
	}()
	_ = New(nil)
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
	New(frameWrapped).Redraw()

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
	f := newTestFrame(t)
	r := New(f)
	if got := r.Frame(); got != f {
		t.Errorf("Frame() = %p, want %p", got, f)
	}
}
