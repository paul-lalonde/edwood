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

func TestBoldFont_BoxWidthMatchesBoldMetrics(t *testing.T) {
	// Regression: when KindBold runs use a font with a wider
	// glyph than the regular variant, the produced box's Wid
	// must be sized to the bold font's BytesWidth — otherwise
	// adjacent boxes overlap and "type", "struct", "map" get
	// clipped by the next character.
	textarea := image.Rect(20, 10, 400, 100)
	display := edwoodtest.NewDisplay(textarea)
	var textcolors [NumColours]draw.Image
	textcolors[ColBack] = display.AllocImageMix(draw.Paleyellow, draw.White)
	textcolors[ColHigh], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Darkyellow)
	textcolors[ColBord], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Yellowgreen)
	textcolors[ColText] = display.Black()
	textcolors[ColHText] = display.Black()

	// Bold is 2px wider per glyph than regular.
	regular := edwoodtest.NewFontWithName("REGULAR_W10", 10, 13)
	bold := edwoodtest.NewFontWithName("BOLD_W12", 12, 13)
	f := new(frameimpl)
	f.Init(textarea,
		OptColors(textcolors),
		OptFont(regular),
		OptBoldFont(bold),
		OptBackground(display.ScreenImage()),
		OptMaxTab(8),
	)

	boldStyle := Style{Kind: KindBold}
	f.InsertWithStyle([]rune("type"), 0, []StyleRun{{Len: 4, Style: boldStyle}})

	// Find the bold box; its Wid should be 4*12 = 48, not 4*10 = 40.
	for _, b := range f.box {
		if b == nil || b.Nrune <= 0 || b.Style.Kind&KindBold == 0 {
			continue
		}
		want := 4 * 12
		if b.Wid != want {
			t.Errorf("bold box Wid = %d, want %d (4 runes × bold-width 12)", b.Wid, want)
		}
	}
}

func TestSetStyleRange_UpdatesBoxWidForVariantFont(t *testing.T) {
	// Regression: SetStyleRange used to assign b.Style without
	// recomputing b.Wid. On the first paint after edcolor styled
	// a token, the bold box still carried its regular-font Wid,
	// the next box's background fill started early, and the right
	// edge of the bold glyph (e.g. the "c" in "func") got clipped.
	textarea := image.Rect(20, 10, 400, 100)
	display := edwoodtest.NewDisplay(textarea)
	var textcolors [NumColours]draw.Image
	textcolors[ColBack] = display.AllocImageMix(draw.Paleyellow, draw.White)
	textcolors[ColHigh], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Darkyellow)
	textcolors[ColBord], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Yellowgreen)
	textcolors[ColText] = display.Black()
	textcolors[ColHText] = display.Black()

	regular := edwoodtest.NewFontWithName("REGULAR_W10", 10, 13)
	bold := edwoodtest.NewFontWithName("BOLD_W12", 12, 13)
	f := new(frameimpl)
	f.Init(textarea,
		OptColors(textcolors),
		OptFont(regular),
		OptBoldFont(bold),
		OptBackground(display.ScreenImage()),
		OptMaxTab(8),
	)

	// Insert "func foo" plain — both halves get sized with the
	// regular font.
	f.Insert([]rune("func foo"), 0)

	// Now re-style the first 4 runes to bold, matching what edcolor
	// would do via the spans/9P path.
	f.SetStyleRange(0, 4, []StyleRun{{Len: 4, Style: Style{Kind: KindBold}}})

	// The bold box must now carry the bold font's width.
	for _, b := range f.box {
		if b == nil || b.Nrune <= 0 || b.Style.Kind&KindBold == 0 {
			continue
		}
		want := b.Nrune * 12
		if b.Wid != want {
			t.Errorf("after SetStyleRange, bold box %q has Wid=%d, want %d (%d runes × bold-width 12)", string(b.Ptr), b.Wid, want, b.Nrune)
		}
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
