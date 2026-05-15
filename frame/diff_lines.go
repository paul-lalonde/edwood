package frame

import "image"

// B2.3 R5 — paint deltas via line-table diff per
// frame-layout-design.md §3.5. The mutator path captures a
// pre-mutation snapshot of f.lines, runs the mutation +
// relayoutFrom, then asks diffLines for the minimum-cost
// sequence of paint ops to update the screen. Today's
// single-shift bulk-blit in deleteimpl / insertbyteimpl is
// the degenerate single-run case of this; R6/R7 wire the
// helpers into those mutators.

// lineSnap is one entry in the pre-mutation snapshot. The
// rune-coordinate FirstRune is the canonical identity of a
// line, stable across box-list shifts caused by inserts/
// deletes in earlier lines. Digest is a content hash (Ptr +
// Style.Kind across the line's boxes) that catches changes
// invisible to TopY/LineH (e.g., a color swap on the same
// runes).
type lineSnap struct {
	FirstRune int
	TopY      int
	LineH     int
	Digest    uint64
}

// paintOpKind tags a paintOp as a blit (move pixels) or a
// paint (clear + redraw).
type paintOpKind int

const (
	OpPaint paintOpKind = iota
	OpBlit
)

// paintOp describes one screen update. Blit ops carry both
// Src (source rect on the current display) and Dst (target
// rect, Y-shifted by the ΔY of the run). Paint ops use only
// Dst.
type paintOp struct {
	Kind paintOpKind
	Src  image.Rectangle
	Dst  image.Rectangle
}

// snapshotLines captures the current line-table state plus
// a per-line content digest. Callers run this BEFORE the
// box-list mutation; diffLines compares it against the post-
// mutation f.lines.
func (f *frameimpl) snapshotLines() []lineSnap {
	out := make([]lineSnap, len(f.lines))
	for i, line := range f.lines {
		out[i] = lineSnap{
			FirstRune: line.FirstRune,
			TopY:      line.TopY,
			LineH:     line.LineH,
			Digest:    f.lineDigest(i),
		}
	}
	return out
}

// lineDigest returns an FNV-1a hash over the box content
// (Ptr bytes + Style.Kind) of the line at index i. Stable
// across box-list shifts (because it walks by index, not by
// rune offset) as long as the line's box range is the same.
func (f *frameimpl) lineDigest(lineIdx int) uint64 {
	const (
		fnvOffsetBasis uint64 = 14695981039346656037
		fnvPrime       uint64 = 1099511628211
	)
	h := fnvOffsetBasis
	if lineIdx < 0 || lineIdx >= len(f.lines) {
		return h
	}
	start := f.lines[lineIdx].FirstBox
	end := len(f.box)
	if lineIdx+1 < len(f.lines) {
		end = f.lines[lineIdx+1].FirstBox
	}
	for i := start; i < end; i++ {
		b := f.box[i]
		for _, by := range b.Ptr {
			h ^= uint64(by)
			h *= fnvPrime
		}
		h ^= uint64(b.Style.Kind)
		h *= fnvPrime
		// Mix in Bc and Nrune so special boxes (tab/newline)
		// digest distinctly from empty content boxes.
		h ^= uint64(b.Bc)
		h *= fnvPrime
		h ^= uint64(uint32(b.Nrune))
		h *= fnvPrime
	}
	return h
}

// diffLines classifies each post-mutation line in f.lines
// against the pre-mutation snapshot and returns a sequence
// of paint ops in screen order (top-to-bottom).
//
// Matching is by content digest (FNV-1a over the line's box
// Ptr + Style.Kind + Bc + Nrune), scanned forward greedily:
// for each new line, find the earliest unused old line whose
// digest matches. This handles insert/delete that shifts rune
// offsets — the moved line's content is still present in the
// snapshot at a different index, and the digest match finds
// it. (The FirstRune field is captured but not used for
// matching today; the design's earlier "FirstRune as identity"
// language assumed the rune coordinate was stable, which it
// isn't.)
//
// Classification per matched pair:
//   - identical: same TopY, same LineH → no op.
//   - shifted:   different TopY, same LineH → blit.
//   - dirty:     no match found (digest unique in old or no
//     forward match remaining) → paint.
//
// Run compression: adjacent shifted lines with the same ΔY
// compose into one blit op (the wire-cheap path for the
// common single-shift case of Insert / Delete). Adjacent
// dirty lines compose into one paint op.
func (f *frameimpl) diffLines(snap []lineSnap) []paintOp {
	if len(snap) == 0 && len(f.lines) == 0 {
		return nil
	}

	type entryKind int
	const (
		clsIdentical entryKind = iota
		clsShifted
		clsDirty
	)
	type entry struct {
		kind entryKind
		dy   int
	}
	cls := make([]entry, len(f.lines))

	oi := 0 // forward cursor into snap
	for i, newLine := range f.lines {
		newDigest := f.lineDigest(i)
		match := -1
		for k := oi; k < len(snap); k++ {
			if snap[k].Digest == newDigest && snap[k].LineH == newLine.LineH {
				match = k
				break
			}
		}
		if match < 0 {
			cls[i] = entry{clsDirty, 0}
			continue
		}
		old := snap[match]
		oi = match + 1
		if old.TopY == newLine.TopY {
			cls[i] = entry{clsIdentical, 0}
		} else {
			cls[i] = entry{clsShifted, newLine.TopY - old.TopY}
		}
	}

	var ops []paintOp
	i := 0
	for i < len(cls) {
		switch cls[i].kind {
		case clsIdentical:
			i++
		case clsShifted:
			dy := cls[i].dy
			j := i
			for j+1 < len(cls) && cls[j+1].kind == clsShifted && cls[j+1].dy == dy {
				j++
			}
			ops = append(ops, paintOp{
				Kind: OpBlit,
				Src: image.Rect(
					f.rect.Min.X,
					f.lines[i].TopY-dy,
					f.rect.Max.X,
					f.lines[j].TopY-dy+f.lines[j].LineH,
				),
				Dst: image.Rect(
					f.rect.Min.X,
					f.lines[i].TopY,
					f.rect.Max.X,
					f.lines[j].TopY+f.lines[j].LineH,
				),
			})
			i = j + 1
		case clsDirty:
			j := i
			for j+1 < len(cls) && cls[j+1].kind == clsDirty {
				j++
			}
			ops = append(ops, paintOp{
				Kind: OpPaint,
				Dst: image.Rect(
					f.rect.Min.X,
					f.lines[i].TopY,
					f.rect.Max.X,
					f.lines[j].TopY+f.lines[j].LineH,
				),
			})
			i = j + 1
		}
	}
	return ops
}
