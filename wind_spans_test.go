package main

import (
	"image"
	"testing"

	"github.com/rjkroege/edwood/edwoodtest"
	"github.com/rjkroege/edwood/spans"
)

// A4.1 — Window construction threads a spans.Store onto the body
// Text. Tags do not get a spans store. The store registers itself
// on the body buffer's observer chain BEFORE the Text observer
// so that buffer mutations propagate to spans before Text reacts
// (per design §8.1).

func TestA41_BodyHasSpansAfterInit(t *testing.T) {
	display := edwoodtest.NewDisplay(image.Rectangle{})
	global.configureGlobals(display)

	w := NewWindow().initHeadless(nil)
	if w.body.spans == nil {
		t.Errorf("w.body.spans = nil, want non-nil store")
	}
}

func TestA41_TagHasNilSpans(t *testing.T) {
	display := edwoodtest.NewDisplay(image.Rectangle{})
	global.configureGlobals(display)

	w := NewWindow().initHeadless(nil)
	if w.tag.spans != nil {
		t.Errorf("w.tag.spans = %v, want nil (tags carry no styling)", w.tag.spans)
	}
}

func TestA41_SpansRegisteredBeforeText(t *testing.T) {
	display := edwoodtest.NewDisplay(image.Rectangle{})
	global.configureGlobals(display)

	w := NewWindow().initHeadless(nil)

	// Walk the body buffer's observer chain in registration order.
	// The spans store must appear before the body Text.
	var seenSpans, seenBody bool
	var spansIdx, bodyIdx int
	idx := 0
	w.body.file.AllObservers(func(o interface{}) {
		if _, ok := o.(spans.Store); ok {
			seenSpans = true
			spansIdx = idx
		}
		if o == &w.body {
			seenBody = true
			bodyIdx = idx
		}
		idx++
	})

	if !seenSpans {
		t.Fatalf("spans store not on body buffer's observer chain")
	}
	if !seenBody {
		t.Fatalf("body Text not on body buffer's observer chain")
	}
	if spansIdx >= bodyIdx {
		t.Errorf("spans registered at idx %d, body Text at idx %d; spans must come first", spansIdx, bodyIdx)
	}
}
