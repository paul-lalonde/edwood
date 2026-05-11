package frame

import "testing"

// Phase A2.3 stubs. Real behavior arrives in Slice C row C2 when
// tall replaced elements need sub-line scroll. For Slice A the
// contract is: Get returns 0, Set is a no-op.

func TestGetOriginYOffset_DefaultZero(t *testing.T) {
	fr, _ := setupStyledFrame(t)
	if got := fr.GetOriginYOffset(); got != 0 {
		t.Errorf("GetOriginYOffset() = %d, want 0", got)
	}
}

func TestSetOriginYOffset_IsNoOp_InSliceA(t *testing.T) {
	// Setting any value must leave Get returning 0 because Slice A
	// has no tall-element layout machinery yet.
	fr, _ := setupStyledFrame(t)
	fr.SetOriginYOffset(7)
	if got := fr.GetOriginYOffset(); got != 0 {
		t.Errorf("after SetOriginYOffset(7), GetOriginYOffset() = %d, want 0 (stub)", got)
	}
}
