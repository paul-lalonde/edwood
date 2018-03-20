package main

import (
	"crypto/sha1"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"9fans.net/go/draw"
	"github.com/rjkroege/acme/frame"
)

const (
	Ldot   = "."
	TABDIR = 3
)

var (
	left1  = []rune{'{', '[', '(', '<', 0xab}
	right1 = []rune{'}', ']', ')', '>', 0xbb}
	left2  = []rune{'\n'}
	left3  = []rune{'\'', '"', '`'}

	left = [][]rune{
		left1,
		left2,
		left3,
	}

	right = [][]rune{
		right1,
		left2,
		left3,
	}
)

type TextKind byte

const (
	Columntag = iota
	Rowtag
	Tag
	Body
)

// Text is a view onto a buffer, managing a frame.
// Files have possible multiple texts corresponding to clones.
type Text struct {
	file    *File
	fr      *frame.Frame
	font    *draw.Font
	org     int // Origin of the frame withing the buffer
	q0      int
	q1      int
	what    TextKind
	tabstop int
	w       *Window
	scrollr image.Rectangle
	lastsr  image.Rectangle
	all     image.Rectangle
	row     *Row
	col     *Column

	iq1         int
	eq0         int
	cq0         int
	ncache      int
	ncachealloc int
	cache       []rune
	nofill      bool
	needundo    bool
}

func (t *Text) Init(f *File, r image.Rectangle, rf *draw.Font, cols [frame.NumColours]*draw.Image) *Text {
	if t == nil {
		t = new(Text)
	}
	t.file = f
	t.all = r
	t.scrollr = r
	t.scrollr.Max.X = r.Min.X + display.ScaleSize(Scrollwid)
	t.lastsr = nullrect
	r.Min.X += display.ScaleSize(Scrollwid) + display.ScaleSize(Scrollgap)
	t.eq0 = math.MaxInt64
	t.ncache = 0
	t.font = rf
	t.tabstop = int(maxtab)
	t.fr = frame.NewFrame(r, rf, display.ScreenImage, cols)
	t.Redraw(r, rf, display.ScreenImage, -1)
	return t
}

func (t *Text) Redraw(r image.Rectangle, f *draw.Font, b *draw.Image, odx int) {
	t.fr.Init(r, f, b, t.fr.Cols)
	rr := t.fr.Rect
	rr.Min.X -= display.ScaleSize(Scrollwid + Scrollgap) /* back fill to scroll bar */
	if !t.fr.NoRedraw {
		t.fr.Background.Draw(rr, t.fr.Cols[frame.ColBack], nil, image.ZP)
	}
	/* use no wider than 3-space tabs in a directory */
	maxt := int(maxtab)
	if t.what == Body {
		if t.w.isdir {
			maxt = min(TABDIR, int(maxtab))
		} else {
			maxt = t.tabstop
		}
	}
	t.fr.Maxtab(maxt * f.StringWidth("0"))
	if t.what == Body && t.w.isdir && odx != t.all.Dx() {
		if t.fr.GetFrameFillStatus().Maxlines > 0 {
			t.Reset()
			t.Columnate(t.w.dirnames, t.w.widths)
			t.Show(0, 0, false)
		}
	} else {
		t.Fill()
		t.SetSelect(t.q0, t.q1)
	}
}

func (t *Text) Resize(r image.Rectangle, keepextra bool) int {
	if r.Dy() <= 0 {
		r.Max.Y = r.Min.Y
	} else {
		if !keepextra {
			r.Max.Y -= r.Dy() % t.fr.Font.DefaultHeight()
		}
	}
	odx := t.all.Dx()
	t.all = r
	t.scrollr = r
	t.scrollr.Max.X = r.Min.X + Scrollwid
	t.lastsr = image.ZR
	r.Min.X += display.ScaleSize(Scrollwid + Scrollgap)
	t.fr.Clear(false)
	t.Redraw(r, t.fr.Font.Impl(), t.fr.Background, odx)
	if keepextra && t.fr.Rect.Max.Y < t.all.Max.Y /* && !t.fr.NoRedraw */ {
		/* draw background in bottom fringe of window */
		r.Min.X -= display.ScaleSize(Scrollgap)
		r.Min.Y = t.fr.Rect.Max.Y
		r.Max.Y = t.all.Max.Y
		display.ScreenImage.Draw(r, t.fr.Cols[frame.ColBack], nil, image.ZP)
	}
	return t.all.Max.Y
}

func (t *Text) Close() {
	t.fr.Clear(true);
	t.file.DelText(t);
	t.file = nil;
	if argtext == t {
		argtext = nil;
	}
	if typetext == t {
		typetext = nil;
	}
	if seltext == t {
		seltext = nil;
	}
	if mousetext == t {
		mousetext = nil;
	}
	if barttext == t {
		barttext = nil;
	}
}

func (t *Text) Columnate(names []string, widths []int) {

	var colw, mint, maxt, ncol, nrow int
	q1 := (0)
	Lnl := []rune("\n")
	Ltab := []rune("\t")

	if len(t.file.text) > 1 {
		return
	}
	mint = t.fr.Font.StringWidth("0")
	/* go for narrower tabs if set more than 3 wide */
	t.fr.Maxtab(min(int(maxtab), TABDIR) * mint)
	maxt = t.fr.GetMaxtab()
	for _, w := range widths {
		if maxt-w%maxt < mint || w%maxt == 0 {
			w += mint
		}
		if w%maxt != 0 {
			w += maxt - (w % maxt)
		}
		if w > colw {
			colw = w
		}
	}
	if colw == 0 {
		ncol = 1
	} else {
		ncol = max(1, t.fr.Rect.Dx()/colw)
	}
	nrow = (len(names) + ncol - 1) / ncol

	q1 = 0
	for i := 0; i < nrow; i++ {
		for j := i; j < len(names); j += nrow {
			dl := names[j]
			t.file.Insert(q1, []rune(dl))
			q1 += (len(dl))
			if j+nrow >= len(names) {
				break
			}
			w := widths[j]
			if maxt-w%maxt < mint {
				t.file.Insert(q1, Ltab)
				q1++
				w += mint
			}
			for {
				t.file.Insert(q1, Ltab)
				q1++
				w += maxt - (w % maxt)
				if !(w < colw) {
					break
				}
			}
		}
		t.file.Insert(q1, Lnl)
		q1++
	}
}

