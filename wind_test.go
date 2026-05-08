package main

import (
	"image"
	"reflect"
	"testing"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/edwoodtest"
	"github.com/rjkroege/edwood/file"
	"github.com/rjkroege/edwood/frame"
)

func TestSetTag1(t *testing.T) {
	const (
		defaultSuffix = " Del Snarf | Look Edit "
		extraSuffix   = "|fmt g setTag1 Ldef"
	)

	for _, name := range []string{
		"/home/gopher/src/hello.go",
		"/home/ゴーファー/src/エドウード.txt",
		"/home/ゴーファー/src/",
	} {
		display := edwoodtest.NewDisplay(image.Rectangle{})
		global.configureGlobals(display)

		w := NewWindow().initHeadless(nil)
		w.display = display
		w.body = Text{
			display: display,
			fr:      &MockFrame{},
			file:    file.MakeObservableEditableBuffer(name, nil),
		}
		w.tag = Text{
			display: display,
			fr:      &MockFrame{},
			file:    file.MakeObservableEditableBuffer("", nil),
		}

		w.col = &Column{
			safe: true,
		}

		w.setTag1()
		got := w.tag.file.String()
		want := name + defaultSuffix
		if got != want {
			t.Errorf("bad initial tag for file %q:\n got: %q\nwant: %q", name, got, want)
		}

		w.tag.file.InsertAt(w.tag.file.Nr(), []rune(extraSuffix))
		w.setTag1()
		got = w.tag.file.String()
		want = name + defaultSuffix + extraSuffix
		if got != want {
			t.Errorf("bad replacement tag for file %q:\n got: %q\nwant: %q", name, got, want)
		}
	}
}

func TestWindowClampAddr(t *testing.T) {
	const hello_世界 = "Hello, 世界"
	runic_hello_世界 := []rune(hello_世界)
	for _, tc := range []struct {
		addr, want Range
	}{
		{Range{-1, -1}, Range{0, 0}},
		{Range{100, 100}, Range{len(runic_hello_世界), len(runic_hello_世界)}},
	} {
		w := &Window{
			addr: tc.addr,
			body: Text{
				file: file.MakeObservableEditableBuffer("", runic_hello_世界),
			},
		}
		w.ClampAddr()
		if got := w.addr; !reflect.DeepEqual(got, tc.want) {
			t.Errorf("got addr %v; want %v", got, tc.want)
		}
	}
}

func TestWindowVisibleRange(t *testing.T) {
	// Non-styled mode: VisibleRange uses body.org + frame Nchars.
	w := &Window{
		body: Text{
			file: file.MakeObservableEditableBuffer("", []rune("Hello, world!\n")),
			fr:   &MockFrame{},
		},
	}
	// MockFrame returns Nchars=0, so end = org + 0 = 0.
	org, end := w.VisibleRange()
	if org != 0 || end != 0 {
		t.Errorf("VisibleRange() = (%d, %d), want (0, 0)", org, end)
	}

	// With body.org set, org should reflect it.
	w.body.org = 5
	org, end = w.VisibleRange()
	if org != 5 || end != 5 {
		t.Errorf("VisibleRange() = (%d, %d), want (5, 5)", org, end)
	}
}

func TestWindowParseTag(t *testing.T) {
	for _, tc := range []struct {
		tag      string
		filename string
	}{
		{"/foo/bar.txt Del Snarf | Look", "/foo/bar.txt"},
		{"'/foo/bar quux.txt' Del Snarf | Look", "'/foo/bar quux.txt'"},
		{"/foo/bar.txt", "/foo/bar.txt"},
		{"/foo/bar.txt | Look", "/foo/bar.txt"},
		{"/foo/bar.txt Del Snarf\t| Look", "/foo/bar.txt"},
		{"/foo/bar.txt Del Snarf Del Snarf", "/foo/bar.txt"},
		{"'/foo/bar.txt ' Del Snarf", "'/foo/bar.txt '"},
		{"'/foo/b|ar.txt ' Del Snarf", "'/foo/b|ar.txt '"},
	} {
		if got, want := parsetaghelper(tc.tag), tc.filename; got != want {
			t.Errorf("tag %q has filename %q; want %q", tc.tag, got, want)
		}
	}
}

func TestWindowClearTag(t *testing.T) {
	tag := "/foo bar/test.txt Del Snarf Undo Put | Look |fmt mk"
	want := "/foo bar/test.txt Del Snarf Undo Put |"
	w := &Window{
		tag: Text{
			file: file.MakeObservableEditableBuffer("", []rune(tag)),
		},
	}
	w.ClearTag()
	got := w.tag.file.String()
	if got != want {
		t.Errorf("got %q; want %q", got, want)
	}
}




































// mockMousectlWithEvents creates a mock Mousectl with a buffered channel
// containing the provided events. This is used for testing drag selection.
func mockMousectlWithEvents(events []draw.Mouse) *draw.Mousectl {
	ch := make(chan draw.Mouse, len(events)+1)
	for _, e := range events {
		ch <- e
	}
	return &draw.Mousectl{C: ch}
}
































// containsSubstring checks if s contains substr.
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr) >= 0
}








// TrackingMockFrame is a MockFrame that tracks DrawSel calls.
type TrackingMockFrame struct {
	MockFrame
	DrawSelCalled bool
	DrawSelCount  int
	nchars        int
	maxlines      int
}

func (mf *TrackingMockFrame) GetFrameFillStatus() frame.FrameFillStatus {
	return frame.FrameFillStatus{
		Nchars:         mf.nchars,
		Nlines:         mf.maxlines,
		Maxlines:       mf.maxlines,
		MaxPixelHeight: mf.maxlines * 14,
	}
}

func (mf *TrackingMockFrame) DrawSel(pt image.Point, p0, p1 int, ticked bool) {
	mf.DrawSelCalled = true
	mf.DrawSelCount++
}

func (mf *TrackingMockFrame) Ptofchar(int) image.Point { return image.Point{0, 0} }


















// searchString returns the index of substr in s, or -1 if not found.
func searchString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}















