// Tests for the bridge functions moved from wind.go in Tier 2.4.
// These exercise the package-private styleAttrsToRichStyle,
// boxStyleToRichStyle, and applyImagePayload helpers. The
// wider Render path is exercised by integration via main package.
package spans

import (
	"image/color"
	"testing"

	"github.com/rjkroege/edwood/rich"
)


// =========================================================================
// styleAttrsToRichStyle tests
// =========================================================================

func TestStyleAttrsToRichStyle_Default(t *testing.T) {
	// Zero-value StyleAttrs should map to rich.DefaultStyle().
	sa := StyleAttrs{}
	got := styleAttrsToRichStyle(sa)
	want := rich.DefaultStyle()

	if got.Scale != want.Scale {
		t.Errorf("Scale = %v, want %v", got.Scale, want.Scale)
	}
	if got.Bold != want.Bold {
		t.Errorf("Bold = %v, want %v", got.Bold, want.Bold)
	}
	if got.Italic != want.Italic {
		t.Errorf("Italic = %v, want %v", got.Italic, want.Italic)
	}
	if got.Fg != want.Fg {
		t.Errorf("Fg = %v, want %v", got.Fg, want.Fg)
	}
	if got.Bg != want.Bg {
		t.Errorf("Bg = %v, want %v", got.Bg, want.Bg)
	}
}

func TestStyleAttrsToRichStyle_ColorsAndFlags(t *testing.T) {
	red := color.RGBA{R: 0xff, A: 0xff}
	green := color.RGBA{G: 0xff, A: 0xff}

	sa := StyleAttrs{
		Fg:     red,
		Bg:     green,
		Bold:   true,
		Italic: true,
	}
	got := styleAttrsToRichStyle(sa)

	if got.Scale != 1.0 {
		t.Errorf("Scale = %v, want 1.0", got.Scale)
	}
	if !got.Bold {
		t.Error("Bold = false, want true")
	}
	if !got.Italic {
		t.Error("Italic = false, want true")
	}

	// Compare colors via RGBA values to handle interface comparison.
	if got.Fg == nil {
		t.Fatal("Fg is nil, want red")
	}
	r, g, b, a := got.Fg.RGBA()
	wr, wg, wb, wa := red.RGBA()
	if r != wr || g != wg || b != wb || a != wa {
		t.Errorf("Fg RGBA = (%d,%d,%d,%d), want (%d,%d,%d,%d)", r, g, b, a, wr, wg, wb, wa)
	}

	if got.Bg == nil {
		t.Fatal("Bg is nil, want green")
	}
	r, g, b, a = got.Bg.RGBA()
	wr, wg, wb, wa = green.RGBA()
	if r != wr || g != wg || b != wb || a != wa {
		t.Errorf("Bg RGBA = (%d,%d,%d,%d), want (%d,%d,%d,%d)", r, g, b, a, wr, wg, wb, wa)
	}
}

func TestStyleAttrsToRichStyle_FgOnly(t *testing.T) {
	blue := color.RGBA{B: 0xff, A: 0xff}
	sa := StyleAttrs{Fg: blue}
	got := styleAttrsToRichStyle(sa)

	if got.Fg == nil {
		t.Fatal("Fg is nil, want blue")
	}
	if got.Bg != nil {
		t.Errorf("Bg = %v, want nil", got.Bg)
	}
	if got.Bold {
		t.Error("Bold = true, want false")
	}
	if got.Italic {
		t.Error("Italic = true, want false")
	}
	if got.Scale != 1.0 {
		t.Errorf("Scale = %v, want 1.0", got.Scale)
	}
}

func TestStyleAttrsToRichStyle_HiddenNotMapped(t *testing.T) {
	// Hidden is reserved and should not affect the rich.Style output
	// beyond what the zero-value already provides.
	sa := StyleAttrs{Hidden: true}
	got := styleAttrsToRichStyle(sa)
	want := rich.DefaultStyle()

	if got.Scale != want.Scale {
		t.Errorf("Scale = %v, want %v", got.Scale, want.Scale)
	}
	if got.Bold != want.Bold {
		t.Errorf("Bold = %v, want %v", got.Bold, want.Bold)
	}
	if got.Italic != want.Italic {
		t.Errorf("Italic = %v, want %v", got.Italic, want.Italic)
	}
}

