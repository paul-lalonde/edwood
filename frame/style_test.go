package frame

import (
	"testing"
)

func TestKindPlain_IsZeroValue(t *testing.T) {
	// Anchors the design contract: KindPlain is exactly the zero
	// value of Kind, so a zero-value Style is plain.
	if KindPlain != 0 {
		t.Errorf("KindPlain = %d, want 0", KindPlain)
	}
}

func TestStyleIsPlain_ZeroValue(t *testing.T) {
	var s Style
	if !s.IsPlain() {
		t.Errorf("Style{}.IsPlain() = false, want true")
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
	if !r.Style.IsPlain() {
		t.Errorf("StyleRun{Style: Style{}}.Style.IsPlain() = false, want true")
	}
}
