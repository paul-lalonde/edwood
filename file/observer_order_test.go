package file

import "testing"

// recordingObserver implements BufferObserver and records the
// order in which its handlers are called via a shared trace.
type recordingObserver struct {
	id    string
	trace *[]string
}

func (r *recordingObserver) Inserted(_ OffsetTuple, _ []byte, _ int) {
	*r.trace = append(*r.trace, r.id)
}

func (r *recordingObserver) Deleted(_, _ OffsetTuple) {
	*r.trace = append(*r.trace, r.id)
}

func TestObservers_FireInRegistrationOrder(t *testing.T) {
	oeb := NewObservableEditableBuffer()

	var trace []string
	a := &recordingObserver{id: "A", trace: &trace}
	b := &recordingObserver{id: "B", trace: &trace}
	c := &recordingObserver{id: "C", trace: &trace}
	oeb.AddObserver(a)
	oeb.AddObserver(b)
	oeb.AddObserver(c)

	oeb.InsertAt(0, []rune("x"))
	if got, want := trace, []string{"A", "B", "C"}; !equalStrings(got, want) {
		t.Errorf("Inserted trace = %v, want %v", got, want)
	}

	trace = nil
	oeb.DeleteAt(0, 1)
	if got, want := trace, []string{"A", "B", "C"}; !equalStrings(got, want) {
		t.Errorf("Deleted trace = %v, want %v", got, want)
	}
}

func TestObservers_AddObserverDedupesRegistration(t *testing.T) {
	oeb := NewObservableEditableBuffer()
	var trace []string
	a := &recordingObserver{id: "A", trace: &trace}
	oeb.AddObserver(a)
	oeb.AddObserver(a) // duplicate
	if got := oeb.GetObserverSize(); got != 1 {
		t.Errorf("after duplicate AddObserver: GetObserverSize() = %d, want 1", got)
	}
	oeb.InsertAt(0, []rune("x"))
	if got, want := trace, []string{"A"}; !equalStrings(got, want) {
		t.Errorf("trace = %v, want %v (observer must fire once despite duplicate add)", got, want)
	}
}

func TestObservers_DelObserverPreservesOrderOfRest(t *testing.T) {
	oeb := NewObservableEditableBuffer()
	var trace []string
	a := &recordingObserver{id: "A", trace: &trace}
	b := &recordingObserver{id: "B", trace: &trace}
	c := &recordingObserver{id: "C", trace: &trace}
	oeb.AddObserver(a)
	oeb.AddObserver(b)
	oeb.AddObserver(c)
	if err := oeb.DelObserver(b); err != nil {
		t.Fatalf("DelObserver(b): %v", err)
	}
	oeb.InsertAt(0, []rune("x"))
	if got, want := trace, []string{"A", "C"}; !equalStrings(got, want) {
		t.Errorf("trace after deleting B = %v, want %v", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
