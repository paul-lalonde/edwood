package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// fakeBodyReader returns canned body bytes for ReadAll("body").
// Any other file name produces an error so misuse fails loudly
// rather than silently returning empty.
type fakeBodyReader struct {
	body []byte
	err  error
}

func (f fakeBodyReader) ReadAll(file string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	if file != "body" {
		return nil, errors.New("fakeBodyReader: only 'body' supported in tests")
	}
	return f.body, nil
}

// fakeSpansFile records all bytes written to it. Tests assert
// what writeSpans / renderOnce produced.
type fakeSpansFile struct {
	writes  [][]byte // one entry per Write call (preserves Twrite boundaries)
	closed  bool
	failOn  int    // fail the Nth Write (0-based); -1 to never fail
	failErr error  // error to return on the failing Write
}

func (f *fakeSpansFile) Write(p []byte) (int, error) {
	idx := len(f.writes)
	f.writes = append(f.writes, append([]byte(nil), p...))
	if f.failOn >= 0 && idx == f.failOn {
		return 0, f.failErr
	}
	return len(p), nil
}

func (f *fakeSpansFile) Close() error {
	f.closed = true
	return nil
}

// allWritten returns the concatenated bytes from all Write calls.
func (f *fakeSpansFile) allWritten() string {
	var b bytes.Buffer
	for _, w := range f.writes {
		b.Write(w)
	}
	return b.String()
}

// fakeSpansOpener returns a pre-populated *fakeSpansFile and
// optionally an error from OpenSpans.
type fakeSpansOpener struct {
	file   *fakeSpansFile
	openErr error
}

func (o *fakeSpansOpener) OpenSpans(winid int) (io.WriteCloser, error) {
	if o.openErr != nil {
		return nil, o.openErr
	}
	return o.file, nil
}

// TestRenderOnceEmptyBody covers the empty-body path: only a
// `c\n` clear is written; the spans payload is empty so
// writeChunked is a no-op.
func TestRenderOnceEmptyBody(t *testing.T) {
	reader := fakeBodyReader{body: []byte{}}
	file := &fakeSpansFile{failOn: -1}
	opener := &fakeSpansOpener{file: file}

	if err := renderOnce(reader, opener, 1); err != nil {
		t.Fatalf("renderOnce: %v", err)
	}
	if got := file.allWritten(); got != "c\n" {
		t.Errorf("write = %q, want %q", got, "c\n")
	}
	if !file.closed {
		t.Error("file not closed")
	}
}

// TestRenderOncePlainBody covers a body with no styling: a
// `c\n` followed by a single default-styled span covering the
// whole body.
func TestRenderOncePlainBody(t *testing.T) {
	reader := fakeBodyReader{body: []byte("hello")}
	file := &fakeSpansFile{failOn: -1}
	opener := &fakeSpansOpener{file: file}

	if err := renderOnce(reader, opener, 1); err != nil {
		t.Fatalf("renderOnce: %v", err)
	}
	got := file.allWritten()
	want := "c\ns 0 5 -\n"
	if got != want {
		t.Errorf("write = %q, want %q", got, want)
	}
	// Two separate Write calls (Twrite boundaries): clear + spans.
	if len(file.writes) != 2 {
		t.Errorf("Write call count = %d, want 2 (clear + spans)", len(file.writes))
	}
	if string(file.writes[0]) != "c\n" {
		t.Errorf("first Write = %q, want %q", file.writes[0], "c\n")
	}
}

// TestRenderOnceStyledBody covers an italic emphasis span: the
// emit produces a default→italic→default sequence covering the
// body contiguously.
func TestRenderOnceStyledBody(t *testing.T) {
	reader := fakeBodyReader{body: []byte("a *b* c")}
	file := &fakeSpansFile{failOn: -1}
	opener := &fakeSpansOpener{file: file}

	if err := renderOnce(reader, opener, 1); err != nil {
		t.Fatalf("renderOnce: %v", err)
	}
	got := file.allWritten()
	// Runes: a=0 ' '=1 *=2 b=3 *=4 ' '=5 c=6 (length 7)
	// Italic span at offset 3, length 1.
	want := "c\ns 0 3 -\ns 3 1 - italic\ns 4 3 -\n"
	if got != want {
		t.Errorf("write = %q, want %q", got, want)
	}
}

