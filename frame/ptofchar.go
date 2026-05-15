package frame

import (
	"image"
	"log"
	"unicode/utf8"
)

// ptofcharptb returns the point of run p based on the current box state.
// NB: it is possible that a different rune at p would give a different
// result. Consequently, the result of this function will not say where a
// new rune at position p should be positioned, only where the current
// rune at p is positioned.
func (f *frameimpl) ptofcharptb(p int, pt image.Point, bn int) image.Point {
	var w int
	var r rune

	for _, b := range f.box[bn:] {
		pt = f.cklinewrap(pt, b)
		l := nrune(b)
		if p < l {
			if b.Nrune > 0 {
				font := f.fontFor(b.Style)
				for s := 0; s < len(b.Ptr) && p > 0; s += w {
					p--
					r, w = utf8.DecodeRune(b.Ptr[s:])
					pt.X += font.BytesWidth(b.Ptr[s : s+w])
					if r == 0 || pt.X > f.rect.Max.X {
						log.Panicf("frptofchar: r=%v pt.X=%v f.rect.Max.X=%v\n", r, pt.X, f.rect.Max.X)
					}
				}
			}
			break
		}

		p -= l
		pt = f.advance(pt, b)
	}

	return pt
}

func (f *frameimpl) Ptofchar(p int) image.Point {
	f.lk.Lock()
	defer f.lk.Unlock()
	return f.ptOfCharReader(p)
}

// ptOfCharReader is the B2.2 R3 pure-reader implementation of
// Ptofchar. It reads each box's stored X / Y (populated by
// relayoutFrom) rather than re-deriving pt via cklinewrap and
// advance. The result must equal ptofcharptb for the same p
// under constant line height (pre-R4); under variable height
// (R4+) only the reader produces correct results — the
// accumulator walk's assumption that each line is
// defaultfontheight breaks once Style.Scale paints a tall line.
//
// Internal mutation paths (deleteimpl, insertbyteimpl
// intermediate computations) still call the legacy
// ptofcharptb because they run BEFORE relayoutFrom has
// produced consistent box.X / box.Y.
func (f *frameimpl) ptOfCharReader(p int) image.Point {
	if len(f.box) == 0 {
		return f.rect.Min
	}
	if p < 0 {
		p = 0
	}
	for _, b := range f.box {
		l := nrune(b)
		if p < l {
			pt := image.Pt(b.X, b.Y)
			if b.Nrune > 0 {
				font := f.fontFor(b.Style)
				s := 0
				for ; s < len(b.Ptr) && p > 0; p-- {
					_, w := utf8.DecodeRune(b.Ptr[s:])
					pt.X += font.BytesWidth(b.Ptr[s : s+w])
					s += w
				}
			}
			return pt
		}
		p -= l
	}
	// p past end: return position one past the last box. If
	// the last box is a hard newline, "one past" lands at the
	// start of the next line (rect.Min.X, last.Y + last.LineH)
	// — matching the legacy walk's advance() behavior.
	last := f.box[len(f.box)-1]
	if last.Nrune < 0 && last.Bc == '\n' {
		lineH := last.LineH
		if lineH == 0 {
			lineH = f.defaultfontheight
		}
		return image.Pt(f.rect.Min.X, last.Y+lineH)
	}
	return image.Pt(last.X+last.Wid, last.Y)
}

func (f *frameimpl) ptofcharnb(p int, _ int) image.Point {
	pt := f.ptofcharptb(p, f.rect.Min, 0)
	return pt
}

func (f *frameimpl) grid(p image.Point) image.Point {
	p.Y -= f.rect.Min.Y
	p.Y -= p.Y % f.defaultfontheight
	p.Y += f.rect.Min.Y
	if p.X > f.rect.Max.X {
		p.X = f.rect.Max.X
	}
	return p
}

func (f *frameimpl) Charofpt(pt image.Point) int {
	f.lk.Lock()
	defer f.lk.Unlock()
	return f.charOfPtReader(pt)
}

// charOfPtReader is the B2.3 R3 pure-reader Charofpt. It
// binary-searches f.lines by TopY to find the line containing
// pt.Y, then walks only that line's boxes — O(log lines + per-
// line boxes) rather than O(total boxes). Per frame-layout-
// design §4.2.
func (f *frameimpl) charOfPtReader(pt image.Point) int {
	if len(f.lines) == 0 {
		return 0
	}
	if pt.Y < f.lines[0].TopY {
		return 0
	}

	// Largest i such that lines[i].TopY <= pt.Y. Lines are
	// Y-sorted by I-LAYOUT-3 so binary search is valid.
	lo, hi := 0, len(f.lines)
	for lo < hi {
		mid := (lo + hi) / 2
		if f.lines[mid].TopY <= pt.Y {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	lineIdx := lo - 1
	line := f.lines[lineIdx]

	// Click below the last line's bottom → end of content.
	// Derive the end-of-content rune offset from the box list
	// directly (not f.nchars) so this works even when callers
	// construct a frame inline without maintaining nchars.
	if lineIdx == len(f.lines)-1 && pt.Y >= line.TopY+line.LineH {
		p := line.FirstRune
		for i := line.FirstBox; i < len(f.box); i++ {
			p += nrune(f.box[i])
		}
		return p
	}

	// Resolve this line's box range, then walk only those boxes.
	boxStart := line.FirstBox
	boxEnd := len(f.box)
	if lineIdx+1 < len(f.lines) {
		boxEnd = f.lines[lineIdx+1].FirstBox
	}

	p := line.FirstRune
	for i := boxStart; i < boxEnd; i++ {
		b := f.box[i]
		if b.X+b.Wid <= pt.X {
			p += nrune(b)
			continue
		}
		// b overlaps pt.X.
		if b.Nrune < 0 {
			if b.X > pt.X {
				return p
			}
			p += nrune(b)
			continue
		}
		// Content box: walk runes by X.
		font := f.fontFor(b.Style)
		x := b.X
		s := 0
		for s < len(b.Ptr) {
			_, w := utf8.DecodeRune(b.Ptr[s:])
			x += font.BytesWidth(b.Ptr[s : s+w])
			if x > pt.X {
				break
			}
			p++
			s += w
		}
		return p
	}
	return p
}

func (f *frameimpl) charofptimpl(pt image.Point) int {
	var w, bn int
	var p int

	pt = f.grid(pt)
	qt := f.rect.Min

	for bn = 0; bn < len(f.box) && qt.Y < pt.Y; bn++ {
		b := f.box[bn]
		qt = f.cklinewrap(qt, b)
		if qt.Y >= pt.Y {
			break
		}
		qt = f.advance(qt, b)
		p += nrune(b)
	}

	var r rune
	for _, b := range f.box[bn:] {
		if qt.X > pt.X {
			break
		}
		qt = f.cklinewrap(qt, b)
		if qt.Y > pt.Y {
			break
		}
		if qt.X+b.Wid > pt.X {
			if b.Nrune < 0 {
				qt = f.advance(qt, b)
			} else {
				font := f.fontFor(b.Style)
				s := 0
				for ; s < len(b.Ptr); s += w {
					r, w = utf8.DecodeRune(b.Ptr[s:])
					if r == 0 {
						panic("end of string in frcharofpt")
					}
					qt.X += font.BytesWidth(b.Ptr[s : s+w])
					if qt.X > pt.X {
						break
					}
					p++
				}
			}
		} else {
			p += nrune(b)
			qt = f.advance(qt, b)
		}
	}
	return p
}