// TestStyledShowScrollsRichText verifies that when Show() is called in
// styled mode for text that is off-screen, the rich text frame scrolls
// (not just the hidden plain text frame).
// =========================================================================
// boxStyleToRichStyle tests
// =========================================================================

func TestBoxStyleToRichStyleImagePayload(t *testing.T) {
	sa := StyleAttrs{
		IsBox:      true,
		BoxWidth:   200,
		BoxHeight:  150,
		BoxPayload: "image:/tmp/diagram.png",
		Fg:         color.RGBA{R: 0xff, A: 0xff},
		Bold:       true,
	}
	got := boxStyleToRichStyle(sa, "alt text")

	if !got.Image {
		t.Error("Image should be true")
	}
	if got.ImageWidth != 200 {
		t.Errorf("ImageWidth = %d; want 200", got.ImageWidth)
	}
	if got.ImageHeight != 150 {
		t.Errorf("ImageHeight = %d; want 150", got.ImageHeight)
	}
	if got.ImageURL != "/tmp/diagram.png" {
		t.Errorf("ImageURL = %q; want %q", got.ImageURL, "/tmp/diagram.png")
	}
	if got.ImageAlt != "alt text" {
		t.Errorf("ImageAlt = %q; want %q", got.ImageAlt, "alt text")
	}
	if !got.Bold {
		t.Error("Bold should be true")
	}
	if got.Scale != 1.0 {
		t.Errorf("Scale = %v; want 1.0", got.Scale)
	}
	if got.Fg == nil {
		t.Error("Fg should not be nil")
	}
}

func TestBoxStyleToRichStyleNoPayload(t *testing.T) {
	sa := StyleAttrs{
		IsBox:     true,
		BoxWidth:  100,
		BoxHeight: 50,
	}
	got := boxStyleToRichStyle(sa, "placeholder")

	if got.Image {
		t.Error("Image should be false for no-payload box")
	}
	if !got.FixedBox {
		t.Error("FixedBox should be true")
	}
	if got.ImageURL != "" {
		t.Errorf("ImageURL = %q; want empty", got.ImageURL)
	}
	if got.ImageAlt != "placeholder" {
		t.Errorf("ImageAlt = %q; want %q", got.ImageAlt, "placeholder")
	}
	if got.ImageWidth != 100 {
		t.Errorf("ImageWidth = %d; want 100", got.ImageWidth)
	}
	if got.ImageHeight != 50 {
		t.Errorf("ImageHeight = %d; want 50", got.ImageHeight)
	}
}

func TestBoxStyleToRichStyleNonImagePayload(t *testing.T) {
	sa := StyleAttrs{
		IsBox:      true,
		BoxWidth:   300,
		BoxHeight:  200,
		BoxPayload: "widget:chart-v2",
	}
	got := boxStyleToRichStyle(sa, "chart")

	if got.ImageURL != "" {
		t.Errorf("ImageURL = %q; want empty for non-image payload", got.ImageURL)
	}
}

// --- BoxPlacement + payload-param plumbing tests (Phase 3 round 4) -----

// TestBoxStyleToRichStyleImageBelow: BoxPlacement="below"
// maps to Style.ImageBelow=true; the box still produces an
// image span, source URL is parsed from the first payload
// token, alt text passes through.
func TestBoxStyleToRichStyleImageBelow(t *testing.T) {
	sa := StyleAttrs{
		IsBox:        true,
		BoxWidth:     0,
		BoxHeight:    0,
		BoxPayload:   "image:./pic.png",
		BoxPlacement: "below",
	}
	got := boxStyleToRichStyle(sa, "alt")

	if !got.ImageBelow {
		t.Error("ImageBelow should be true for BoxPlacement=below")
	}
	if !got.Image {
		t.Error("Image should be true")
	}
	if got.ImageURL != "./pic.png" {
		t.Errorf("ImageURL = %q; want %q", got.ImageURL, "./pic.png")
	}
	if got.ImageAlt != "alt" {
		t.Errorf("ImageAlt = %q; want %q", got.ImageAlt, "alt")
	}
}