func (t *Text) Load(q0 int, filename string, setqid bool) (nread int, err error) {
	if t.ncache != 0 || t.file.b.nc() > 0 || t.w == nil || t != &t.w.body {
		panic("text.load")
	}
	if t.w.isdir && t.file.name == "" {
		warning(nil, "empty directory name")
		return 0, fmt.Errorf("empty directory name")
	}
	if ismtpt(filename) {
		warning(nil, "will not open self mount point %s\n", filename)
		return 0, fmt.Errorf("will not open self mount point %s\n", filename)
	}
	fd, err := os.Open(filename)
	if err != nil {
		warning(nil, "can't open %s: %v\n", filename, err)
		return 0, fmt.Errorf("can't open %s: %v\n", filename, err)
	}
	defer fd.Close()
	d, err := fd.Stat()
	if err != nil {
		warning(nil, "can't fstat %s: %v\n", filename, err)
		return 0, fmt.Errorf("can't fstat %s: %v\n", filename, err)
	}

	var count int
	q1 := (0)
	hasNulls := false
	var sha1 [sha1.Size]byte
	if d.IsDir() {
		/* this is checked in get() but it's possible the file changed underfoot */
		if len(t.file.text) > 1 {
			warning(nil, "%s is a directory; can't read with multiple windows on it\n", filename)
			return 0, fmt.Errorf("%s is a directory; can't read with multiple windows on it\n", filename)
		}
		t.w.isdir = true
		t.w.filemenu = false
		// TODO(flux): Find all '/' and replace with filepath.Separator properly
		if len(t.file.name) > 0 && !strings.HasSuffix(t.file.name, "/") {
			t.file.name = t.file.name + "/"
			t.w.SetName(t.file.name)
		}
		dirNames, err := fd.Readdirnames(0)
		if err != nil {
			warning(nil, "failed to Readdirnames: %s\n", filename)
			return 0, fmt.Errorf("failed to Readdirnames: %s\n", filename)
		}
		for i, dn := range dirNames {
			f, err := os.Open(dn)
			if err != nil {
				warning(nil, "can't open %s: %v\n", dn, err)
			}
			s, err := f.Stat()
			if err != nil {
				warning(nil, "can't fstat %s: %r\n", dn, err)
			} else {
				if s.IsDir() {
					dirNames[i] = dn + "/"
				}
			}
		}
		sort.Strings(dirNames)
		widths := make([]int, len(dirNames))
		for i, s := range dirNames {
			widths[i] = t.fr.Font.StringWidth(s)
		}
		t.Columnate(dirNames, widths)
		t.w.dirnames = dirNames
		t.w.widths = widths
		q1 = t.file.b.nc()
	} else {
		t.w.isdir = false
		t.w.filemenu = true
		count, sha1, hasNulls, err = t.file.Load(q0, fd)
		if err != nil {
			warning(nil, "Error reading file %s: %v", filename, err)
			return 0, fmt.Errorf("Error reading file %s: %v", filename, err)
		}
		q1 = q0 + count
	}
	if setqid {
		if q0 == 0 {
			t.file.sha1 = sha1
		}
		//t.file.dev = d.dev;
		t.file.mtime = d.ModTime().UnixNano()
		t.file.qidpath = d.Name() // TODO(flux): Gross hack to use filename as unique ID of file.
	}
	fd.Close()
	n := q1 - q0
	if q0 < t.org { // TODO(flux) I don't understand this test, moving origin of frame past the text.
		t.org += n
	} else {
		if q0 <= t.org+(t.fr.GetFrameFillStatus().Nchars) { // Text is within the window, put it there.
			t.fr.Insert(t.file.b[q0:q0+n], int(q0-t.org))
		}
	}
	// For each clone, redraw
	for _, u := range t.file.text {
		if u != t { // Skip the one we just redrew
			if u.org > u.file.b.nc() { /* will be 0 because of reset(), but safety first */
				u.org = 0
			}
			u.Resize(u.all, true)
			u.Backnl(u.org, 0) /* go to beginning of line */
		}
		u.SetSelect(q0, q0)
	}
	if hasNulls {
		warning(nil, "%s: NUL bytes elided\n", filename)
	}
	return q1 - q0, nil

}

func (t *Text) Backnl(p int, n int) int {
	/* look for start of this line if n==0 */
	if n == 0 && p > 0 && t.ReadRune(p-1) != '\n' {
		n = 1
	}
	i := n
	for i > 0 && p > 0 {
		i--
		p-- /* it's at a newline now; back over it */
		if p == 0 {
			break
		}
		/* at 128 chars, call it a line anyway */
		for j := 128; j > 0 && p > 0; p-- {
			j--
			if t.ReadRune(p-1) == '\n' {
				break
			}
		}
	}
	return p
}

func (t *Text) BsInsert(q0 int, r []rune, tofile bool) (q, nrp int) {
var (
	tp []rune
	bp, up, i, initial int
)
	n := len(r)
	if t.what == Tag {	// can't happen but safety first: mustn't backspace over file name
		t.Insert( q0, r, tofile);
		nrp = n
		return q0, nrp;
	}
	bp = 0 // bp indexes r
	for i=0; i<n; i++  {
		if r[bp] == '\b' {
			initial = 0;
			tp = make([]rune, n);
			copy(tp, r[:i]);
			up = i; // up indexes tp, starting at i
			for ; i<n; i++ {
				tp[up] = r[bp];
				bp++
				if tp[up] == '\b'  {
					if up == 0 {
						initial++;
					} else {
						up--;
					}
				} else {
					up++;
				}
			}
			if initial != 0 {
				if initial > q0 {
					initial = q0;
				}
				q0 -= initial;
				t.Delete( q0, q0+initial, tofile);
			}
			n = up;
			t.Insert( q0, tp[:n], tofile);
			nrp = n;
			return q0, nrp;
		} else {
			bp++
		}
	}
	t.Insert( q0, r, tofile);
	nrp = n
	return q0, nrp;
}


