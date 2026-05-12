package spans

import (
	"image/color"
	"testing"
)

// Spec source: /Users/paul/dev/edwood/docs/designs/spans-protocol.md
// (Slice A subset implemented here.)

// =====================================================================
// ParseDirective (single line)
// =====================================================================

func TestParseDirective_ClearNoArgs(t *testing.T) {
	d, err := ParseDirective("c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Op != OpClearAll {
		t.Errorf("Op = %v, want OpClearAll", d.Op)
	}
	if d.Off != 0 || d.Len != 0 || d.Fg != nil || d.Bg != nil {
		t.Errorf("clear must carry no payload: %+v", d)
	}
}

func TestParseDirective_ClearRejectsArgs(t *testing.T) {
	if _, err := ParseDirective("c 0"); err == nil {
		t.Errorf("expected error: `c` takes no args")
	}
	if _, err := ParseDirective("c 0 5"); err == nil {
		t.Errorf("expected error: `c` takes no args")
	}
}

func TestParseDirective_SetStyle_FgOnly(t *testing.T) {
	d, err := ParseDirective("s 0 12 #000080")
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
		t.Errorf("Bg should be nil when omitted; got %v", d.Bg)
	}
}

func TestParseDirective_SetStyle_DashFg(t *testing.T) {
	// `-` means "default foreground"; producers use this to fill
	// gaps between styled runs.
	d, err := ParseDirective("s 0 5 -")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Op != OpSetStyle || d.Off != 0 || d.Len != 5 {
		t.Errorf("got %+v, want {OpSetStyle 0 5}", d)
	}
	if d.Fg != nil {
		t.Errorf("Fg = %v, want nil (dash means default)", d.Fg)
	}
}

func TestParseDirective_SetStyle_FgAndBg(t *testing.T) {
	d, err := ParseDirective("s 2 4 #112233 #445566")
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

func TestParseDirective_SetStyle_DashBg(t *testing.T) {
	d, err := ParseDirective("s 0 5 #ff0000 -")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Fg == nil {
		t.Errorf("Fg should not be nil")
	}
	if d.Bg != nil {
		t.Errorf("Bg = %v, want nil (explicit `-`)", d.Bg)
	}
}

func TestParseDirective_SetStyle_DashFgDashBg(t *testing.T) {
	// `s <off> <len> - -` is the explicit "default both" form.
	d, err := ParseDirective("s 0 5 - -")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Fg != nil || d.Bg != nil {
		t.Errorf("both colors should be nil: Fg=%v Bg=%v", d.Fg, d.Bg)
	}
}

// =====================================================================
// Rejections (Slice A scope)
// =====================================================================

func TestParseDirective_RejectsBDirective(t *testing.T) {
	_, err := ParseDirective("b 0 1 100 50 - - image:/x")
	if err == nil {
		t.Errorf("expected error for `b` directive (Slice C feature), got nil")
	}
}

func TestParseDirective_RejectsRegionDirectives(t *testing.T) {
	for _, line := range []string{
		"begin region code",
		"end region",
	} {
		if _, err := ParseDirective(line); err == nil {
			t.Errorf("expected error for region directive %q (Slice B/C)", line)
		}
	}
}

func TestParseDirective_RejectsFlags(t *testing.T) {
	// Per protocol Slice B/C: bold, italic, scale=, family=, hrule.
	// All rejected in Slice A.
	for _, line := range []string{
		"s 0 5 #ff0000 bold",
		"s 0 5 #ff0000 italic",
		"s 0 5 #ff0000 #00ff00 bold",
		"s 0 5 #ff0000 hrule",
		"s 0 5 #ff0000 scale=2.0",
		"s 0 5 #ff0000 family=code",
	} {
		if _, err := ParseDirective(line); err == nil {
			t.Errorf("expected error for flag in %q, got nil", line)
		}
	}
}

func TestParseDirective_RejectsUnknownOp(t *testing.T) {
	if _, err := ParseDirective("z 0 5 #ff0000"); err == nil {
		t.Errorf("expected error for unknown op `z`")
	}
}

func TestParseDirective_RejectsMalformedInteger(t *testing.T) {
	cases := []string{
		"s abc 5 #ff0000",
		"s 0 xyz #ff0000",
		"s -1 5 #ff0000",
		"s 0 -1 #ff0000",
	}
	for _, line := range cases {
		if _, err := ParseDirective(line); err == nil {
			t.Errorf("expected error for malformed integer in %q", line)
		}
	}
}

func TestParseDirective_RejectsMalformedColor(t *testing.T) {
	cases := []string{
		"s 0 5 000080",     // missing #
		"s 0 5 #12345",     // wrong length
		"s 0 5 #zzzzzz",    // non-hex
		"s 0 5 #1234567",   // too long
		"s 0 5 blue",       // named colors not supported
		"s 0 5 #ff0000 #1", // bad bg
	}
	for _, line := range cases {
		if _, err := ParseDirective(line); err == nil {
			t.Errorf("expected error for malformed color in %q", line)
		}
	}
}

func TestParseDirective_RejectsTooFewFields(t *testing.T) {
	cases := []string{
		"s",
		"s 0",
		"s 0 5", // missing fg
		"",      // empty
	}
	for _, line := range cases {
		if _, err := ParseDirective(line); err == nil {
			t.Errorf("expected error for too-few-fields %q", line)
		}
	}
}

// =====================================================================
// ParseAll (multi-line, write-level invariants)
// =====================================================================

func TestParseAll_SkipsBlankLines(t *testing.T) {
	input := `
s 0 5 #ff0000

s 5 3 -
`
	got, err := ParseAll(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d directives, want 2: %+v", len(got), got)
	}
}

func TestParseAll_ContiguityEnforced(t *testing.T) {
	// Non-contiguous offsets: second `s` starts at 7 but the first
	// ended at 5. The parser must reject.
	input := "s 0 5 #ff0000\ns 7 3 #00ff00"
	if _, err := ParseAll(input); err == nil {
		t.Errorf("expected contiguity violation, got nil")
	}
}

func TestParseAll_ContiguityHonored(t *testing.T) {
	// s 0 5, s 5 3 — contiguous; no error.
	input := "s 0 5 #ff0000\ns 5 3 #00ff00"
	got, err := ParseAll(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d directives, want 2: %+v", len(got), got)
	}
}

func TestParseAll_ClearMustBeExclusive(t *testing.T) {
	input := "c\ns 0 5 #ff0000"
	if _, err := ParseAll(input); err == nil {
		t.Errorf("expected error for c mixed with s, got nil")
	}
}

func TestParseAll_ClearAlone(t *testing.T) {
	got, err := ParseAll("c\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Op != OpClearAll {
		t.Errorf("got %+v, want one OpClearAll directive", got)
	}
}

func TestParseAll_StopsAtFirstError(t *testing.T) {
	input := "s 0 5 #ff0000\nb 5 1 0 0 - - image:x\ns 6 3 -"
	_, err := ParseAll(input)
	if err == nil {
		t.Errorf("expected error from `b` directive, got nil")
	}
}

// =====================================================================
// Helpers
// =====================================================================

func toRGBA(c color.Color) color.RGBA {
	r, g, b, a := c.RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}
