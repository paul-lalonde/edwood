package main

import (
	"image"
	"testing"

	"github.com/rjkroege/edwood/edwoodtest"
	"github.com/rjkroege/edwood/file"
	"github.com/rjkroege/edwood/frame"
	"github.com/rjkroege/edwood/spans"
)

// recordingFrame embeds MockFrame and additionally records the
// args passed to InsertByte / InsertWithStyle, so A4.2 tests can
// verify Text.Inserted's routing.
type recordingFrame struct {
	*MockFrame

	// nchars is the Nchars value GetFrameFillStatus reports.
	// Tests adjust this to model how much content the frame is
	// currently displaying — large for visibility-gated paths
	// (Text.Inserted), zero for fill tests that want fill to see
	// the whole buffer as "not yet drawn."
	nchars int

	insertByteCalls int
	lastByteData    []byte
	lastByteP0      int

	insertCalls     int
	lastInsertRunes []rune
	lastInsertP0    int

	insertWithStyleCalls int
	lastWithStyleRunes   []rune
	lastWithStyleP0      int
	lastWithStyleStyles  []frame.StyleRun

	setOriginYOffsetCalls int
	lastSetOriginYPx      int

	setStyleRangeCalls   int
	lastStyleRangeP0     int
	lastStyleRangeP1     int
	lastStyleRangeStyles []frame.StyleRun
}

func newRecordingFrame() *recordingFrame {
	return &recordingFrame{MockFrame: &MockFrame{}}
}

// GetFrameFillStatus reports rf.nchars so tests can model the
// frame's current display state. Maxlines is reported large so
// fill loops see room to add content.
func (rf *recordingFrame) GetFrameFillStatus() frame.FrameFillStatus {
	return frame.FrameFillStatus{Nchars: rf.nchars, Maxlines: 1 << 20}
}

func (rf *recordingFrame) InsertByte(b []byte, p0 int) bool {
	rf.insertByteCalls++
	rf.lastByteData = append([]byte(nil), b...)
	rf.lastByteP0 = p0
	// Model a real frame growing as content is inserted, so
	// fill loops see Nchars increase.
	rf.nchars += len([]rune(string(b)))
	return false
}

func (rf *recordingFrame) Insert(r []rune, p0 int) bool {
	rf.insertCalls++
	rf.lastInsertRunes = append([]rune(nil), r...)
	rf.lastInsertP0 = p0
	rf.nchars += len(r)
	return false
}

func (rf *recordingFrame) InsertWithStyle(r []rune, p0 int, styles []frame.StyleRun) bool {
	rf.insertWithStyleCalls++
	rf.lastWithStyleRunes = append([]rune(nil), r...)
	rf.lastWithStyleP0 = p0
	rf.lastWithStyleStyles = append([]frame.StyleRun(nil), styles...)
	rf.nchars += len(r)
	return false
}

func (rf *recordingFrame) SetOriginYOffset(yPx int) {
	rf.setOriginYOffsetCalls++
	rf.lastSetOriginYPx = yPx
}