func (t *Text) Insert(q0 int, r []rune, tofile bool) {
	if tofile && t.ncache != 0 {
		panic("text.insert")
	}
	if len(r) == 0 {
		return
	}
	if tofile {
		t.file.Insert(q0, r)
		if t.what == Body {
			t.w.dirty = true
			t.w.utflastqid = -1
		}
		if len(t.file.text) > 1 {
			for _, u := range t.file.text {
				if u != t {
					u.w.dirty = true /* always a body */
					u.Insert(q0, r, false)
					u.SetSelect(u.q0, u.q1)
					u.ScrDraw()
				}
			}
		}
	}
	n := (len(r))
	if q0 < t.iq1 {
		t.iq1 += n
	}
	if q0 < t.q1 {
		t.q1 += n
	}
	if q0 < t.q0 {
		t.q0 += n
	}
	if q0 < t.org {
		t.org += n
	} else {
		if q0 <= t.org+(t.fr.GetFrameFillStatus().Nchars) {
			t.fr.Insert(r[:n], int(q0-t.org))
		}
	}
	if t.w != nil {
		c := 'i'
		if t.what == Body {
			c = 'I'
		}
		if n <= EVENTSIZE {
			t.w.Event("%c%d %d 0 %d %.*S\n", c, q0, q0+n, n, n, r)
		} else {
			t.w.Event("%c%d %d 0 0 \n", c, q0, q0+n, n)
		}
	}
}

func (t *Text) TypeCommit() {
	if t.w != nil {
		t.w.Commit(t)
	} else {
		t.Commit(true)
	}
}

func (t *Text) Fill() {
	if t.fr.LastLineFull != 0 || t.nofill {
		return
	}
	if t.ncache > 0 {
		t.TypeCommit()
	}
	/*
		nl := t.fr.GetFrameFillStatus().Maxlines - t.fr.GetFrameFillStatus().Nlines
		lines := runesplitN(t.file.b[t.org+(t.fr.GetFrameFillStatus().Nchars):], []rune("\n"), nl)
		for _, s := range lines {
			t.fr.Insert(s, t.fr.GetFrameFillStatus().Nchars)
			if t.fr.LastLineFull != 0 {
				break
			}
		}
	*/
	for {
		n := t.file.b.nc() - (t.org + t.fr.GetFrameFillStatus().Nchars)
		if n == 0 {
			break
		}
		if n > 2000 { /* educated guess at reasonable amount */
			n = 2000
		}
		rp := t.file.b.Read(t.org+t.fr.GetFrameFillStatus().Nchars, n)
		/*
		 * it's expensive to frinsert more than we need, so
		 * count newlines.
		 */
		nl := t.fr.GetFrameFillStatus().Maxlines - t.fr.GetFrameFillStatus().Nlines
		m := 0
		var i int
		for i = 0; i < n; {
			i++
			if rp[i-1] == '\n' {
				m++
				if m >= nl {
					break
				}
			}
		}
fmt.Printf("Fill: nlines %v, nchars %v, t.org %v, i %v\n", t.fr.GetFrameFillStatus().Nlines, t.fr.GetFrameFillStatus().Nchars, t.org, i)
		t.fr.Insert(rp[:i], t.fr.GetFrameFillStatus().Nchars)
		if (t.fr.LastLineFull != 0) {
			break
		}
fmt.Printf("Fill: nlines %v, nchars %v, t.org %v\n", t.fr.GetFrameFillStatus().Nlines, t.fr.GetFrameFillStatus().Nchars, t.org)
	}
}

func (t *Text) Delete(q0, q1 int, tofile bool) {
	if tofile && t.ncache != 0 {
		panic("text.delete")
	}
	n := q1 - q0
	if n == 0 {
		return
	}
	if tofile {
		t.file.Delete(q0, q1)
		if t.what == Body {
			t.w.dirty = true
			t.w.utflastqid = -1
		}
		if len(t.file.text) > 1 {
			for _, u := range t.file.text {
				if u != t {
					u.w.dirty = true /* always a body */
					u.Delete(q0, q1, false)
					u.SetSelect(u.q0, u.q1)
					u.ScrDraw()
				}
			}
		}
	}
	if q0 < t.iq1 {
		t.iq1 -= min(n, t.iq1-q0)
	}
	if q0 < t.q0 {
		t.q0 -= min(n, t.q0-q0)
	}
	if q0 < t.q1 {
		t.q1 -= min(n, t.q1-q0)
	}
	if q1 <= t.org {
		t.org -= n
	} else if q0 < t.org+(t.fr.GetFrameFillStatus().Nchars) {
		p1 := q1 - t.org
		p0 := (0)
		if p1 > (t.fr.GetFrameFillStatus().Nchars) {
			p1 = (t.fr.GetFrameFillStatus().Nchars)
		}
		if q0 < t.org {
			t.org = q0
			p0 = 0
		} else {
			p0 = q0 - t.org
		}
		t.fr.Delete((p0), (p1))
		t.Fill()
	}
	if t.w != nil {
		c := 'd'
		if t.what == Body {
			c = 'D'
		}
		t.w.Event("%c%d %d 0 0 \n", c, q0, q1)
	}
}

func (t *Text) Read(q, n int) []rune { return t.file.b.Read(q, n) }
func (t *Text) nc() int              { return t.file.b.nc() }
func (t *Text) Q0() int              { return t.q0 }
func (t *Text) Q1() int              { return t.q1 }
func (t *Text) SetQ0(q0 int)         { t.q0 = q0 }
func (t *Text) SetQ1(q1 int)         { t.q1 = q1 }
func (t *Text) Constrain(q0, q1 int) (p0, p1 int) {
	p0 = min(q0, t.file.b.nc())
	p1 = min(q1, t.file.b.nc())
	return p0, p1
}

func (t *Text) ReadRune(q int) rune {
	if t.cq0 <= q && q < t.cq0+(t.ncache) {
		return t.cache[q-t.cq0]
	} else {
		return t.file.b.Read(q, 1)[0]
	}
}

