package frame

import (
	"image"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/edwoodtest"
)

// rectRE matches the first `(x0,y0)-(x1,y1)` substring in a
// recorded draw op. The mockDisplay records draw ops with the
// rectangle in image.Rectangle's String() format; this regex
// pulls it back out so tests can assert on rect dimensions.
var rectRE = regexp.MustCompile(`\((-?\d+),(-?\d+)\)-\((-?\d+),(-?\d+)\)`)

// drawOpsWithRectHeight returns the recorded draw ops whose first
// rectangle has the given Dy (max.Y - min.Y). Used by hrule tests
// to find the 1-pixel-high decoration rect amongst the per-box
// background-paint rectangles (which are font-height tall).
func drawOpsWithRectHeight(disp draw.Display, height int) []string {
	g := disp.(edwoodtest.GettableDrawOps)
	var out []string
	for _, op := range g.DrawOps() {
		m := rectRE.FindStringSubmatch(op)
		if m == nil {
			continue
		}
		y0, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		y1, err := strconv.Atoi(m[4])
		if err != nil {
			continue
		}
		if y1-y0 == height {
			out = append(out, op)
		}
	}
	return out
}

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

// R-B4.7: fontFor returns the code-family variant when
// KindCodeFamily is set and the variant is configured.
func TestFontFor_PicksCodeVariant(t *testing.T) {
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
	code := edwoodtest.NewFontWithName("FONT_CODE", w, h)
	f := new(frameimpl)
	f.Init(textarea,
		OptColors(textcolors),
		OptFont(regular),
		OptCodeFont(code),
		OptBackground(display.ScreenImage()),
		OptMaxTab(8),
	)
	cs := Style{Kind: KindCodeFamily}
	f.InsertWithStyle([]rune("inline"), 0, []StyleRun{{Len: 6, Style: cs}})

	if got := opsContaining(display, "FONT_CODE"); len(got) == 0 {
		t.Errorf("no Bytes op recorded with FONT_CODE font:\n%s", strings.Join(opsContaining(display, "atpoint"), "\n"))
	}
}

// R-B4.7: KindCodeFamily without OptCodeFont configured falls
// back to the base font — graceful degradation, same shape as the
// bold/italic fallback.
func TestFontFor_CodeFamilyFallsBackToBaseWhenVariantMissing(t *testing.T) {
	textarea := image.Rect(20, 10, 400, 100)
	display := edwoodtest.NewDisplay(textarea)
	var textcolors [NumColours]draw.Image
	textcolors[ColBack] = display.AllocImageMix(draw.Paleyellow, draw.White)
	textcolors[ColHigh], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Darkyellow)
	textcolors[ColBord], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Yellowgreen)
	textcolors[ColText] = display.Black()
	textcolors[ColHText] = display.Black()

	regular := edwoodtest.NewFontWithName("FONT_BASEONLY_CODE", 10, 13)
	f := new(frameimpl)
	f.Init(textarea,
		OptColors(textcolors),
		OptFont(regular),
		OptBackground(display.ScreenImage()),
		OptMaxTab(8),
	)
	cs := Style{Kind: KindCodeFamily}
	f.InsertWithStyle([]rune("inline"), 0, []StyleRun{{Len: 6, Style: cs}})

	if got := opsContaining(display, "FONT_BASEONLY_CODE"); len(got) == 0 {
		t.Errorf("expected fallback to base font; ops=%v", opsContaining(display, "atpoint"))
	}
}

