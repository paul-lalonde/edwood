package frame

import (
	"image"
	"testing"

	"github.com/rjkroege/edwood/edwoodtest"
)

func TestStyleIsZero_ZeroValue(t *testing.T) {
	var s Style
	if !s.IsZero() {
		t.Errorf("Style{}.IsZero() = false, want true")
	}
}

func TestStyleIsZero_FgSet(t *testing.T) {
	disp := edwoodtest.NewDisplay(image.Rect(0, 0, 1, 1))
	s := Style{Fg: disp.White()}
	if s.IsZero() {
		t.Errorf("Style{Fg: <non-nil>}.IsZero() = true, want false")
	}
}

func TestStyleIsZero_BgSet(t *testing.T) {
	disp := edwoodtest.NewDisplay(image.Rect(0, 0, 1, 1))
	s := Style{Bg: disp.White()}
	if s.IsZero() {
		t.Errorf("Style{Bg: <non-nil>}.IsZero() = true, want false")
	}
}

func TestStyleIsZero_BothSet(t *testing.T) {
	disp := edwoodtest.NewDisplay(image.Rect(0, 0, 1, 1))
	s := Style{Fg: disp.White(), Bg: disp.Black()}
	if s.IsZero() {
		t.Errorf("Style{Fg, Bg: <non-nil>}.IsZero() = true, want false")
	}
}

func TestReplacedKind_ZeroIsNone(t *testing.T) {
	if ReplacedKind(0) != ReplacedNone {
		t.Errorf("ReplacedKind(0) = %v, want ReplacedNone", ReplacedKind(0))
	}
}

func TestStyleRun_StructLayout(t *testing.T) {
	r := StyleRun{Len: 3, Style: Style{}}
	if r.Len != 3 {
		t.Errorf("StyleRun{Len:3}.Len = %d, want 3", r.Len)
	}
	if !r.Style.IsZero() {
		t.Errorf("StyleRun{Style: Style{}}.Style.IsZero() = false, want true")
	}
}
