package main

import (
	"image"
	"testing"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/edwoodtest"
)

// fakeScrollModel returns whatever Geometry was set; drag/jump are
// no-op stubs for tests that don't exercise them.
type fakeScrollModel struct {
	totalPx, viewPx, originPx int
}

func (m *fakeScrollModel) Geometry() (int, int, int) {
	return m.totalPx, m.viewPx, m.originPx
}
func (m *fakeScrollModel) DragTopToPixel(int)     {}
func (m *fakeScrollModel) DragPixelToTop(int)     {}
func (m *fakeScrollModel) JumpToFraction(float64) {}

func TestComputeThumbRect_FullTrackWhenViewLargerThanDoc(t *testing.T) {
	track := image.Rect(0, 0, 12, 100)
	got := computeThumbRect(track, 50, 100, 0)
	if !got.Eq(track) {
		t.Errorf("thumb=%v, want full track %v", got, track)
	}
}

func TestComputeThumbRect_ZeroTotal(t *testing.T) {
	track := image.Rect(0, 0, 12, 100)
	got := computeThumbRect(track, 0, 0, 0)
	if !got.Eq(track) {
		t.Errorf("thumb=%v, want full track for empty doc %v", got, track)
	}
}

func TestComputeThumbRect_OriginAtTop(t *testing.T) {
	track := image.Rect(0, 0, 12, 100)
	got := computeThumbRect(track, 1000, 200, 0)
	if got.Min.Y != 0 {
		t.Errorf("thumb.Min.Y=%d, want 0 (origin at top)", got.Min.Y)
	}
	if got.Max.Y != 20 {
		t.Errorf("thumb.Max.Y=%d, want 20 (200/1000 of 100)", got.Max.Y)
	}
}

func TestComputeThumbRect_OriginAtBottom(t *testing.T) {
	track := image.Rect(0, 0, 12, 100)
	// 200px viewport at 800px origin in a 1000px document: thumb spans
	// 80% to 100% of the track.
	got := computeThumbRect(track, 1000, 200, 800)
	if got.Min.Y != 80 {
		t.Errorf("thumb.Min.Y=%d, want 80", got.Min.Y)
	}
	if got.Max.Y != 100 {
		t.Errorf("thumb.Max.Y=%d, want 100", got.Max.Y)
	}
}

func TestComputeThumbRect_ProportionalForMidDocument(t *testing.T) {
	track := image.Rect(0, 0, 12, 100)
	// 200px viewport at 400px origin in a 1000px doc: thumb 40-60.
	got := computeThumbRect(track, 1000, 200, 400)
	if got.Min.Y != 40 {
		t.Errorf("thumb.Min.Y=%d, want 40", got.Min.Y)
	}
	if got.Max.Y != 60 {
		t.Errorf("thumb.Max.Y=%d, want 60", got.Max.Y)
	}
}

func TestComputeThumbRect_MinHeightClamp(t *testing.T) {
	// 1px viewport in a 1M-px document on a 1000px-tall track: a
	// strictly proportional thumb would be sub-pixel. Must clamp to
	// MinThumbHeightPx.
	track := image.Rect(0, 0, 12, 1000)
	got := computeThumbRect(track, 1_000_000, 1, 500_000)
	if got.Dy() < MinThumbHeightPx {
		t.Errorf("thumb height %d < MinThumbHeightPx %d", got.Dy(), MinThumbHeightPx)
	}
	if got.Min.Y < track.Min.Y || got.Max.Y > track.Max.Y {
		t.Errorf("thumb %v escapes track %v", got, track)
	}
}

func TestComputeThumbRect_MinHeightClampPinsToBottom(t *testing.T) {
	// Origin near the very end of a huge document: clamping must pin
	// the thumb to the bottom of the track, not extend past it.
	track := image.Rect(0, 0, 12, 1000)
	got := computeThumbRect(track, 1_000_000, 1, 999_999)
	if got.Max.Y > track.Max.Y {
		t.Errorf("thumb.Max.Y=%d exceeds track.Max.Y=%d", got.Max.Y, track.Max.Y)
	}
	if got.Dy() < MinThumbHeightPx {
		t.Errorf("thumb height %d < MinThumbHeightPx %d", got.Dy(), MinThumbHeightPx)
	}
}

