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

// charOfPtReader is the B2.2 R3 pure-reader Charofpt. It finds
// the box whose stored rect (X, Y, X+Wid, Y+LineH) contains pt
// and walks runes within that box to identify the exact rune.
// Same rationale as ptOfCharReader.
func (f *frameimpl) charOfPtReader(pt image.Point) int {
	p := 0
	for _, b := range f.box {
		// Skip boxes whose line is fully above pt.Y.
		if b.Y+b.LineH <= pt.Y {
			p += nrune(b)
			continue
		}
		// Stop once we've passed pt vertically.
		if b.Y > pt.Y {
			break
		}
		// b is on the line containing pt.Y. Skip boxes left
		// of pt.X.
		if b.X+b.Wid <= pt.X {
			p += nrune(b)
			continue
		}
		// b overlaps pt.X. If b is special (tab/newline),
		// the whole box maps to "before pt" or "after pt"
		// based on X.
		if b.Nrune < 0 {
			if b.X > pt.X {
				break
			}
			p += nrune(b)
			continue
		}
		// Content box: walk runes.
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
