package main

import (
	"fmt"
	"strings"
	"testing"
)

// TestFormatSpansEmptyBody: zero rune count emits empty string.
func TestFormatSpansEmptyBody(t *testing.T) {
	if got := FormatSpans(nil, 0); got != "" {
		t.Errorf("FormatSpans(nil, 0) = %q, want empty", got)
	}
}

// TestFormatSpansAllDefault: no styled spans, body of 10 runes
// emits a single default-styled span covering [0, 10).
func TestFormatSpansAllDefault(t *testing.T) {
	got := FormatSpans(nil, 10)
	want := "s 0 10 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansOneItalicMidBody: one italic span at [3, 8) in
// a body of 10 runes produces a contiguous sequence:
//
//	default [0, 3), italic [3, 8), default [8, 10).
func TestFormatSpansOneItalicMidBody(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 3, Length: 5, Italic: true}}, 10)
	want := "s 0 3 -\ns 3 5 - italic\ns 8 2 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansOneBoldAtStart: span at offset 0 needs no
// leading default.
func TestFormatSpansOneBoldAtStart(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 4, Bold: true}}, 10)
	want := "s 0 4 - bold\ns 4 6 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansOneAtEnd: span ending at totalRunes needs no
// trailing default.
func TestFormatSpansOneAtEnd(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 6, Length: 4, Italic: true}}, 10)
	want := "s 0 6 -\ns 6 4 - italic\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansAdjacentStyled: two styled spans touching with
// no gap between produce no intermediate default.
func TestFormatSpansAdjacentStyled(t *testing.T) {
	got := FormatSpans([]Span{
		{Offset: 0, Length: 3, Bold: true},
		{Offset: 3, Length: 4, Italic: true},
	}, 10)
	want := "s 0 3 - bold\ns 3 4 - italic\ns 7 3 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansFg: a colored span emits the hex.
func TestFormatSpansFg(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 5, Fg: "#0000cc"}}, 10)
	want := "s 0 5 #0000cc\ns 5 5 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansFgPlusFlags: color + flags coexist.
func TestFormatSpansFgPlusFlags(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 4, Fg: "#0000cc", Italic: true}}, 5)
	want := "s 0 4 #0000cc italic\ns 4 1 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansMultiple: three styled spans separated by gaps,
// in a body of 22 runes — matches the layout from the link-
// adjacent-to-emphasis parser test ("*pre* [mid](u) *post*",
// minus the bracket runes that aren't covered by spans).
func TestFormatSpansMultiple(t *testing.T) {
	styled := []Span{
		{Offset: 1, Length: 3, Italic: true},
		{Offset: 7, Length: 3, Fg: "#0000cc"},
		{Offset: 16, Length: 4, Italic: true},
	}
	got := FormatSpans(styled, 22)
	want := "s 0 1 -\n" +
		"s 1 3 - italic\n" +
		"s 4 3 -\n" +
		"s 7 3 #0000cc\n" +
		"s 10 6 -\n" +
		"s 16 4 - italic\n" +
		"s 20 2 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansContiguous: pin the contiguity invariant — every
// `s` line's offset must equal the previous line's offset+length.
// This is what spanparse.go enforces and what we must produce.
func TestFormatSpansContiguous(t *testing.T) {
	styled := []Span{
		{Offset: 5, Length: 3, Italic: true},
		{Offset: 12, Length: 4, Bold: true},
		{Offset: 20, Length: 5, Fg: "#0000cc"},
	}
	got := FormatSpans(styled, 30)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	cursor := 0
	for i, line := range lines {
		var off, length int
		var fg string
		// "s OFFSET LENGTH FG ..." — Sscanf reads the first
		// three numeric/text fields and ignores trailing flags.
		if _, err := fmt.Sscanf(line, "s %d %d %s", &off, &length, &fg); err != nil {
			t.Fatalf("line %d %q: parse error %v", i, line, err)
		}
		if off != cursor {
			t.Errorf("line %d: offset %d, want %d (non-contiguous)", i, off, cursor)
		}
		cursor = off + length
	}
	if cursor != 30 {
		t.Errorf("final cursor = %d, want 30", cursor)
	}
}

// TestFormatSpansClipsToBody: a styled span past the body length
// is clipped to the body bound.
func TestFormatSpansClipsToBody(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 100, Italic: true}}, 5)
	want := "s 0 5 - italic\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansDropsOutOfRangeSpan: a styled span entirely past