func TestComputeThumbRect_LargeDocOverflowGuard(t *testing.T) {
	// totalPx > 1<<20 triggers the >>10 internal downscale. Must
	// produce a sane thumb (within bounds, roughly proportional).
	track := image.Rect(0, 0, 12, 100)
	got := computeThumbRect(track, 1<<24, 1<<20, 1<<23) // origin at 50%
	if got.Min.Y < 0 || got.Max.Y > 100 {
		t.Errorf("thumb %v out of track bounds [0,100]", got)
	}
	if got.Min.Y > got.Max.Y {
		t.Errorf("thumb inverted: %v", got)
	}
	if got.Min.Y < 45 || got.Min.Y > 55 {
		t.Errorf("thumb.Min.Y=%d, want ~50 for origin at 50%%", got.Min.Y)
	}
}

func TestComputeThumbRect_TrackOffsetFromZero(t *testing.T) {
	// Track does not start at Y=0. Thumb must be in track-relative
	// coordinates plus the track offset.
	track := image.Rect(50, 200, 62, 300) // Y from 200 to 300
	got := computeThumbRect(track, 1000, 200, 400)
	// Same as the mid-document test, shifted by 200.
	if got.Min.Y != 240 {
		t.Errorf("thumb.Min.Y=%d, want 240 (track.Min.Y=200 + 40)", got.Min.Y)
	}
	if got.Max.Y != 260 {
		t.Errorf("thumb.Max.Y=%d, want 260 (track.Min.Y=200 + 60)", got.Max.Y)
	}
}

// scrollbarTestHarness sets up a mock display, allocates two color
// images, and returns a Scrollbar bound to a fake model the caller can
// mutate.
func scrollbarTestHarness(t *testing.T) (*Scrollbar, *fakeScrollModel, draw.Display) {
	t.Helper()
	display := edwoodtest.NewDisplay(image.Rect(0, 0, 800, 600))
	track, err := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Yellowgreen)
	if err != nil {
		t.Fatalf("alloc track: %v", err)
	}
	thumb, err := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Paleyellow)
	if err != nil {
		t.Fatalf("alloc thumb: %v", err)
	}
	model := &fakeScrollModel{totalPx: 1000, viewPx: 200, originPx: 400}
	s := NewScrollbar(display, model, track, thumb)
	s.SetRect(image.Rect(0, 0, 12, 100))
	return s, model, display
}

// expectedFirstDrawOps is the count of Draw operations recorded by
// the mock display for a non-cache-hit scrollbar paint:
//   1. Track fill on the scratch image.
//   2. Thumb fill on the scratch image.
//   3. 1px right edge on the scratch image (when localThumb width
//      ≥ 1, which is always true for a non-degenerate scrollbar).
//   4. Blit from scratch image to screen.
const expectedFirstDrawOps = 4

func TestScrollbar_FirstDrawProducesExpectedOpCount(t *testing.T) {
	s, _, display := scrollbarTestHarness(t)
	display.(edwoodtest.GettableDrawOps).Clear()
	s.Draw()
	if got := len(display.(edwoodtest.GettableDrawOps).DrawOps()); got != expectedFirstDrawOps {
		t.Errorf("first Draw recorded %d ops; want %d (track + thumb + edge + blit)",
			got, expectedFirstDrawOps)
	}
}

func TestScrollbar_RepeatedDrawIsNoopWhenStateUnchanged(t *testing.T) {
	s, _, display := scrollbarTestHarness(t)
	s.Draw() // populate cache
	display.(edwoodtest.GettableDrawOps).Clear()
	s.Draw() // model unchanged → cache hit → no screen ops
	if got := len(display.(edwoodtest.GettableDrawOps).DrawOps()); got != 0 {
		t.Errorf("repeated Draw recorded %d ops; want 0 (cache hit)", got)
	}
}

func TestScrollbar_DrawAfterModelChangeRepaints(t *testing.T) {
	s, model, display := scrollbarTestHarness(t)
	s.Draw()
	display.(edwoodtest.GettableDrawOps).Clear()
	model.originPx = 600 // changes thumb rect
	s.Draw()
	if got := len(display.(edwoodtest.GettableDrawOps).DrawOps()); got != expectedFirstDrawOps {
		t.Errorf("Draw after model change recorded %d ops; want %d (full repaint)",
			got, expectedFirstDrawOps)
	}
}

