package spans

import (
	"image/color"
	"testing"

	"github.com/rjkroege/edwood/frame"
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

// R-B4.1: `b`, `begin region`, `end region` are silently accepted
// and emit OpNoOp directives. The applier ignores them. Lets
// md2spans output flow through unmodified until Slice C lands
// rendering for replaced elements / block context.

func TestParseDirective_BAcceptedAsNoOp(t *testing.T) {
	cases := []string{
		"b 0 1 100 50 - - image:/x",
		"b 5 0 200 100 #ff0000 #00ff00 placement=below image:/foo",
		"b 12 3 80 80 - -",
	}
	for _, line := range cases {
		d, err := ParseDirective(line)
		if err != nil {
			t.Errorf("expected accept-as-noop for %q, got error: %v", line, err)
			continue
		}
		if d.Op != OpNoOp {
			t.Errorf("%q: Op = %v, want OpNoOp", line, d.Op)
		}
	}
}

func TestParseDirective_BRejectsMalformed(t *testing.T) {
	// Silent-accept only covers well-formed `b` lines per the
	// published spec (at least off, len, width, height, fg, bg).
	// Truncated forms still error so producers don't accidentally
	// generate broken-but-accepted output.
	cases := []string{
		"b",
		"b 0",
		"b 0 1",
		"b 0 1 100",
		"b 0 1 100 50",   // missing fg
		"b 0 1 100 50 -", // missing bg
		"b xyz 1 100 50 - -",
		"b 0 xyz 100 50 - -",
		"b -1 1 100 50 - -",
		"b 0 1 -100 50 - -",
		"b 0 1 100 50 garbage -",
	}
	for _, line := range cases {
		if _, err := ParseDirective(line); err == nil {
			t.Errorf("expected error for malformed `b` line %q, got nil", line)
		}
	}
}

func TestParseDirective_RegionAcceptedAsNoOp(t *testing.T) {
	cases := []string{
		"begin region code",
		"begin region listitem marker=-",
		"begin region listitem number=3",
		"begin region blockquote",
		"begin region code lang=go",
		"end region",
	}
	for _, line := range cases {
		d, err := ParseDirective(line)
		if err != nil {
			t.Errorf("expected accept-as-noop for %q, got error: %v", line, err)
			continue
		}
		if d.Op != OpNoOp {
			t.Errorf("%q: Op = %v, want OpNoOp", line, d.Op)
		}
	}
}

func TestParseDirective_BeginEndRejectsMissingRegionKeyword(t *testing.T) {
	// `begin` and `end` are only valid prefixes when followed by
	// `region`. Anything else stays an unknown-op error so
	// typos surface.
	cases := []string{
		"begin",
		"begin foo",
		"end",
		"end foo",
	}
	for _, line := range cases {
		if _, err := ParseDirective(line); err == nil {
			t.Errorf("expected error for %q, got nil", line)
		}
	}
}

func TestParseDirective_AcceptsKnownFlags(t *testing.T) {
	// Slice B / Phase B4 translates bold / italic / hidden /
	// hrule / family=code into Kind bits. Phase B2.2 R4 adds
	// scale=N.N → KindScale (with Directive.Scale carrying the
	// float). family=NAME-other-than-code is still silently
	// accepted (no bits) — non-code font families wait for
	// Slice C. All forms must succeed at the parse layer so
	// producers emitting the full published protocol work
	// without modification.
	cases := []struct {
		line     string
		wantKind frame.Kind
	}{
		{"s 0 5 #ff0000 bold", frame.KindBold},
		{"s 0 5 #ff0000 italic", frame.KindItalic},
		{"s 0 5 #ff0000 hidden", frame.KindHidden},
		{"s 0 5 #ff0000 hrule", frame.KindHRule},
		{"s 0 5 #ff0000 #00ff00 bold", frame.KindBold},
		{"s 0 5 - bold italic", frame.KindBold | frame.KindItalic},
		{"s 0 5 #ff0000 scale=2.0", frame.KindScale},
		{"s 0 5 #ff0000 family=code", frame.KindCodeFamily},
		{"s 0 5 #ff0000 family=serif", 0},
		{"s 0 5 - - bold scale=1.5 family=code", frame.KindBold | frame.KindCodeFamily | frame.KindScale},
		{"s 0 5 #ff0000 hrule family=code", frame.KindHRule | frame.KindCodeFamily},
	}
	for _, c := range cases {
		d, err := ParseDirective(c.line)
		if err != nil {
			t.Errorf("expected accept for %q, got error: %v", c.line, err)
			continue
		}
		if d.Op != OpSetStyle {
			t.Errorf("%q: Op = %v, want OpSetStyle", c.line, d.Op)
		}
		if d.Kind != c.wantKind {
			t.Errorf("%q: Kind = 0x%x, want 0x%x", c.line, d.Kind, c.wantKind)
		}
	}
}

func TestParseDirective_RejectsUnknownFlags(t *testing.T) {
	for _, line := range []string{
		"s 0 5 #ff0000 wibble",
		"s 0 5 #ff0000 wobble=42",
		"s 0 5 #ff0000 #00ff00 unknown",
		"s 0 5 - badflag",
	} {
		if _, err := ParseDirective(line); err == nil {
			t.Errorf("expected error for unknown flag in %q, got nil", line)
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
	// After Phase B4 a well-formed `b` line is OpNoOp; use an
	// unknown op to provoke the same error class.
	input := "s 0 5 #ff0000\nzzz garbage\ns 5 3 -"
	_, err := ParseAll(input)
	if err == nil {
		t.Errorf("expected error from unknown op, got nil")
	}
}

// R-B4.2: contiguity is enforced between consecutive OpSetStyle
// directives even when OpNoOp directives (regions, replaced
// elements) are interleaved. md2spans emits its `begin region`
// markers between style spans; we must still catch a real gap.

func TestParseAll_ContiguityAcrossNoOps(t *testing.T) {
	// s 0 5, begin region, s 5 3 — contiguous at the SetStyle
	// level despite the OpNoOp between them.
	input := "s 0 5 #ff0000\nbegin region code\ns 5 3 -\nend region"
	got, err := ParseAll(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Two OpSetStyle, two OpNoOp.
	var nSet, nNoop int
	for _, d := range got {
		switch d.Op {
		case OpSetStyle:
			nSet++
		case OpNoOp:
			nNoop++
		}
	}
	if nSet != 2 || nNoop != 2 {
		t.Errorf("got %d SetStyle + %d NoOp, want 2 + 2: %+v", nSet, nNoop, got)
	}
}

func TestParseAll_ContiguityViolationAcrossNoOps(t *testing.T) {
	// s 0 5, begin region, s 7 3 — gap at the SetStyle level (5 → 7);
	// the OpNoOp between them must NOT mask the violation.
	input := "s 0 5 #ff0000\nbegin region code\ns 7 3 -"
	if _, err := ParseAll(input); err == nil {
		t.Errorf("expected contiguity violation across OpNoOp, got nil")
	}
}

func TestParseAll_BAsNoOpDoesNotBreakContiguity(t *testing.T) {
	// A `b` line stays inert at the contiguity layer too. The
	// two `s` directives are contiguous and must parse cleanly.
	input := "s 0 5 #ff0000\nb 5 0 80 80 - -\ns 5 3 -"
	got, err := ParseAll(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("got %d directives, want 3: %+v", len(got), got)
	}
}

// =====================================================================
// Helpers
// =====================================================================

func toRGBA(c color.Color) color.RGBA {
	r, g, b, a := c.RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}
