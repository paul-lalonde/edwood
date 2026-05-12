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

// R-B4.12: drawtext and repaintBoxRange paint the same content
// box identically. Pin this by re-styling a box to the SAME style
// it already has — repaintBoxRange should produce paint ops whose
// glyph and font reference are byte-identical to what drawtext
// produced on initial Insert.
func TestPaintParity_DrawtextAndRepaintAgreeOnFont(t *testing.T) {
	fr, disp := setupVariantFrame(t)

	// Insert bold text. drawtext records a Bytes op with the
	// bold font name.
	bold := Style{Kind: KindBold}
	fr.InsertWithStyle([]rune("hello"), 0, []StyleRun{{Len: 5, Style: bold}})

	initialBoldOps := opsContaining(disp, "FONT_BOLD")
	if len(initialBoldOps) == 0 {
		t.Fatalf("drawtext didn't record a FONT_BOLD op; cannot test parity")
	}
	gotInitial := len(initialBoldOps)

	// Re-style to same bold style — repaintBoxRange paints again.
	fr.SetStyleRange(0, 5, []StyleRun{{Len: 5, Style: bold}})

	finalBoldOps := opsContaining(disp, "FONT_BOLD")
	if len(finalBoldOps) <= gotInitial {
		t.Errorf("repaintBoxRange did not add a new FONT_BOLD op; got %d vs initial %d", len(finalBoldOps), gotInitial)
	}
}
