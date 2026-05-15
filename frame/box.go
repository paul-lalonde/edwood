package frame

import (
	"flag"
	"fmt"
	"log"
	"unicode/utf8"
)

// boxWid returns the width a content box (Nrune > 0) should
// carry for its current (Style, Ptr). It is the single place
// that resolves Style.Kind to a font variant for width
// computation: every site that needs "the right width for this
// box" goes through here. validateboxmodel uses it as the
// invariant for b.Wid.
//
// Special boxes (Nrune < 0, i.e. tabs/newlines) are out of scope
// — their widths come from the tabstop / metric paths in
// newwid0 — and calling boxWid on one is a programmer error.
func (f *frameimpl) boxWid(b *frbox) int {
	if b.Nrune < 0 {
		panic(fmt.Sprintf("frame.boxWid: not valid for special boxes (Nrune=%d)", b.Nrune))
	}
	return f.fontFor(b.Style).BytesWidth(b.Ptr)
}

// isSpaceOnlyBox reports whether b is a content box whose Ptr
// contains only U+0020 spaces. Used by clean's merge predicate
// (Phase B5) to preserve the word/space boundary that bxscan
// introduced: merging a word box with an adjacent space box
// would defeat cklinewrap's word-boundary soft-wrap.
//
// Special boxes (Nrune < 0) and empty boxes are not
// space-only.
func isSpaceOnlyBox(b *frbox) bool {
	if b.Nrune <= 0 {
		return false
	}
	for _, by := range b.Ptr {
		if by != ' ' {
			return false
		}
	}
	return true
}

// addbox adds  n boxes after bn and shifts the rest up: * box[bn+n]==box[bn]
func (f *frameimpl) addbox(bn, n int) {
	if bn > len(f.box) {
		panic(fmt.Sprint("Frame.addbox", " bn=", bn, " len(f.box)", len(f.box)))
	}
	f.box = append(f.box, make([]*frbox, n)...)
	copy(f.box[bn+n:], f.box[bn:])
}

func (f *frameimpl) closebox(n0, n1 int) {
	if n0 >= len(f.box) || n1 >= len(f.box) || n1 < n0 {
		panic(fmt.Sprint("Frame.closebox bounds bad", " n0=", n0, " n1=", n1, " len(box)", len(f.box)))
	}

	n1++
	copy(f.box[n0:], f.box[n1:])
	f.box = f.box[0 : len(f.box)-(n1-n0)]
}

func (f *frameimpl) delbox(n0, n1 int) {
	// TODO(rjk): One of delbox and closebox don't belong.
	f.closebox(n0, n1)
}

func (b *frbox) clone() *frbox {
	// Shallow copy.
	cp := new(frbox)
	*cp = *b

	// Now deep copy the byte array
	// TODO(rjk): Adjust when we use strings.
	cp.Ptr = make([]byte, len(b.Ptr))
	copy(cp.Ptr, b.Ptr)
	return cp
}

// dupbox duplicates box i. box i must exist.
func (f *frameimpl) dupbox(i int) {
	if i >= len(f.box) {
		f.Logboxes("-- dupbox sadness -- ")
		panic(fmt.Sprint("dupbox i is out of bounds", " i=", i))
	}
	if f.box[i].Nrune < 0 {
		panic("dupbox invalid Nrune")
	}

	nb := f.box[i].clone()
	f.box = append(f.box, nil)
	copy(f.box[i+1:], f.box[i:])
	f.box[i] = nb

}

// TODO(rjk): Nicer way when we have a string for box contents.
func runeindex(p []byte, n int) int {
	offs := 0
	for i := 0; i < n; i++ {
		if p[offs] < 0x80 {
			offs++
		} else {
			_, size := utf8.DecodeRune(p[offs:])
			offs += size
		}
	}
	return offs
}

