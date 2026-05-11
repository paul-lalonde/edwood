package file

// BufferObserver implementations can register themselves
// with an ObservableEditableBuffer so the observers can be
// notified of all buffer mutations made.
type BufferObserver interface {

	// inserted informs the implementer that byte array b was inserted at position q0.
	Inserted(q0 OffsetTuple, b []byte, nr int)

	// deleted informs the implementer that character range [q0,q1) was deleted.
	Deleted(q0, q1 OffsetTuple)
}

// AuxiliaryObserver is an optional interface a BufferObserver
// may implement to declare itself a *sidecar* observer — present
// on the chain for bookkeeping (e.g. styled-text spans), not as
// a primary view of the buffer. HasMultipleObservers excludes
// auxiliary observers from its count so callers can continue to
// use it as a "is this buffer shared by multiple Texts/clones?"
// check.
type AuxiliaryObserver interface {
	IsAuxiliary() bool
}
