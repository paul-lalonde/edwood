package frame

import (
	"image"
	"strings"
	"testing"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/edwoodtest"
)

// Phase B4.1.5 refactor: paintBox + boxWid centralize per-box
// paint and width computation. These tests pin the observable
// invariants spelled out in R-B4.12 and R-B4.13.

// R-B4.13: boxWid(b) returns the width the box should carry for
// its current (Style, Ptr). For plain content boxes that's the
// base font's BytesWidth(Ptr).
func TestBoxWid_PlainBoxMatchesBaseFont(t *testing.T) {
	textarea := image.Rect(20, 10, 400, 100)
	display := edwoodtest.NewDisplay(textarea)
	var textcolors [NumColours]draw.Image
	textcolors[ColBack] = display.AllocImageMix(draw.Paleyellow, draw.White)
	textcolors[ColHigh], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Darkyellow)
	textcolors[ColBord], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Yellowgreen)
	textcolors[ColText] = display.Black()
	textcolors[ColHText] = display.Black()

	regular := edwoodtest.NewFontWithName("REG_W10", 10, 13)
	f := new(frameimpl)
	f.Init(textarea,
		OptColors(textcolors),
		OptFont(regular),
		OptBackground(display.ScreenImage()),
		OptMaxTab(8),
	)

	b := &frbox{Nrune: 5, Ptr: []byte("hello")}
	got := f.boxWid(b)
	want := regular.BytesWidth([]byte("hello"))
	if got != want {
		t.Errorf("boxWid(plain) = %d, want %d", got, want)
	}
}

// R-B4.13: boxWid honors Style.Kind — a bold box uses the bold
// font's metrics, not the base font's.
func TestBoxWid_BoldBoxUsesBoldFont(t *testing.T) {
	textarea := image.Rect(20, 10, 400, 100)
	display := edwoodtest.NewDisplay(textarea)
	var textcolors [NumColours]draw.Image
	textcolors[ColBack] = display.AllocImageMix(draw.Paleyellow, draw.White)
	textcolors[ColHigh], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Darkyellow)
	textcolors[ColBord], _ = display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, draw.Yellowgreen)
	textcolors[ColText] = display.Black()
	textcolors[ColHText] = display.Black()

	regular := edwoodtest.NewFontWithName("REG_W10", 10, 13)
	bold := edwoodtest.NewFontWithName("BOLD_W12", 12, 13)
	f := new(frameimpl)
	f.Init(textarea,
		OptColors(textcolors),
		OptFont(regular),
		OptBoldFont(bold),
		OptBackground(display.ScreenImage()),
		OptMaxTab(8),
	)

	b := &frbox{Nrune: 4, Ptr: []byte("type"), Style: Style{Kind: KindBold}}
	got := f.boxWid(b)
	want := 4 * 12
	if got != want {
		t.Errorf("boxWid(bold) = %d, want %d (bold-width=12)", got, want)
	}
}

// R-B4.13: validateboxmodel asserts b.Wid == f.boxWid(b). With
// -validateboxes set, a hand-corrupted Wid panics — this is the
// structural guard against the SetStyleRange-forgot-Wid bug class.
func TestValidateBoxModel_PanicsOnWidthMismatch(t *testing.T) {
	saved := *validate
	*validate = true
	defer func() { *validate = saved }()

	fr, _ := setupVariantFrame(t)
	f := fr.(*frameimpl)
	f.Insert([]rune("hello"), 0)
	if len(f.box) == 0 {
		t.Fatal("expected at least one box after Insert")
	}
	// Find a content box.
	var b *frbox
	for _, bx := range f.box {
		if bx != nil && bx.Nrune > 0 {
			b = bx
			break
		}
	}
	if b == nil {
		t.Fatal("no content box found")
	}
	b.Wid = b.Wid + 17

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected validateboxmodel to panic on corrupted Wid; got no panic")
		}
	}()
	f.validateboxmodel("test")
}

// R-B4.12: paintBox is the single point that resolves font, fg/bg,
// KindHidden suppression and decorations. The behavior that
// observably proves this: KindHidden suppresses glyphs on BOTH
// initial paint (drawtext) and re-style paint (repaintBoxRange).
// We already cover drawtext in font_variants_test.go; this test
// covers the repaint path, so adding a new decoration to paintBox
// is guaranteed to land in both.
func TestKindHidden_SkipsGlyphPaintInRepaintBoxRange(t *testing.T) {
	fr, disp := setupVariantFrame(t)

	// Insert plain text first so the initial paint records ops
	// for "hello" — those are expected and don't count.
	fr.InsertWithStyle([]rune("hello"), 0, []StyleRun{{Len: 5, Style: Style{}}})

	// Snapshot the pre-restyle op count for "hello" so we can
	// distinguish ops produced by SetStyleRange from the ones
	// produced by Insert above.
	preCount := len(opsContaining(disp, `"hello"`))

	// Re-style as hidden. SetStyleRange goes through
	// repaintBoxRange (the path under test).
	fr.SetStyleRange(0, 5, []StyleRun{{Len: 5, Style: Style{Kind: KindHidden}}})

	post := opsContaining(disp, "atpoint")
	for i := preCount; i < len(post); i++ {
		if strings.Contains(post[i], `"hello"`) {
			t.Errorf("KindHidden via repaint produced glyph paint: %s", post[i])
		}
	}
}

// R-B4.12: drawtext and repaintBoxRange paint content boxes
// using the same font-selection path. B2.3 R8 changed
// SetStyleRange to skip repaint when the diff says the line
// is identical, so this test now exercises a real style
// transition (plain → bold). Both paint paths must produce
// font ops appropriate to their style; the parity guarantee
// is that the bold-styled box renders with FONT_BOLD whether
// it came in via Insert (drawtext) or via SetStyleRange
// (repaintBoxRange).
func TestPaintParity_DrawtextAndRepaintAgreeOnFont(t *testing.T) {
	fr, disp := setupVariantFrame(t)

	// Insert plain text. drawtext records ops with the default
	// font; no FONT_BOLD op yet.
	fr.InsertWithStyle([]rune("hello"), 0, []StyleRun{{Len: 5, Style: Style{}}})
	preBoldOps := len(opsContaining(disp, "FONT_BOLD"))
	if preBoldOps != 0 {
		t.Fatalf("plain Insert leaked a FONT_BOLD op (%d); test premises broken", preBoldOps)
	}

	// Re-style to bold — repaintBoxRange paints the affected
	// range with the bold font.
	bold := Style{Kind: KindBold}
	fr.SetStyleRange(0, 5, []StyleRun{{Len: 5, Style: bold}})

	postBoldOps := len(opsContaining(disp, "FONT_BOLD"))
	if postBoldOps == 0 {
		t.Errorf("SetStyleRange to bold did not produce any FONT_BOLD op (repaint path didn't use the bold font)")
	}
}
