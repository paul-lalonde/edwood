package frame

import (
	"image"
	"strings"
	"testing"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/edwoodtest"
)

// setupVariantFrame mirrors setupStyledFrame but installs four
// distinct named fonts so tests can confirm which variant the
// frame picked for a given Style.Kind.
func setupVariantFrame(t *testing.T) (Frame, draw.Display) {
	t.Helper()
	textarea := image.Rect(20, 10, 400, 100)
	display := edwoodtest.NewDisplay(textarea)

	var textcolors [NumColours]draw.Image
	textcolors[ColBack] = display.AllocImageMix(draw.Paleyellow, draw.White)
	textcolors[ColHigh], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Darkyellow)
	textcolors[ColBord], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Yellowgreen)
	textcolors[ColText] = display.Black()
	textcolors[ColHText] = display.Black()

	const w, h = 10, 13
	regular := edwoodtest.NewFontWithName("FONT_REGULAR", w, h)
	bold := edwoodtest.NewFontWithName("FONT_BOLD", w, h)
	italic := edwoodtest.NewFontWithName("FONT_ITALIC", w, h)
	bolditalic := edwoodtest.NewFontWithName("FONT_BOLDITALIC", w, h)

	f := new(frameimpl)
	f.Init(textarea,
		OptColors(textcolors),
		OptFont(regular),
		OptBoldFont(bold),
		OptItalicFont(italic),
		OptBoldItalicFont(bolditalic),
		OptBackground(display.ScreenImage()),
		OptMaxTab(8),
	)
	return f, display
}

func opsContaining(disp draw.Display, needle string) []string {
	g := disp.(edwoodtest.GettableDrawOps)
	var out []string
	for _, op := range g.DrawOps() {
		if strings.Contains(op, needle) {
			out = append(out, op)
		}
	}
	return out
}

func TestFontFor_PicksBoldVariant(t *testing.T) {
	fr, disp := setupVariantFrame(t)
	bold := Style{Kind: KindBold}
	fr.InsertWithStyle([]rune("hello"), 0, []StyleRun{{Len: 5, Style: bold}})

	if got := opsContaining(disp, "FONT_BOLD"); len(got) == 0 {
		t.Errorf("no Bytes op recorded with FONT_BOLD font:\n%s", strings.Join(opsContaining(disp, "atpoint"), "\n"))
	}
}

func TestFontFor_PicksItalicVariant(t *testing.T) {
	fr, disp := setupVariantFrame(t)
	it := Style{Kind: KindItalic}
	fr.InsertWithStyle([]rune("hello"), 0, []StyleRun{{Len: 5, Style: it}})

	if got := opsContaining(disp, "FONT_ITALIC"); len(got) == 0 {
		t.Errorf("no Bytes op recorded with FONT_ITALIC font:\n%s", strings.Join(opsContaining(disp, "atpoint"), "\n"))
	}
}

func TestFontFor_PicksBoldItalicVariant(t *testing.T) {
	fr, disp := setupVariantFrame(t)
	bi := Style{Kind: KindBold | KindItalic}
	fr.InsertWithStyle([]rune("hello"), 0, []StyleRun{{Len: 5, Style: bi}})

	if got := opsContaining(disp, "FONT_BOLDITALIC"); len(got) == 0 {
		t.Errorf("no Bytes op recorded with FONT_BOLDITALIC font:\n%s", strings.Join(opsContaining(disp, "atpoint"), "\n"))
	}
}

func TestFontFor_FallsBackToBaseWhenVariantMissing(t *testing.T) {
	// Build a frame that has only the regular font (no bold variant)
	// and verify a KindBold run renders with the regular font.
	textarea := image.Rect(20, 10, 400, 100)
	display := edwoodtest.NewDisplay(textarea)
	var textcolors [NumColours]draw.Image
	textcolors[ColBack] = display.AllocImageMix(draw.Paleyellow, draw.White)
	textcolors[ColHigh], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Darkyellow)
	textcolors[ColBord], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Yellowgreen)
	textcolors[ColText] = display.Black()
	textcolors[ColHText] = display.Black()

	regular := edwoodtest.NewFontWithName("FONT_BASEONLY", 10, 13)
	f := new(frameimpl)
	f.Init(textarea,
		OptColors(textcolors),
		OptFont(regular),
		OptBackground(display.ScreenImage()),
		OptMaxTab(8),
	)

	bold := Style{Kind: KindBold}
	f.InsertWithStyle([]rune("hello"), 0, []StyleRun{{Len: 5, Style: bold}})

	if got := opsContaining(display, "FONT_BASEONLY"); len(got) == 0 {
		t.Errorf("expected fallback to FONT_BASEONLY when bold variant absent; ops=%v", opsContaining(display, "atpoint"))
	}
}

func TestKindHidden_SkipsGlyphPaintInDrawtext(t *testing.T) {
	fr, disp := setupVariantFrame(t)
	hidden := Style{Kind: KindHidden}
	fr.InsertWithStyle([]rune("hello"), 0, []StyleRun{{Len: 5, Style: hidden}})

	// No "atpoint" ops should reference "hello" — the glyph paint
	// is suppressed. (Background ops still happen but those don't
	// include the string.)
	for _, op := range opsContaining(disp, "atpoint") {
		if strings.Contains(op, `"hello"`) {
			t.Errorf("KindHidden box still produced glyph paint: %s", op)
		}
	}
}
