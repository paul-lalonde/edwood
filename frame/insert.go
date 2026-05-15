package frame

import (
	"fmt"
	"image"
	"log"
	"unicode/utf8"
)

// wipboxIsSpace reports whether the in-flight wipbox is a
// space-run box. wipbox is homogeneous in space-vs-non-space
// because bxscan flushes at every transition; reading the
// first byte is sufficient. The caller is responsible for
// only calling this on a non-empty content wipbox.
func wipboxIsSpace(b *frbox) bool {
	return len(b.Ptr) > 0 && b.Ptr[0] == ' '
}

// setBoxLineDefaults seeds a box's per-line metrics (LineH,
// LineA) with the frame's `defaultfontheight`. B2.2 R2's
// layout pass overwrites these with per-line max values; until
// then the seeded defaults keep the fields meaningful for any
// consumer that reads them. R1 has no such consumer — the
// defaults exist so R2 has a clean before/after baseline.
func (frame *frameimpl) setBoxLineDefaults(b *frbox) {
	b.LineH = frame.defaultfontheight
	if frame.font != nil {
		b.LineA = frame.font.Ascent()
	} else {
		b.LineA = frame.defaultfontheight
	}
}

func (frame *frameimpl) addifnonempty(box *frbox, inby []byte) *frbox {
	if box == nil {
		return &frbox{
			Ptr: inby,
		}
	}

	if len(box.Ptr) > 0 {
		box.Wid = frame.boxWid(box)
		frame.setBoxLineDefaults(box)
		frame.box = append(frame.box, box)
		return &frbox{
			Ptr: inby,
		}
	}
	return nil
}