// truncatebox drops the  last n characters from box b without allocation.
// TODO(rjk): make a method on a frbox
// TODO(rjk): measure height.
func (f *frameimpl) truncatebox(b *frbox, n int) {
	if b.Nrune < 0 || b.Nrune < int(n) {
		f.Logboxes("-- truncatebox panic -- ")
		panic(fmt.Sprint("Frame.truncatebox", " Nrune=", b.Nrune, " n=", n))
	}
	b.Nrune -= n
	b.Ptr = b.Ptr[0:runeindex(b.Ptr, b.Nrune)]
	b.Wid = f.boxWid(b)
}

// chopbox removes the first n chars from box b without allocation.
// TODO(rjk): measure height
func (f *frameimpl) chopbox(b *frbox, n int) {
	if b.Nrune < 0 || b.Nrune < n {
		f.Logboxes("-- panic in chopbox --")
		panic(fmt.Sprint("chopbox", " b.Nrune=", b.Nrune, " n=", n))
	}
	i := runeindex(b.Ptr, n)
	b.Ptr = b.Ptr[i:]
	b.Nrune -= n
	b.Wid = f.boxWid(b)
}

// splitbox duplicates box [bn] and divides it at rune n into prefix and suffix boxes.
// It is an error to try to split a non-existent box?
// TODO(rjk): Figure out if you want this to be so.
func (f *frameimpl) splitbox(bn, n int) {
	if bn > len(f.box) {
		panic(fmt.Sprint("splitbox", "bn=", bn, "n=", n))
	}
	f.dupbox(bn)
	f.truncatebox(f.box[bn], f.box[bn].Nrune-n)
	f.chopbox(f.box[bn+1], n)
}

// mergebox combines boxes bn and bn+1
func (f *frameimpl) mergebox(bn int) {
	b1n := len(f.box[bn].Ptr)
	b2n := len(f.box[bn+1].Ptr)

	b := make([]byte, 0, b1n+b2n)
	b = append(b, f.box[bn].Ptr[0:b1n]...)
	b = append(b, f.box[bn+1].Ptr[0:b2n]...)
	f.box[bn].Ptr = b
	f.box[bn].Nrune += f.box[bn+1].Nrune
	f.box[bn].Wid += f.box[bn+1].Wid

	f.delbox(bn+1, bn+1)
}

// findbox finds the box containing q and puts q on a box boundary starting from
// rune p in box bn. NB: p must be the first rune in box[bn].
func (f *frameimpl) findbox(bn, p, q int) int {
	for _, b := range f.box[bn:] {
		if p+nrune(b) > q {
			break
		}
		p += nrune(b)
		bn++
	}
	if p != q {
		f.splitbox(bn, q-p)
		bn++
	}
	return bn
}

// TODO(rjk): Consider moving this code to a new file.
var validate = flag.Bool("validateboxes", false, "Check that box model is valid")