func (rf *recordingFrame) SetStyleRange(p0, p1 int, styles []frame.StyleRun) {
	rf.setStyleRangeCalls++
	rf.lastStyleRangeP0 = p0
	rf.lastStyleRangeP1 = p1
	rf.lastStyleRangeStyles = append([]frame.StyleRun(nil), styles...)
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
	rf.nchars = 1 << 16 // model a frame with plenty of visible content
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

// =====================================================================
// A6 — `S` event emission on body selection change (§9.3)
// =====================================================================

// enableEventReader configures w so Eventf actually appends to
// w.events (otherwise it returns early). nopen[QWevent] > 0 is
// the "is anyone listening" gate; owner != 0 is the same
// precondition acme has for any per-window event emission.
func enableEventReader(w *Window) {
	w.nopen[QWevent] = 1
	w.owner = 'E'
}

func TestA6_S_FiresOnBodySelectionChange(t *testing.T) {
	w, _ := setupBodyForInsertedTest(t)
	enableEventReader(w)

	w.body.SetSelect(2, 5)

	if len(w.events) == 0 {
		t.Fatalf("no S event recorded")
	}
	// w.events[0] is the owner byte; the rest is the payload.
	payload := string(w.events[1:])
	if payload != "S2 5 0 0 \n" {
		t.Errorf("event payload = %q, want %q", payload, "S2 5 0 0 \n")
	}
}

func TestA6_S_NotFiredOnTag(t *testing.T) {
	w, _ := setupBodyForInsertedTest(t)
	enableEventReader(w)
	// Wire the tag with the same recording frame so SetSelect on
	// the tag doesn't NPE in DrawSel.
	w.tag.fr = w.body.fr
	w.tag.display = w.body.display
	w.tag.what = Tag

	w.tag.SetSelect(2, 5)

	if len(w.events) != 0 {
		t.Errorf("tag selection fired an event: %q", string(w.events))
	}
}

func TestA6_S_NotFiredWithNilSpans(t *testing.T) {
	w, _ := setupBodyForInsertedTest(t)
	enableEventReader(w)
	w.body.spans = nil

	w.body.SetSelect(2, 5)

	if len(w.events) != 0 {
		t.Errorf("event fired with nil spans: %q", string(w.events))
	}
}

func TestA6_S_NotFiredWithoutListener(t *testing.T) {
	// No call to enableEventReader: nopen[QWevent] stays 0,
	// Eventf returns early.
	w, _ := setupBodyForInsertedTest(t)

	w.body.SetSelect(2, 5)

	if len(w.events) != 0 {
		t.Errorf("event fired with no listener: %q", string(w.events))
	}
}

func TestA6_S_NotFiredWhenSelectionUnchanged(t *testing.T) {
	w, _ := setupBodyForInsertedTest(t)
	enableEventReader(w)
	w.body.q0 = 2
	w.body.q1 = 5

	w.body.SetSelect(2, 5)

	if len(w.events) != 0 {
		t.Errorf("event fired on unchanged selection: %q", string(w.events))
	}
}

func TestA6_S_FiresOnSubsequentChange(t *testing.T) {
	w, _ := setupBodyForInsertedTest(t)
	enableEventReader(w)

	w.body.SetSelect(2, 5)
	// One event captured so far. A second SetSelect with new
	// bounds should fire another event.
	beforeLen := len(w.events)
	w.body.SetSelect(3, 7)

	if len(w.events) <= beforeLen {
		t.Errorf("second SetSelect did not fire; events=%q", string(w.events))
	}
}

// =====================================================================
// A4.3 — style-aware fill and setorigin
// =====================================================================

// setupTextForFillTest gives the body Text a pre-loaded buffer
// and a spans store keyed off it. This bypasses initHeadless's
// empty-buffer setup so fill has runes to read.
func setupTextForFillTest(t *testing.T, content string) (*Window, *recordingFrame) {
	t.Helper()
	display := edwoodtest.NewDisplay(image.Rectangle{})
	global.configureGlobals(display)
	w := NewWindow().initHeadless(nil)

	// Replace the body buffer with one carrying our test content,
	// and build a fresh spans.Store keyed off it. attachSpans
	// registers the Observe callback that the A4.4 tests assert
	// on.
	buf := file.MakeObservableEditableBuffer("test", []rune(content))
	w.body.file = buf
	w.body.attachSpans(spans.NewStore(buf))

	rf := newRecordingFrame()
	w.body.fr = rf
	w.body.display = display
	w.body.what = Body
	w.col = &Column{safe: true}
	w.tag.fr = &MockFrame{}
	return w, rf
}

func TestA43_Fill_NilSpans_UsesInsert(t *testing.T) {
	w, rf := setupTextForFillTest(t, "hello")
	w.body.spans = nil

	if err := w.body.fill(rf); err != nil {
		t.Fatalf("fill: %v", err)
	}
	if rf.insertCalls != 1 {
		t.Errorf("Insert calls = %d, want 1", rf.insertCalls)
	}
	if rf.insertWithStyleCalls != 0 {
		t.Errorf("InsertWithStyle calls = %d, want 0", rf.insertWithStyleCalls)
	}
	if string(rf.lastInsertRunes) != "hello" {
		t.Errorf("Insert runes = %q, want %q", string(rf.lastInsertRunes), "hello")
	}
}

func TestA43_Fill_EmptySpans_UsesInsert(t *testing.T) {
	// spans is non-nil but Empty() — fast path.
	w, rf := setupTextForFillTest(t, "hello")
	if !w.body.spans.Empty() {
		t.Fatalf("spans should be empty after seeding from plain buffer")
	}

	if err := w.body.fill(rf); err != nil {
		t.Fatalf("fill: %v", err)
	}
	if rf.insertCalls != 1 {
		t.Errorf("Insert calls = %d, want 1 (empty spans)", rf.insertCalls)
	}
	if rf.insertWithStyleCalls != 0 {
		t.Errorf("InsertWithStyle calls = %d, want 0", rf.insertWithStyleCalls)
	}
}

func TestA43_Fill_StyledSpans_UsesInsertWithStyle(t *testing.T) {
	w, rf := setupTextForFillTest(t, "hello")
	colored := frame.Style{Kind: frame.KindColored}
	w.body.spans.SetRegion(1, 4, colored) // "ell" colored

	if err := w.body.fill(rf); err != nil {
		t.Fatalf("fill: %v", err)
	}
	if rf.insertWithStyleCalls != 1 {
		t.Errorf("InsertWithStyle calls = %d, want 1 (styled spans)", rf.insertWithStyleCalls)
	}
	if rf.insertCalls != 0 {
		t.Errorf("Insert calls = %d, want 0", rf.insertCalls)
	}
	if string(rf.lastWithStyleRunes) != "hello" {
		t.Errorf("InsertWithStyle runes = %q, want %q", string(rf.lastWithStyleRunes), "hello")
	}
	// Sum-of-Lens invariant.
	sum := 0
	for _, sr := range rf.lastWithStyleStyles {
		sum += sr.Len
	}
	if sum != 5 {
		t.Errorf("sum of styles.Len = %d, want 5; styles=%+v", sum, rf.lastWithStyleStyles)
	}
}

func TestA43_Setorigin_CallsSetOriginYOffset(t *testing.T) {
	// setorigin should call SetOriginYOffset on the frame (Slice A
	// stub returns 0; Slice C will compute a real tall-element
	// y-offset).
	w, rf := setupTextForFillTest(t, "hello world")

	w.body.setorigin(rf, 0, true, false)

	if rf.setOriginYOffsetCalls != 1 {
		t.Errorf("SetOriginYOffset calls = %d, want 1", rf.setOriginYOffsetCalls)
	}
	if rf.lastSetOriginYPx != 0 {
		t.Errorf("SetOriginYOffset arg = %d, want 0 (Slice A stub)", rf.lastSetOriginYPx)
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

// =====================================================================
// A4.4 — attachSpans Observe callback
// =====================================================================

func TestA44_AttachSpans_SetRegionCallsSetStyleRange(t *testing.T) {
	// A SetRegion within the visible window triggers SetStyleRange
	// on the frame, with frame-relative offsets.
	w, rf := setupTextForFillTest(t, "hello world")
	rf.nchars = 11 // frame shows all runes
	w.body.org = 0

	colored := frame.Style{Kind: frame.KindColored}
	w.body.spans.SetRegion(2, 7, colored)

	if rf.setStyleRangeCalls != 1 {
		t.Fatalf("SetStyleRange calls = %d, want 1", rf.setStyleRangeCalls)
	}
	if rf.lastStyleRangeP0 != 2 || rf.lastStyleRangeP1 != 7 {
		t.Errorf("SetStyleRange args = (%d,%d), want (2,7)", rf.lastStyleRangeP0, rf.lastStyleRangeP1)
	}
}

func TestA44_AttachSpans_OutOfVisibleWindow_SkipsSetStyleRange(t *testing.T) {
	// A SetRegion entirely outside [t.org, t.org+Nchars) does NOT
	// fire SetStyleRange — there's no visible region to repaint.
	w, rf := setupTextForFillTest(t, "hello world")
	rf.nchars = 5 // frame shows runes [0, 5)
	w.body.org = 0

	colored := frame.Style{Kind: frame.KindColored}
	w.body.spans.SetRegion(7, 10, colored) // entirely outside visible

	if rf.setStyleRangeCalls != 0 {
		t.Errorf("SetStyleRange calls = %d, want 0 (range outside visible)", rf.setStyleRangeCalls)
	}
}

func TestA44_AttachSpans_ClipsToVisibleWindow(t *testing.T) {
	// A SetRegion partially overlapping the visible window gets
	// clipped before SetStyleRange.
	w, rf := setupTextForFillTest(t, "hello world")
	rf.nchars = 5 // visible window is [0, 5) (runes "hello")
	w.body.org = 0

	colored := frame.Style{Kind: frame.KindColored}
	w.body.spans.SetRegion(3, 8, colored) // overlaps; clip to [3, 5)

	if rf.setStyleRangeCalls != 1 {
		t.Fatalf("SetStyleRange calls = %d, want 1", rf.setStyleRangeCalls)
	}
	if rf.lastStyleRangeP0 != 3 || rf.lastStyleRangeP1 != 5 {
		t.Errorf("SetStyleRange args = (%d,%d), want (3,5)", rf.lastStyleRangeP0, rf.lastStyleRangeP1)
	}
}

func TestA44_AttachSpans_NonZeroOrigin_ConvertsToFrameRelative(t *testing.T) {
	// With t.org > 0, the visible window starts at some
	// document-absolute offset. SetStyleRange is called with
	// FRAME-relative offsets (p0 - t.org).
	w, rf := setupTextForFillTest(t, "hello world")
	rf.nchars = 8
	w.body.org = 3 // visible window: [3, 11) — runes "lo world"

	colored := frame.Style{Kind: frame.KindColored}
	w.body.spans.SetRegion(5, 9, colored) // doc-absolute [5, 9)

	if rf.setStyleRangeCalls != 1 {
		t.Fatalf("SetStyleRange calls = %d, want 1", rf.setStyleRangeCalls)
	}
	// Frame-relative: (5-3, 9-3) = (2, 6).
	if rf.lastStyleRangeP0 != 2 || rf.lastStyleRangeP1 != 6 {
		t.Errorf("SetStyleRange args = (%d,%d), want frame-relative (2,6)", rf.lastStyleRangeP0, rf.lastStyleRangeP1)
	}
}
