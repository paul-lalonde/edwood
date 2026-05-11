package main

import (
	"image"
	"testing"

	"github.com/rjkroege/edwood/edwoodtest"
	"github.com/rjkroege/edwood/file"
	"github.com/rjkroege/edwood/frame"
)

// recordingFrame embeds MockFrame and additionally records the
// args passed to InsertByte / InsertWithStyle, so A4.2 tests can
// verify Text.Inserted's routing.
type recordingFrame struct {
	*MockFrame

	insertByteCalls int
	lastByteData    []byte
	lastByteP0      int

	insertWithStyleCalls int
	lastWithStyleRunes   []rune
	lastWithStyleP0      int
	lastWithStyleStyles  []frame.StyleRun
}

func newRecordingFrame() *recordingFrame {
	return &recordingFrame{MockFrame: &MockFrame{}}
}

// GetFrameFillStatus reports a large Nchars so Text.Inserted's
// visibility check (`q0 <= t.org + Nchars`) always succeeds in
// tests. The MockFrame default of 0 would gate every test insert
// out of the InsertByte / InsertWithStyle branch.
func (rf *recordingFrame) GetFrameFillStatus() frame.FrameFillStatus {
	return frame.FrameFillStatus{Nchars: 1 << 30}
}

func (rf *recordingFrame) InsertByte(b []byte, p0 int) bool {
	rf.insertByteCalls++
	rf.lastByteData = append([]byte(nil), b...)
	rf.lastByteP0 = p0
	return false
}

func (rf *recordingFrame) InsertWithStyle(r []rune, p0 int, styles []frame.StyleRun) bool {
	rf.insertWithStyleCalls++
	rf.lastWithStyleRunes = append([]rune(nil), r...)
	rf.lastWithStyleP0 = p0
	rf.lastWithStyleStyles = append([]frame.StyleRun(nil), styles...)
	return false
}

// setupBodyForInsertedTest builds a Window via initHeadless, then
// outfits w.body with a recording frame and the minimal field set
// Text.Inserted's body path requires. Spans is left as
// initHeadless set it (non-nil); tests that need nil/empty set or
// clear it themselves.
//
// We do NOT drive the buffer here — tests call w.body.Inserted
// directly so that the full tag-status-observer chain (UpdateTag,
// setTag1, Resize) stays out of scope. When a test needs the
// spans store to be pre-updated (as the buffer-driven order would
// do via the spans.Store observer firing first), it calls
// updateSpansForInserted explicitly.
func setupBodyForInsertedTest(t *testing.T) (*Window, *recordingFrame) {
	t.Helper()
	display := edwoodtest.NewDisplay(image.Rectangle{})
	global.configureGlobals(display)
	w := NewWindow().initHeadless(nil)
	rf := newRecordingFrame()
	w.body.fr = rf
	w.body.what = Body
	return w, rf
}

// updateSpansForInserted simulates the buffer-driven order: spans
// observer fires before Text observer. We let tests call this
// before w.body.Inserted to keep spans aligned without going
// through the real buffer.
func updateSpansForInserted(w *Window, q0, nr int) {
	if s, ok := w.body.spans.(file.BufferObserver); ok {
		s.Inserted(file.Ot(0, q0), nil, nr)
	}
}

func TestA42_Inserted_NilSpans_UsesInsertByte(t *testing.T) {
	w, rf := setupBodyForInsertedTest(t)
	w.body.spans = nil

	w.body.Inserted(file.Ot(0, 0), []byte("hello"), 5)

	if rf.insertByteCalls != 1 {
		t.Errorf("InsertByte calls = %d, want 1", rf.insertByteCalls)
	}
	if rf.insertWithStyleCalls != 0 {
		t.Errorf("InsertWithStyle calls = %d, want 0 (nil spans)", rf.insertWithStyleCalls)
	}
	if string(rf.lastByteData) != "hello" {
		t.Errorf("InsertByte data = %q, want %q", rf.lastByteData, "hello")
	}
}

func TestA42_Inserted_EmptySpans_UsesInsertByte(t *testing.T) {
	// Body spans is non-nil but Empty() — no producer has styled
	// anything. Fast path: InsertByte.
	w, rf := setupBodyForInsertedTest(t)
	if !w.body.spans.Empty() {
		t.Fatalf("fresh body spans should report Empty() == true")
	}

	updateSpansForInserted(w, 0, 5)
	w.body.Inserted(file.Ot(0, 0), []byte("hello"), 5)

	if rf.insertByteCalls != 1 {
		t.Errorf("InsertByte calls = %d, want 1", rf.insertByteCalls)
	}
	if rf.insertWithStyleCalls != 0 {
		t.Errorf("InsertWithStyle calls = %d, want 0 (empty spans)", rf.insertWithStyleCalls)
	}
}

