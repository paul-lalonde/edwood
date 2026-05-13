package spans

import (
	"testing"

	"github.com/rjkroege/edwood/frame"
)

// Phase B2.2 R4 — spans parser surfaces scale=N.N as
// frame.KindScale plus Directive.Scale (float32). Pre-R4 the
// parser silently accepted scale= and dropped the value; R4
// surfaces it.

func TestParseDirective_Scale_SetsKindAndValue(t *testing.T) {
	d, err := ParseDirective("s 0 5 - - scale=1.5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Kind&frame.KindScale == 0 {
		t.Errorf("Kind=%v, want KindScale bit set", d.Kind)
	}
	if d.Scale != 1.5 {
		t.Errorf("Scale=%v, want 1.5", d.Scale)
	}
}

func TestParseDirective_Scale_IntegerForm(t *testing.T) {
	// scale=2 (no dot) is well-formed per the protocol — accept it.
	d, err := ParseDirective("s 0 5 - - scale=2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Scale != 2.0 {
		t.Errorf("Scale=%v, want 2.0", d.Scale)
	}
}

func TestParseDirective_Scale_Malformed_ReturnsError(t *testing.T) {
	if _, err := ParseDirective("s 0 5 - - scale=abc"); err == nil {
		t.Errorf("expected error for scale=abc; got nil")
	}
}

func TestParseDirective_NoScale_LeavesZero(t *testing.T) {
	d, err := ParseDirective("s 0 5 - - bold")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Kind&frame.KindScale != 0 {
		t.Errorf("Kind=%v has KindScale bit, want it clear", d.Kind)
	}
	if d.Scale != 0 {
		t.Errorf("Scale=%v, want 0 (no scale directive)", d.Scale)
	}
}
