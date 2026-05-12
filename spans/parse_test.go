package spans

import (
	"image/color"
	"testing"
)

// =====================================================================
// ParseDirective (single line)
// =====================================================================

func TestParseDirective_ClearOnly(t *testing.T) {
	d, err := ParseDirective("c 5 3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Op != OpClearStyle || d.Off != 5 || d.Len != 3 {
		t.Errorf("got %+v, want {OpClearStyle 5 3}", d)
	}
	if d.Fg != nil || d.Bg != nil {
		t.Errorf("clear directive must not carry colors: %+v", d)
	}
}

func TestParseDirective_SetStyle_FgOnly(t *testing.T) {
	d, err := ParseDirective("s 0 12 fg=#000080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Op != OpSetStyle || d.Off != 0 || d.Len != 12 {
		t.Errorf("got %+v, want {OpSetStyle 0 12}", d)
	}
	wantFg := color.RGBA{R: 0x00, G: 0x00, B: 0x80, A: 0xff}
	if got := toRGBA(d.Fg); got != wantFg {
		t.Errorf("Fg = %v, want %v", got, wantFg)
	}
	if d.Bg != nil {
		t.Errorf("Bg should be nil when bg= omitted; got %v", d.Bg)
	}
}

func TestParseDirective_SetStyle_BgOnly(t *testing.T) {
	d, err := ParseDirective("s 10 5 bg=#ffeedd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Fg != nil {
		t.Errorf("Fg should be nil; got %v", d.Fg)
	}
	wantBg := color.RGBA{R: 0xff, G: 0xee, B: 0xdd, A: 0xff}
	if got := toRGBA(d.Bg); got != wantBg {
		t.Errorf("Bg = %v, want %v", got, wantBg)
	}
}

func TestParseDirective_SetStyle_FgAndBg(t *testing.T) {
	d, err := ParseDirective("s 2 4 fg=#112233 bg=#445566")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantFg := color.RGBA{R: 0x11, G: 0x22, B: 0x33, A: 0xff}
	wantBg := color.RGBA{R: 0x44, G: 0x55, B: 0x66, A: 0xff}
	if got := toRGBA(d.Fg); got != wantFg {
		t.Errorf("Fg = %v, want %v", got, wantFg)
	}
	if got := toRGBA(d.Bg); got != wantBg {
		t.Errorf("Bg = %v, want %v", got, wantBg)
	}
}

func TestParseDirective_SetStyle_NoKeys_IsLegal(t *testing.T) {
	// `s` with no style keys is a "set to plain" directive — i.e.
	// equivalent to a clear. The parser accepts it; the applier
	// can decide semantics.
	d, err := ParseDirective("s 0 5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Op != OpSetStyle {
		t.Errorf("Op = %v, want OpSetStyle", d.Op)
	}
	if d.Fg != nil || d.Bg != nil {
		t.Errorf("`s` with no keys must carry no colors: %+v", d)
	}
}

// =====================================================================
// Rejections
// =====================================================================

func TestParseDirective_RejectsBDirective(t *testing.T) {
	// Replaced elements are Slice C; parser must reject `b` lines
	// until then.
	_, err := ParseDirective("b 0 1 image w=400 h=300 ref=/tmp/cat.png")
	if err == nil {
		t.Errorf("expected error for `b` directive (Slice C feature), got nil")
	}
}

func TestParseDirective_RejectsUnknownOp(t *testing.T) {
	_, err := ParseDirective("z 0 5")
	if err == nil {
		t.Errorf("expected error for unknown op `z`, got nil")
	}
}

func TestParseDirective_RejectsUnknownKey(t *testing.T) {
	// bold/italic/underline/font are Slice B; parser must reject.
	cases := []string{
		"s 0 5 bold=1",
		"s 0 5 italic=1",
		"s 0 5 underline=1",
		"s 0 5 font=2",
	}
	for _, line := range cases {
		if _, err := ParseDirective(line); err == nil {
			t.Errorf("expected error for unknown key in %q, got nil", line)
		}
	}
}

func TestParseDirective_RejectsMalformedInteger(t *testing.T) {
	cases := []string{
		"s abc 5",
		"s 0 xyz",
		"c -1 5", // negative offsets aren't valid
		"c 5 -1",
	}
	for _, line := range cases {
		if _, err := ParseDirective(line); err == nil {
			t.Errorf("expected error for malformed integer in %q, got nil", line)
		}
	}
}

func TestParseDirective_RejectsMalformedColor(t *testing.T) {
	cases := []string{
		"s 0 5 fg=000080",   // missing #
		"s 0 5 fg=#12345",   // wrong length
		"s 0 5 fg=#zzzzzz",  // non-hex
		"s 0 5 fg=#1234567", // too long
		"s 0 5 fg=blue",     // named colors not supported in Slice A
	}
	for _, line := range cases {
		if _, err := ParseDirective(line); err == nil {
			t.Errorf("expected error for malformed color in %q, got nil", line)
		}
	}
}

func TestParseDirective_RejectsTooFewFields(t *testing.T) {
	cases := []string{
		"s",   // no off, no len
		"s 0", // no len
		"c",
		"c 0",
		"", // empty — error for ParseDirective; ParseAll skips
	}
	for _, line := range cases {
		if _, err := ParseDirective(line); err == nil {
			t.Errorf("expected error for too-few-fields %q, got nil", line)
		}
	}
}

// =====================================================================
// ParseAll (multi-line)
// =====================================================================

func TestParseAll_SkipsBlankLines(t *testing.T) {
	input := `
s 0 5 fg=#ff0000

c 10 3
`
	got, err := ParseAll(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d directives, want 2", len(got))
	}
	if got[0].Op != OpSetStyle || got[1].Op != OpClearStyle {
		t.Errorf("ops = (%v,%v), want (OpSetStyle, OpClearStyle)", got[0].Op, got[1].Op)
	}
}

func TestParseAll_StopsAtFirstError(t *testing.T) {
	input := `s 0 5 fg=#ff0000
b 5 1 image w=10 h=10 ref=x
c 6 3`
	_, err := ParseAll(input)
	if err == nil {
		t.Errorf("expected error from `b` directive, got nil")
	}
}

// Helper: extract RGBA from any color.Color so equality is direct.
func toRGBA(c color.Color) color.RGBA {
	r, g, b, a := c.RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}