// TestBoxStyleToRichStyleImageBelowReplaceExplicit:
// BoxPlacement="replace" is treated the same as "" — no
// ImageBelow.
func TestBoxStyleToRichStyleImageBelowReplaceExplicit(t *testing.T) {
	sa := StyleAttrs{
		IsBox:        true,
		BoxWidth:     100,
		BoxHeight:    50,
		BoxPayload:   "image:./pic.png",
		BoxPlacement: "replace",
	}
	got := boxStyleToRichStyle(sa, "alt")
	if got.ImageBelow {
		t.Error("ImageBelow should be false for BoxPlacement=replace")
	}
}

// TestBoxStyleToRichStyleImageBelowAbsent: empty
// BoxPlacement → Style.ImageBelow=false (default).
func TestBoxStyleToRichStyleImageBelowAbsent(t *testing.T) {
	sa := StyleAttrs{
		IsBox:      true,
		BoxWidth:   100,
		BoxHeight:  50,
		BoxPayload: "image:./pic.png",
	}
	got := boxStyleToRichStyle(sa, "alt")
	if got.ImageBelow {
		t.Error("ImageBelow should be false for empty BoxPlacement")
	}
}

// TestBoxStyleToRichStylePayloadWidthParam: a payload of
// "image:URL width=N" applies N to Style.ImageWidth. The
// `width=N` token follows the URL and is parsed by the
// consumer, not by the wire-format parser.
func TestBoxStyleToRichStylePayloadWidthParam(t *testing.T) {
	sa := StyleAttrs{
		IsBox:        true,
		BoxPayload:   "image:./pic.png width=200",
		BoxPlacement: "below",
	}
	got := boxStyleToRichStyle(sa, "alt")
	if got.ImageURL != "./pic.png" {
		t.Errorf("ImageURL = %q; want %q (URL only)", got.ImageURL, "./pic.png")
	}
	if got.ImageWidth != 200 {
		t.Errorf("ImageWidth = %d; want 200 (from payload param)", got.ImageWidth)
	}
}

// TestBoxStyleToRichStylePayloadUnknownParamIgnored: an
// unknown payload param is silently ignored (forward-compat
// for future params on older renderers).
func TestBoxStyleToRichStylePayloadUnknownParamIgnored(t *testing.T) {
	sa := StyleAttrs{
		IsBox:        true,
		BoxPayload:   "image:./pic.png alignment=center caption=hello",
		BoxPlacement: "below",
	}
	got := boxStyleToRichStyle(sa, "alt")
	// URL still parses correctly; unknown params don't break.
	if got.ImageURL != "./pic.png" {
		t.Errorf("ImageURL = %q; want %q", got.ImageURL, "./pic.png")
	}
}

// TestBoxStyleToRichStylePayloadMultipleParams: multiple
// recognized params apply (currently only width=N is
// recognized, but the parser must handle param ordering
// and multiple-token payloads cleanly).
func TestBoxStyleToRichStylePayloadMultipleParams(t *testing.T) {
	sa := StyleAttrs{
		IsBox:        true,
		BoxPayload:   "image:./p.png width=300 unknown=foo",
		BoxPlacement: "below",
	}
	got := boxStyleToRichStyle(sa, "")
	if got.ImageWidth != 300 {
		t.Errorf("ImageWidth = %d; want 300", got.ImageWidth)
	}
	if got.ImageURL != "./p.png" {
		t.Errorf("ImageURL = %q; want %q", got.ImageURL, "./p.png")
	}
}

