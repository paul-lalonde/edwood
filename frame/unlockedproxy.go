package frame

import (
	"image"
)

type selectscrollupdaterimpl frameimpl

func (up *selectscrollupdaterimpl) GetFrameFillStatus() FrameFillStatus {
	// log.Println("selectscrollupdaterimpl.GetFrameFillStatus")
	f := (*frameimpl)(up)
	return FrameFillStatus{
		Nchars:   f.nchars,
		Nlines:   f.nlines,
		Maxlines: f.maxlines,
	}
}

func (up *selectscrollupdaterimpl) Charofpt(pt image.Point) int {
	// log.Println("selectscrollupdaterimpl.Charofpt")
	f := (*frameimpl)(up)
	return f.charOfPtReader(pt)
}

func (up *selectscrollupdaterimpl) DefaultFontHeight() int {
	// log.Println("selectscrollupdaterimpl.DefaultFontHeight")
	f := (*frameimpl)(up)
	return f.defaultfontheight
}

func (up *selectscrollupdaterimpl) LineYOffset(n int) int {
	f := (*frameimpl)(up)
	return f.lineYOffsetLocked(n)
}

func (up *selectscrollupdaterimpl) Delete(p0, p1 int) int {
	// log.Println("selectscrollupdaterimpl.Delete")
	f := (*frameimpl)(up)
	return f.deleteimpl(p0, p1)
}

func (up *selectscrollupdaterimpl) Insert(r []rune, p0 int) bool {
	// log.Println("selectscrollupdaterimpl.Insert")
	f := (*frameimpl)(up)
	return f.insertimpl(r, p0)
}

func (up *selectscrollupdaterimpl) InsertByte(b []byte, p0 int) bool {
	// log.Println("selectscrollupdaterimpl.InsertByte")
	f := (*frameimpl)(up)
	return f.insertbyteimpl(b, p0, nil)
}

func (up *selectscrollupdaterimpl) InsertWithStyle(r []rune, p0 int, styles []StyleRun) bool {
	f := (*frameimpl)(up)
	if styles != nil {
		validateStyleRunsLen(styles, len(r))
	}
	if allPlain(styles) {
		return f.insertimpl(r, p0)
	}
	return f.insertbyteimpl([]byte(string(r)), p0, expandStyles(styles, len(r)))
}

func (up *selectscrollupdaterimpl) IsLastLineFull() bool {
	// log.Println("selectscrollupdaterimpl.IsLastLineFull")
	f := (*frameimpl)(up)
	return f.lastlinefull
}

func (up *selectscrollupdaterimpl) Rect() image.Rectangle {
	// log.Println("selectscrollupdaterimpl.Rect")
	f := (*frameimpl)(up)
	return f.rect
}

func (up *selectscrollupdaterimpl) TextOccupiedHeight(r image.Rectangle) int {
	// log.Println("selectscrollupdaterimpl.TextOccupiedHeight")
	f := (*frameimpl)(up)
	return f.textoccupiedheightimpl(r)
}
