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

func TestScrollbar_FirstDrawProducesOps(t *testing.T) {
	s, _, display := scrollbarTestHarness(t)
	display.(edwoodtest.GettableDrawOps).Clear()
	s.Draw()
	if got := len(display.(edwoodtest.GettableDrawOps).DrawOps()); got == 0 {
		t.Error("first Draw produced no ops")
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
	if got := len(display.(edwoodtest.GettableDrawOps).DrawOps()); got == 0 {
		t.Error("Draw after model change produced no ops; want repaint")
	}
}

func TestScrollbar_SetRectInvalidatesCache(t *testing.T) {
	s, _, display := scrollbarTestHarness(t)
	s.Draw()
	display.(edwoodtest.GettableDrawOps).Clear()
	s.SetRect(image.Rect(0, 0, 12, 200)) // different rect
	s.Draw()
	if got := len(display.(edwoodtest.GettableDrawOps).DrawOps()); got == 0 {
		t.Error("Draw after SetRect produced no ops; want repaint")
	}
}
