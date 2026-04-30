package main

import (
	"bytes"
	"strings"
	"testing"
)

// envFunc builds a minimal env-getter from a map for tests.
func envFunc(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// TestRunRejectsUnknownArgs covers R1 (negative path): invocations
// with unrecognized arguments exit with code 2 and print usage to
// stderr.
func TestRunRejectsUnknownArgs(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"--bogus"}, envFunc(nil), &stderr)
	if code != 2 {
		t.Errorf("run with unknown arg returned %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage") && !strings.Contains(stderr.String(), "Usage") {
		t.Errorf("stderr did not mention usage; got %q", stderr.String())
	}
}

// TestRunHelpExitsZero covers R1: -h prints usage and exits 0.
func TestRunHelpExitsZero(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"-h"}, envFunc(nil), &stderr)
	if code != 0 {
		t.Errorf("run -h returned %d, want 0", code)
	}
	// Usage may go to stderr or stdout depending on convention; we
	// don't pin which here. Just check the exit code.
}

// TestRunMissingWinidExitsOne covers R1: with no -h and no
// invalid args, but $winid unset, the tool exits 1 with an
// informative stderr message. This catches the most common
// mis-invocation (forgetting that md2spans must be launched
// with $winid in scope, e.g. as a B2 command from edwood).
func TestRunMissingWinidExitsOne(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{}, envFunc(nil), &stderr)
	if code != 1 {
		t.Errorf("run without $winid returned %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "winid") {
		t.Errorf("stderr did not mention winid; got %q", stderr.String())
	}
}

// TestRunRejectsExtraPositionalArgs covers R1: positional args are
// not accepted in v1 (md2spans takes its input from the window via
// $winid, not from a file path or stdin).
func TestRunRejectsExtraPositionalArgs(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"some-file.md"}, envFunc(map[string]string{"winid": "1"}), &stderr)
	if code != 2 {
		t.Errorf("run with positional arg returned %d, want 2", code)
	}
}

// TestRunBadWinidExitsOne covers a degenerate input: $winid is set
// but not parseable as an integer. Exit 1 with a useful message.
func TestRunBadWinidExitsOne(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{}, envFunc(map[string]string{"winid": "not-a-number"}), &stderr)
	if code != 1 {
		t.Errorf("run with non-integer $winid returned %d, want 1", code)
	}
}
