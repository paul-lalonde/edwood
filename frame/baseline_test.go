package frame

import (
	"image"
	"strings"
	"testing"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/edwoodtest"
)

// Phase B2.2 R5 — baseline-aligned glyph paint. paintBox now
// emits glyphs at (b.X, b.Y + b.LineA - fontAscent(b)) so a
// line containing fonts of different ascents (e.g., a heading
// scaled font + body font on the same wrapped line) renders
// with all glyphs on a common baseline. The Bytes op's atpoint
// reflects this offset.

// TestR5_LineA_EqualsMaxAscent confirms relayoutFrom sets each
// line's LineA to the max Ascent of the boxes on that line.
func TestR5_LineA_EqualsMaxAscent(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 200),
	}
	display := edwoodtest.NewDisplay(iv.textarea)
	tallFont := edwoodtest.NewFont(10, 26) // mockFont: Ascent = Height-1 = 25

	var textcolors [NumColours]draw.Image
	textcolors[ColBack] = display.AllocImageMix(draw.Paleyellow, draw.White)
	textcolors[ColHigh], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Darkyellow)
	textcolors[ColBord], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Yellowgreen)
	textcolors[ColText] = display.Black()
	textcolors[ColHText] = display.Black()
	baseFont, _ := display.OpenFont("helvetica")
	fr := NewFrame(iv.textarea, baseFont, display.ScreenImage(), textcolors,
		OptScaleFonts(map[float32]draw.Font{1.5: tallFont}))

	scaled := Style{Kind: KindScale, Scale: 1.5}
	fr.InsertWithStyle([]rune("Ab\n"), 0, []StyleRun{{Len: 3, Style: scaled}})

	fimpl := fr.(*frameimpl)
	wantAscent := tallFont.Ascent()
	for _, b := range fimpl.box {
		if b.Y == fimpl.rect.Min.Y && b.Nrune > 0 {
			if b.LineA != wantAscent {
				t.Errorf("scaled-line box %q LineA = %d, want %d (max Ascent on line)",
					string(b.Ptr), b.LineA, wantAscent)
			}
		}
	}
}

// TestR5_PlainLine_LineA_EqualsBaseAscent confirms a plain
// line still gets LineA from the base font (so single-font
// renders stay visually byte-identical to pre-R5).
func TestR5_PlainLine_LineA_EqualsBaseAscent(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("plain"), 0)

	fimpl := fr.(*frameimpl)
	wantAscent := fimpl.font.Ascent()
	for _, b := range fimpl.box {
		if b.LineA != wantAscent {
			t.Errorf("plain box %q LineA = %d, want %d (base font Ascent)",
				string(b.Ptr), b.LineA, wantAscent)
		}
	}
}

// TestR5_PaintBox_BaselineOffset confirms paintBox issues the
// glyph Bytes op at pt.Y + (LineA - fontAscent(box)). For a
// base-font box on a scaled line, that's a positive offset
// (pushing the small glyph down to share the baseline of the
// tall line).
func TestR5_PaintBox_BaselineOffset(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(0, 0),
		textarea:  image.Rect(0, 0, 400, 200),
	}
	display := edwoodtest.NewDisplay(iv.textarea)
	tallFont := edwoodtest.NewFont(10, 26) // Ascent = 25

	var textcolors [NumColours]draw.Image
	textcolors[ColBack] = display.AllocImageMix(draw.Paleyellow, draw.White)
	textcolors[ColHigh], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Darkyellow)
	textcolors[ColBord], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Yellowgreen)
	textcolors[ColText] = display.Black()
	textcolors[ColHText] = display.Black()
	baseFont, _ := display.OpenFont("helvetica")
	baseAscent := baseFont.Ascent()
	fr := NewFrame(iv.textarea, baseFont, display.ScreenImage(), textcolors,
		OptScaleFonts(map[float32]draw.Font{1.5: tallFont}))

	// One styled box on the same line as one plain box.
	// scale=1.5 widens the line's LineA to tallFont.Ascent.
	scaled := Style{Kind: KindScale, Scale: 1.5}
	fr.InsertWithStyle([]rune("Hb"), 0, []StyleRun{
		{Len: 1, Style: scaled},  // "H" scaled
		{Len: 1, Style: Style{}}, // "b" plain
	})

	g := gdo(t, fr)
	wantOffset := tallFont.Ascent() - baseAscent // positive
	want := "atpoint: (10," + itoa(wantOffset) + ")"
	found := false
	for _, op := range g.DrawOps() {
		if strings.Contains(op, `string "b"`) && strings.Contains(op, want) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'b' glyph at %s (baseline-aligned with tall line); ops:\n%s",
			want, strings.Join(g.DrawOps(), "\n"))
	}
}

// TestR5_PaintBox_ScaledGlyph_NoBaselineOffset confirms that
// the dominant-ascent box paints at pt.Y (offset 0): the line's
// LineA equals fontAscent of the tall box, so the offset is
// LineA - LineA = 0.
func TestR5_PaintBox_ScaledGlyph_NoBaselineOffset(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(0, 0),
		textarea:  image.Rect(0, 0, 400, 200),
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

	scaled := Style{Kind: KindScale, Scale: 1.5}
	fr.InsertWithStyle([]rune("H"), 0, []StyleRun{{Len: 1, Style: scaled}})

	g := gdo(t, fr)
	// Scaled 'H' should paint at atpoint (0, 0) — its ascent
	// IS the line's max ascent, so offset is zero.
	want := "atpoint: (0,0)"
	found := false
	for _, op := range g.DrawOps() {
		if strings.Contains(op, `string "H"`) && strings.Contains(op, want) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected scaled 'H' at %s (no baseline offset for max-ascent box); ops:\n%s",
			want, strings.Join(g.DrawOps(), "\n"))
	}
}

// itoa is a tiny helper for assembling the expected atpoint
// string. Keeps the test from pulling strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