// the body length is dropped.
func TestFormatSpansDropsOutOfRangeSpan(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 100, Length: 5, Italic: true}}, 10)
	want := "s 0 10 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansNegativeOffsetClipped: a styled span with a
// negative Offset is clipped to start at 0. This guards the
// `start < 0` defense in fillGaps.
func TestFormatSpansNegativeOffsetClipped(t *testing.T) {
	got := FormatSpans([]Span{{Offset: -3, Length: 5, Italic: true}}, 10)
	// Clip: start=0, end=2. Italic over [0, 2). Default [2, 10).
	want := "s 0 2 - italic\ns 2 8 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansOverlapDefense: overlapping styled spans (which
// Parse should never produce, but a future refactor might) get
// the earlier-wins handling — the second span is clipped to
// start at the first's end. This guards the `start < cursor`
// defense in fillGaps.
func TestFormatSpansOverlapDefense(t *testing.T) {
	got := FormatSpans([]Span{
		{Offset: 0, Length: 5, Italic: true},
		{Offset: 3, Length: 4, Bold: true}, // overlaps the italic
	}, 10)
	// First italic [0, 5). Second bold should clip to [5, 7).
	// Then default [7, 10).
	want := "s 0 5 - italic\ns 5 2 - bold\ns 7 3 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- Scale emission tests (Phase 3 round 1) -----------------------------

// TestFormatSpansScaleOmittedForZero: Scale=0 (unset sentinel)
// produces no `scale=` flag on the wire.
func TestFormatSpansScaleOmittedForZero(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 5, Italic: true, Scale: 0}}, 5)
	want := "s 0 5 - italic\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansScaleEmitted: Scale > 0 produces a
// `scale=N.N` flag.
func TestFormatSpansScaleEmitted(t *testing.T) {
	cases := []struct {
		span Span
		want string
	}{
		{Span{Offset: 0, Length: 5, Scale: 2.0}, "s 0 5 - scale=2\n"},
		{Span{Offset: 0, Length: 5, Scale: 1.5}, "s 0 5 - scale=1.5\n"},
		{Span{Offset: 0, Length: 5, Scale: 1.25}, "s 0 5 - scale=1.25\n"},
	}
	for _, tc := range cases {
		got := FormatSpans([]Span{tc.span}, 5)
		if got != tc.want {
			t.Errorf("Span %+v: got %q, want %q", tc.span, got, tc.want)
		}
	}
}

// TestFormatSpansScaleWithFlags: scale coexists with bold/italic
// on the wire.
func TestFormatSpansScaleWithFlags(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 5, Bold: true, Italic: true, Scale: 1.5}}, 5)
	want := "s 0 5 - bold italic scale=1.5\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- Family emission tests (Phase 3 round 2) ----------------------------

// TestFormatSpansFamilyOmittedForEmpty: Family="" produces no
// `family=` flag on the wire.
func TestFormatSpansFamilyOmittedForEmpty(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 5, Italic: true, Family: ""}}, 5)
	want := "s 0 5 - italic\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansFamilyEmitted: Family="code" produces a
// `family=code` flag.
func TestFormatSpansFamilyEmitted(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 5, Family: "code"}}, 5)
	want := "s 0 5 - family=code\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansFamilyWithFlags: family coexists with bold,
// italic, scale on the wire.
func TestFormatSpansFamilyWithFlags(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 5, Bold: true, Family: "code", Scale: 1.5}}, 5)
	want := "s 0 5 - bold scale=1.5 family=code\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- HRule emission tests (Phase 3 round 3) -----------------------------