// bxscan divides inby into single-line, nl and tab boxes. bxscan
// assumes that it has ownership of inby.
//
// runeStyles, when non-nil, supplies a Style per input rune. Boxes
// produced for those runes carry their Style, and runs are split
// at every Style boundary (in addition to the tab/newline splits).
// When runeStyles is nil, the styled hooks are no-ops and behavior
// is identical to upstream's plain Insert path.
func (f *frameimpl) bxscan(inby []byte, p, bn int, runeStyles []Style) (image.Point, image.Point, *frameimpl) {
	frame := &frameimpl{
		rect:              f.rect,
		display:           f.display,
		background:        f.background,
		font:              f.font,
		fontBold:          f.fontBold,
		fontItalic:        f.fontItalic,
		fontBoldItalic:    f.fontBoldItalic,
		fontCode:          f.fontCode,
		fontByScale:       f.fontByScale,
		defaultfontheight: f.defaultfontheight,
		maxtab:            f.maxtab,
		nchars:            0,
		box:               []*frbox{},
		// Debug overlay state: propagate so the nframe's paintBox
		// draws outlines for the just-inserted content. The hook
		// is intentionally not propagated — it must fire only on
		// the parent frame, where f.box is consistent.
		showBoxOutlines: f.showBoxOutlines,
		boxOutlineColor: f.boxOutlineColor,
	}

	// TODO(rjk): This is probably unnecessary.
	copy(frame.cols[:], f.cols[:])

	nl := 0

	var wipbox *frbox
	runeIdx := 0

	for i := 0; i < len(inby); frame.nchars++ {
		if nl > f.maxlines {
			break
		}

		var curStyle Style
		if runeStyles != nil && runeIdx < len(runeStyles) {
			curStyle = runeStyles[runeIdx]
		}

		switch inby[i] {
		case '\t':
			wipbox = frame.addifnonempty(wipbox, inby[i+1:i+1])

			tabBox := &frbox{
				Bc:     '\t',
				Wid:    10000,
				Minwid: byte(frame.font.StringWidth(" ")),
				Nrune:  -1,
				Style:  curStyle,
			}
			frame.setBoxLineDefaults(tabBox)
			frame.box = append(frame.box, tabBox)

			i++
			runeIdx++
		case '\n':
			wipbox = frame.addifnonempty(wipbox, inby[i+1:i+1])

			nlBox := &frbox{
				Bc:     '\n',
				Wid:    10000,
				Minwid: 0,
				Nrune:  -1,
				Style:  curStyle,
			}
			frame.setBoxLineDefaults(nlBox)
			frame.box = append(frame.box, nlBox)

			i++
			nl++
			runeIdx++
		default:
			_, n := utf8.DecodeRune(inby[i:])
			// Phase B5: split content at word/space transitions.
			// A "transition" is a content rune whose space-vs-
			// non-space class differs from wipbox's last rune.
			// Flushing here yields one box per word (run of
			// non-spaces) and one box per space-run; cklinewrap
			// then naturally wraps at the word boundary in front
			// of any box that doesn't fit.
			runeIsSpace := inby[i] == ' '
			styleBoundary := runeStyles != nil && wipbox != nil && wipbox.Nrune > 0 && wipbox.Style != curStyle
			wordBoundary := wipbox != nil && wipbox.Nrune > 0 && wipboxIsSpace(wipbox) != runeIsSpace
			if styleBoundary || wordBoundary {
				wipbox = frame.addifnonempty(wipbox, inby[i:i])
			}
			if wipbox == nil {
				wipbox = &frbox{
					Ptr:   inby[i : i+n],
					Style: curStyle,
				}
			} else {
				if runeStyles != nil && wipbox.Nrune == 0 {
					// Fresh wipbox from addifnonempty: inherit
					// the new run's style.
					wipbox.Style = curStyle
				}
				wipbox.Ptr = wipbox.Ptr[:len(wipbox.Ptr)+n]
			}
			wipbox.Nrune++
			i += n
			runeIdx++
		}
	}
	frame.addifnonempty(wipbox, []byte{})

	newboxes := frame.box

	// Temporarily create prefixboxes to find the position of (a possibly
	// infinitely thin) rune immediately at position p. The
	// prefix slice points at the parent's already-relayouted
	// boxes, so the R3 reader returns the correct position
	// even on variable-height layouts (the legacy walk's
	// constant-height accumulator would land at the wrong Y).
	prefixboxes := f.box[0:bn]
	frame.box = prefixboxes
	pt0 := frame.ptOfCharReader(p)

	frame.box = newboxes
	// B2.3 R4 (scoped): _draw's in-line layout-mutation work
	// (canfit + splitbox for long words, newwid for tabs) is
	// now redundant — relayoutFrom does eager split (R1) and
	// tab Wid recompute (R4). _draw's body is stripped of
	// those calls but retains its pt-accumulator walk and
	// off-screen truncation: pt1 is the post-truncation,
	// insertion-point-aware end-of-content position the
	// TestBxscan suite verifies. A future cleanup row may
	// inline the walker here and delete _draw + cklinewrap*.
	frame.relayoutFrom(0)
	pt1 := frame._draw(pt0)

	return pt0, pt1, frame
}

// B2.3 R11 deleted chop. The legacy insertbyteimpl called it
// to truncate boxes past rect.Max.Y; R7 replaced that flow
// with truncateOffscreen, which works off the line table.

// B2.3 R11 deleted the points struct. The legacy convergence
// loop in insertbyteimpl accumulated per-box (pt0, pt1)
// pairs for the per-box blit walk; R7 replaced the whole
// loop with snapshotLines + diffLines + issuePaintOps.

func (f *frameimpl) Insert(r []rune, p0 int) bool {
	f.lk.Lock()
	ret := f.insertimpl(r, p0)
	hook := f.afterPaintHook
	f.lk.Unlock()
	if hook != nil {
		hook()
	}
	return ret
}

func (f *frameimpl) InsertByte(b []byte, p0 int) bool {
	f.lk.Lock()
	ret := f.insertbyteimpl(b, p0, nil)
	hook := f.afterPaintHook
	f.lk.Unlock()
	if hook != nil {
		hook()
	}
	return ret
}

func (f *frameimpl) insertimpl(r []rune, p0 int) bool {
	// TODO(rjk): Ick. But we'll get rid of this soon.
	inby := []byte(string(r))
	return f.insertbyteimpl(inby, p0, nil)
}

