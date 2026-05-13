package frame

import (
	"image"
	"testing"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/edwoodtest"
)

// Phase B2.2 R4 — variable line height arrives. A box whose
// Style.Kind has KindScale and Style.Scale matches a key in
// the frame's scale-font map renders with the scaled font;
// updateLineMaxes uses the scaled font's height so the line
// containing such a box has a taller LineH.

// TestKindScale_FontForReturnsScaledFont confirms fontFor
// dispatches to OptScaleFonts when the style carries
// KindScale + a registered Scale.
func TestKindScale_FontForReturnsScaledFont(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	display := edwoodtest.NewDisplay(iv.textarea)
	tallFont := edwoodtest.NewFont(10, 26) // 2x base height

	var textcolors [NumColours]draw.Image
	textcolors[ColBack] = display.AllocImageMix(draw.Paleyellow, draw.White)
	textcolors[ColHigh], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Darkyellow)
	textcolors[ColBord], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Yellowgreen)
	textcolors[ColText] = display.Black()
	textcolors[ColHText] = display.Black()

	baseFont, _ := display.OpenFont("helvetica")
	fr := NewFrame(iv.textarea, baseFont, display.ScreenImage(), textcolors,
		OptScaleFonts(map[float32]draw.Font{1.5: tallFont}))

	fimpl := fr.(*frameimpl)
	got := fimpl.fontFor(Style{Kind: KindScale, Scale: 1.5})
	if got != tallFont {
		t.Errorf("fontFor(scale=1.5) = %v, want tallFont", got)
	}
}

// TestKindScale_FontFor_NoMatch_FallsBackToBase confirms that
// a KindScale style whose Scale isn't in the map falls back to
// the base font (graceful degradation when the user supplies
// a partial map).
func TestKindScale_FontFor_NoMatch_FallsBackToBase(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	display := edwoodtest.NewDisplay(iv.textarea)
	baseFont, _ := display.OpenFont("helvetica")

	var textcolors [NumColours]draw.Image
	textcolors[ColBack] = display.AllocImageMix(draw.Paleyellow, draw.White)
	textcolors[ColHigh], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Darkyellow)
	textcolors[ColBord], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Yellowgreen)
	textcolors[ColText] = display.Black()
	textcolors[ColHText] = display.Black()
	fr := NewFrame(iv.textarea, baseFont, display.ScreenImage(), textcolors,
		OptScaleFonts(map[float32]draw.Font{1.5: edwoodtest.NewFont(10, 26)}))

	fimpl := fr.(*frameimpl)
	got := fimpl.fontFor(Style{Kind: KindScale, Scale: 2.5}) // unregistered
	if got != baseFont {
		t.Errorf("fontFor(scale=2.5 unregistered) = %v, want baseFont", got)
	}
}

// TestKindScale_Plain_FontForReturnsBase confirms a plain
// style (no KindScale) is unaffected by the scale map.
func TestKindScale_Plain_FontForReturnsBase(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	display := edwoodtest.NewDisplay(iv.textarea)
	baseFont, _ := display.OpenFont("helvetica")

	var textcolors [NumColours]draw.Image
	textcolors[ColBack] = display.AllocImageMix(draw.Paleyellow, draw.White)
	textcolors[ColHigh], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Darkyellow)
	textcolors[ColBord], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Yellowgreen)
	textcolors[ColText] = display.Black()
	textcolors[ColHText] = display.Black()
	fr := NewFrame(iv.textarea, baseFont, display.ScreenImage(), textcolors,
		OptScaleFonts(map[float32]draw.Font{1.5: edwoodtest.NewFont(10, 26)}))

	fimpl := fr.(*frameimpl)
	got := fimpl.fontFor(Style{}) // plain
	if got != baseFont {
		t.Errorf("fontFor(plain) = %v, want baseFont", got)
	}
}

// TestKindScale_LineHReflectsScaledFontHeight confirms the
// layout pass produces a tall LineH when a line contains a
// KindScale box. The whole line shares the same LineH (max
// over boxes on the line, per §3.2).
func TestKindScale_LineHReflectsScaledFontHeight(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	display := edwoodtest.NewDisplay(iv.textarea)
	tallFont := edwoodtest.NewFont(10, 26)

	var textcolors [NumColours]draw.Image
	textcolors[ColBack] = display.AllocImageMix(draw.Paleyellow, draw.White)
	textcolors[ColHigh], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Darkyellow)
	textcolors[ColBord], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Yellowgreen)
	textcolors[ColText] = display.Black()
	textcolors[ColHText] = display.Black()
	baseFont, _ := display.OpenFont("helvetica")
	fr := NewFrame(iv.textarea, baseFont, display.ScreenImage(), textcolors,
		OptScaleFonts(map[float32]draw.Font{1.5: tallFont}))

	// Insert "heading\nbody" with the first line scaled and
	// the second line plain.
	scaledStyle := Style{Kind: KindScale, Scale: 1.5}
	fr.InsertWithStyle([]rune("heading\nbody"), 0, []StyleRun{
		{Len: 8, Style: scaledStyle}, // "heading" + "\n"
		{Len: 4, Style: Style{}},     // "body"
	})

	fimpl := fr.(*frameimpl)

	// Find a 'heading' content box and a 'body' content box;
	// assert their LineH values.
	var headingBox, bodyBox *frbox
	for _, b := range fimpl.box {
		if b.Nrune <= 0 {
			continue
		}
		if string(b.Ptr) == "heading" && headingBox == nil {
			headingBox = b
		}
		if string(b.Ptr) == "body" && bodyBox == nil {
			bodyBox = b
		}
	}
	if headingBox == nil || bodyBox == nil {
		t.Fatalf("expected 'heading' and 'body' boxes; box model:\n%+v", fimpl.box)
	}

	if headingBox.LineH != tallFont.Height() {
		t.Errorf("'heading' line LineH = %d, want %d (scaled font height)",
			headingBox.LineH, tallFont.Height())
	}
	if bodyBox.LineH != fimpl.defaultfontheight {
		t.Errorf("'body' line LineH = %d, want %d (base font height)",
			bodyBox.LineH, fimpl.defaultfontheight)
	}
}

// TestKindScale_PlainTextFastPath_Unchanged confirms that a
// frame with no scale fonts loaded behaves identically to
// pre-R4: every line has LineH = defaultfontheight.
func TestKindScale_PlainTextFastPath_Unchanged(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("a\nb\nc"), 0)

	fimpl := fr.(*frameimpl)
	for i, b := range fimpl.box {
		if b.LineH != fimpl.defaultfontheight {
			t.Errorf("box[%d].LineH = %d, want %d (plain fast path)",
				i, b.LineH, fimpl.defaultfontheight)
		}
	}
}