func TestA42_Inserted_StyledSpans_UsesInsertWithStyle(t *testing.T) {
	w, rf := setupBodyForInsertedTest(t)
	// Seed the buffer + spans state to model "hello" already
	// present (so SetRegion below has a valid range).
	updateSpansForInserted(w, 0, 5)

	colored := frame.Style{Kind: frame.KindColored}
	w.body.spans.SetRegion(1, 4, colored)
	if w.body.spans.Empty() {
		t.Fatalf("spans should be non-empty after SetRegion")
	}

	rf.insertByteCalls = 0
	rf.insertWithStyleCalls = 0

	// Insert "X" at q0=2, mid-colored. spans observer fires first
	// (we drive it manually), then Text.Inserted routes through
	// InsertWithStyle.
	updateSpansForInserted(w, 2, 1)
	w.body.Inserted(file.Ot(0, 2), []byte("X"), 1)

	if rf.insertWithStyleCalls != 1 {
		t.Errorf("InsertWithStyle calls = %d, want 1 (non-empty spans)", rf.insertWithStyleCalls)
	}
	if rf.insertByteCalls != 0 {
		t.Errorf("InsertByte calls = %d, want 0", rf.insertByteCalls)
	}
	if string(rf.lastWithStyleRunes) != "X" {
		t.Errorf("InsertWithStyle runes = %q, want %q", string(rf.lastWithStyleRunes), "X")
	}
}

func TestA42_Inserted_StyledSpans_PropagatesCorrectStyles(t *testing.T) {
	w, rf := setupBodyForInsertedTest(t)
	updateSpansForInserted(w, 0, 5)

	colored := frame.Style{Kind: frame.KindColored}
	w.body.spans.SetRegion(1, 4, colored)

	rf.insertByteCalls = 0
	rf.insertWithStyleCalls = 0

	// Insert "XY" at q0=2 (mid-colored). spans extends by 2;
	// Text queries spans for [2, 4) and gets two colored runes.
	updateSpansForInserted(w, 2, 2)
	w.body.Inserted(file.Ot(0, 2), []byte("XY"), 2)

	if got := rf.insertWithStyleCalls; got != 1 {
		t.Fatalf("InsertWithStyle calls = %d, want 1", got)
	}
	sum := 0
	for _, sr := range rf.lastWithStyleStyles {
		sum += sr.Len
	}
	if sum != 2 {
		t.Errorf("sum of styles.Len = %d, want 2; styles=%+v", sum, rf.lastWithStyleStyles)
	}
	for i, sr := range rf.lastWithStyleStyles {
		if sr.Style != colored {
			t.Errorf("styles[%d].Style = %+v, want %+v", i, sr.Style, colored)
		}
	}
}

func TestA42_Inserted_StyledSpans_PlainRangeStillRoutesThroughInsertWithStyle(t *testing.T) {
	// spans has SOME non-plain region but the inserted range is
	// outside it. We still route through InsertWithStyle (spans
	// is non-empty); the styles slice is all plain so the frame's
	// fast path inside InsertWithStyle applies.
	w, rf := setupBodyForInsertedTest(t)
	updateSpansForInserted(w, 0, 5)

	colored := frame.Style{Kind: frame.KindColored}
	w.body.spans.SetRegion(0, 2, colored) // "he" colored

	rf.insertByteCalls = 0
	rf.insertWithStyleCalls = 0

	// Insert "Z" at q0=5 (end of buffer; trailing edge of the
	// plain region that follows the colored). New rune is plain.
	updateSpansForInserted(w, 5, 1)
	w.body.Inserted(file.Ot(0, 5), []byte("Z"), 1)

	if rf.insertWithStyleCalls != 1 {
		t.Errorf("InsertWithStyle calls = %d, want 1 (spans non-empty)", rf.insertWithStyleCalls)
	}
	if rf.insertByteCalls != 0 {
		t.Errorf("InsertByte calls = %d, want 0", rf.insertByteCalls)
	}
	for i, sr := range rf.lastWithStyleStyles {
		if !sr.Style.IsPlain() {
			t.Errorf("styles[%d].Style = %+v, want plain (inserted Z is in a plain area)", i, sr.Style)
		}
	}
}