func TestScrollbar_SetRectInvalidatesCache(t *testing.T) {
	s, _, display := scrollbarTestHarness(t)
	s.Draw()
	if s.lastDrawnThumb.Empty() {
		t.Fatal("lastDrawnThumb should be set after first Draw")
	}
	s.SetRect(image.Rect(0, 0, 12, 200)) // different rect
	if !s.lastDrawnThumb.Empty() {
		t.Error("SetRect with new rect must invalidate lastDrawnThumb cache")
	}
	display.(edwoodtest.GettableDrawOps).Clear()
	s.Draw()
	if got := len(display.(edwoodtest.GettableDrawOps).DrawOps()); got == 0 {
		t.Error("Draw after SetRect produced no ops; want repaint")
	}
}

func TestScrollbar_SetRectWithSameRectIsIdempotent(t *testing.T) {
	// Window resize handlers can call SetRect with the existing rect
	// (e.g. during font cache hits). The cache must not be
	// invalidated; otherwise we force an unnecessary repaint.
	s, _, _ := scrollbarTestHarness(t)
	s.Draw()
	cached := s.lastDrawnThumb
	s.SetRect(s.rect) // identical rect
	if !s.lastDrawnThumb.Eq(cached) {
		t.Error("SetRect with identical rect must not invalidate cache")
	}
}

// recordingScrollModel captures every method call so dispatch tests
// can assert exact button-to-method routing.
type recordingScrollModel struct {
	fakeScrollModel
	dragTopCalls    []int
	dragToTopCalls  []int
	jumpFractCalls  []float64
}

func (m *recordingScrollModel) DragTopToPixel(y int) {
	m.dragTopCalls = append(m.dragTopCalls, y)
}
func (m *recordingScrollModel) DragPixelToTop(y int) {
	m.dragToTopCalls = append(m.dragToTopCalls, y)
}
func (m *recordingScrollModel) JumpToFraction(f float64) {
	m.jumpFractCalls = append(m.jumpFractCalls, f)
}

func TestScrollbar_DispatchB1CallsDragTopToPixel(t *testing.T) {
	// Use 37/100 (asymmetric, non-fractional) so any off-by-track-
	// height or off-by-mid-track error would produce an unambiguous
	// wrong value (137 or 63), not coincidentally match.
	model := &recordingScrollModel{}
	s := &Scrollbar{model: model}
	s.dispatch(1, 37, 100)
	s.dispatch(1, 88, 200)
	if got := model.dragTopCalls; len(got) != 2 || got[0] != 37 || got[1] != 88 {
		t.Errorf("dragTopCalls=%v, want [37 88]", got)
	}
	if len(model.dragToTopCalls) != 0 || len(model.jumpFractCalls) != 0 {
		t.Errorf("B1 should not call B2/B3 paths")
	}
}

func TestScrollbar_DispatchB3CallsDragPixelToTop(t *testing.T) {
	model := &recordingScrollModel{}
	s := &Scrollbar{model: model}
	s.dispatch(3, 73, 100)
	s.dispatch(3, 17, 50)
	if got := model.dragToTopCalls; len(got) != 2 || got[0] != 73 || got[1] != 17 {
		t.Errorf("dragToTopCalls=%v, want [73 17]", got)
	}
	if len(model.dragTopCalls) != 0 || len(model.jumpFractCalls) != 0 {
		t.Errorf("B3 should not call B1/B2 paths")
	}
}

func TestScrollbar_DispatchB2CallsJumpToFraction(t *testing.T) {
	model := &recordingScrollModel{}
	s := &Scrollbar{model: model}
	s.dispatch(2, 25, 100) // 25% down the track
	s.dispatch(2, 33, 99)  // exactly 1/3
	if got := model.jumpFractCalls; len(got) != 2 || got[0] != 0.25 || got[1] != 1.0/3 {
		t.Errorf("jumpFractCalls=%v, want [0.25 0.333...]", got)
	}
}

func TestScrollbar_DispatchB2ZeroTrackHeightDoesNotPanic(t *testing.T) {
	model := &recordingScrollModel{}
	s := &Scrollbar{model: model}
	s.dispatch(2, 0, 0) // would divide by zero if not guarded
	if len(model.jumpFractCalls) != 0 {
		t.Errorf("expected no JumpToFraction call when trackHeight=0; got %v", model.jumpFractCalls)
	}
}

func TestScrollbar_DispatchUnknownButtonIsNoop(t *testing.T) {
	model := &recordingScrollModel{}
	s := &Scrollbar{model: model}
	s.dispatch(7, 50, 100)
	if len(model.dragTopCalls)+len(model.dragToTopCalls)+len(model.jumpFractCalls) != 0 {
		t.Errorf("unknown button should be a no-op")
	}
}

