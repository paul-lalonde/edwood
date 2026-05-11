package frame

import (
	"image"
	"reflect"
	"testing"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/edwoodtest"
)

// setupStyledFrame returns a fresh frame plus the display backing it
// so tests can allocate test colors. Mirrors setupFrame's setup; kept
// separate so this file is self-contained.
func setupStyledFrame(t *testing.T) (Frame, draw.Display) {
	t.Helper()

	textarea := image.Rect(20, 10, 400, 100)
	display := edwoodtest.NewDisplay(textarea)

	var textcolors [NumColours]draw.Image
	textcolors[ColBack] = display.AllocImageMix(draw.Paleyellow, draw.White)
	textcolors[ColHigh], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Darkyellow)
	textcolors[ColBord], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Yellowgreen)
	textcolors[ColText] = display.Black()
	textcolors[ColHText] = display.Black()

	font, err := display.OpenFont("helvetica")
	if err != nil {
		t.Fatalf("can't make mock font: %v", err)
	}
	fr := NewFrame(textarea, font, display.ScreenImage(), textcolors)
	return fr, display
}

// copyBoxes deep-copies a frame's box slice so we can compare
// snapshots taken before/after an operation.
func copyBoxes(boxes []*frbox) []*frbox {
	out := make([]*frbox, len(boxes))
	for i, b := range boxes {
		if b == nil {
			continue
		}
		cp := *b
		out[i] = &cp
	}
	return out
}

func TestInsertWithStyle_NilStylesIsFastPath(t *testing.T) {
	// nil styles must produce byte-identical box state to upstream
	// Insert. Anchors the §5.4 fast-path contract.
	fr1, _ := setupStyledFrame(t)
	fr1.Insert([]rune("hello"), 0)
	boxes1 := copyBoxes(fr1.(*frameimpl).box)

	fr2, _ := setupStyledFrame(t)
	fr2.InsertWithStyle([]rune("hello"), 0, nil)
	boxes2 := fr2.(*frameimpl).box

	if !reflect.DeepEqual(boxes1, boxes2) {
		t.Errorf("InsertWithStyle(r, 0, nil) produced different boxes than Insert(r, 0)\nInsert:           %v\nInsertWithStyle:  %v", boxes1, boxes2)
	}
}

func TestInsertWithStyle_AllPlainStylesIsFastPath(t *testing.T) {
	// A non-nil styles slice whose entries are all IsPlain() must
	// also take the fast path — observable boxes identical to
	// upstream Insert.
	fr1, _ := setupStyledFrame(t)
	fr1.Insert([]rune("hello"), 0)
	boxes1 := copyBoxes(fr1.(*frameimpl).box)

	fr2, _ := setupStyledFrame(t)
	styles := []StyleRun{{Len: 5, Style: Style{}}}
	fr2.InsertWithStyle([]rune("hello"), 0, styles)
	boxes2 := fr2.(*frameimpl).box

	if !reflect.DeepEqual(boxes1, boxes2) {
		t.Errorf("InsertWithStyle with all-IsPlain styles took non-fast path\nInsert:           %v\nInsertWithStyle:  %v", boxes1, boxes2)
	}
}

func TestInsertWithStyle_AppliesColorToBoxes(t *testing.T) {
	// A single colored run results in box state where the produced
	// boxes carry the producer's Style.
	fr, display := setupStyledFrame(t)
	red, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Medblue)

	want := Style{Kind: KindColored, Fg: red}
	styles := []StyleRun{{Len: 5, Style: want}}
	fr.InsertWithStyle([]rune("hello"), 0, styles)

	boxes := fr.(*frameimpl).box
	if len(boxes) == 0 {
		t.Fatalf("no boxes produced")
	}
	for i, b := range boxes {
		if b == nil || b.Nrune <= 0 {
			continue
		}
		if b.Style != want {
			t.Errorf("box[%d].Style = %+v, want %+v", i, b.Style, want)
		}
	}
}

func TestInsertWithStyle_SplitsAtStyleBoundary(t *testing.T) {
	// Mixed plain/colored input must split into separate boxes at
	// the style boundary.
	fr, display := setupStyledFrame(t)
	red, _ := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Medblue)
	colored := Style{Kind: KindColored, Fg: red}

	// 3 plain + 3 colored
	styles := []StyleRun{
		{Len: 3, Style: Style{}},
		{Len: 3, Style: colored},
	}
	fr.InsertWithStyle([]rune("abcdef"), 0, styles)

	boxes := fr.(*frameimpl).box

	// Expect exactly two boxes for the six glyphs (one plain, one
	// colored). Tab/newline splits don't apply to this input.
	if len(boxes) < 2 {
		t.Fatalf("expected at least 2 boxes (split at style boundary), got %d: %v", len(boxes), boxes)
	}

	// First glyph box must be plain.
	first := boxes[0]
	if first == nil || !first.Style.IsPlain() {
		t.Errorf("box[0].Style = %+v, want plain", first.Style)
	}

	// Locate the first non-plain glyph box.
	found := false
	for i, b := range boxes {
		if b == nil || b.Nrune <= 0 {
			continue
		}
		if !b.Style.IsPlain() {
			if b.Style != colored {
				t.Errorf("box[%d].Style = %+v, want %+v", i, b.Style, colored)
			}
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no colored glyph box found: %v", boxes)
	}
}

func TestInsertWithStyle_PanicsOnLenMismatch(t *testing.T) {
	// Sum of StyleRun.Lens must equal len(runes); the §5.4 contract
	// permits panic on mismatch. We assert it.
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on Len mismatch, got none")
		}
	}()
	fr, _ := setupStyledFrame(t)
	bad := []StyleRun{{Len: 3, Style: Style{}}} // sum 3 != len("hello") = 5
	fr.InsertWithStyle([]rune("hello"), 0, bad)
}

func TestInsertWithStyle_ReturnValueMatchesInsert(t *testing.T) {
	// The bool return (lastlinefull) must agree with Insert for the
	// same input, both fast-path (nil styles) and styled paths.
	fr1, _ := setupStyledFrame(t)
	got1 := fr1.Insert([]rune("hello"), 0)

	fr2, _ := setupStyledFrame(t)
	got2 := fr2.InsertWithStyle([]rune("hello"), 0, nil)

	if got1 != got2 {
		t.Errorf("InsertWithStyle(nil) returned %v, Insert returned %v; want match", got2, got1)
	}
}
