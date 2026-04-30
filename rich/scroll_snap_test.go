package rich

import (
	"image"
	"testing"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/edwoodtest"
)

// snapTestFixture builds a frame with content that overflows the
// viewport: 5 lines × 14 px = 70 px in a 50 px frame. Caller may
// set origin > 0 to land mid-document where SnapBottom has actual
// work to do (file-top override forces SnapTop at origin=0).
//
// Caller may pass extra Options (e.g. WithDefaultScrollSnap) to
// configure the frame.
func snapTestFixture(t *testing.T, extraOpts ...Option) (*frameImpl, Content) {
	t.Helper()
	rect := image.Rect(0, 0, 400, 50)
	display := edwoodtest.NewDisplay(rect)
	font := edwoodtest.NewFont(10, 14)
	bg, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.White)
	text, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Black)

	opts := []Option{
		WithDisplay(display), WithBackground(bg), WithFont(font), WithTextColor(text),
	}
	opts = append(opts, extraOpts...)

	f := NewFrame()
	f.Init(opts...)
	f.SetRect(rect)
	content := Plain("aaa\nbbb\nccc\nddd\neee")
	f.SetContent(content)
	return f.(*frameImpl), content
}

// TestScrollSnap_DefaultIsSnapTop pins the new default. A freshly-
// constructed frame should not shift content up — the first line
// aligns to Y=0.
func TestScrollSnap_DefaultIsSnapTop(t *testing.T) {
	f, _ := snapTestFixture(t)
	if got := f.ScrollSnap(); got != SnapTop {
		t.Errorf("default ScrollSnap = %v, want SnapTop", got)
	}
	lines, _ := f.layoutFromOrigin()
	if len(lines) == 0 {
		t.Fatal("layoutFromOrigin returned no lines")
	}
	if lines[0].Y != 0 {
		t.Errorf("default snap: lines[0].Y = %d, want 0 (top-aligned)", lines[0].Y)
	}
}

// TestScrollSnap_BottomShiftsLastLineToFrameBottom is the legacy
// snapBottomLine behavior under its new name. Mid-document (origin
// past line 0 to defeat the file-top override): with 50-px frame
// and 4-line viewport (56 px), SnapBottom shifts up by 6 so the
// last line ends at Y=50 (frame bottom).
func TestScrollSnap_BottomShiftsLastLineToFrameBottom(t *testing.T) {
	f, _ := snapTestFixture(t, WithDefaultScrollSnap(SnapBottom))
	f.SetOrigin(4) // start of "bbb" — past origin=0 to defeat file-top override
	f.SetScrollSnap(SnapBottom)
	lines, _ := f.layoutFromOrigin()
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 visible lines, got %d", len(lines))
	}
	// Find the last visible line (Y < frameHeight).
	frameHeight := 50
	lastIdx := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if lines[i].Y < frameHeight {
			lastIdx = i
			break
		}
	}
	if lastIdx < 0 {
		t.Fatal("no line within viewport")
	}
	last := lines[lastIdx]
	if last.Y+last.Height != frameHeight {
		t.Errorf("SnapBottom: last visible line bottom = %d, want %d",
			last.Y+last.Height, frameHeight)
	}
	if lines[0].Y >= 0 {
		t.Errorf("SnapBottom: lines[0].Y = %d, want negative (top absorbs clip)",
			lines[0].Y)
	}
}

// TestScrollSnap_TopDoesNotShift complements the above. Mid-document
// with SnapTop, the first visible line stays at Y=0 (its top
// aligns with viewport top); the last line may be partially clipped
// at the bottom.
func TestScrollSnap_TopDoesNotShift(t *testing.T) {
	f, _ := snapTestFixture(t, WithDefaultScrollSnap(SnapTop))
	f.SetOrigin(4) // mid-document
	f.SetScrollSnap(SnapTop)
	lines, _ := f.layoutFromOrigin()
	if len(lines) == 0 {
		t.Fatal("layoutFromOrigin returned no lines")
	}
	if lines[0].Y != 0 {
		t.Errorf("SnapTop mid-document: lines[0].Y = %d, want 0", lines[0].Y)
	}
}