func (t *Text) BsWidth(c rune) int {
	/* there is known to be at least one character to erase */
	if c == 0x08  {	/* ^H: erase character */
		return 1;
	}
	q := t.q0;
	skipping := true;
	for(q > 0){
		r := t.ReadC(q-1);
		if r == '\n' {		/* eat at most one more character */
			if q == t.q0 {	/* eat the newline */
				q--
			}
			break; 
		}
		if c == 0x17 {
			eq := isalnum(r);
			if eq && skipping {	/* found one; stop skipping */
				skipping = false;
			} else {
				if !eq && !skipping {
					break;
				}
			}
		}
		q--
	}
	return t.q0-q;
}

func (t *Text) FileWidth(q0 int, oneelement bool) int {
	q := q0;
	for(q > 0){
		r := t.ReadC(q-1);
		if r <= ' ' {
			break;
		}
		if oneelement && r=='/' {
			break;
		}
		q--
	}
	return q0-q;
}

func (t *Text) Complete() []rune {
	Unimpl()
	return nil
}

func (t *Text) Type(r rune) {
	var (
		q0, q1        int
		nnb, nb, n, i int
		nr            int
	)
	if t.what != Body && t.what != Tag && r == '\n' {
		return
	}
	if t.what == Tag {
		t.w.tagsafe = false
	}
	nr = 1
	rp := []rune{r}

	Tagdown := func() {
		/* expand tag to show all text */
		if !t.w.tagexpand {
			t.w.tagexpand = true
			t.w.Resize(t.w.r, false, true)
		}
		return
	}

	Tagup := func() {
		/* shrink tag to single line */
		if t.w.tagexpand {
			t.w.tagexpand = false
			t.w.taglines = 1
			t.w.Resize(t.w.r, false, true)
		}
		return
	}

	case_Down := func() {
		q0 = t.org + t.fr.Charofpt(image.Pt(t.fr.Rect.Min.X, t.fr.Rect.Min.Y+n*t.fr.Font.DefaultHeight()))
		t.SetOrigin(q0, true)
		return
	}
	case_Up := func() {
		q0 = t.Backnl(t.org, n)
		t.SetOrigin(q0, true)
		return
	}

	switch r {
	case draw.KeyLeft:
		t.TypeCommit()
		if t.q0 > 0 {
			t.Show(t.q0-1, t.q0-1, true)
		}
		return
	case draw.KeyRight:
		t.TypeCommit()
		if t.q1 < t.file.b.nc() {
			t.Show(t.q1+1, t.q1+1, true)
		}
		return
	case draw.KeyDown:
		if t.what == Tag {
			Tagdown()
			return
		}
		n = t.fr.GetFrameFillStatus().Maxlines / 3
		case_Down()
		return
	case Kscrollonedown:
		if t.what == Tag {
			Tagdown()
			return
		}
		n = mousescrollsize(t.fr.GetFrameFillStatus().Maxlines)
		if n <= 0 {
			n = 1
		}
		case_Down()
		return
	case draw.KeyPageDown:
		n = 2 * t.fr.GetFrameFillStatus().Maxlines / 3
		case_Down()
		return
	case draw.KeyUp:
		if t.what == Tag {
			Tagup()
			return
		}
		n = t.fr.GetFrameFillStatus().Maxlines / 3
		case_Up()
		return
	case Kscrolloneup:
		if t.what == Tag {
			Tagup()
			return
		}
		n = mousescrollsize(t.fr.GetFrameFillStatus().Maxlines)
		case_Up()
		return
	case draw.KeyPageUp:
		n = 2 * t.fr.GetFrameFillStatus().Maxlines / 3
		case_Up()
		return
	case draw.KeyHome:
		t.TypeCommit()
		if t.org > t.iq1 {
			q0 = t.Backnl(t.iq1, 1)
			t.SetOrigin(q0, true)
		} else {
			t.Show(0, 0, false)
		}
		return
	case draw.KeyEnd:
		t.TypeCommit()
		if t.iq1 > t.org+t.fr.GetFrameFillStatus().Nchars {
			if t.iq1 > t.file.b.nc() {
				// should not happen, but does. and it will crash textbacknl.
				t.iq1 = t.file.b.nc()
			}
			q0 = t.Backnl(t.iq1, 1)
			t.SetOrigin(q0, true)
		} else {
			t.Show(t.file.b.nc(), t.file.b.nc(), false)
		}
		return
	case 0x01: /* ^A: beginning of line */
		t.TypeCommit()
		/* go to where ^U would erase, if not already at BOL */
		nnb = 0
		if t.q0 > 0 && t.ReadC(t.q0-1) != '\n' {
			nnb = t.BsWidth(0x15)
		}
		t.Show(t.q0-nnb, t.q0-nnb, true)
		return
	case 0x05: /* ^E: end of line */
		t.TypeCommit()
		q0 = t.q0
		for q0 < t.file.b.nc() && t.ReadC(q0) != '\n' {
			q0++
		}
		t.Show(q0, q0, true)
		return
	case draw.KeyCmd + 'c': /* %C: copy */
		t.TypeCommit()
		cut(t, t, nil, true, false, nil, 0)
		return
	case draw.KeyCmd + 'z': /* %Z: undo */
		t.TypeCommit()
		undo(t, nil, nil, true, false, nil, 0)
		return
	case draw.KeyCmd + 'Z': /* %-shift-Z: redo */
		t.TypeCommit()
		undo(t, nil, nil, false, false, nil, 0)
		return

	}
	if t.what == Body {
		seq++
		t.file.Mark()
	}
	/* cut/paste must be done after the seq++/filemark */
	switch r {
	case draw.KeyCmd + 'x': /* %X: cut */
		t.TypeCommit()
		if t.what == Body {
			seq++
			t.file.Mark()
		}
		cut(t, t, nil, true, true, nil, 0)
		t.Show(t.q0, t.q0, true)
		t.iq1 = t.q0
		return
	case draw.KeyCmd + 'v': /* %V: paste */
		t.TypeCommit()
		if t.what == Body {
			seq++
			t.file.Mark()
		}
		paste(t, t, nil, true, false, nil, 0)
		t.Show(t.q0, t.q1, true)
		t.iq1 = t.q1
		return
	}
	if t.q1 > t.q0 {
		if t.ncache != 0 {
			acmeerror("text.type", nil)
		}
		cut(t, t, nil, true, true, nil, 0)
		t.eq0 = ^0
	}
	t.Show(t.q0, t.q0, true)
	switch r {
	case 0x06:
		fallthrough /* ^F: complete */
	case draw.KeyInsert:
		t.TypeCommit()
		rp = t.Complete()
		if rp == nil {
			return
		}
		nr = len(rp) // runestrlen(rp);
		break        /* fall through to normal insertion case */
	case 0x1B:
		if t.eq0 != ^0 {
			if t.eq0 <= t.q0 {
				t.SetSelect(t.eq0, t.q0)
			} else {
				t.SetSelect(t.q0, t.eq0)
			}
		}
		if t.ncache > 0 {
			t.TypeCommit()
		}
		t.iq1 = t.q0
		return
	case 0x08:
		fallthrough /* ^H: erase character */
	case 0x15:
		fallthrough /* ^U: erase line */
	case 0x17: /* ^W: erase word */
		if t.q0 == 0 { /* nothing to erase */
			return
		}
		nnb = t.BsWidth(r)
		q1 = t.q0
		q0 = q1 - nnb
		/* if selection is at beginning of window, avoid deleting invisible text */
		if q0 < t.org {
			q0 = t.org
			nnb = q1 - q0
		}
		if nnb <= 0 {
			return
		}
		for _, u := range t.file.text { // u is *Text
			u.nofill = true
			nb = nnb
			n = u.ncache
			if n > 0 {
				if q1 != u.cq0+n {
					acmeerror("text.type backspace", nil)
				}
				if n > nb {
					n = nb
				}
				u.ncache -= n
				u.Delete(q1-n, q1, false)
				nb -= n
			}
			if u.eq0 == q1 || u.eq0 == ^0 {
				u.eq0 = q0
			}
			if nb != 0 && u == t {
				u.Delete(q0, q0+nb, true)
			}
			if u != t {
				u.SetSelect(u.q0, u.q1)
			} else {
				t.SetSelect(q0, q0)
			}
			u.nofill = false
		}
		for _, t := range t.file.text {
			t.Fill()
		}
		t.iq1 = t.q0
		return
	case '\n':
		if t.w.autoindent {
			/* find beginning of previous line using backspace code */
			nnb = t.BsWidth(0x15)    /* ^U case */
			rp = make([]rune, nnb+1) //runemalloc(nnb + 1);
			nr = 0
			rp[nr] = r
			nr++
			for i = 0; i < nnb; i++ {
				r = t.ReadC(t.q0 - nnb + i)
				if r != ' ' && r != '\t' {
					break
				}
				rp[nr] = r
				nr++
			}
		}
		break /* fall through to normal code */
	}
	/* otherwise ordinary character; just insert, typically in caches of all texts */
	for _, u := range t.file.text { // u is *Text
		if u.eq0 == ^0 {
			u.eq0 = t.q0
		}
		if u.ncache == 0 {
			u.cq0 = t.q0
		} else {
			if t.q0 != u.cq0+u.ncache {
				acmeerror("text.type cq1", nil)
			}
		}
		/*
		 * Change the tag before we add to ncache,
		 * so that if the window body is resized the
		 * commit will not find anything in ncache.
		 */
		if u.what == Body && u.ncache == 0 {
			u.needundo = true
			t.w.SetTag()
			u.needundo = false
		}
		u.Insert(t.q0, rp, false)
		if u != t {
			u.SetSelect(u.q0, u.q1)
		}
		if u.ncache+nr > u.ncachealloc {
			u.ncachealloc += 10 + nr
			u.cache = make([]rune, u.ncachealloc) //runerealloc(u.cache, u.ncachealloc);
		}
		//runemove(u.cache+u.ncache, rp, nr);
		copy(u.cache[u.ncache:], rp[:nr])
		u.ncache += nr
	}
	t.SetSelect(t.q0+nr, t.q0+nr)
	if r == '\n' && t.w != nil {
		t.w.Commit(t)
	}
	t.iq1 = t.q0

}

