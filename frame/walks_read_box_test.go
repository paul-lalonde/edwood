package frame

import (
	"image"
	"testing"
)

// Phase B2.2 R3 — paint walks and position queries read
// box.X / box.Y / box.LineH from the box model rather than
// recomputing pt via cklinewrap+advance accumulators. These
// tests poke "wrong" values into box.X / box.Y and observe
// that the readers honor them — proving the seam was actually
// flipped.
//
// Pre-R3, Ptofchar et al. recompute pt from scratch and ignore
// stored box.X / box.Y; poked values would be invisible to
// callers. Post-R3 they're load-bearing.

// TestR3_Ptofchar_ReadsBoxY confirms Ptofchar returns the Y
// stored on the box containing rune p, not one derived from
// the cklinewrap walk.
func TestR3_Ptofchar_ReadsBoxY(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("hello"), 0)

	fimpl := fr.(*frameimpl)
	// Replace the Y of the first box with a sentinel value
	// that the legacy walk could never produce. If Ptofchar
	// still reports the sentinel, it reads from box.Y.
	const sentinelY = 4242
	for _, b := range fimpl.box {
		b.Y = sentinelY
	}

	got := fr.Ptofchar(0)
	if got.Y != sentinelY {
		t.Errorf("Ptofchar(0).Y = %d, want %d (sentinel); Ptofchar must read box.Y", got.Y, sentinelY)
	}
}

// TestR3_Ptofchar_ReadsBoxX confirms Ptofchar's X equals the
// box's X plus the per-rune width walk within the box's Ptr.
// We test by poking box.X for the single content box and
// asserting Ptofchar(0).X reports the poked value.
func TestR3_Ptofchar_ReadsBoxX(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("hello"), 0)

	fimpl := fr.(*frameimpl)
	const sentinelX = 9999
	fimpl.box[0].X = sentinelX

	got := fr.Ptofchar(0)
	if got.X != sentinelX {
		t.Errorf("Ptofchar(0).X = %d, want %d; Ptofchar must read box.X", got.X, sentinelX)
	}
}

// TestR3_PaintBox_PaintsAtBoxXY confirms that drawtext /
// paintBox writes the glyph Bytes op at the box's stored X/Y,
// not at the pt the walk accumulator would compute. We poke
// box.X to a sentinel and verify the recorded paint op's
// atpoint reflects it.
func TestR3_PaintBox_PaintsAtBoxXY(t *testing.T) {
	iv := &invariants{
		topcorner: image.Pt(20, 10),
		textarea:  image.Rect(20, 10, 400, 100),
	}
	fr := setupFrame(t, iv)
	fr.Insert([]rune("X"), 0)

	fimpl := fr.(*frameimpl)
	// Poke box[0] to a sentinel position pre-repaint.
	const sentinelX, sentinelY = 88, 55
	fimpl.box[0].X = sentinelX
	fimpl.box[0].Y = sentinelY

	g := gdo(t, fr)
	g.Clear()

	// Drive a repaint via SetStyleRange (uses repaintBoxRange).
	// We can't repaint without changing styles, so set the
	// same plain style — repaintBoxRange still fires.
	fr.SetStyleRange(0, 1, []StyleRun{{Len: 1, Style: Style{}}})

	// Look for a screen-string op at the sentinel point.
	want := "atpoint: (88,55)"
	found := false
	for _, op := range g.DrawOps() {
		if containsSubstr(op, "string \"X\"") && containsSubstr(op, want) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected glyph paint op at sentinel position %s; ops:\n%s",
			want, joinOps(g.DrawOps()))
	}
}

func containsSubstr(s, sub string) bool {
	return len(sub) <= len(s) && (s == sub || indexSubstr(s, sub) >= 0)
}

func indexSubstr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func joinOps(ops []string) string {
	out := ""
	for _, op := range ops {
		out += op + "\n"
	}
	return out
}