// TestBoxStyleToRichStylePayloadWidthOverride: an explicit
// BoxWidth from the wire format takes effect when no
// width=N param is present. When BOTH are set, the payload
// param wins (treats wire BoxWidth as a legacy hint that
// payload params override).
func TestBoxStyleToRichStylePayloadWidthOverride(t *testing.T) {
	sa := StyleAttrs{
		IsBox:        true,
		BoxWidth:     100, // wire-format hint (legacy mode)
		BoxHeight:    80,
		BoxPayload:   "image:./p.png width=200",
		BoxPlacement: "below",
	}
	got := boxStyleToRichStyle(sa, "")
	// Payload param wins for width.
	if got.ImageWidth != 200 {
		t.Errorf("ImageWidth = %d; want 200 (payload param wins)", got.ImageWidth)
	}
}

// TestBoxStyleToRichStylePayloadInvalidWidth: a
// non-numeric width=X is silently ignored (treated like an
// unknown param).
func TestBoxStyleToRichStylePayloadInvalidWidth(t *testing.T) {
	sa := StyleAttrs{
		IsBox:        true,
		BoxPayload:   "image:./p.png width=abc",
		BoxPlacement: "below",
	}
	got := boxStyleToRichStyle(sa, "")
	if got.ImageURL != "./p.png" {
		t.Errorf("ImageURL = %q; want %q", got.ImageURL, "./p.png")
	}
	// width=abc is invalid → ImageWidth stays 0 (unset).
	if got.ImageWidth != 0 {
		t.Errorf("ImageWidth = %d; want 0 (invalid width=abc ignored)", got.ImageWidth)
	}
}

// TestBoxStyleToRichStylePayloadMalformedFirstToken: when
// the first payload token doesn't have an `image:` prefix,
// applyImagePayload silently returns without setting any
// image fields. This protects the consumer from a stale or
// misshapen payload (e.g., "widget:foo" or "image" alone or
// just "garbage"); Style.Image stays false and the rich.Style
// renders as a plain box.
func TestBoxStyleToRichStylePayloadMalformedFirstToken(t *testing.T) {
	cases := []struct {
		name    string
		payload string
	}{
		{"non-image prefix", "widget:foo width=200"},
		{"image without colon", "image width=200"},
		{"empty payload", ""},
		{"only whitespace", "   "},
		{"plain text", "garbage payload"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sa := StyleAttrs{
				IsBox:        true,
				BoxWidth:     100,
				BoxHeight:    50,
				BoxPayload:   tc.payload,
				BoxPlacement: "below",
			}
			got := boxStyleToRichStyle(sa, "alt")
			if got.Image {
				t.Errorf("Image = true for malformed payload %q; want false", tc.payload)
			}
			if got.ImageURL != "" {
				t.Errorf("ImageURL = %q for malformed payload %q; want empty",
					got.ImageURL, tc.payload)
			}
			// Width param shouldn't apply if the URL token didn't
			// match (param parsing should bail early).
			if got.ImageWidth != sa.BoxWidth {
				t.Errorf("ImageWidth = %d for malformed payload %q; want %d (wire-format BoxWidth unchanged)",
					got.ImageWidth, tc.payload, sa.BoxWidth)
			}
		})
	}
}

// --- Scale mapping tests (Phase 3 round 1) -------------------------------

// TestStyleAttrsToRichStyle_ScaleUnsetMapsToOne: StyleAttrs.Scale=0
// (the unset sentinel) maps to rich.Style.Scale=1.0 (body baseline).
func TestStyleAttrsToRichStyle_ScaleUnsetMapsToOne(t *testing.T) {
	sa := StyleAttrs{Scale: 0}
	got := styleAttrsToRichStyle(sa)
	if got.Scale != 1.0 {
		t.Errorf("Scale = %v, want 1.0 (Scale=0 must map to 1.0 baseline)", got.Scale)
	}
}

// TestStyleAttrsToRichStyle_ScalePassedThrough: positive Scale
// values pass through directly (no transformation, no clamp).
// The parser already clamped/validated.
func TestStyleAttrsToRichStyle_ScalePassedThrough(t *testing.T) {
	cases := []float64{0.5, 1.0, 1.25, 1.5, 2.0, 5.0}
	for _, scale := range cases {
		sa := StyleAttrs{Scale: scale}
		got := styleAttrsToRichStyle(sa)
		if got.Scale != scale {
			t.Errorf("Scale=%v passed through as %v", scale, got.Scale)
		}
	}
}