func (t *Text) Commit(tofile bool) {
	if t.ncache == 0 {
		return
	}
	if tofile {
		t.file.Insert(t.cq0, t.cache)
	}
	if t.what == Body {
		t.w.dirty = true
		t.w.utflastqid = -1
	}
	t.ncache = 0
}

func getP0(fr *frame.Frame) int {
	p0, _ := fr.GetSelectionExtent()
	return p0
}
func getP1(fr *frame.Frame) int {
	_, p1 := fr.GetSelectionExtent()
	return p1
}

func (t *Text) FrameScroll(dl int) {
	var q0 int
	if dl == 0 {
		ScrSleep(100)
		return
	}
	if dl < 0 {
		q0 = t.Backnl(t.org, (-dl))
		if selectq > t.org+(getP0((t.fr))) {
			t.SetSelect(t.org+(getP0((t.fr))), selectq)
		} else {
			t.SetSelect(selectq, t.org+(getP0((t.fr))))
		}
	} else {
		if t.org+(t.fr.GetFrameFillStatus().Nchars) == t.file.b.nc() {
			return
		}
		q0 = t.org + (t.fr.Charofpt(image.Pt(t.fr.Rect.Min.X, t.fr.Rect.Min.Y+dl*t.fr.Font.Impl().Height)))
		if selectq > t.org+(getP1((t.fr))) {
			t.SetSelect(t.org+(getP1((t.fr))), selectq)
		} else {
			t.SetSelect(selectq, t.org+(getP1((t.fr))))
		}
	}
	t.SetOrigin(q0, true)
}

var (
	clicktext  *Text
	clickmsec  uint32
	selecttext *Text
	selectq    int
)

/*
 * called from frame library
 */
func framescroll(f *frame.Frame, dl int) {
	if f != selecttext.fr {
		panic("frameselect not right frame")
	}
	selecttext.FrameScroll(dl)
}