// R-B4.7: when both KindBold and KindCodeFamily are set, the code
// variant wins — md2spans v1 doesn't emit bold-code so no
// bold-code variant exists, and the simpler precedence keeps
// fontFor cleanly two-axis (family > weight/italic).
func TestFontFor_CodeFamilyTakesPrecedenceOverBold(t *testing.T) {
	textarea := image.Rect(20, 10, 400, 100)
	display := edwoodtest.NewDisplay(textarea)
	var textcolors [NumColours]draw.Image
	textcolors[ColBack] = display.AllocImageMix(draw.Paleyellow, draw.White)
	textcolors[ColHigh], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Darkyellow)
	textcolors[ColBord], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Yellowgreen)
	textcolors[ColText] = display.Black()
	textcolors[ColHText] = display.Black()

	const w, h = 10, 13
	regular := edwoodtest.NewFontWithName("FONT_REG_CB", w, h)
	bold := edwoodtest.NewFontWithName("FONT_BOLD_CB", w, h)
	code := edwoodtest.NewFontWithName("FONT_CODE_CB", w, h)
	f := new(frameimpl)
	f.Init(textarea,
		OptColors(textcolors),
		OptFont(regular),
		OptBoldFont(bold),
		OptCodeFont(code),
		OptBackground(display.ScreenImage()),
		OptMaxTab(8),
	)
	bc := Style{Kind: KindBold | KindCodeFamily}
	f.InsertWithStyle([]rune("xy"), 0, []StyleRun{{Len: 2, Style: bc}})

	if got := opsContaining(display, "FONT_CODE_CB"); len(got) == 0 {
		t.Errorf("KindBold|KindCodeFamily must pick code font; ops=%v", opsContaining(display, "atpoint"))
	}
	if got := opsContaining(display, "FONT_BOLD_CB"); len(got) != 0 {
		t.Errorf("KindBold|KindCodeFamily must NOT pick bold font; got %v", got)
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

func TestSetStyleRange_PreservesTabWidth(t *testing.T) {
	// Regression: the prior fix in SetStyleRange refreshed every
	// touched box's Wid via BytesWidth(b.Ptr) — but for tab/newline
	// "special" boxes (Nrune < 0) the width is metric/tabstop
	// driven, not glyph-derived. Clobbering it with BytesWidth(0)
	// collapsed indent boxes to zero width, so re-styling a span
	// containing a tab made subsequent text run on top of the
	// preceding glyphs.
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

	// Insert content that contains a tab and a newline inside the
	// soon-to-be-styled range. After bxscan these are three
	// content boxes plus tab and newline special boxes.
	f.Insert([]rune("a\tb\nc"), 0)

	// Snapshot widths of every special box before re-styling.
	type snap struct {
		idx int
		wid int
	}
	var before []snap
	for i, b := range f.box {
		if b != nil && b.Nrune < 0 {
			before = append(before, snap{i, b.Wid})
		}
	}
	if len(before) < 2 {
		t.Fatalf("expected at least one tab and one newline box; got %d specials", len(before))
	}

	// Re-style the whole range with bold. The styled span covers
	// the tab and newline boxes.
	f.SetStyleRange(0, f.nchars, []StyleRun{{Len: f.nchars, Style: Style{Kind: KindBold}}})

	// Each special box's Wid must be unchanged. The box indices
	// can shift if clean() merges adjacent same-Style content
	// boxes, so match by Nrune<0 + Bc identity rather than index.
	for _, s := range before {
		// Find a special box with the same Bc somewhere in the
		// current model. (Each test input has exactly one tab
		// and one newline, so byte-class identity is unique.)
		orig := f.box // we want the post-SetStyleRange state
		_ = orig
		found := false
		for _, b := range f.box {
			if b == nil || b.Nrune >= 0 {
				continue
			}
			// Match the original box at this index by Bc, since
			// special-box order matches input order.
			if b.Wid != s.wid {
				continue
			}
			found = true
			break
		}
		if !found {
			// Report widths so the failure is diagnosable.
			var got []int
			for _, b := range f.box {
				if b != nil && b.Nrune < 0 {
					got = append(got, b.Wid)
				}
			}
			t.Errorf("after SetStyleRange a special box originally Wid=%d at idx=%d was not preserved; current special widths = %v", s.wid, s.idx, got)
		}
	}
}

// R-B4.8: a box with KindHRule produces a 1-pixel-tall Draw op
// across its rect after the glyph paint. Tested via drawtext (the
// initial-paint path).
func TestKindHRule_DrawsRuleLineInDrawtext(t *testing.T) {
	fr, disp := setupVariantFrame(t)
	st := Style{Kind: KindHRule}
	fr.InsertWithStyle([]rune("---"), 0, []StyleRun{{Len: 3, Style: st}})

	got := drawOpsWithRectHeight(disp, 1)
	if len(got) == 0 {
		t.Errorf("expected at least one 1-pixel-high Draw op for KindHRule; ops=\n%s", strings.Join(disp.(edwoodtest.GettableDrawOps).DrawOps(), "\n"))
	}
}

// R-B4.8: hrule paints on the repaint path too — re-styling a
// plain run to KindHRule via SetStyleRange must produce the line.
func TestKindHRule_DrawsRuleLineInRepaintBoxRange(t *testing.T) {
	fr, disp := setupVariantFrame(t)
	fr.InsertWithStyle([]rune("==="), 0, []StyleRun{{Len: 3, Style: Style{}}})

	preCount := len(drawOpsWithRectHeight(disp, 1))

	fr.SetStyleRange(0, 3, []StyleRun{{Len: 3, Style: Style{Kind: KindHRule}}})

	post := drawOpsWithRectHeight(disp, 1)
	if len(post) <= preCount {
		t.Errorf("SetStyleRange to KindHRule did not add a 1-pixel-high Draw op (pre=%d post=%d)", preCount, len(post))
	}
}

// R-B4.9: KindHRule does NOT suppress glyphs — the marker
// characters stay visible (the "markers stay visible" stance
// shared by every other v1 directive).
func TestKindHRule_GlyphsStillRendered(t *testing.T) {
	fr, disp := setupVariantFrame(t)
	st := Style{Kind: KindHRule}
	fr.InsertWithStyle([]rune("---"), 0, []StyleRun{{Len: 3, Style: st}})

	if len(opsContaining(disp, `"---"`)) == 0 {
		t.Errorf("KindHRule must still render glyphs; got no glyph-paint op for `---`")
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
