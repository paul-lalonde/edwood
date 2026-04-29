package main

import (
	"image"
	"testing"

	"github.com/rjkroege/edwood/file"
	"github.com/rjkroege/edwood/frame"
)

// scrollFrameMock embeds MockFrame and overrides the few methods the
// scroll adapter actually exercises (Charofpt, DefaultFontHeight,
// GetFrameFillStatus). Records Charofpt arguments so tests can assert
// the exact pixel point passed in.
type scrollFrameMock struct {
	MockFrame
	fontHeight     int
	charofptResult int
	nchars         int
	charofptCalls  []image.Point
}

func (m *scrollFrameMock) DefaultFontHeight() int { return m.fontHeight }
func (m *scrollFrameMock) Charofpt(p image.Point) int {
	m.charofptCalls = append(m.charofptCalls, p)
	return m.charofptResult
}
func (m *scrollFrameMock) GetFrameFillStatus() frame.FrameFillStatus {
	return frame.FrameFillStatus{Nchars: m.nchars}
}

func newScrollTestText(buf string, fr *scrollFrameMock, scrollr image.Rectangle) *Text {
	return &Text{
		file:    file.MakeObservableEditableBuffer("", []rune(buf)),
		fr:      fr,
		scrollr: scrollr,
	}
}

func TestTextScrollModel_GeometryReturnsRuneCounts(t *testing.T) {
	buf := "line1\nline2\nline3\n"
	text := newScrollTestText(buf, &scrollFrameMock{nchars: 12}, image.Rect(0, 0, 12, 100))
	text.org = 6
	m := &textScrollModel{t: text}
	total, view, origin := m.Geometry()
	if total != text.file.Nr() {
		t.Errorf("total=%d, want %d (file.Nr)", total, text.file.Nr())
	}
	if view != 12 {
		t.Errorf("view=%d, want 12 (frame Nchars)", view)
	}
	if origin != 6 {
		t.Errorf("origin=%d, want 6", origin)
	}
}

func TestTextScrollModel_GeometryHandlesNilFrame(t *testing.T) {
	text := newScrollTestText("hello\n", nil, image.Rect(0, 0, 12, 100))
	text.fr = nil
	m := &textScrollModel{t: text}
	total, view, origin := m.Geometry()
	if total != 0 || view != 0 || origin != 0 {
		t.Errorf("nil frame Geometry=(%d,%d,%d), want (0,0,0)", total, view, origin)
	}
}

func TestTextScrollModel_DragTopToPixelGoesBackByLines(t *testing.T) {
	// Three lines of length 6 (5 chars + \n). org at start of third
	// line (rune 12). clickY=20 with fontH=10 → back 2 lines → org=0.
	buf := "line1\nline2\nline3\n"
	fr := &scrollFrameMock{fontHeight: 10}
	text := newScrollTestText(buf, fr, image.Rect(0, 0, 12, 100))
	text.org = 12
	m := &textScrollModel{t: text}
	m.DragTopToPixel(20)
	if text.org != 0 {
		t.Errorf("org=%d, want 0 (back 2 lines from rune 12)", text.org)
	}
}

func TestTextScrollModel_DragTopToPixelZeroIsNoop(t *testing.T) {
	// clickY=0 means N=0 lines, but BackNL(p, 0) snaps to start of
	// current line if not already there. Verify against legacy.
	buf := "line1\nline2\nline3\n"
	fr := &scrollFrameMock{fontHeight: 10}
	text := newScrollTestText(buf, fr, image.Rect(0, 0, 12, 100))
	text.org = 6 // start of "line2"
	m := &textScrollModel{t: text}
	m.DragTopToPixel(0)
	// BackNL(6, 0) returns 6 (already at line start).
	if text.org != 6 {
		t.Errorf("org=%d, want 6 (BackNL(6,0) at line start)", text.org)
	}
}

func TestTextScrollModel_DragPixelToTopAddsCharofptToOrigin(t *testing.T) {
	// scrollr=(0,0,12,100); rect.Inset(1) = (1,1,11,99); s.Max.X=11,
	// s.Min.Y=1. clickY=50 → Charofpt called with (11, 51).
	// charofptResult=7 means 7 chars into the current frame.
	buf := "AAAAAA\nBBBBBB\n"
	fr := &scrollFrameMock{charofptResult: 7}
	text := newScrollTestText(buf, fr, image.Rect(0, 0, 12, 100))
	text.org = 5
	m := &textScrollModel{t: text}
	m.DragPixelToTop(50)
	if text.org != 12 {
		t.Errorf("org=%d, want 12 (5+7)", text.org)
	}
	if len(fr.charofptCalls) != 1 {
		t.Fatalf("Charofpt call count=%d, want 1", len(fr.charofptCalls))
	}
	if got := fr.charofptCalls[0]; got != (image.Point{X: 11, Y: 51}) {
		t.Errorf("Charofpt arg=%v, want (11,51)", got)
	}
}

func TestTextScrollModel_JumpToFractionAtZero(t *testing.T) {
	buf := "0123456789"
	text := newScrollTestText(buf, &scrollFrameMock{}, image.Rect(0, 0, 12, 100))
	text.org = 5
	text.q1 = 8
	m := &textScrollModel{t: text}
	m.JumpToFraction(0.0)
	if text.org != 0 {
		t.Errorf("org=%d, want 0 for f=0", text.org)
	}
}

func TestTextScrollModel_JumpToFractionSnapsBackTwoLinesWhenAtOrPastQ1(t *testing.T) {
	// Three full lines. q1=14 (start of "line3"). f=1.0 → p0=18 ≥ q1
	// → snap with BackNL(p0, 2). BackNL(18, 2) = 6 ("line2" start).
	buf := "line1\nline2\nline3\n"
	text := newScrollTestText(buf, &scrollFrameMock{}, image.Rect(0, 0, 12, 100))
	text.org = 0
	text.q1 = 14
	m := &textScrollModel{t: text}
	m.JumpToFraction(1.0)
	if text.org != 6 {
		t.Errorf("org=%d, want 6 (BackNL(18,2) when p0 >= q1)", text.org)
	}
}

func TestTextScrollModel_JumpToFractionMidDocBelowQ1(t *testing.T) {
	// f=0.5 of 18-rune doc → p0=9. q1=18, so p0 < q1 → no snap.
	// SetOrigin(9, false) does newline-search (exact=false): rune at
	// pos 9-1=8 is 'e' (in "line2"), not \n → step forward to next
	// \n at position 11, then +1 = 12. So org ends up at 12.
	buf := "line1\nline2\nline3\n"
	text := newScrollTestText(buf, &scrollFrameMock{}, image.Rect(0, 0, 12, 100))
	text.org = 0
	text.q1 = 18
	m := &textScrollModel{t: text}
	m.JumpToFraction(0.5)
	if text.org != 12 {
		t.Errorf("org=%d, want 12 (SetOrigin newline-skip from p0=9)", text.org)
	}
}
