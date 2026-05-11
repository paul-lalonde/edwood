package spans

import (
	"testing"

	"github.com/rjkroege/edwood/file"
)

// Observe (§6.1) fires fn(p0, p1) for style-only updates —
// SetRegion and ClearRegion. Buffer-driven shifts (Inserted /
// Deleted) are bookkeeping and do NOT fire Observe.

type callRecord struct {
	p0, p1 int
}

func TestObserve_FiresOnSetRegion(t *testing.T) {
	s := newStoreWithLen(10)
	var calls []callRecord
	s.Observe(func(p0, p1 int) {
		calls = append(calls, callRecord{p0, p1})
	})
	s.SetRegion(2, 6, colored)
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(calls), calls)
	}
	if calls[0] != (callRecord{2, 6}) {
		t.Errorf("call = %+v, want {2,6}", calls[0])
	}
}

func TestObserve_FiresOnClearRegion(t *testing.T) {
	s := newStoreWithLen(10)
	s.SetRegion(0, 10, colored)
	var calls []callRecord
	s.Observe(func(p0, p1 int) {
		calls = append(calls, callRecord{p0, p1})
	})
	s.ClearRegion(3, 7)
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1: %+v", len(calls), calls)
	}
	if calls[0] != (callRecord{3, 7}) {
		t.Errorf("call = %+v, want {3,7}", calls[0])
	}
}

func TestObserve_NotFiredByInsertedDeleted(t *testing.T) {
	s := newStoreWithLen(10)
	called := false
	s.Observe(func(p0, p1 int) { called = true })
	s.Inserted(file.Ot(0, 5), nil, 3)
	if called {
		t.Errorf("Observe fired on Inserted; should not")
	}
	called = false
	s.Deleted(file.Ot(0, 2), file.Ot(0, 5))
	if called {
		t.Errorf("Observe fired on Deleted; should not")
	}
}

func TestObserve_MultipleObservers(t *testing.T) {
	s := newStoreWithLen(10)
	calls1, calls2 := 0, 0
	s.Observe(func(p0, p1 int) { calls1++ })
	s.Observe(func(p0, p1 int) { calls2++ })
	s.SetRegion(0, 4, colored)
	s.SetRegion(6, 10, colored)
	if calls1 != 2 {
		t.Errorf("observer 1: got %d calls, want 2", calls1)
	}
	if calls2 != 2 {
		t.Errorf("observer 2: got %d calls, want 2", calls2)
	}
}

func TestObserve_NotFiredOnEmptyRange(t *testing.T) {
	// SetRegion / ClearRegion with p0 >= p1 is a no-op and must
	// not fire the callback.
	s := newStoreWithLen(10)
	called := false
	s.Observe(func(p0, p1 int) { called = true })
	s.SetRegion(5, 5, colored)
	s.ClearRegion(5, 5)
	if called {
		t.Errorf("Observe fired on empty range; should not")
	}
}