func (t *Text) Select() {
	const (
		None = iota
		Cut
		Paste
	)

	selecttext = t
	/*
	 * To have double-clicking and chording, we double-click
	 * immediately if it might make sense.
	 */
	b := mouse.Buttons
	q0 := t.q0
	q1 := t.q1
	selectq = t.org + (t.fr.Charofpt(mouse.Point))
	//	fmt.Printf("Text.Select: mouse.Msec %v, clickmsec %v\n", mouse.Msec, clickmsec)
	//	fmt.Printf("clicktext==t %v, (q0==q1 && selectq==q0): %v", clicktext == t, q0 == q1 && selectq == q0)
	if (clicktext == t && mouse.Msec-uint32(clickmsec) < 500) && (q0 == q1 && selectq == q0) {
		q0, q1 = t.DoubleClick(q0)
		fmt.Printf("Text.Select: DoubleClick returned %d, %d\n", q0, q1)
		t.SetSelect(q0, q1)
		display.Flush()
		x := mouse.Point.X
		y := mouse.Point.Y
		/* stay here until something interesting happens */
		for {
			mousectl.Read()
			if !(mouse.Buttons == b && abs(mouse.Point.X-x) < 3 && abs(mouse.Point.Y-y) < 3) {
				break
			}
		}
		mouse.Point.X = x /* in case we're calling frselect */
		mouse.Point.Y = y
		q0 = t.q0 /* may have changed */
		q1 = t.q1
		selectq = q0
	}
	if mouse.Buttons == b {
		t.fr.Scroll = framescroll
		t.fr.Select(*mousectl)
		/* horrible botch: while asleep, may have lost selection altogether */
		if selectq > t.file.b.nc() {
			selectq = t.org + (getP0((t.fr)))
		}
		t.fr.Scroll = nil
		if selectq < t.org {
			q0 = selectq
		} else {
			q0 = t.org + (getP0((t.fr)))
		}
		if selectq > t.org+(t.fr.GetFrameFillStatus().Nchars) {
			q1 = selectq
		} else {
			q1 = t.org + (getP1((t.fr)))
		}
	}
	if q0 == q1 {
		if q0 == t.q0 && clicktext == t && mouse.Msec-uint32(clickmsec) < 500 {
			q0, q1 = t.DoubleClick(q0)
			clicktext = nil
		} else {
			clicktext = t
			clickmsec = mouse.Msec
		}
	} else {
		clicktext = nil
	}
	t.SetSelect(q0, q1)
	display.Flush()
	state := None /* what we've done; undo when possible */
	for mouse.Buttons != 0 {
		mouse.Msec = 0
		b := mouse.Buttons
		if (b&1) != 0 && (b&6) != 0 {
			if state == None && t.what == Body {
				seq++
				t.w.body.file.Mark()
			}
			if b&2 != 0 {
				if state == Paste && t.what == Body {
					t.w.Undo(true)
					t.SetSelect(q0, t.q1)
					state = None
				} else {
					if state != Cut {
						cut(t, t, nil, true, true, nil, 0)
						state = Cut
					}
				}
			} else {
				if state == Cut && t.what == Body {
					t.w.Undo(true)
					t.SetSelect(q0, t.q1)
					state = None
				} else {
					if state != Paste {
						paste(t, t, nil, true, false, nil, 0)
						state = Paste
					}
				}
			}
			t.ScrDraw()
			clearmouse()
		}
		display.Flush()
		for mouse.Buttons == b {
			mousectl.Read()
		}
		clicktext = nil
	}
}

func (t *Text) Show(q0, q1 int, doselect bool) {
	var (
		qe  int
		nl  int
		tsd bool
		nc  int
		q   int
	)
	if t.what != Body {
		if doselect {
			t.SetSelect(0, q1)
		}
		return
	}
	if t.w != nil && t.fr.GetFrameFillStatus().Maxlines == 0 {
		t.col.Grow(t.w, 1)
	}
	if doselect {
		t.SetSelect(q0, q1)
	}
	qe = t.org + t.fr.GetFrameFillStatus().Nchars
	tsd = false /* do we call textscrdraw? */
	nc = t.file.b.nc() + t.ncache
	if t.org <= q0 {
		if nc == 0 || q0 < qe {
			tsd = true
		} else {
			if q0 == qe && qe == nc {
				if t.ReadC(nc-1) == '\n' {
					if t.fr.GetFrameFillStatus().Nlines < t.fr.GetFrameFillStatus().Maxlines {
						tsd = true
					}
				} else {
					tsd = true
				}
			}
		}
	}
	if tsd {
		t.ScrDraw()
	} else {
		if t.w.nopen[QWevent] > 0 {
			nl = 3 * t.fr.GetFrameFillStatus().Maxlines / 4
		} else {
			nl = t.fr.GetFrameFillStatus().Maxlines / 4
		}
		q = t.Backnl(q0, nl)
		/* avoid going backwards if trying to go forwards - long lines! */
		if !(q0 > t.org && q < t.org) {
			t.SetOrigin(q, true)
		}
		for q0 > t.org+t.fr.GetFrameFillStatus().Nchars {
			t.SetOrigin(t.org+1, false)
		}
	}
}

func (t *Text) ReadC(q int) (r rune) {
	if t.cq0 <= q && q < t.cq0+(t.ncache) {
		r = t.cache[q-t.cq0]
	} else {
		r = t.file.b.Read(q, 1)[0]
	}
	return r

}

