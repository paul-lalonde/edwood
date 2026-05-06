package main

import (
	"strings"
	"testing"
)

func TestFormatSpansEmpty(t *testing.T) {
	if got := FormatSpans(nil, 0); got != "" {
		t.Errorf("empty totalRunes produced %q, want \"\"", got)
	}
	got := FormatSpans(nil, 5)
	want := "s 0 5 -\n"
	if got != want {
		t.Errorf("no input, totalRunes=5: got %q, want %q", got, want)
	}
}

func TestFormatSpansBoxOnly(t *testing.T) {
	spans := []Span{
		{
			Kind:         SpanBox,
			Offset:       0,
			Length:       7,
			BoxWidth:     100,
			BoxPlacement: "below",
			BoxPayload:   "image:/x/foo.png",
		},
	}
	got := FormatSpans(spans, 8) // 7 runes + 1 newline rune
	want := "b 0 7 100 0 - - placement=below image:/x/foo.png\ns 7 1 -\n"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestFormatSpansMixedGapFill(t *testing.T) {
	spans := []Span{
		{
			Kind:         SpanBox,
			Offset:       0,
			Length:       7,
			BoxWidth:     100,
			BoxPlacement: "below",
			BoxPayload:   "image:/x/a.png",
		},
		{
			Kind:         SpanBox,
			Offset:       16,
			Length:       7,
			BoxWidth:     100,
			BoxPlacement: "below",
			BoxPayload:   "image:/x/b.jpg",
		},
	}
	got := FormatSpans(spans, 24) // a.png\n + bar.txt\n + b.jpg\n
	wantLines := []string{
		"b 0 7 100 0 - - placement=below image:/x/a.png",
		"s 7 9 -",
		"b 16 7 100 0 - - placement=below image:/x/b.jpg",
		"s 23 1 -",
		"",
	}
	want := strings.Join(wantLines, "\n")
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestFormatSpansClipsOutOfRange(t *testing.T) {
	spans := []Span{
		{
			Kind:         SpanBox,
			Offset:       3,
			Length:       100, // overshoots totalRunes
			BoxWidth:     50,
			BoxPlacement: "below",
			BoxPayload:   "image:/x/y.png",
		},
	}
	got := FormatSpans(spans, 5)
	// Box clipped to length 2 (runes 3..5).
	want := "s 0 3 -\nb 3 2 50 0 - - placement=below image:/x/y.png\n"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestNextChunkEndUnderMax(t *testing.T) {
	payload := "s 0 5 -\nb 5 1 100 0 - -\n"
	end, err := nextChunkEnd(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if end != len(payload) {
		t.Errorf("end = %d, want %d", end, len(payload))
	}
}

func TestNextChunkEndSplitsAtNewline(t *testing.T) {
	// Build a payload longer than maxChunk with newlines.
	var b strings.Builder
	for i := 0; b.Len() < maxChunk+200; i++ {
		b.WriteString("s 0 10 -\n")
	}
	payload := b.String()
	end, err := nextChunkEnd(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if end > maxChunk {
		t.Errorf("chunk end %d > maxChunk %d (no newline preceding maxChunk)", end, maxChunk)
	}
	if payload[end-1] != '\n' {
		t.Errorf("chunk end byte = %q, want newline", payload[end-1])
	}
}

func TestNextChunkEndSingleLineTooLong(t *testing.T) {
	payload := strings.Repeat("x", maxChunk+100) // no newline anywhere
	if _, err := nextChunkEnd(payload); err == nil {
		t.Errorf("expected error for single line longer than maxChunk")
	}
}

func TestWriteChunkedPreservesPayload(t *testing.T) {
	var b strings.Builder
	for i := 0; b.Len() < maxChunk+200; i++ {
		b.WriteString("s 0 10 -\n")
	}
	payload := b.String()
	var out strings.Builder
	if err := writeChunked(&out, payload); err != nil {
		t.Fatalf("writeChunked: %v", err)
	}
	if out.String() != payload {
		t.Errorf("writeChunked altered payload")
	}
}