// TestFormatSpansHRuleEmitted: HRule=true produces an `hrule`
// flag; absent when HRule=false.
func TestFormatSpansHRuleEmitted(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 3, HRule: true}}, 3)
	want := "s 0 3 - hrule\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansHRuleOmittedWhenFalse: HRule=false → no
// `hrule` flag.
func TestFormatSpansHRuleOmittedWhenFalse(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 0, Length: 3}}, 3)
	want := "s 0 3 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansHRuleSurvivesGapClipping: a styled HRule span
// whose start is clipped by fillGaps (overlap-defense path)
// must keep HRule=true on the emitted run. Pins the
// HRule-passthrough field copy in fillGaps.
func TestFormatSpansHRuleSurvivesGapClipping(t *testing.T) {
	got := FormatSpans([]Span{
		{Offset: 0, Length: 5, Italic: true},
		{Offset: 3, Length: 4, HRule: true}, // overlaps; clips to [5, 7).
	}, 10)
	want := "s 0 5 - italic\ns 5 2 - hrule\ns 7 3 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- Box emission tests (Phase 3 round 4) -----------------------------

// TestFormatSpansBoxAtStart: a placement=below box at
// offset 0 covers the source markdown runes [0, length);
// emit produces a single `b` line plus a trailing default
// fill for the rest of the body.
func TestFormatSpansBoxAtStart(t *testing.T) {
	got := FormatSpans([]Span{
		{
			Offset: 0, Length: 5,
			Kind:         SpanBox,
			IsBox:        true,
			BoxPayload:   "image:./pic.png",
			BoxPlacement: "below",
		},
	}, 10)
	want := "b 0 5 0 0 - - placement=below image:./pic.png\n" +
		"s 5 5 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansBoxMidBuffer: a placement=below box at
// offset 4 (length=5) covering source runes [4, 9). Emit
// produces default fill [0, 4), the box, then default fill
// [9, 10).
func TestFormatSpansBoxMidBuffer(t *testing.T) {
	got := FormatSpans([]Span{
		{
			Offset: 4, Length: 5,
			Kind:         SpanBox,
			IsBox:        true,
			BoxPayload:   "image:./pic.png",
			BoxPlacement: "below",
		},
	}, 10)
	want := "s 0 4 -\n" +
		"b 4 5 0 0 - - placement=below image:./pic.png\n" +
		"s 9 1 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansBoxWithWidthParam: payload includes a
// width=N parameter; emitter passes it through verbatim
// (parameters are space-separated tokens after the URL).
func TestFormatSpansBoxWithWidthParam(t *testing.T) {
	got := FormatSpans([]Span{
		{
			Offset: 4, Length: 5,
			Kind:         SpanBox,
			IsBox:        true,
			BoxPayload:   "image:./pic.png width=200",
			BoxPlacement: "below",
		},
	}, 10)
	want := "s 0 4 -\n" +
		"b 4 5 0 0 - - placement=below image:./pic.png width=200\n" +
		"s 9 1 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansBoxPlacementReplaceExplicit: a box with
// placement="replace" emits placement=replace on the wire
// (round-trip). Useful for producers that want to be
// explicit about the existing replacing semantic.
func TestFormatSpansBoxPlacementReplaceExplicit(t *testing.T) {
	got := FormatSpans([]Span{
		{
			Offset: 0, Length: 1,
			Kind:     SpanBox,
			IsBox:    true,
			BoxWidth: 100, BoxHeight: 50,
			BoxPayload:   "image:./pic.png",
			BoxPlacement: "replace",
		},
	}, 10)
	want := "b 0 1 100 50 - - placement=replace image:./pic.png\n" +
		"s 1 9 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- Region directive emit tests (Phase 3 round 5) -------------------

// TestFormatSpansRegionBasic: a fenced code block produces
// the expected wire output: default fill before the fence,
// `begin region code`, the body span, `end region`,
// default fill after.
func TestFormatSpansRegionBasic(t *testing.T) {
	got := FormatSpans([]Span{
		{Offset: 4, Length: 0, Kind: SpanRegionBegin, RegionBegin: "code"},
		{Offset: 4, Length: 4, Family: "code"},
		{Offset: 8, Length: 0, Kind: SpanRegionEnd, RegionEnd: true},
	}, 11)
	want := "s 0 4 -\n" +
		"begin region code\n" +
		"s 4 4 - family=code\n" +
		"end region\n" +
		"s 8 3 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansRegionWithLang: a region with a `lang`
// param emits the param after the kind.
func TestFormatSpansRegionWithLang(t *testing.T) {
	got := FormatSpans([]Span{
		{
			Kind:   SpanRegionBegin,
			Offset: 0, Length: 0,
			RegionBegin:  "code",
			RegionParams: map[string]string{"lang": "go"},
		},
		{Offset: 0, Length: 5, Family: "code"},
		{Offset: 5, Length: 0, Kind: SpanRegionEnd, RegionEnd: true},
	}, 5)
	want := "begin region code lang=go\n" +
		"s 0 5 - family=code\n" +
		"end region\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansRegionAtStart: a region beginning at
// offset 0 with no preceding styled spans produces the
// begin directive first (no leading default fill).
func TestFormatSpansRegionAtStart(t *testing.T) {
	got := FormatSpans([]Span{
		{Offset: 0, Length: 0, Kind: SpanRegionBegin, RegionBegin: "code"},
		{Offset: 0, Length: 5, Family: "code"},
		{Offset: 5, Length: 0, Kind: SpanRegionEnd, RegionEnd: true},
	}, 10)
	want := "begin region code\n" +
		"s 0 5 - family=code\n" +
		"end region\n" +
		"s 5 5 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansRegionAtEnd: a region ending at exactly
// totalRunes produces the end directive after the last
// styled span; no trailing default fill.
func TestFormatSpansRegionAtEnd(t *testing.T) {
	got := FormatSpans([]Span{
		{Offset: 0, Length: 0, Kind: SpanRegionBegin, RegionBegin: "code"},
		{Offset: 0, Length: 10, Family: "code"},
		{Offset: 10, Length: 0, Kind: SpanRegionEnd, RegionEnd: true},
	}, 10)
	want := "begin region code\n" +
		"s 0 10 - family=code\n" +
		"end region\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansRegionEmpty: a region with no body (begin
// immediately followed by end at the same offset) emits
// just the directive pair, no body span.
func TestFormatSpansRegionEmpty(t *testing.T) {
	got := FormatSpans([]Span{
		{Offset: 5, Length: 0, Kind: SpanRegionBegin, RegionBegin: "code"},
		{Offset: 5, Length: 0, Kind: SpanRegionEnd, RegionEnd: true},
	}, 10)
	want := "s 0 5 -\n" +
		"begin region code\n" +
		"end region\n" +
		"s 5 5 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansBoxPlacementOmittedWhenEmpty: a box with
// BoxPlacement="" omits the placement= flag (default
// replacing semantic).
func TestFormatSpansBoxPlacementOmittedWhenEmpty(t *testing.T) {
	got := FormatSpans([]Span{
		{
			Offset: 0, Length: 1,
			Kind:     SpanBox,
			IsBox:    true,
			BoxWidth: 100, BoxHeight: 50,
			BoxPayload: "image:./pic.png",
		},
	}, 10)
	want := "b 0 1 100 50 - - image:./pic.png\n" +
		"s 1 9 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestFormatSpansSpanAtExactlyTotalRunes: a styled span starting
// AT totalRunes (zero remaining body) is dropped.
func TestFormatSpansSpanAtExactlyTotalRunes(t *testing.T) {
	got := FormatSpans([]Span{{Offset: 10, Length: 1, Italic: true}}, 10)
	want := "s 0 10 -\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