// TestRenderOnceUTF8: rune offsets are correctly counted across
// multi-byte UTF-8 input.
func TestRenderOnceUTF8(t *testing.T) {
	reader := fakeBodyReader{body: []byte("*世界*")}
	file := &fakeSpansFile{failOn: -1}
	opener := &fakeSpansOpener{file: file}

	if err := renderOnce(reader, opener, 1); err != nil {
		t.Fatalf("renderOnce: %v", err)
	}
	got := file.allWritten()
	// Rune offsets: *=0 世=1 界=2 *=3 (length 4 runes).
	// Italic span: offset 1, length 2.
	// Default-fill: offset 0 (length 1) + offset 3 (length 1)
	// covers the surrounding asterisks contiguously.
	want := "c\ns 0 1 -\ns 1 2 - italic\ns 3 1 -\n"
	if got != want {
		t.Errorf("write = %q, want %q", got, want)
	}
}

// TestRenderOnceReadBodyFails: reader error propagates with a
// wrapped message.
func TestRenderOnceReadBodyFails(t *testing.T) {
	reader := fakeBodyReader{err: errors.New("read failed")}
	opener := &fakeSpansOpener{file: &fakeSpansFile{failOn: -1}}

	err := renderOnce(reader, opener, 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "read body") {
		t.Errorf("error message = %q, want to contain 'read body'", err.Error())
	}
}

// TestRenderOnceOpenSpansFails: opener error propagates.
func TestRenderOnceOpenSpansFails(t *testing.T) {
	reader := fakeBodyReader{body: []byte("hello")}
	opener := &fakeSpansOpener{openErr: errors.New("9P open failed")}

	err := renderOnce(reader, opener, 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "open spans") {
		t.Errorf("error message = %q, want to contain 'open spans'", err.Error())
	}
}

// TestRenderOnceClearWriteFails: a failure during the clear
// Twrite is reported with a "write clear" wrapper.
func TestRenderOnceClearWriteFails(t *testing.T) {
	reader := fakeBodyReader{body: []byte("hello")}
	file := &fakeSpansFile{failOn: 0, failErr: errors.New("9P write failed")}
	opener := &fakeSpansOpener{file: file}

	err := renderOnce(reader, opener, 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "write clear") {
		t.Errorf("error message = %q, want to contain 'write clear'", err.Error())
	}
}

// TestRenderOnceSpansWriteFails: a failure during the spans
// Twrite is reported with a "write spans" wrapper.
func TestRenderOnceSpansWriteFails(t *testing.T) {
	reader := fakeBodyReader{body: []byte("hello")}
	// failOn: 1 means the second Write call (spans payload).
	file := &fakeSpansFile{failOn: 1, failErr: errors.New("9P write failed")}
	opener := &fakeSpansOpener{file: file}

	err := renderOnce(reader, opener, 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "write spans") {
		t.Errorf("error message = %q, want to contain 'write spans'", err.Error())
	}
}

// TestWriteChunkedSplitsAtNewlines: payloads larger than maxChunk
// split on newline boundaries so each Twrite is a complete set of
// directives. Pin via a synthetic large payload.
func TestWriteChunkedSplitsAtNewlines(t *testing.T) {
	// Build a payload of N short s-lines totaling ~10000 bytes so
	// the chunker emits multiple Twrites.
	var b strings.Builder
	for i := 0; i < 1000; i++ {
		b.WriteString("s 0 1 -\n")
	}
	payload := b.String()
	if len(payload) < maxChunk {
		t.Fatalf("test setup: payload too short (%d) to exercise chunking", len(payload))
	}

	file := &fakeSpansFile{failOn: -1}
	if err := writeChunked(file, payload); err != nil {
		t.Fatalf("writeChunked: %v", err)
	}
	if len(file.writes) < 2 {
		t.Errorf("expected >=2 Twrites for payload size %d, got %d", len(payload), len(file.writes))
	}
	// Every Twrite must end with '\n' (split-at-newline invariant).
	for i, w := range file.writes {
		if len(w) == 0 || w[len(w)-1] != '\n' {
			t.Errorf("write %d does not end with newline: %q", i, w)
		}
	}
	// Concatenated writes recover the full payload.
	if got := file.allWritten(); got != payload {
		t.Errorf("concatenated writes != original payload (lengths %d vs %d)", len(got), len(payload))
	}
}

// TestWriteChunkedRejectsLineTooLong: a payload with a single line
// exceeding maxChunk is rejected (no infinite loop).
func TestWriteChunkedRejectsLineTooLong(t *testing.T) {
	// One line of maxChunk+10 bytes, no newline anywhere.
	payload := strings.Repeat("x", maxChunk+10) + "\n"
	file := &fakeSpansFile{failOn: -1}
	err := writeChunked(file, payload)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "longer than") {
		t.Errorf("error message = %q, want to mention length", err.Error())
	}
}