// TestBoxStyleToRichStyle_ScaleAlsoPassedThrough: the box-style
// path also honors Scale (consistency with span path).
func TestBoxStyleToRichStyle_ScaleAlsoPassedThrough(t *testing.T) {
	sa := StyleAttrs{Scale: 1.5, IsBox: true, BoxWidth: 100, BoxHeight: 50}
	got := boxStyleToRichStyle(sa, "alt")
	if got.Scale != 1.5 {
		t.Errorf("box Scale = %v, want 1.5", got.Scale)
	}
}

// --- Family mapping tests (Phase 3 round 2) ------------------------------

// TestStyleAttrsToRichStyle_FamilyEmptyLeavesCodeFalse: the unset
// Family ("") leaves rich.Style.Code at its zero value (false).
func TestStyleAttrsToRichStyle_FamilyEmptyLeavesCodeFalse(t *testing.T) {
	sa := StyleAttrs{Family: ""}
	got := styleAttrsToRichStyle(sa)
	if got.Code {
		t.Error("Code should be false for empty Family")
	}
}

// TestStyleAttrsToRichStyle_FamilyCodeMapsToCodeTrue: Family="code"
// maps to rich.Style.Code=true.
func TestStyleAttrsToRichStyle_FamilyCodeMapsToCodeTrue(t *testing.T) {
	sa := StyleAttrs{Family: "code"}
	got := styleAttrsToRichStyle(sa)
	if !got.Code {
		t.Error("Code should be true for Family=\"code\"")
	}
}

// TestStyleAttrsToRichStyle_FamilyUnknownIgnored: unknown family
// values (which shouldn't reach this layer because the parser
// rejects them, but defensively...) leave Code=false.
func TestStyleAttrsToRichStyle_FamilyUnknownIgnored(t *testing.T) {
	sa := StyleAttrs{Family: "serif"}
	got := styleAttrsToRichStyle(sa)
	if got.Code {
		t.Error("Code should not be true for unknown Family")
	}
}

// TestBoxStyleToRichStyle_FamilyAlsoMapped: the box-style path
// also honors Family (consistency with span path).
func TestBoxStyleToRichStyle_FamilyAlsoMapped(t *testing.T) {
	sa := StyleAttrs{Family: "code", IsBox: true, BoxWidth: 100, BoxHeight: 50}
	got := boxStyleToRichStyle(sa, "alt")
	if !got.Code {
		t.Error("box Code should be true for Family=\"code\"")
	}
}

// --- HRule mapping tests (Phase 3 round 3) -------------------------------

// TestStyleAttrsToRichStyle_HRulePassedThrough: HRule=true →
// rich.Style.HRule=true.
func TestStyleAttrsToRichStyle_HRulePassedThrough(t *testing.T) {
	sa := StyleAttrs{HRule: true}
	got := styleAttrsToRichStyle(sa)
	if !got.HRule {
		t.Error("rich.Style.HRule should be true for StyleAttrs.HRule=true")
	}
}

// TestStyleAttrsToRichStyle_HRuleFalsePassedThrough: HRule=false
// → rich.Style.HRule=false.
func TestStyleAttrsToRichStyle_HRuleFalsePassedThrough(t *testing.T) {
	sa := StyleAttrs{HRule: false}
	got := styleAttrsToRichStyle(sa)
	if got.HRule {
		t.Error("rich.Style.HRule should be false for StyleAttrs.HRule=false")
	}
}

// TestBoxStyleToRichStyle_HRuleAlsoMapped: box path honors HRule.
func TestBoxStyleToRichStyle_HRuleAlsoMapped(t *testing.T) {
	sa := StyleAttrs{HRule: true, IsBox: true, BoxWidth: 100, BoxHeight: 1}
	got := boxStyleToRichStyle(sa, "alt")
	if !got.HRule {
		t.Error("box rich.Style.HRule should be true for StyleAttrs.HRule=true")
	}
}
