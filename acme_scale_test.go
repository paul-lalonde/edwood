package main

import (
	"testing"
)

// Phase B2.2 R4.1 — Text-side scaled-font loading. fontsrv
// serves Plan 9-style fonts at paths like
//   /mnt/font/<family>/<size>a/font
// scaledFontPathFor scales the <size> segment by a multiplier
// and returns the resulting path. tryLoadScaledFont opens it.

func TestScaledFontPathFor_Doubles(t *testing.T) {
	got := scaledFontPathFor("/mnt/font/GoRegular/16a/font", 2.0)
	want := "/mnt/font/GoRegular/32a/font"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScaledFontPathFor_OneAndAHalf(t *testing.T) {
	got := scaledFontPathFor("/mnt/font/GoRegular/16a/font", 1.5)
	want := "/mnt/font/GoRegular/24a/font"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScaledFontPathFor_OneTwentyFive(t *testing.T) {
	got := scaledFontPathFor("/mnt/font/GoRegular/16a/font", 1.25)
	want := "/mnt/font/GoRegular/20a/font"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScaledFontPathFor_OneIsIdentity(t *testing.T) {
	// Scale 1.0 returns the base path unchanged so the map
	// builder doesn't double-store the base font.
	base := "/mnt/font/GoRegular/16a/font"
	got := scaledFontPathFor(base, 1.0)
	if got != base {
		t.Errorf("identity: got %q, want %q", got, base)
	}
}

func TestScaledFontPathFor_RoundsToInt(t *testing.T) {
	// 13 * 1.1 = 14.3 → 14.
	got := scaledFontPathFor("/mnt/font/GoRegular/13a/font", 1.1)
	want := "/mnt/font/GoRegular/14a/font"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScaledFontPathFor_NoSizeSegment(t *testing.T) {
	// A path with no <N>a/ segment is unparseable; return ""
	// so the caller falls back to the base font.
	got := scaledFontPathFor("/usr/local/share/fonts/regular.font", 1.5)
	if got != "" {
		t.Errorf("got %q, want \"\" for unparseable path", got)
	}
}

func TestScaledFontPathFor_EmptyBase(t *testing.T) {
	if got := scaledFontPathFor("", 1.5); got != "" {
		t.Errorf("got %q, want \"\" for empty base", got)
	}
}

func TestScaledFontPathFor_ZeroScale(t *testing.T) {
	if got := scaledFontPathFor("/mnt/font/GoRegular/16a/font", 0); got != "" {
		t.Errorf("got %q, want \"\" for zero scale", got)
	}
}