// validateboxmodel returns true if f's box model is valid.
func (f *frameimpl) validateboxmodel(format string, args ...interface{}) {
	if !*validate {
		return
	}

	// Test 0. No holes in the array of boxes.
	for _, b := range f.box {
		if b == nil {
			log.Printf(format, args...)
			f.Logboxes("-- holes in nbox portion of box array --")
			panic("-- holes in nbox portion of box array --")
		}
	}

	// Test 1. NChars is valid
	total := 0
	for _, b := range f.box {
		if b.Nrune < 0 {
			total++
		} else {
			total += b.Nrune
		}
	}
	if total != f.nchars {
		log.Printf(format, args...)
		f.Logboxes("-- runes in boxes != NChars --")
		panic("-- runes in boxes != NChars --")
	}

	// TODO(rjk): Every box is sane.
	for _, b := range f.box {
		// Nrune is right for this box.
		if b.Nrune >= 0 {
			s := string(b.Ptr)
			c := 0
			for range s {
				c++
			}
			if c != b.Nrune {
				log.Printf(format, args...)
				f.Logboxes("-- box with contents has invalid rune count --")
				panic("-- box with contents has invalid rune count --")
			}
		}

		// The width is right.
		if b.Nrune > 0 {
			if b.Wid != f.boxWid(b) {
				log.Printf(format, args...)
				f.Logboxes("-- box with contents has invalid width --")
				panic("-- box with contents has invalid width --")
			}
		}

		// TODO(rjk): newline and tab boxes are rational.
	}

	// TODO(rjk): Every box fits in Rect.

	// B2.3 R12: I-LAYOUT-* invariants from
	// frame-layout-design.md §7. I-LAYOUT-1 (sole-writer)
	// is structurally guaranteed by the code shape and
	// can't easily be checked at runtime; I-LAYOUT-5 (paint
	// matches layout) is paint-time and lives in paintBox
	// when -validatelayout is added. The static line-table
	// invariants below fire whenever -validateboxes is set.

	// I-LAYOUT-2: line-table consistency.
	for i, line := range f.lines {
		if line.FirstBox < 0 || line.FirstBox >= len(f.box) {
			log.Printf(format, args...)
			f.Logboxes("-- I-LAYOUT-2: lines[%d].FirstBox=%d out of range --", i, line.FirstBox)
			panic("-- I-LAYOUT-2 violated --")
		}
		fb := f.box[line.FirstBox]
		if fb.Y != line.TopY || fb.LineH != line.LineH || fb.LineA != line.LineA {
			log.Printf(format, args...)
			f.Logboxes("-- I-LAYOUT-2: lines[%d]={Y=%d LineH=%d LineA=%d}, box[FirstBox]={Y=%d LineH=%d LineA=%d} --",
				i, line.TopY, line.LineH, line.LineA, fb.Y, fb.LineH, fb.LineA)
			panic("-- I-LAYOUT-2 violated --")
		}
		end := len(f.box)
		if i+1 < len(f.lines) {
			end = f.lines[i+1].FirstBox
		}
		for j := line.FirstBox; j < end; j++ {
			b := f.box[j]
			if b.Y != line.TopY || b.LineH != line.LineH || b.LineA != line.LineA {
				log.Printf(format, args...)
				f.Logboxes("-- I-LAYOUT-2: box[%d] doesn't match its line[%d] metrics --", j, i)
				panic("-- I-LAYOUT-2 violated --")
			}
		}
	}

	// I-LAYOUT-3: monotone TopY across adjacent lines.
	for i := 1; i < len(f.lines); i++ {
		prev, cur := f.lines[i-1], f.lines[i]
		want := prev.TopY + prev.LineH
		if cur.TopY != want {
			log.Printf(format, args...)
			f.Logboxes("-- I-LAYOUT-3: lines[%d].TopY=%d, want %d (prev.TopY=%d + prev.LineH=%d) --",
				i, cur.TopY, want, prev.TopY, prev.LineH)
			panic("-- I-LAYOUT-3 violated --")
		}
	}

	// I-LAYOUT-4: lastlinefull is derived from the line table.
	wantLLF := false
	if n := len(f.lines); n > 0 {
		last := f.lines[n-1]
		wantLLF = last.TopY+last.LineH >= f.rect.Max.Y
	}
	if f.lastlinefull != wantLLF {
		log.Printf(format, args...)
		f.Logboxes("-- I-LAYOUT-4: lastlinefull=%v, want %v --", f.lastlinefull, wantLLF)
		panic("-- I-LAYOUT-4 violated --")
	}

	// I-LAYOUT-6: no layout-only fragmentation.
	for i := 0; i+1 < len(f.box); i++ {
		a, c := f.box[i], f.box[i+1]
		if a.Nrune <= 0 || c.Nrune <= 0 {
			continue
		}
		if a.Style != c.Style {
			continue
		}
		if isSpaceOnlyBox(a) != isSpaceOnlyBox(c) {
			continue
		}
		if a.Y != c.Y {
			continue
		}
		if a.Wid+c.Wid <= f.rect.Max.X-a.X {
			log.Printf(format, args...)
			f.Logboxes("-- I-LAYOUT-6: box[%d] (%q) and box[%d] (%q) could be coalesced at Y=%d --",
				i, string(a.Ptr), i+1, string(c.Ptr), a.Y)
			panic("-- I-LAYOUT-6 violated --")
		}
	}
}