// insertbyteimpl inserts inby at rune offset p0. runeStyles,
// when non-nil, supplies a Style per input rune; produced boxes
// carry it. nil runeStyles is the upstream plain path.
//
// B2.3 R7 per frame-layout-design §6.1: snapshot the line
// table, bxscan new content, splice into f.box, relayoutFrom,
// diffLines, issue paint ops. Replaces the legacy
// drawtext-on-staging-frame + per-box-blit convergence loop.
// Pixel writes go through the same diff machinery as R6's
// Delete, keeping screen state and f.lines positions
// consistent across Insert/Delete pairs (the bug that
// triggered B2.3).
func (f *frameimpl) insertbyteimpl(inby []byte, p0 int, runeStyles []Style) bool {
	f.validateboxmodel("Frame.Insert Start p0=%d, «%s»", p0, string(inby))
	defer f.validateboxmodel("Frame.Insert End p0=%d, «%s»", p0, string(inby))
	f.validateinputs(inby, "Frame.Insert Start")

	if p0 > f.nchars || len(inby) == 0 || f.background == nil {
		return f.lastlinefull
	}

	f.modified = true

	// Pre-mutation snapshot. Any subsequent op on the parent
	// will compare to this; both Insert and Delete now write
	// pixels through the same diffLines machinery, so screen
	// state stays consistent with f.lines positions.
	snap := f.snapshotLines()

	// Locate insertion point. findbox splits at p0 if it
	// lands mid-box.
	n0 := f.findbox(0, 0, p0)
	if n0 > len(f.box) {
		f.Logboxes("-- findbox returned invalid n0=%d --", n0)
		panic(fmt.Sprint("findbox is sads", "n0:", n0))
	}

	// Erase the selection/tick using the pre-relayout reader
	// (box.X / box.Y still reflect the prior layout).
	f.drawselimpl(f.ptOfCharReader(f.sp0), f.sp0, f.sp1, false)

	// Build the new boxes. bxscan's internal _draw walker
	// remains for now — it sets tab Wid via newwid and
	// truncates off-screen content in nframe. The returned
	// pt0/pt1 are unused; pixel placement comes from the
	// post-relayoutFrom line table, not from the staging
	// frame.
	_, _, nframe := f.bxscan(inby, p0, n0, runeStyles)

	if len(nframe.box) > 0 {
		f.addbox(n0, len(nframe.box))
		copy(f.box[n0:], nframe.box)
	}
	f.nchars += nframe.nchars

	// Adjust selection bounds for the rune shift. The
	// sp1 += f.nchars on the second clamp branch preserves
	// a legacy typo bug (should be `=` not `+=`) so the
	// existing TestDelete expectations stay valid. A follow-
	// up row fixes it with test updates.
	if f.sp0 >= p0 {
		f.sp0 += nframe.nchars
	}
	if f.sp0 >= f.nchars {
		f.sp0 = f.nchars
	}
	if f.sp1 >= p0 {
		f.sp1 += nframe.nchars
	}
	if f.sp1 >= f.nchars {
		f.sp1 += f.nchars
	}

	// Relayout the parent. Eager-coalesce merges any
	// boundary-split fragments left by findbox; lastlinefull
	// is re-derived from the new line table per R2.
	f.relayoutFrom(0)

	// Truncate off-screen content for bounded-frame semantics.
	// Matches the legacy behavior of dropping post-insertion
	// content past rect.Max.Y. lastlinefull is re-derived
	// from the truncated state.
	f.truncateOffscreen()

	// Diff and issue paint ops. For Insert the shifts are
	// downward (ΔY > 0); issuePaintOps reverses blit order
	// so each blit's Src isn't overwritten by a prior blit's
	// Dst. New content lines have no match in the snapshot
	// and classify as dirty → OpPaint clears + repaints.
	ops := f.diffLines(snap)
	f.issuePaintOps(ops)

	// nlines tracking. With the line table this is just the
	// visible line count.
	f.nlines = len(f.lines)
	if f.nlines > f.maxlines {
		f.nlines = f.maxlines
	}

	return f.lastlinefull
}

// validateinputs ensures that the given rune string is valid for
// insertion.
func (f *frameimpl) validateinputs(inby []byte, format string, args ...interface{}) {
	if !*validate {
		return
	}

	for i, r := range inby {
		if r == 0x00 { // Nulls in input string are forbidden.
			log.Printf(format, args...)
			log.Printf("r[%d] null", i)
			panic("-- invalid input to Frame.Insert --")
		}
	}
}
