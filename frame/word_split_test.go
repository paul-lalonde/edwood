package frame

import (
	"image"
	"testing"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/edwoodtest"
)

// Phase B5: bxscan splits content at U+0020 SPACE boundaries.
// Each contiguous non-space run is a word box; each contiguous
// run of spaces is a space box. Tabs and newlines remain
// special boxes. Style boundaries still trigger splits.
//
// Tests pin R-B5.1 (the split) and R-B5.2 (clean's merge
// predicate preserves the word/space boundary).

// buildPlainFrame is a small helper for word-split tests: a
// frame wide enough to hold the test content on one line, base
// font only.
func buildPlainFrame(t *testing.T) Frame {
	t.Helper()
	rect := image.Rect(0, 0, 400, 100)
	display := edwoodtest.NewDisplay(rect)
	var textcolors [NumColours]draw.Image
	textcolors[ColBack] = display.AllocImageMix(draw.Paleyellow, draw.White)
	textcolors[ColHigh], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Darkyellow)
	textcolors[ColBord], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Yellowgreen)
	textcolors[ColText] = display.Black()
	textcolors[ColHText] = display.Black()

	f := new(frameimpl)
	f.Init(rect,
		OptColors(textcolors),
		OptFont(edwoodtest.NewFont(10, 13)),
		OptBackground(display.ScreenImage()),
		OptMaxTab(8),
	)
	return f
}

// boxContents returns the byte contents of each content box in
// the frame, in order. Special boxes (Nrune < 0) render as
// "\\n" / "\\t" for readability.
func boxContents(f *frameimpl) []string {
	var out []string
	for _, b := range f.box {
		switch {
		case b.Nrune < 0 && b.Bc == '\n':
			out = append(out, "\\n")
		case b.Nrune < 0 && b.Bc == '\t':
			out = append(out, "\\t")
		default:
			out = append(out, string(b.Ptr))
		}
	}
	return out
}

func TestBxscan_SplitsAtSpaces_PlainContent(t *testing.T) {
	fr := buildPlainFrame(t)
	f := fr.(*frameimpl)

	fr.Insert([]rune("one two three"), 0)

	got := boxContents(f)
	want := []string{"one", " ", "two", " ", "three"}
	if !equalSlices(got, want) {
		t.Errorf("box contents = %q, want %q", got, want)
	}
}

func TestBxscan_SplitsAtSpaces_MultipleSpaces(t *testing.T) {
	// Adjacent spaces stay in one space box.
	fr := buildPlainFrame(t)
	f := fr.(*frameimpl)

	fr.Insert([]rune("hi   there"), 0)

	got := boxContents(f)
	want := []string{"hi", "   ", "there"}
	if !equalSlices(got, want) {
		t.Errorf("box contents = %q, want %q", got, want)
	}
}

func TestBxscan_SplitsAtSpaces_LeadingAndTrailing(t *testing.T) {
	fr := buildPlainFrame(t)
	f := fr.(*frameimpl)

	fr.Insert([]rune("  word  "), 0)

	got := boxContents(f)
	want := []string{"  ", "word", "  "}
	if !equalSlices(got, want) {
		t.Errorf("box contents = %q, want %q", got, want)
	}
}

func TestBxscan_SplitsAtSpaces_AcrossNewlines(t *testing.T) {
	// Newlines remain their own special boxes; word/space
	// splitting happens within each line.
	fr := buildPlainFrame(t)
	f := fr.(*frameimpl)

	fr.Insert([]rune("a b\nc d"), 0)

	got := boxContents(f)
	want := []string{"a", " ", "b", "\\n", "c", " ", "d"}
	if !equalSlices(got, want) {
		t.Errorf("box contents = %q, want %q", got, want)
	}
}

func TestBxscan_SplitsAtSpaces_AcrossTabs(t *testing.T) {
	fr := buildPlainFrame(t)
	f := fr.(*frameimpl)

	fr.Insert([]rune("a b\tc d"), 0)

	got := boxContents(f)
	want := []string{"a", " ", "b", "\\t", "c", " ", "d"}
	if !equalSlices(got, want) {
		t.Errorf("box contents = %q, want %q", got, want)
	}
}

func TestBxscan_SplitsAtSpaces_StyleBoundaryInsideWord(t *testing.T) {
	// A style change mid-word splits the word into two boxes,
	// each carrying its own Style. Spaces still split too.
	fr := buildPlainFrame(t)
	f := fr.(*frameimpl)

	src := []rune("ab cd")
	// First two runes ("ab") plain; next rune (" ") plain;
	// remaining ("cd") bold-styled.
	styles := []StyleRun{
		{Len: 3, Style: Style{}},
		{Len: 2, Style: Style{Kind: KindBold}},
	}
	fr.InsertWithStyle(src, 0, styles)

	got := boxContents(f)
	want := []string{"ab", " ", "cd"}
	if !equalSlices(got, want) {
		t.Errorf("box contents = %q, want %q", got, want)
	}
	// The bold "cd" must carry KindBold.
	if f.box[2].Style.Kind&KindBold == 0 {
		t.Errorf("box[2] should be bold; got Style.Kind = %v", f.box[2].Style.Kind)
	}
}

// R-B5.2: clean's merge predicate is relaxed — it does NOT
// merge two adjacent same-Style content boxes if exactly one is
// space-only. (Two adjacent space-only boxes can still merge;
// two adjacent non-space same-style boxes can still merge.)

func TestIsSpaceOnlyBox(t *testing.T) {
	cases := []struct {
		ptr  string
		want bool
	}{
		{"  ", true},
		{" ", true},
		{"x", false},
		{"x ", false},
		{" x", false},
		{"", false}, // empty boxes aren't space-only by convention
	}
	for _, c := range cases {
		b := &frbox{Ptr: []byte(c.ptr), Nrune: len([]rune(c.ptr))}
		got := isSpaceOnlyBox(b)
		if got != c.want {
			t.Errorf("isSpaceOnlyBox(%q) = %v, want %v", c.ptr, got, c.want)
		}
	}
	// Special boxes are not space-only regardless of Bc.
	nl := &frbox{Nrune: -1, Bc: '\n'}
	if isSpaceOnlyBox(nl) {
		t.Errorf("newline should not be space-only")
	}
	tab := &frbox{Nrune: -1, Bc: '\t'}
	if isSpaceOnlyBox(tab) {
		t.Errorf("tab should not be space-only")
	}
}

func TestClean_DoesNotMergeWordAndSpace(t *testing.T) {
	// After Insert, clean runs at the end. With word/space
	// splits, the boxes should stay separate.
	fr := buildPlainFrame(t)
	f := fr.(*frameimpl)

	fr.Insert([]rune("alpha bravo"), 0)

	// Five-ish boxes expected: "alpha", " ", "bravo". clean
	// merging would collapse to one box.
	if len(f.box) < 3 {
		t.Errorf("expected at least 3 boxes after Insert+clean; got %d: %q", len(f.box), boxContents(f))
	}
	got := boxContents(f)
	want := []string{"alpha", " ", "bravo"}
	if !equalSlices(got, want) {
		t.Errorf("box contents after clean = %q, want %q", got, want)
	}
}

func equalSlices(a, b []string) bool {
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
