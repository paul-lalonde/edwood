package frame

import (
	"image"
)

func (f *frameimpl) Delete(p0, p1 int) int {
	f.lk.Lock()
	defer f.lk.Unlock()
	return f.deleteimpl(p0, p1)
}

// deleteimpl removes runes [p0, p1) per frame-layout-design
// §6.2: snapshot pre-mutation line table, splice out boxes,
// relayoutFrom, diffLines, issue paint ops. Replaces the
// legacy per-box blit walk (B2.3 R6).
func (f *frameimpl) deleteimpl(p0, p1 int) int {
	f.validateboxmodel("Frame.Delete Start p0=%d p1=%d", p0, p1)
	defer f.validateboxmodel("Frame.Delete End p0=%d p1=%d", p0, p1)

	if p1 > f.nchars {
		p1 = f.nchars - 1
	}
	if p0 >= f.nchars || p0 == p1 || f.background == nil {
		return 0
	}

	f.modified = true

	// Pre-mutation: capture the line table and the content
	// bottom Y. The bottom is used after relayoutFrom to
	// clear the vacated region at the bottom of the frame.
	snap := f.snapshotLines()
	oldBottom := 0
	if n := len(f.lines); n > 0 {
		last := f.lines[n-1]
		oldBottom = last.TopY + last.LineH
	}

	// Erase the selection or tick using the pre-mutation
	// reader (box.X/Y still reflect the prior relayout).
	f.drawselimpl(f.ptOfCharReader(f.sp0), f.sp0, f.sp1, false)

	// Splice out boxes [n0, n1). findbox calls splitbox at the
	// p0/p1 boundaries so the range is exactly the deleted
	// rune range; the boundary split is the "delete boundary"
	// case distinct from §3.3's long-word split inside
	// relayoutFrom.
	n0 := f.findbox(0, 0, p0)
	if n0 == len(f.box) {
		panic("off end in Frame.Delete")
	}
	n1 := f.findbox(n0, p0, p1)
	if n1 > n0 {
		f.closebox(n0, n1-1)
	}

	// Adjust nchars and selection bounds.
	f.nchars -= p1 - p0
	if f.sp1 > p1 {
		f.sp1 -= p1 - p0
	} else if f.sp1 > p0 {
		f.sp1 = p0
	}
	if f.sp0 > p1 {
		f.sp0 -= p1 - p0
	} else if f.sp0 > p0 {
		f.sp0 = p0
	}

	// Relayout: eager-coalesce re-merges any boundary-split
	// fragments left by findbox; lastlinefull is re-derived
	// from the new line table per R2.
	f.relayoutFrom(0)

	// Compute paint ops and issue them. Delete's shifts are
	// all upward (ΔY < 0), so input order (top-to-bottom) is
	// safe for blits — each Src is below its Dst, and earlier
	// blits don't disturb later blits' Src regions.
	ops := f.diffLines(snap)
	f.issuePaintOps(ops)

	// Clear the region at the bottom that content vacated.
	// The diff handles shifts within the visible range but
	// doesn't emit a "clear" op for the trailing void.
	newBottom := 0
	if n := len(f.lines); n > 0 {
		last := f.lines[n-1]
		newBottom = last.TopY + last.LineH
	}
	if oldBottom > newBottom {
		clearRect := image.Rect(f.rect.Min.X, newBottom, f.rect.Max.X, oldBottom)
		if clearRect.Max.Y > f.rect.Max.Y {
			clearRect.Max.Y = f.rect.Max.Y
		}
		if clearRect.Min.Y < f.rect.Min.Y {
			clearRect.Min.Y = f.rect.Min.Y
		}
		if clearRect.Min.Y < clearRect.Max.Y {
			f.background.Draw(clearRect, f.cols[ColBack], nil, image.Point{})
		}
	}

	// Tick and nlines update.
	if f.sp0 == f.sp1 {
		f.Tick(f.ptOfCharReader(f.sp0), true)
	}
	pt0 := f.ptOfCharReader(f.nchars)
	n := f.nlines
	f.nlines = (pt0.Y - f.rect.Min.Y) / f.defaultfontheight
	if pt0.X > f.rect.Min.X {
		f.nlines++
	}
	return n - f.nlines
}