func (t *Text) SetSelect(q0, q1 int) {
	/* (getP0((t.fr))) and (getP1((t.fr))) are always right; t.q0 and t.q1 may be off */
	t.q0 = q0
	t.q1 = q1
	/* compute desired p0,p1 from q0,q1 */
	p0 := q0 - t.org
	p1 := q1 - t.org
	ticked := true
	if p0 < 0 {
		ticked = false
		p0 = 0
	}
	if p1 < 0 {
		p1 = 0
	}
	if p0 > (t.fr.GetFrameFillStatus().Nchars) {
		p0 = (t.fr.GetFrameFillStatus().Nchars)
	}
	if p1 > (t.fr.GetFrameFillStatus().Nchars) {
		ticked = false
		p1 = (t.fr.GetFrameFillStatus().Nchars)
	}
	if p0 == (getP0((t.fr))) && p1 == (getP1((t.fr))) {
		if p0 == p1 && ticked != t.fr.Ticked {
			t.fr.Tick(t.fr.Ptofchar((p0)), ticked)
		}
		return
	}
	if p0 > p1 {
		panic(fmt.Sprintf("acme: textsetselect p0=%d p1=%d q0=%v q1=%v t.org=%d nchars=%d", p0, p1, q0, q1, t.org, t.fr.GetFrameFillStatus().Nchars))
	}
	/* screen disagrees with desired selection */
	if (getP1((t.fr))) <= p0 || p1 <= (getP0((t.fr))) || p0 == p1 || (getP1((t.fr))) == (getP0((t.fr))) {
		/* no overlap or too easy to bother trying */
		t.fr.DrawSel(t.fr.Ptofchar(getP0((t.fr))),(getP0( t.fr)),(getP1( t.fr)), false)
		if p0 != p1 || ticked {
			t.fr.DrawSel(t.fr.Ptofchar(int(p0)), int(p0), int(p1), true)
		}
		goto Return
	}
	/* overlap; avoid unnecessary painting */
	if p0 < (getP0((t.fr))) {
		/* extend selection backwards */
		t.fr.DrawSel(t.fr.Ptofchar((p0)), (p0),(getP0( t.fr)), true)
	} else {
		if p0 > (getP0((t.fr))) {
			/* trim first part of selection */
			t.fr.DrawSel(t.fr.Ptofchar(getP0((t.fr))),(getP0( t.fr)), (p0), false)
		}
	}
	if p1 > (getP1((t.fr))) {
		/* extend selection forwards */
		t.fr.DrawSel(t.fr.Ptofchar(getP1((t.fr))),(getP1( t.fr)), (p1), true)
	} else if p1 < (getP1((t.fr))) {
		/* trim last part of selection */
		t.fr.DrawSel(t.fr.Ptofchar(int(p1)), (p1),(getP1( t.fr)), false)
	}

Return:
	t.fr.SetSelectionExtent(p0, p1)
}

func selrestore(f *frame.Frame, pt0 image.Point, p0, p1 int) {

	if p1 <= (getP0((f))) || p0 >= (getP1((f))) {
		/* no overlap */
		f.Drawsel0(pt0, (p0), (p1), f.Cols[frame.ColBack], f.Cols[frame.ColText])
		return
	}
	if p0 >= (getP0((f))) && p1 <= (getP1((f))) {
		/* entirely inside */
		f.Drawsel0(pt0, (p0), (p1), f.Cols[frame.ColHigh], f.Cols[frame.ColHText])
		return
	}

	/* they now are known to overlap */

	/* before selection */
	if p0 < (getP0((f))) {
		f.Drawsel0(pt0, (p0),(getP0( f)), f.Cols[frame.ColBack], f.Cols[frame.ColText])
		p0 = (getP0((f)))
		pt0 = f.Ptofchar((p0))
	}
	/* after selection */
	if p1 > (getP1((f))) {
		f.Drawsel0(f.Ptofchar(getP1((f))),(getP1( f)), int(p1), f.Cols[frame.ColBack], f.Cols[frame.ColText])
		p1 = (getP1((f)))
	}
	/* inside selection */
	f.Drawsel0(pt0, (p0), (p1), f.Cols[frame.ColHigh], f.Cols[frame.ColHText])
}

const (
	DELAY   = 2
	MINMOVE = 4
)

// When called, button is down.
func xselect(f *frame.Frame, mc *draw.Mousectl, col *draw.Image) (p0p, p1p int) {
	mp := mc.Mouse.Point
	b := mc.Mouse.Buttons
	msec := mc.Mouse.Msec

	/* remove tick */
	if(getP0( f)) ==(getP1( f)) {
		f.Tick(f.Ptofchar(getP0((f))), false)
	}
	p0 := (f.Charofpt(mp))
	p1 := (p0)
	pt0 := f.Ptofchar((p0))
	pt1 := f.Ptofchar((p1))
	reg := 0
	f.Tick(pt0, true)
	for {
		q := (f.Charofpt(mc.Mouse.Point))
		if p1 != q {
			if p0 == p1 {
				f.Tick(pt0, false)
			}
			if reg != region(q, p0) { /* crossed starting point; reset */
				if reg > 0 {
					selrestore(f, pt0, p0, p1)
				} else {
					if reg < 0 {
						selrestore(f, pt1, p1, p0)
					}
				}
				p1 = p0
				pt1 = pt0
				reg = region(q, p0)
				if reg == 0 {
					f.Drawsel0(pt0, int(p0), int(p1), col, display.White)
				}
			}
			qt := f.Ptofchar(int(q))
			if reg > 0 {
				if q > p1 {
					f.Drawsel0(pt1, int(p1), int(q), col, display.White)
				} else {
					if q < p1 {
						selrestore(f, qt, q, p1)
					}
				}
			} else {
				if reg < 0 {
					if q > p1 {
						selrestore(f, pt1, p1, q)
					} else {
						f.Drawsel0(qt, int(q), int(p1), col, display.White)
					}
				}
			}
			p1 = q
			pt1 = qt
		}
		if p0 == p1 {
			f.Tick(pt0, true)
		}
		display.Flush()
		mc.Read()
		if mc.Mouse.Buttons != b {
			break
		}
	}
	if mc.Mouse.Msec-msec < DELAY && p0 != p1 && abs(mp.X-mc.Mouse.Point.X) < MINMOVE && abs(mp.Y-mc.Mouse.Point.Y) < MINMOVE {
		if reg > 0 {
			selrestore(f, pt0, p0, p1)
		} else {
			if reg < 0 {
				selrestore(f, pt1, p1, p0)
			}
		}
		p1 = p0
	}
	if p1 < p0 {
		p0, p1 = p1, p0
	}
	pt0 = f.Ptofchar(int(p0))
	if p0 == p1 {
		f.Tick(pt0, false)
	}
	selrestore(f, pt0, p0, p1)
	/* restore tick */
	if(getP0( f)) ==(getP1( f)) {
		f.Tick(f.Ptofchar(getP0((f))), true)
	}
	display.Flush()
	return p0, p1
}