func TestScrollbar_DrawWithEmptyRectIsNoop(t *testing.T) {
	s, _, display := scrollbarTestHarness(t)
	s.SetRect(image.Rectangle{}) // empty rect
	display.(edwoodtest.GettableDrawOps).Clear()
	s.Draw()
	if got := len(display.(edwoodtest.GettableDrawOps).DrawOps()); got != 0 {
		t.Errorf("Draw with empty rect produced %d ops; want 0", got)
	}
}

func TestScrollbar_ReallocatesScratchWhenTooSmall(t *testing.T) {
	// Simulate a widget whose scratch image was allocated against a
	// smaller screen (e.g. the screen later grew). Draw must
	// reallocate; otherwise renderThumb's blit would clip.
	s, _, display := scrollbarTestHarness(t)
	screenH := display.ScreenImage().R().Max.Y
	smallH := screenH / 4
	if smallH < 1 {
		t.Fatal("test display too small to simulate resize")
	}
	small, err := display.AllocImage(image.Rect(0, 0, 32, smallH), display.ScreenImage().Pix(), false, draw.Nofill)
	if err != nil {
		t.Fatalf("alloc small tmp: %v", err)
	}
	s.tmp = small
	s.Draw()
	if s.tmp == small {
		t.Error("expected scratch to be reallocated when too small for screen")
	}
	if s.tmp.R().Max.Y < screenH {
		t.Errorf("reallocated scratch height %d < screen height %d", s.tmp.R().Max.Y, screenH)
	}
}

func TestScrollbar_KeepsScratchWhenAdequateSize(t *testing.T) {
	// If the existing scratch is at least as tall as the screen,
	// Draw must not reallocate. (Avoids freeing a perfectly good
	// image on every paint.)
	s, _, _ := scrollbarTestHarness(t)
	s.Draw() // first draw allocates
	original := s.tmp
	s.Draw() // second draw with no change should reuse
	if s.tmp != original {
		t.Error("scratch reallocated unnecessarily on second Draw")
	}
}

func TestClampMouseY_BelowRectClampsToMin(t *testing.T) {
	rect := image.Rect(0, 100, 12, 200)
	withGlobalMouseY(t, 50, func() {
		if got := clampMouseY(rect); got != 100 {
			t.Errorf("clampMouseY=%d, want 100", got)
		}
	})
}

func TestClampMouseY_AboveRectClampsToMax(t *testing.T) {
	rect := image.Rect(0, 100, 12, 200)
	withGlobalMouseY(t, 250, func() {
		// Legacy clamps `>=` to Max (inclusive), not Max-1.
		if got := clampMouseY(rect); got != 200 {
			t.Errorf("clampMouseY=%d, want 200", got)
		}
	})
}

func TestClampMouseY_AtMaxBoundaryClampsToMax(t *testing.T) {
	rect := image.Rect(0, 100, 12, 200)
	withGlobalMouseY(t, 200, func() {
		// `>= Max.Y` triggers clamp; result is Max.Y itself.
		if got := clampMouseY(rect); got != 200 {
			t.Errorf("clampMouseY at boundary=%d, want 200", got)
		}
	})
}

func TestClampMouseY_InsideRectIsUnchanged(t *testing.T) {
	rect := image.Rect(0, 100, 12, 200)
	withGlobalMouseY(t, 150, func() {
		if got := clampMouseY(rect); got != 150 {
			t.Errorf("clampMouseY=%d, want 150", got)
		}
	})
}

// withGlobalMouseY temporarily sets global.mouse.Point.Y to y,
// running fn under the override and restoring on return. Tests of
// clampMouseY use this rather than touching globals directly so
// failures don't leak state into other tests.
func withGlobalMouseY(t *testing.T, y int, fn func()) {
	t.Helper()
	if global == nil {
		global = makeglobals()
	}
	if global.mouse == nil {
		global.mouse = new(draw.Mouse)
	}
	saved := global.mouse.Point
	global.mouse.Point.Y = y
	defer func() { global.mouse.Point = saved }()
	fn()
}

// HandleClick's full latch loop is not unit-tested. It mixes timing,
// global mouse state, and a real-display-only Mousectl.Read (see
// 9fans.net/go/draw/mouse.go); reproducing those in tests requires
// either a real X server or invasive abstraction work. Verification
// of the loop is part of the manual test pass in
// PLAN_unified-scrollbar.md §2.3.
