package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

// fakeReader implements bodyReader. It returns canned bytes
// per file name; missing names return an error.
type fakeReader struct {
	files map[string][]byte
}

func (f fakeReader) ReadAll(file string) ([]byte, error) {
	b, ok := f.files[file]
	if !ok {
		return nil, fmt.Errorf("no canned content for %q", file)
	}
	return b, nil
}

func TestReadDirPathTrailingSlash(t *testing.T) {
	r := fakeReader{files: map[string][]byte{
		"tag": []byte("/Users/paul/dev/ Del Snarf | Look "),
	}}
	got, err := readDirPath(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/Users/paul/dev/" {
		t.Errorf("got %q, want %q", got, "/Users/paul/dev/")
	}
}

func TestReadDirPathFileWindow(t *testing.T) {
	r := fakeReader{files: map[string][]byte{
		"tag": []byte("/Users/paul/dev/notes.md Del Snarf"),
	}}
	got, err := readDirPath(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/Users/paul/dev/" {
		t.Errorf("got %q, want %q (parent dir)", got, "/Users/paul/dev/")
	}
}

func TestReadDirPathTabSeparated(t *testing.T) {
	r := fakeReader{files: map[string][]byte{
		"tag": []byte("/x/y/\tDel\tSnarf"),
	}}
	got, err := readDirPath(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/x/y/" {
		t.Errorf("got %q, want /x/y/", got)
	}
}

func TestReadDirPathEmptyTag(t *testing.T) {
	r := fakeReader{files: map[string][]byte{
		"tag": []byte(""),
	}}
	if _, err := readDirPath(r); err == nil {
		t.Errorf("expected error for empty tag")
	}
}

func TestReadDirPathReadError(t *testing.T) {
	r := fakeReader{} // tag missing
	if _, err := readDirPath(r); err == nil {
		t.Errorf("expected error when tag read fails")
	}
}

// fakeOpener implements spansOpener. It captures every byte
// written, in order, into Buf.
type fakeOpener struct {
	Buf   *bytes.Buffer
	Fails error
}

func (o *fakeOpener) OpenSpans(int) (io.WriteCloser, error) {
	if o.Fails != nil {
		return nil, o.Fails
	}
	return nopWriteCloser{o.Buf}, nil
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

func TestRenderOnceEndToEnd(t *testing.T) {
	reader := fakeReader{files: map[string][]byte{
		"body": []byte("a.png\nREADME\nb.jpg\n"),
		"tag":  []byte("/dir/ Del Snarf"),
	}}
	buf := &bytes.Buffer{}
	opener := &fakeOpener{Buf: buf}
	if err := renderOnce(reader, opener, 7); err != nil {
		t.Fatalf("renderOnce: %v", err)
	}
	got := buf.String()
	wantSubstrings := []string{
		"c\n",
		"b 0 5 0 0 - - placement=below image:/dir/a.png width=100\n",
		"b 13 5 0 0 - - placement=below image:/dir/b.jpg width=100\n",
	}
	for _, s := range wantSubstrings {
		if !strings.Contains(got, s) {
			t.Errorf("missing %q in output:\n%s", s, got)
		}
	}
}

func TestRenderOnceOpenFails(t *testing.T) {
	reader := fakeReader{files: map[string][]byte{
		"body": []byte("foo.png\n"),
		"tag":  []byte("/dir/ "),
	}}
	opener := &fakeOpener{Buf: &bytes.Buffer{}, Fails: errors.New("mount nope")}
	if err := renderOnce(reader, opener, 1); err == nil {
		t.Errorf("expected error when opener fails")
	}
}

func TestRunMissingWinid(t *testing.T) {
	getenv := func(k string) string { return "" }
	var stdout, stderr bytes.Buffer
	if rc := run(nil, getenv, &stdout, &stderr); rc != 1 {
		t.Errorf("rc = %d, want 1", rc)
	}
	if !strings.Contains(stderr.String(), "winid") {
		t.Errorf("stderr did not mention winid: %q", stderr.String())
	}
}

func TestRunBadWinid(t *testing.T) {
	getenv := func(k string) string {
		if k == "winid" {
			return "not-a-number"
		}
		return ""
	}
	var stdout, stderr bytes.Buffer
	if rc := run(nil, getenv, &stdout, &stderr); rc != 1 {
		t.Errorf("rc = %d, want 1", rc)
	}
}

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if rc := run([]string{"-h"}, func(string) string { return "" }, &stdout, &stderr); rc != 0 {
		t.Errorf("rc = %d, want 0", rc)
	}
	if !strings.Contains(stdout.String(), "usage") {
		t.Errorf("stdout missing usage text")
	}
}

func TestRunBadArg(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if rc := run([]string{"unexpected-positional"}, func(string) string { return "" }, &stdout, &stderr); rc != 2 {
		t.Errorf("rc = %d, want 2", rc)
	}
}

func TestNewStoppedTimerDoesNotFire(t *testing.T) {
	tm := newStoppedTimer()
	defer tm.Stop()
	select {
	case <-tm.C:
		t.Errorf("stopped timer should not have fired")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestResetTimerFires(t *testing.T) {
	tm := newStoppedTimer()
	defer tm.Stop()
	resetTimer(tm, 5*time.Millisecond)
	select {
	case <-tm.C:
	case <-time.After(200 * time.Millisecond):
		t.Errorf("reset timer never fired")
	}
}

func TestResetTimerDrainsPendingFire(t *testing.T) {
	// First arming: let it fire and accumulate in the channel.
	tm := newStoppedTimer()
	defer tm.Stop()
	resetTimer(tm, 1*time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	// Second arming without first reading C; resetTimer must
	// drain the pending fire.
	resetTimer(tm, 5*time.Millisecond)
	// Wait for the second fire.
	select {
	case <-tm.C:
	case <-time.After(200 * time.Millisecond):
		t.Errorf("re-armed timer never fired")
	}
	// And the channel should now be empty (no double fire).
	select {
	case <-tm.C:
		t.Errorf("timer fired twice; drain logic broken")
	case <-time.After(20 * time.Millisecond):
	}
}