func (t *Text) Select23(high *draw.Image, mask uint) (q0, q1 int, buts uint) {
	p0, p1 := xselect(t.fr, mousectl, high)
	buts = uint(mousectl.Mouse.Buttons)
	if (buts & mask) == 0 {
		q0 = p0 + t.org
		q1 = p1 + t.org
	}

	for mousectl.Mouse.Buttons != 0 {
		mousectl.Read()
	}
	return q0, q1, buts
}

func (t *Text) Select2() (q0, q1 int, tp *Text, ret bool) {
	q0, q1, buts := t.Select23(but2col, 4)
	if (buts & 4) == 0 {
		return q0, q1, nil, false
	}
	if (buts & 1) != 0 { /* pick up argument */
		return q0, q1, argtext, true
	}
	return q0, q1, nil, true
}

func (t *Text) Select3() (q0, q1 int, r bool) {
	q0, q1, buts := t.Select23(but3col, 1|2)
	return q0, q1, buts == 0
}

func (t *Text) DoubleClick(inq0 int) (q0, q1 int) {
	q0 = inq0
	if q0, q1, ok := t.ClickHTMLMatch(inq0); ok {
		return q0, q1
	}
	var c rune
	for i, l := range left {
		q := inq0
		r := right[i]
		/* try matching character to left, looking right */
		if q == 0 {
			c = '\n'
		} else {
			c = t.ReadC(q - 1)
		}
		p := runestrchr(l, c)
		if p != -1 {
			if q, ok := t.ClickMatch(c, r[p], 1, q); ok {
				q1 = q
				if c != '\n' {
					q1--
				}
			}
			return
		}
		/* try matching character to right, looking left */
		if q == t.file.b.nc() {
			c = '\n'
		} else {
			c = t.ReadC(q)
		}
		p = runestrchr(r, c)
		if p != -1 {
			if q, ok := t.ClickMatch(c, l[p], -1, q); ok {
				q1 = inq0
				if q0 < t.file.b.nc() && c == '\n' {
					q1++
				}
				q0 = q
				if c != '\n' || q != 0 || t.ReadC(0) == '\n' {
					q0++
				}
			}
			return
		}
	}

	/* try filling out word to right */
	for q1 < t.file.b.nc() && isalnum(t.ReadC(q1)) {
		q1++
	}
	/* try filling out word to left */
	for q0 > 0 && isalnum(t.ReadC(q0-1)) {
		q0--
	}

	return q0, q1
}

func (t *Text) ClickMatch(cl, cr rune, dir int, inq int) (q int, r bool) {
	nest := 1
	var c rune
	for {
		if dir > 0 {
			if inq == t.file.b.nc() {
				break
			}
			c = t.ReadC(inq)
			(inq)++
		} else {
			if inq == 0 {
				break
			}
			(inq)--
			c = t.ReadC(inq)
		}
		if c == cr {
			nest--
			if nest == 0 {
				return inq, true
			}
		} else {
			if c == cl {
				nest++
			}
		}
	}
	return inq, cl == '\n' && nest == 1
}

func (t *Text) ishtmlstart(q uint, q1 *int) bool {
	Unimpl()
	return false
}

func (t *Text) ishtmlend(q uint, q0 *int) bool {
	Unimpl()
	return false
}

func (t *Text) ClickHTMLMatch(inq0 int) (q0, q1 int, r bool) {
	Unimpl()
	return 0, 0, false
}

func (t *Text) BackNL(p, n int) int {
	var i int

	/* look for start of this line if n==0 */
	if n==0 && p>0 && t.ReadC(p-1)!='\n' {
		n = 1;
	}
	i = n;
	for(i>0 && p>0){
		i--
		p--	/* it's at a newline now; back over it */
		if p == 0 {
			break;
		}
		/* at 128 chars, call it a line anyway */
		for j:=128; j>0 && p>0; p--  {
			if t.ReadC(p-1)=='\n' {
				break;
			}
			j--
		}
	}
	return p;
}

func (t *Text) SetOrigin(org int, exact bool) {
	var (
		i, a  int
		fixup bool
		r     []rune
		n     int
	)
	if org > 0 && !exact && t.ReadC(org-1) != '\n' {
		/* org is an estimate of the char posn; find a newline */
		/* don't try harder than 256 chars */
		for i = 0; i < 256 && org < t.file.b.nc(); i++ {
			if t.ReadC(org) == '\n' {
				org++
				break
			}
			org++
		}
	}
	a = org - t.org
	fixup = false
	if a >= 0 && a < t.fr.GetFrameFillStatus().Nchars {
		t.fr.Delete(0, a)
		fixup = true /* frdelete can leave end of last line in wrong selection mode; it doesn't know what follows */
	} else {
		if a < 0 && -a < t.fr.GetFrameFillStatus().Nchars {
			n = t.org - org
			r = t.file.b.Read(org, n)
			t.fr.Insert(r, 0)
		} else {
			t.fr.Delete(0, t.fr.GetFrameFillStatus().Nchars)
		}
	}
	t.org = org
fmt.Printf("Text.SetOrigin: t.org = %v\n", t.org)
	t.Fill()
	t.ScrDraw()
	t.SetSelect(t.q0, t.q1)
	if fixup &&(getP1( t.fr)) >(getP0( t.fr)) {
		t.fr.DrawSel(t.fr.Ptofchar(getP1((t.fr))-1),(getP1( t.fr))-1,(getP1( t.fr)), true)
	}
}

func (t *Text) Reset() {
	t.file.seq = 0;
	t.eq0 = ^0;
	/* do t.delete(0, t.nc, true) without building backup stuff */
	t.SetSelect(t.org, t.org);
	t.fr.Delete(0, t.fr.GetFrameFillStatus().Nchars);
	t.org = 0;
	t.q0 = 0;
	t.q1 = 0;
	t.file.Reset();
	t.file.b.Reset();
}

func (t *Text) DirName() string {
	if t == nil || t.w == nil {
		return "."
	}
	b := t.w.tag.file.b.Read(0, t.w.tag.file.b.nc())
	spl := strings.SplitN(string(b), " ", 1)[0]
	return filepath.Dir(spl)
}