// TestScrollSnap_FileTopForcesSnapTop is the regression test for
// the bug the user reported: at origin=0, the user must be able to
// see the first line aligned to viewport top regardless of the
// configured snap. Even with SnapBottom configured, the
// origin=0/offset=0 override forces SnapTop.
func TestScrollSnap_FileTopForcesSnapTop(t *testing.T) {
	f, _ := snapTestFixture(t, WithDefaultScrollSnap(SnapBottom))
	// Origin=0, offset=0 (the file-top condition).
	f.SetOrigin(0)
	f.SetOriginYOffset(0)
	lines, _ := f.layoutFromOrigin()
	if len(lines) == 0 {
		t.Fatal("layoutFromOrigin returned no lines")
	}
	if lines[0].Y != 0 {
		t.Errorf("at origin=0 with SnapBottom configured: lines[0].Y = %d, "+
			"want 0 (file-top must override snap to keep first line visible)",
			lines[0].Y)
	}
}

// TestScrollSnap_TallLineForcesSnapPixel: when the origin line is
// taller than the frame (e.g. a large image), no line-boundary
// snap should apply — the user must be able to scroll within the
// line pixel-by-pixel via originYOffset.
func TestScrollSnap_TallLineForcesSnapPixel(t *testing.T) {
	rect := image.Rect(0, 0, 400, 50)
	display := edwoodtest.NewDisplay(rect)
	font := edwoodtest.NewFont(10, 14)
	bg, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.White)
	text, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Black)

	f := NewFrame()
	f.Init(WithDisplay(display), WithBackground(bg), WithFont(font),
		WithTextColor(text), WithDefaultScrollSnap(SnapBottom))
	f.SetRect(rect)
	fi := f.(*frameImpl)

	// Synthesize a single tall line by setting the origin line's
	// Height directly on the cached layout. Cleaner than
	// constructing real image content because the snap logic only
	// depends on Height vs frameHeight.
	f.SetContent(Plain("xxx"))
	_, _ = fi.layoutFromOrigin()
	if len(fi.cachedBaseLines) == 0 {
		t.Fatal("no cached lines to mutate")
	}
	fi.cachedBaseLines[0].Height = 200 // taller than frame height (50)

	// Origin in the middle of the tall line.
	f.SetOrigin(0)
	f.SetOriginYOffset(75)
	lines, _ := fi.layoutFromOrigin()
	if len(lines) == 0 {
		t.Fatal("layoutFromOrigin returned no lines")
	}
	if lines[0].Y != -75 {
		t.Errorf("tall-line override: lines[0].Y = %d, want -75 "+
			"(originYOffset honored, no snap applied)", lines[0].Y)
	}
}

// TestSetOrigin_ResetsToSnapTop pins the contract that programmatic
// scrolls (Look, search, auto-scroll) don't inherit a stale
// SnapBottom from a prior B1 click. SetOrigin always resets snap
// to SnapTop.
func TestSetOrigin_ResetsToSnapTop(t *testing.T) {
	f, _ := snapTestFixture(t)
	f.SetScrollSnap(SnapBottom)
	if got := f.ScrollSnap(); got != SnapBottom {
		t.Fatalf("SetScrollSnap precondition failed: got %v", got)
	}
	f.SetOrigin(3)
	if got := f.ScrollSnap(); got != SnapTop {
		t.Errorf("after SetOrigin: ScrollSnap = %v, want SnapTop "+
			"(programmatic scrolls reset snap)", got)
	}
}

// TestSetOriginYOffset_ResetsToSnapTop: same contract for the pixel-
// offset entry point.
func TestSetOriginYOffset_ResetsToSnapTop(t *testing.T) {
	f, _ := snapTestFixture(t)
	f.SetScrollSnap(SnapBottom)
	f.SetOriginYOffset(5)
	if got := f.ScrollSnap(); got != SnapTop {
		t.Errorf("after SetOriginYOffset: ScrollSnap = %v, want SnapTop", got)
	}
}

// TestSetScrollSnap_StoresValue is the trivial setter/getter test.
func TestSetScrollSnap_StoresValue(t *testing.T) {
	f, _ := snapTestFixture(t)
	for _, want := range []ScrollSnap{SnapTop, SnapBottom, SnapPixel} {
		f.SetScrollSnap(want)
		if got := f.ScrollSnap(); got != want {
			t.Errorf("SetScrollSnap(%v); ScrollSnap() = %v", want, got)
		}
	}
}
