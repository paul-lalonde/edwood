package main

import (
	"reflect"
	"testing"
)

func TestTokenizeSimple(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []token
	}{
		{
			name: "empty",
			body: "",
			want: nil,
		},
		{
			name: "single token",
			body: "foo.png",
			want: []token{{runeStart: 0, runeLen: 7, name: "foo.png"}},
		},
		{
			name: "trailing newline",
			body: "foo.png\n",
			want: []token{{runeStart: 0, runeLen: 7, name: "foo.png"}},
		},
		{
			name: "newline-separated",
			body: "a.png\nb.jpg\n",
			want: []token{
				{runeStart: 0, runeLen: 5, name: "a.png"},
				{runeStart: 6, runeLen: 5, name: "b.jpg"},
			},
		},
		{
			name: "tab-separated columnar",
			body: "a.png\tb.jpg\n",
			want: []token{
				{runeStart: 0, runeLen: 5, name: "a.png"},
				{runeStart: 6, runeLen: 5, name: "b.jpg"},
			},
		},
		{
			name: "multiple tabs as one separator",
			body: "a.png\t\t\tb.jpg",
			want: []token{
				{runeStart: 0, runeLen: 5, name: "a.png"},
				{runeStart: 8, runeLen: 5, name: "b.jpg"},
			},
		},
		{
			name: "leading whitespace",
			body: "  foo.png",
			want: []token{{runeStart: 2, runeLen: 7, name: "foo.png"}},
		},
		{
			name: "directories interleaved",
			body: "src/\tdoc.png\tREADME\n",
			want: []token{
				{runeStart: 0, runeLen: 4, name: "src/"},
				{runeStart: 5, runeLen: 7, name: "doc.png"},
				{runeStart: 13, runeLen: 6, name: "README"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenize(tt.body)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestTokenizeBackslashEscape(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []token
	}{
		{
			name: "escaped space inside filename",
			body: `foo\ bar.png`,
			want: []token{{runeStart: 0, runeLen: 12, name: "foo bar.png"}},
		},
		{
			name: "escaped tab inside filename",
			body: "a\\\tb.png",
			want: []token{{runeStart: 0, runeLen: 8, name: "a\tb.png"}},
		},
		{
			name: "escaped backslash",
			body: `a\\b.png`,
			want: []token{{runeStart: 0, runeLen: 8, name: `a\b.png`}},
		},
		{
			name: "two tokens, one with escaped space",
			body: `a.png foo\ bar.jpg`,
			want: []token{
				{runeStart: 0, runeLen: 5, name: "a.png"},
				{runeStart: 6, runeLen: 12, name: "foo bar.jpg"},
			},
		},
		{
			name: "trailing backslash at EOF (degenerate)",
			body: `foo\`,
			want: []token{{runeStart: 0, runeLen: 4, name: `foo\`}},
		},
		{
			name: "escaped newline inside filename",
			body: "a\\\nb",
			want: []token{{runeStart: 0, runeLen: 4, name: "a\nb"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenize(tt.body)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestIsImageName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"foo.png", true},
		{"BAR.PNG", true},
		{"x.jpg", true},
		{"x.jpeg", true},
		{"x.gif", true},
		{"x.webp", true},
		{"src/", false},
		{"notes.txt", false},
		{"README", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isImageName(tt.name); got != tt.want {
			t.Errorf("isImageName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestScanDirectoryColumnar(t *testing.T) {
	body := "a.png\tREADME\tb.jpg\nsrc/\tc.gif\n"
	got := scanDirectory(body, "/x/")
	want := []Span{
		{
			Kind:         SpanBox,
			Offset:       0,
			Length:       5,
			BoxWidth:     0,
			BoxPlacement: "below",
			BoxPayload:   "image:/x/a.png width=200",
		},
		{
			Kind:         SpanBox,
			Offset:       13,
			Length:       5,
			BoxWidth:     0,
			BoxPlacement: "below",
			BoxPayload:   "image:/x/b.jpg width=200",
		},
		{
			Kind:         SpanBox,
			Offset:       24,
			Length:       5,
			BoxWidth:     0,
			BoxPlacement: "below",
			BoxPayload:   "image:/x/c.gif width=200",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestScanDirectoryEscapedSpace(t *testing.T) {
	body := `a.png foo\ bar.jpg c.gif`
	got := scanDirectory(body, "/x/")
	if len(got) != 3 {
		t.Fatalf("got %d spans, want 3", len(got))
	}
	// Middle token: "foo\ bar.jpg" — 12 on-screen runes,
	// payload uses unescaped "foo bar.jpg".
	mid := got[1]
	if mid.Length != 12 {
		t.Errorf("escaped-space token length = %d, want 12", mid.Length)
	}
	if mid.BoxPayload != "image:/x/foo bar.jpg width=200" {
		t.Errorf("payload = %q, want image:/x/foo bar.jpg width=200", mid.BoxPayload)
	}
	if mid.Offset != 6 {
		t.Errorf("offset = %d, want 6", mid.Offset)
	}
}

func TestScanDirectorySkipsDirectories(t *testing.T) {
	got := scanDirectory("subdir/\timage.png\tREADME\n", "/x/")
	if len(got) != 1 {
		t.Fatalf("got %d spans, want 1", len(got))
	}
	if got[0].BoxPayload != "image:/x/image.png width=200" {
		t.Errorf("payload = %q", got[0].BoxPayload)
	}
}

func TestScanDirectoryUnicode(t *testing.T) {
	body := "café.png\tb.png\n"
	got := scanDirectory(body, "/x/")
	if len(got) != 2 {
		t.Fatalf("got %d spans, want 2", len(got))
	}
	if got[0].Length != 8 { // "café.png" = 8 runes
		t.Errorf("first span length = %d, want 8", got[0].Length)
	}
	if got[1].Offset != 9 { // "café.png\t" = 9 runes
		t.Errorf("second span offset = %d, want 9", got[1].Offset)
	}
}

func TestScanDirectoryEmpty(t *testing.T) {
	if got := scanDirectory("", "/x/"); len(got) != 0 {
		t.Errorf("empty body produced %d spans, want 0", len(got))
	}
}
