package main

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/file"
	"github.com/rjkroege/edwood/frame"
	"github.com/rjkroege/edwood/internal/ui"
	"github.com/rjkroege/edwood/rich"
	"github.com/rjkroege/edwood/util"
)

type Window struct {
	display draw.Display
	lk      sync.Mutex
	ref     Ref
	tag     Text
	body    Text
	r       image.Rectangle

	//	isdir      bool // true if this Window is showing a directory in its body.
	filemenu   bool
	autoindent bool
	showdel    bool

	id    int
	addr  Range
	limit Range

	nopen      [QMAX]byte // number of open Fid for each file in the file server
	nomark     bool
	wrselrange Range
	rdselfd    *os.File // temporary file for rdsel read requests

	col    *Column
	eventx *Xfid
	events []byte

	owner       int // TODO(fhs): change type to rune
	maxlines    int
	dirnames    []string
	widths      []int
	incl        []string
	ctrllock    sync.Mutex // used for lock/unlock ctl mesage
	ctlfid      uint32     // ctl file Fid which has the ctrllock
	dumpstr     string
	dumpdir     string
	utflastqid  int    // Qid of last read request (QWbody or QWtag)
	utflastboff uint64 // Byte offset of last read of body or tag
	utflastq    int    // Rune offset of last read of body or tag

	tagfilenameend     int
	tagfilenamechanged bool
	tagsetting         bool
	tagsafe            bool // What is tagsafe for?
	tagexpand          bool
	taglines           int
	tagtop             image.Rectangle

	editoutlk chan bool

	// Rich-text renderer (used by styled mode). Image cache is
	// shared with styled mode and lazily allocated.
	richBody   *RichText
	imageCache *rich.ImageCache

	// Double-click state for the styled-mode mouse handler:
	// position, timestamp, and which RichText received the
	// last B1 null-click.
	richClickRT   *RichText
	richClickPos  int
	richClickMsec uint32

	spanStore        *SpanStore   // styled text runs (nil when no spans)
	regionStore      *RegionStore // sidecar region tree (nil when no regions); Phase 3 round 5
	styledMode       bool         // true when showing span-styled text via rich.Frame
	styledSuppressed bool         // true when user explicitly chose Plain; suppresses auto-enable

	fontTables map[string]*richFontTable // cached font tables, keyed by font path
}

var (
	_ file.TagStatusObserver = (*Window)(nil) // Enforce at compile time that Window implements BufferObserver
	_ file.BufferObserver    = (*Window)(nil) // Enforce at compile time that TagIndex implements BufferObserver
)

func NewWindow() *Window {
	return &Window{}
}

// Initialize the headless parts of the window.
func (w *Window) initHeadless(clone *Window) *Window {
	w.tag.w = w
	w.taglines = 1
	w.tagsafe = false
	w.tagexpand = true
	w.body.w = w
	w.incl = []string{}
	global.WinID++
	w.id = global.WinID
	w.ref.Inc()
	if global.globalincref {
		w.ref.Inc()
	}

	w.ctlfid = MaxFid
	w.utflastqid = -1

	// Tag setup.
	f := file.MakeObservableEditableBuffer("", nil)

	if clone != nil {
		// TODO(rjk): Support something nicer like initializing from a Reader.
		// (Can refactor ObservableEditableBuffer.Load perhaps.
		clonebuff := make([]rune, clone.tag.Nc())
		clone.tag.file.Read(0, clonebuff)
		f = file.MakeObservableEditableBuffer("", clonebuff)
	}
	f.AddObserver(&w.tag)
	// w observes tag to update the tag index.
	// TODO(rjk): Add the tag index facility.
	f.AddObserver(w)
	w.tag.file = f

	// Body setup.
	f = file.MakeObservableEditableBuffer("", nil)
	if clone != nil {
		f = clone.body.file
		w.body.org = clone.body.org
	}
	f.AddObserver(&w.body)
	w.body.file = f
	w.filemenu = true
	w.autoindent = *globalAutoIndent
	// w observes body to update the tag in response to actions on the body.
	f.AddTagStatusObserver(w)

	if clone != nil {
		w.autoindent = clone.autoindent
	}
	w.editoutlk = make(chan bool, 1)
	return w
}

func (w *Window) Init(clone *Window, r image.Rectangle, dis draw.Display) {
	w.initHeadless(clone)
	w.display = dis
	r1 := r

	w.tagtop = r
	w.tagtop.Max.Y = r.Min.Y + fontget(global.tagfont, w.display).Height()
	r1.Max.Y = r1.Min.Y + w.taglines*fontget(global.tagfont, w.display).Height()

	w.tag.Init(r1, global.tagfont, global.tagcolors, w.display)
	w.tag.what = Tag

	// When cloning, we copy the tag so that the tag contents can evolve
	// independently.
	if clone != nil {
		w.tag.SetSelect(w.tag.Nc(), w.tag.Nc())
	}
	r1 = r
	r1.Min.Y += w.taglines*fontget(global.tagfont, w.display).Height() + 1
	if r1.Max.Y < r1.Min.Y {
		r1.Max.Y = r1.Min.Y
	}

	var rf string
	if clone != nil {
		rf = clone.body.font
	} else {
		rf = global.tagfont
	}
	w.body.Init(r1, rf, global.textcolors, w.display)
	w.body.what = Body
	r1.Min.Y--
	r1.Max.Y = r1.Min.Y + 1
	if w.display != nil {
		w.display.ScreenImage().Draw(r1, global.tagcolors[frame.ColBord], nil, image.Point{})
	}
	w.body.ScrDraw()
	w.r = r
	var br image.Rectangle
	br.Min = w.tag.scrollr.Min
	br.Max.X = br.Min.X + global.button.R().Dx()
	br.Max.Y = br.Min.Y + global.button.R().Dy()
	if w.display != nil {
		w.display.ScreenImage().Draw(br, global.button, nil, global.button.R().Min)
	}
	w.maxlines = w.body.fr.GetFrameFillStatus().Maxlines
	if clone != nil {
		w.body.SetSelect(clone.body.q0, clone.body.q1)
	}
}

// Display returns the window's display as a ui.MouseMover.
// This implements the ui.MouseWindow interface.
func (w *Window) Display() ui.MouseMover {
	return w.display
}

func (w *Window) DrawButton() {
	b := global.button
	if w.body.file.SaveableAndDirty() {
		b = global.modbutton
	}
	var br image.Rectangle

	br.Min = w.tag.scrollr.Min
	br.Max.X = br.Min.X + b.R().Dx()
	br.Max.Y = br.Min.Y + b.R().Dy()
	if w.display != nil {
		w.display.ScreenImage().Draw(br, b, nil, b.R().Min)
	}
}

func (w *Window) delRunePos() int {
	i := w.tagfilenameend + 2
	if i >= w.tag.Nc() {
		return -1
	}
	return i
}

func (w *Window) moveToDel() {
	n := w.delRunePos()
	if n < 0 {
		return
	}
	if w.display != nil {
		w.display.MoveTo(w.tag.fr.Ptofchar(n).Add(image.Pt(4, w.tag.fr.DefaultFontHeight()-4)))
	}
}

// TagLines computes the number of lines in the tag that can fit in r.
func (w *Window) TagLines(r image.Rectangle) int {
	if !w.tagexpand && !w.showdel {
		return 1
	}
	w.showdel = false
	w.tag.Resize(r, true, true /* noredraw */)
	w.tagsafe = false

	if !w.tagexpand {
		// use just as many lines as needed to show the Del
		n := w.delRunePos()
		if n < 0 {
			return 1
		}
		p := w.tag.fr.Ptofchar(n).Sub(w.tag.fr.Rect().Min)
		return 1 + p.Y/w.tag.fr.DefaultFontHeight()
	}

	// can't use more than we have
	if w.tag.fr.GetFrameFillStatus().Nlines >= w.tag.fr.GetFrameFillStatus().Maxlines {
		return w.tag.fr.GetFrameFillStatus().Maxlines
	}

	// if tag ends with \n, include empty line at end for typing
	n := w.tag.fr.GetFrameFillStatus().Nlines
	if w.tag.file.Nr() > 0 {
		c := w.tag.file.ReadC(w.tag.file.Nr() - 1)
		if c == '\n' {
			n++
		}
	}
	if n == 0 {
		n = 1
	}
	return n
}

// Resize the specified Window to rectangle r.
// TODO(rjk): when collapsing the tag, this is called twice. Once would seem
// sufficient.
// TODO(rjk): This function does not appear to update the Window's rect correctly
// in all cases.
func (w *Window) Resize(r image.Rectangle, safe, keepextra bool) int {
	// log.Printf("Window.Resize r=%v safe=%v keepextra=%v\n", r, safe, keepextra)
	// defer log.Println("Window.Resize End\n")

	// TODO(rjk): Do not leak global event state into this function.
	mouseintag := global.mouse.Point.In(w.tag.all)
	mouseinbody := global.mouse.Point.In(w.body.all)

	// Tagtop is a rectangle corresponding to one line of tag.
	w.tagtop = r
	w.tagtop.Max.Y = r.Min.Y + fontget(global.tagfont, w.display).Height()

	r1 := r
	r1.Max.Y = util.Min(r.Max.Y, r1.Min.Y+w.taglines*fontget(global.tagfont, w.display).Height())

	// If needed, recompute number of lines in tag.
	if !safe || !w.tagsafe || !w.tag.all.Eq(r1) {
		w.taglines = w.TagLines(r)
		r1.Max.Y = util.Min(r.Max.Y, r1.Min.Y+w.taglines*fontget(global.tagfont, w.display).Height())
	}

	// Resize/redraw tag TODO(flux)
	y := r1.Max.Y
	if !safe || !w.tagsafe || !w.tag.all.Eq(r1) {
		w.tag.Resize(r1, true, false /* noredraw */)
		y = w.tag.fr.Rect().Max.Y
		w.DrawButton()
		w.tagsafe = true

		// If mouse is in tag, pull up as tag closes.
		if mouseintag && !global.mouse.Point.In(w.tag.all) {
			p := global.mouse.Point
			p.Y = w.tag.all.Max.Y - 3
			if w.display != nil {
				w.display.MoveTo(p)
			}
		}
		// If mouse is in body, push down as tag expands.
		if mouseinbody && global.mouse.Point.In(w.tag.all) {
			p := global.mouse.Point
			p.Y = w.tag.all.Max.Y + 3
			if w.display != nil {
				w.display.MoveTo(p)
			}
		}
	}
	// Redraw body
	r1 = r
	r1.Min.Y = y
	if !safe || !w.body.all.Eq(r1) {
		oy := y
		if y+1+w.body.fr.DefaultFontHeight() <= r.Max.Y { // room for one line
			r1.Min.Y = y
			r1.Max.Y = y + 1
			if w.display != nil {
				w.display.ScreenImage().Draw(r1, global.tagcolors[frame.ColBord], nil, image.Point{})
			}
			y++
			r1.Min.Y = util.Min(y, r.Max.Y)
			r1.Max.Y = r.Max.Y
		} else {
			r1.Min.Y = y
			r1.Max.Y = y
		}
		// Always resize body Text to maintain canonical rectangle
		// Pass noredraw=true if in styled mode (we'll render ourselves)
		y = w.body.Resize(r1, keepextra, w.styledMode /* noredraw */)
		w.r = r
		w.r.Max.Y = y
		w.body.all.Min.Y = oy

		// Render the appropriate view
		if w.styledMode && w.richBody != nil {
			w.richBody.Render(w.body.all)
		} else {
			w.body.ScrDraw()
		}
	}
	w.maxlines = util.Min(w.body.fr.GetFrameFillStatus().Nlines, util.Max(w.maxlines, w.body.fr.GetFrameFillStatus().Maxlines))
	// TODO(rjk): this value doesn't make sense when we've collapsed
	// the tag if the rectangle update block is not executed.
	return w.r.Max.Y
}

// Lock1 locks just this Window. This is a helper for Lock.
// TODO(rjk): This should be an internal detail of Window.
func (w *Window) lock1(owner int) {
	w.lk.Lock()
	w.ref.Inc()
	w.owner = owner
}

// Lock locks every text/clone of w
func (w *Window) Lock(owner int) {
	w.lk.Lock()
	w.ref.Inc()
	w.owner = owner
	f := w.body.file
	f.AllObservers(func(i interface{}) {
		if t, ok := i.(*Text); ok && t.w != w {
			t.w.lock1(owner)
		}
	})
}

// unlock1 unlocks a single window.
func (w *Window) unlock1() {
	w.owner = 0
	w.Close()
	w.lk.Unlock()
}

// Unlock releases the lock on each clone of w
func (w *Window) Unlock() {
	w.body.file.AllObservers(func(i interface{}) {
		if t, ok := i.(*Text); ok && t.w != w {
			t.w.unlock1()
		}
	})
	w.unlock1()
}

func (w *Window) MouseBut() {
	if w.display != nil {
		w.display.MoveTo(w.tag.scrollr.Min.Add(
			image.Pt(w.tag.scrollr.Dx(), fontget(global.tagfont, w.display).Height()).Div(2)))
	}
}

func (w *Window) Close() {
	if w.ref.Dec() == 0 {
		w.styledMode = false
		w.styledSuppressed = false
		w.richBody = nil
		w.fontTables = nil
		xfidlog(w, "del")
		w.tag.file.DelObserver(w)
		w.body.file.DelTagStatusObserver(w)
		w.tag.Close()
		w.body.Close()
		if global.activewin == w {
			global.activewin = nil
		}
	}
}

func (w *Window) Delete() {
	x := w.eventx
	if x != nil {
		w.events = w.events[0:0]
		w.eventx = nil
		x.c <- nil // wake him up
	}
}

func (w *Window) Undo(isundo bool) {
	w.utflastqid = -1
	body := &w.body

	// End any in-progress typing sequence so that the next typed character
	// creates a fresh undo point via Mark(). Without this, eq0 stays set
	// from the previous typing session and Text.Type skips Mark(), leaving
	// seq at the value returned by the undo — which is
	// 0 when all changes have been undone. A subsequent Insert with seq 0
	// triggers FlattenHistory, permanently destroying the undo stack.
	body.eq0 = ^0

	if q0, q1, ok := body.file.Undo(isundo); ok {
		body.q0, body.q1 = q0, q1
	}

	// TODO(rjk): Updates the scrollbar and selection.
	// Be sure not to do this inside of the Undo operation's callbacks.
	body.Show(body.q0, body.q1, true)

	// Undo/Redo bypasses the buffer's Insert/Delete observers, so
	// update the styled view directly.
	if w.IsStyledMode() {
		w.UpdateStyledView()
	}
}

func (w *Window) SetName(name string) {
	t := &w.body
	t.file.SetName(name)
}

func (w *Window) Type(t *Text, r rune) {
	t.Type(r)
}

// TODO(rjk): In the future of File's replacement with undo buffer,
// this method could be renamed to something like "UpdateTag"?
func (w *Window) Commit(t *Text) {
	t.Commit()
	if t.what == Body {
		return
	}
	// TODO(rjk): By virtue of being an observer, we know when this has
	// changed. No need to extract it here unless its changed.
	if w.tagfilenamechanged {
		filename := w.ParseTag()
		if filename != w.body.file.Name() {
			global.seq++
			w.body.file.Mark(global.seq)
			w.SetName(filename)
		}
		w.tagfilenamechanged = false
	}
}

func isDir(r string) (bool, error) {
	f, err := os.Open(r)
	if err != nil {
		return false, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return false, err
	}

	if fi.IsDir() {
		return true, nil
	}

	return false, nil
}

// Should include file lookup be built-in? Or provided by a helper?
// TODO(rjk): This should be provided by an external helper.
func (w *Window) AddIncl(r string) {
	// Tries to open absolute paths, and if fails, tries
	// to use dirname instead.
	d, err := isDir(r)
	if !d {
		if filepath.IsAbs(r) {
			warning(nil, "%s: Not a directory: %v", r, err)
			return
		}
		r = w.body.DirName(r)
		d, err := isDir(r)
		if !d {
			warning(nil, "%s: Not a directory: %v", r, err)
			return
		}
	}
	w.incl = append(w.incl, r)
}

// Clean returns true iff w can be treated as unmodified.
// This will modify the File so that the next call to Clean will return true
// even if this one returned false.
func (w *Window) Clean(conservative bool) bool {
	if w.body.file.IsDirOrScratch() { // don't whine if it's a guide file, error window, etc.
		return true
	}
	if !conservative && w.nopen[QWevent] > 0 {
		return true
	}
	if w.body.file.TreatAsDirty() {
		if w.body.file.Name() != "" {
			warning(nil, "%v modified\n", w.body.file.Name())
		} else {
			if w.body.Nc() < 100 { // don't whine if it's too small
				return true
			}
			warning(nil, "unnamed file modified\n")
		}
		// This toggle permits checking if we can safely destroy the window.
		w.body.file.TreatAsClean()
		return false
	}
	return true
}

// VisibleRange returns the rune range [org, end) currently visible in the
// window body. In styled mode it queries the rich.Frame; otherwise
// it uses the plain text frame.
func (w *Window) VisibleRange() (org, end int) {
	if w.styledMode && w.richBody != nil && w.richBody.Frame() != nil {
		org = w.richBody.Origin()
		lineStarts := w.richBody.Frame().LineStartRunes()
		visLines := w.richBody.Frame().VisibleLines()
		// Find the origin line.
		originLine := 0
		for i, start := range lineStarts {
			if start > org {
				break
			}
			originLine = i
		}
		endLine := originLine + visLines
		if endLine >= len(lineStarts) {
			end = w.body.Nc()
		} else {
			end = lineStarts[endLine]
		}
	} else {
		org = w.body.org
		nchars := w.body.fr.GetFrameFillStatus().Nchars
		end = org + nchars
	}
	return
}

// CtlPrint generates the contents of the fsys's acme/<id>/ctl pseduo-file if fonts is true.
// Otherwise, it emits a portion of the per-window dump file contents.
func (w *Window) CtlPrint(fonts bool) string {
	isdir := 0
	if w.body.file.IsDir() {
		isdir = 1
	}
	dirty := 0
	if w.body.file.Dirty() {
		dirty = 1
	}
	buf := fmt.Sprintf("%11d %11d %11d %11d %11d ", w.id, w.tag.Nc(),
		w.body.Nc(), isdir, dirty)
	if fonts {
		// fsys exposes the actual physical font name.
		buf = fmt.Sprintf("%s%11d %s %11d ", buf, w.body.fr.Rect().Dx(),
			quote(fontget(w.body.font, w.display).Name()), w.body.fr.GetMaxtab())
		// Append viewport range for edcolor and other span-aware clients.
		org, end := w.VisibleRange()
		buf = fmt.Sprintf("%s%11d %11d ", buf, org, end)
	}
	return buf
}

func (w *Window) Eventf(format string, args ...interface{}) {
	var (
		x *Xfid
	)
	if w.nopen[QWevent] == 0 {
		return
	}
	if w.owner == 0 {
		util.AcmeError("no window owner", nil)
	}
	buffy := new(bytes.Buffer)
	fmt.Fprintf(buffy, format, args...)
	b := buffy.Bytes()

	// TODO(rjk): events should be a bytes.Buffer?

	w.events = append(w.events, byte(w.owner))
	w.events = append(w.events, b...)
	x = w.eventx
	if x != nil {
		w.eventx = nil
		x.c <- nil
	}
}

// ClampAddr clamps address range based on the body buffer.
func (w *Window) ClampAddr() {
	if w.addr.q0 < 0 {
		w.addr.q0 = 0
	}
	if w.addr.q1 < 0 {
		w.addr.q1 = 0
	}
	if w.addr.q0 > w.body.Nc() {
		w.addr.q0 = w.body.Nc()
	}
	if w.addr.q1 > w.body.Nc() {
		w.addr.q1 = w.body.Nc()
	}
}

func (w *Window) UpdateTag(newtagstatus file.TagStatus) {
	// log.Printf("Window.UpdateTag, status %+v, %d", newtagstatus, global.seq)
	w.setTag1()
}



// RichBody returns the styled-mode rich-text renderer, or nil if not initialized.
func (w *Window) RichBody() *RichText {
	return w.richBody
}

// Draw renders the window. In styled mode, it renders the richBody;
// otherwise, it uses the normal body rendering.
func (w *Window) Draw() {
	if w.styledMode && w.richBody != nil {
		w.richBody.Render(w.body.all)
	} else {
		// Normal body rendering is handled by the existing Text.Redraw
		// mechanism which is called through Text.Resize and other paths.
		// For explicit Draw() calls, we trigger a redraw of the body frame.
		if w.body.fr != nil {
			enclosing := w.body.fr.Rect()
			if w.display != nil {
				enclosing.Min.X -= w.display.ScaleSize(Scrollwid + Scrollgap)
			}
			w.body.fr.Redraw(enclosing)
		}
	}
}


// drainScrollEvents consumes all mouse events from mc that arrive
// within the given duration, leaving the LATEST event's state in
// mc.Mouse. The outer latch loop then makes a single
// button-held-or-released decision based on the final state.
//
// This replaces the previous "wait once then read one event" pattern
// (previewScrSleep, scrollbarSleep), which dispatched an extra
// auto-repeat scroll for every mouse event observed during the
// debounce window. The OS commonly emits a handful of cursor-jitter
// move events between a button-press and the subsequent release —
// pressing physically wiggles the cursor a pixel or two — and each
// one used to fire the latch's per-iteration dispatch even on a
// single physical click. Draining absorbs the jitter; only the
// final state (release vs still-held) drives the loop.
//
// Returns once the timer fires (regardless of whether any events
// were consumed during the wait).
func drainScrollEvents(mc *draw.Mousectl, d time.Duration) {
	if d <= 0 {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	for {
		select {
		case mc.Mouse = <-mc.C:
			// consumed an event; keep draining until the timer
		case <-timer.C:
			return
		}
	}
}

// hscrollLatch implements acme-style latching for horizontal
// scrollbars on rich.Frame block regions (tables, code blocks,
// images). The cursor is warped to stay within the scrollbar band.
// Used by the styled-mode mouse handler.
func (w *Window) hscrollLatch(rt *RichText, mc *draw.Mousectl, button int, regionIndex int) {
	buttonBit := 1 << uint(button-1)
	frameRect := rt.Frame().Rect()

	// Get the scrollbar rectangle for cursor warping.
	barRect := rt.Frame().HScrollBarRect(regionIndex)
	centerY := (barRect.Min.Y + barRect.Max.Y) / 2

	// Initial scroll action.
	rt.Frame().HScrollClick(button, mc.Mouse.Point, regionIndex)
	w.Draw()
	if w.display != nil {
		w.display.Flush()
	}

	first := true
	for {
		if button == 2 {
			mc.Mouse = <-mc.C
		} else {
			if first {
				if w.display != nil {
					w.display.Flush()
				}
				drainScrollEvents(mc, 200*time.Millisecond)
				first = false
			} else {
				drainScrollEvents(mc, 80*time.Millisecond)
			}
		}

		if mc.Mouse.Buttons&buttonBit == 0 {
			break
		}

		// Clamp X to scrollbar bounds and lock Y to center of scrollbar band.
		mx := mc.Mouse.Point.X
		if mx < barRect.Min.X {
			mx = barRect.Min.X
		}
		if mx >= frameRect.Max.X {
			mx = frameRect.Max.X
		}
		warpPt := image.Pt(mx, centerY)

		// Warp cursor back into scrollbar if it has moved away.
		if !mc.Mouse.Point.Eq(warpPt) {
			if w.display != nil {
				w.display.MoveTo(warpPt)
			}
			mc.Mouse = <-mc.C // absorb synthetic move event
		}

		rt.Frame().HScrollClick(button, warpPt, regionIndex)
		w.Draw()
		if w.display != nil {
			w.display.Flush()
		}
	}

	for mc.Mouse.Buttons != 0 {
		mc.Mouse = <-mc.C
	}
}


// ShowInStyledMode sets the rich text selection and scrolls to make it
// visible. In styled mode, rune positions are 1:1 with the source text
// (no source map needed).
func (w *Window) ShowInStyledMode(q0, q1 int) {
	if !w.styledMode || w.richBody == nil {
		return
	}
	rt := w.richBody
	rt.SetSelection(q0, q1)
	w.scrollRichToMatch(rt, q0)
	w.body.org = rt.Origin() // Keep plain frame origin in sync
	w.Draw()
	if w.display != nil {
		w.display.Flush()
	}
}

// scrollRichToMatch scrolls a rich.Frame so that the match at
// rendStart is visible, placing it roughly 1/4 from the top of the
// frame (matching Acme's Show() scroll behavior). If the match is
// already visible, no scrolling occurs.
func (w *Window) scrollRichToMatch(rt *RichText, rendStart int) {
	fr := rt.Frame()
	if fr == nil {
		return
	}

	// Check if the match is already visible using the actual rune range
	// in the viewport. MaxLines() overestimates for rich text with variable-
	// height content (images, code blocks, paragraph spacing), so instead
	// we use Charofpt at the frame bottom to find the last visible rune.
	frameRect := fr.Rect()
	origin := rt.Origin()
	lastVisible := fr.Charofpt(image.Pt(frameRect.Max.X-1, frameRect.Max.Y-1))
	if rendStart >= origin && rendStart <= lastVisible && lastVisible > origin {
		// Even if the rune position is technically in the viewport,
		// a slide break between origin and match means the match is
		// on a different "page" due to fill spacing. Must scroll.
		if !fr.HasSlideBreakBetween(origin, rendStart) {
			return
		}
	}

	// Need to scroll. Find line-based coordinates for positioning.
	lineStarts := fr.LineStartRunes()
	maxLines := fr.MaxLines()
	if maxLines <= 0 || len(lineStarts) == 0 {
		return
	}

	matchLine := 0
	for i, start := range lineStarts {
		if rendStart >= start {
			matchLine = i
		} else {
			break
		}
	}

	// Back up so the target appears ~3/4 down the window,
	// matching plain-mode Show() behavior (text.go:1373-1378).
	backLines := maxLines / 4
	targetLine := matchLine - backLines
	if targetLine < 0 {
		targetLine = 0
	}
	if targetLine < len(lineStarts) {
		targetOrigin := lineStarts[targetLine]

		// Snap to slide start based on where the MATCH is, not where the
		// backed-up target falls. The backed-up position often lands in the
		// slide break zone or previous slide, causing the snap to miss.
		adjustedOrigin := fr.SnapOriginToSlideStart(rendStart)
		if adjustedOrigin == rendStart {
			// Match isn't in a slide region; use normal positioning.
			adjustedOrigin = targetOrigin
		}

		rt.SetOrigin(adjustedOrigin)
	}
}











// isNearEnd reports whether the scroll origin is close enough to the end of
// the rendered content to be considered "following the tail". This is used
// to auto-scroll win windows when new shell output is appended.
func isNearEnd(origin, contentLen int) bool {
	// Empty or brand-new content — treat as following.
	if contentLen == 0 {
		return true
	}
	// "Near the end" means the content after the origin fits in roughly
	// one screen worth of text. 500 runes is a generous approximation
	// (typical 80-col terminal × 6 lines ≈ 480 chars).
	const tailThreshold = 500
	return contentLen-origin <= tailThreshold
}





// isWordChar returns true if the rune is part of a word (alphanumeric or underscore).





// resolveImagePath resolves an image path relative to the markdown file's directory.
// If the image path is absolute (starts with /), it is returned unchanged.
// Otherwise, it is resolved relative to the directory containing basePath.
func resolveImagePath(basePath, imgPath string) string {
	// Absolute paths are returned as-is
	if filepath.IsAbs(imgPath) {
		return imgPath
	}

	// Get the directory containing the markdown file
	baseDir := filepath.Dir(basePath)

	// Join and clean the path
	resolved := filepath.Join(baseDir, imgPath)
	return filepath.Clean(resolved)
}

// IsStyledMode returns true if the window is in styled rendering mode.
func (w *Window) IsStyledMode() bool {
	return w.styledMode
}

// richFontTable groups a base font with its styling variants (bold, italic,
// bold-italic). Each table is built from a single font path and holds
// everything needed to initialize a rich.Frame.
type richFontTable struct {
	basePath   string    // font path this table was built from
	base       draw.Font // regular/base font
	bold       draw.Font // bold variant (nil if unavailable)
	italic     draw.Font // italic variant (nil if unavailable)
	boldItalic draw.Font // bold+italic variant (nil if unavailable)
}

// buildRichFontTable builds a richFontTable from the given font path.
// Returns nil if the base font cannot be loaded.
func buildRichFontTable(display draw.Display, fontPath string) *richFontTable {
	base := fontget(fontPath, display)
	if base == nil {
		return nil
	}
	return &richFontTable{
		basePath:   fontPath,
		base:       base,
		bold:       tryLoadFontVariant(display, fontPath, "bold"),
		italic:     tryLoadFontVariant(display, fontPath, "italic"),
		boldItalic: tryLoadFontVariant(display, fontPath, "bolditalic"),
	}
}

// getOrBuildFontTable returns a cached richFontTable for the given font path,
// building and caching it on first access.
func (w *Window) getOrBuildFontTable(fontPath string) *richFontTable {
	if w.fontTables == nil {
		w.fontTables = make(map[string]*richFontTable)
	}
	if ft, ok := w.fontTables[fontPath]; ok {
		return ft
	}
	display := w.display
	if display == nil {
		display = global.row.display
	}
	ft := buildRichFontTable(display, fontPath)
	if ft != nil {
		w.fontTables[fontPath] = ft
	}
	return ft
}

// initStyledMode switches the window from plain text to styled rendering mode.
// It initializes a RichText renderer for span-styled content. No-op if already
// in styled mode.
func (w *Window) initStyledMode() {
	if w.styledMode {
		return
	}

	display := w.display
	if display == nil {
		display = global.row.display
	}
	if display == nil {
		return
	}

	fontPath := w.body.font
	if fontPath == "" {
		fontPath = global.tagfont
	}
	ft := w.getOrBuildFontTable(fontPath)
	if ft == nil {
		return
	}
	font := ft.base
	boldFont := ft.bold
	italicFont := ft.italic
	boldItalicFont := ft.boldItalic

	// Scaled fonts for headings. Without these, fontForStyle in
	// rich.Frame falls through to the base font for any Scale > 1,
	// so spans-protocol scale=N directives (Phase 3 round 1) would
	// render at body size despite the StyleAttrs.Scale field being
	// plumbed through styleAttrsToRichStyle correctly.
	h1Font := tryLoadScaledFont(display, fontPath, 2.0)
	h2Font := tryLoadScaledFont(display, fontPath, 1.5)
	h3Font := tryLoadScaledFont(display, fontPath, 1.25)

	// Code font for monospace rendering. Without this,
	// fontForStyle returns the base font for any span with
	// Code=true, so spans-protocol family=code directives
	// (Phase 3 round 2) would render in the proportional body
	// font despite StyleAttrs.Family="code" being plumbed
	// through styleAttrsToRichStyle correctly.
	codeFont := tryLoadCodeFont(display, fontPath)

	rt := NewRichText()

	rtOpts := []RichTextOption{
		WithRichTextBackground(global.textcolors[frame.ColBack]),
		WithRichTextColor(global.textcolors[frame.ColText]),
		WithRichTextSelectionColor(global.textcolors[frame.ColHigh]),
		WithScrollbarColors(
			global.textcolors[frame.ColBord],
			global.textcolors[frame.ColBack]),
		WithRichTextMaxTab(int(global.maxtab)),
		// Default ScrollSnap (SnapTop) is what we want for a
		// freshly-displayed document. The B1 handler switches to
		// SnapBottom when the user starts scrolling. Removing
		// the legacy WithRichTextSnapBottomLine(true) per
		// docs/designs/features/unified-scrollbar.md
		// § "Scroll snap policy (rich mode)".
	}
	if boldFont != nil {
		rtOpts = append(rtOpts, WithRichTextBoldFont(boldFont))
	}
	if italicFont != nil {
		rtOpts = append(rtOpts, WithRichTextItalicFont(italicFont))
	}
	if boldItalicFont != nil {
		rtOpts = append(rtOpts, WithRichTextBoldItalicFont(boldItalicFont))
	}
	if h1Font != nil {
		rtOpts = append(rtOpts, WithRichTextScaledFont(2.0, h1Font))
	}
	if h2Font != nil {
		rtOpts = append(rtOpts, WithRichTextScaledFont(1.5, h2Font))
	}
	if h3Font != nil {
		rtOpts = append(rtOpts, WithRichTextScaledFont(1.25, h3Font))
	}
	if codeFont != nil {
		rtOpts = append(rtOpts, WithRichTextCodeFont(codeFont))
	}

	// Create an image cache for box elements that reference
	// images. Lazy allocation; first styled-mode entry
	// initializes it.
	if w.imageCache == nil {
		w.imageCache = rich.NewImageCache(0)
	}

	// Image-related options (cache + onImageLoaded callback +
	// basePath) are routed through a shared helper so the
	// option list stays in one place.
	rtOpts = w.addImageRichTextOptions(rtOpts, func() bool { return w.styledMode })

	rt.Init(display, font, rtOpts...)

	w.richBody = rt
	w.styledMode = true
	w.styledSuppressed = false
}

// addImageRichTextOptions appends the image-related options
// to the rich-text option list: image cache, async-load
// callback, and base path for relative image resolution. The
// caller passes a closure (`isCurrentMode`) that gates the
// async-load redraw on the current styled-mode flag; when
// the closure returns false or the rich body is gone, the
// redraw is skipped.
func (w *Window) addImageRichTextOptions(rtOpts []RichTextOption, isCurrentMode func() bool) []RichTextOption {
	if w.imageCache != nil {
		rtOpts = append(rtOpts, WithRichTextImageCache(w.imageCache))
	}
	rtOpts = append(rtOpts, WithRichTextOnImageLoaded(func(path string) {
		go func() {
			global.row.lk.Lock()
			defer global.row.lk.Unlock()
			if !isCurrentMode() || w.richBody == nil {
				return
			}
			// Invalidate the layout cache before redrawing.
			// The cached layout was built when this image's
			// ImageData had Width=Height=0 (still loading);
			// the line therefore got no extra height and no
			// ImageBelow ghost line was inserted. A bare
			// Render reuses the cached layout, so the now-
			// loaded image never paints. Flipping
			// layoutDirty makes the next paint run a fresh
			// layout pass that picks up the loaded
			// dimensions and inserts the ghost.
			w.richBody.Frame().InvalidateLayout()
			w.richBody.Render(w.body.all)
			if w.display != nil {
				w.display.Flush()
			}
		}()
	}))
	// Surface image-load failures to +Errors instead of inserting
	// the error text into the rendered buffer (which would shift
	// every subsequent caret position by the suffix length and
	// break source-map mapping).
	rtOpts = append(rtOpts, WithRichTextOnImageError(func(path, msg string) {
		warning(nil, "image %s: %s\n", path, msg)
	}))
	name := w.body.file.Name()
	basePath := name
	if !filepath.IsAbs(basePath) {
		if abs, err := filepath.Abs(basePath); err == nil {
			basePath = abs
		}
	}
	rtOpts = append(rtOpts, WithRichTextBasePath(basePath))
	return rtOpts
}

// renderStyledFromBody rebuilds the styled content from the
// span/region stores and pushes it to richBody, syncing the
// scroll origin and dot/selection from the body so a
// freshly-initialized richBody picks up the user's cursor
// position. Called from xfidspanswrite. No-op if richBody is
// nil. Phase 3 round 7 — extracted from xfidspanswrite to fix
// the cursor-resets-to-#0 bug.
func (w *Window) renderStyledFromBody() {
	if !w.styledMode || w.richBody == nil {
		return
	}
	content := w.buildStyledContent()
	w.richBody.SetContent(content)
	w.richBody.SetOrigin(w.body.org)
	w.richBody.SetSelection(w.body.q0, w.body.q1)
	w.richBody.Render(w.body.all)
	if w.display != nil {
		w.display.Flush()
	}
}

// applyParsedSpans applies a successfully-parsed span/region
// write to the window's spanStore and regionStore. Called
// from xfidspanswrite after parseSpanMessage returns.
//
// The spanStore is created lazily and seeded with a default
// run covering the buffer; subsequent writes apply via
// RegionUpdate. The regionStore is created lazily on the
// first write that contains regions; existing regions are
// preserved across writes (the protocol's `c` directive is
// the explicit reset). Phase 3 round 5.
func (w *Window) applyParsedSpans(regionStart int, runs []StyleRun, regions []*Region, bufLen int) {
	if w.spanStore == nil {
		w.spanStore = NewSpanStore()
		w.spanStore.Insert(0, bufLen)
	} else if w.spanStore.TotalLen() != bufLen {
		w.spanStore.Clear()
		w.spanStore.Insert(0, bufLen)
	}
	w.spanStore.RegionUpdate(regionStart, runs)

	if len(regions) > 0 {
		if w.regionStore == nil {
			w.regionStore = NewRegionStore()
		}
		for _, r := range regions {
			if err := tryAddRegion(w.regionStore, r); err != nil {
				// A producer bug landed a partially-overlapping
				// region. Log to +Errors and skip it so the
				// editor stays usable; the next re-render from
				// the producer typically clears and replaces
				// state, recovering on its own.
				warning(nil, "spans: %v\n", err)
			}
		}
	}
}

// tryAddRegion calls regionStore.Add and converts any panic
// (the only error condition; partial overlap with an existing
// region) into a returned error. Used by applyParsedSpans so
// that a producer bug doesn't take down the whole session.
func tryAddRegion(s *RegionStore, r *Region) (err error) {
	defer func() {
		if x := recover(); x != nil {
			err = fmt.Errorf("%v", x)
		}
	}()
	s.Add(r)
	return nil
}

// clearSpansAndRegions empties both the spanStore and the
// regionStore. Called from xfidspanswrite on the protocol's
// `c` (clear) directive — `c` is a full reset of the
// window's styling state including any begin/end region
// directives previously written. Phase 3 round 5.
func (w *Window) clearSpansAndRegions() {
	if w.spanStore != nil {
		w.spanStore.Clear()
	}
	if w.regionStore != nil {
		w.regionStore.Clear()
	}
}

// exitStyledMode switches the window from styled rendering back to plain mode.
// No-op if not in styled mode.
func (w *Window) exitStyledMode() {
	if !w.styledMode {
		return
	}

	// Sync scroll position from rich frame back to the plain body
	// so the plain frame shows the same region of text.
	if w.richBody != nil {
		w.body.org = w.richBody.Origin()
	}

	w.styledMode = false
	w.styledSuppressed = true
	w.richBody = nil

	if w.display != nil {
		w.body.Resize(w.body.all, true, false)
		w.body.ScrDraw()
		w.display.Flush()
	}
}

// rebuildStyledFont tears down and rebuilds richBody with the current
// w.body.font, preserving scroll position and content. Called when the
// user changes the font while in styled mode.
func (w *Window) rebuildStyledFont() {
	if !w.styledMode || w.richBody == nil {
		return
	}

	// Save scroll position.
	savedOrigin := w.richBody.Origin()
	savedYOffset := w.richBody.GetOriginYOffset()

	// Tear down.
	w.styledMode = false
	w.richBody = nil

	// Rebuild with current w.body.font.
	w.initStyledMode()

	if w.styledMode && w.richBody != nil {
		// Rebuild content and restore scroll.
		content := w.buildStyledContent()
		w.richBody.SetContent(content)
		w.richBody.SetOrigin(savedOrigin)
		w.richBody.SetOriginYOffset(savedYOffset)
		w.richBody.Render(w.body.all)
		if w.display != nil {
			w.display.Flush()
		}
	}
}


// UpdateStyledView rebuilds and re-renders the styled content.
// Called after editing operations that modify the body buffer while
// in styled mode.
func (w *Window) UpdateStyledView() {
	if !w.styledMode || w.richBody == nil || w.spanStore == nil {
		return
	}
	content := w.buildStyledContent()
	w.richBody.SetContent(content)
	w.richBody.Render(w.body.all)
	if w.display != nil {
		w.display.Flush()
	}
}

// buildStyledContent builds rich.Content from the body text and span store.
// If a regionStore is present (Phase 3 round 5+), each rich.Span gains
// per-rune Style flags derived from its enclosing region: a `code`
// region adds Block + Code + Bg(InlineCodeBg); a `blockquote` region
// adds Blockquote + BlockquoteDepth (counted from ancestors). These
// drive the existing rich.Frame block-element layout (gutter indent,
// full-line bg, monospace font, blockquote bar via the wrapper).
//
// Round 6 addition: the bridge SPLITS each spanStore run at any
// region-boundary offset within its range. Without splitting, a
// default-styled run that crosses a blockquote boundary would have
// blockquote flags applied based only on the run's start offset
// (either all or nothing); with splitting, the inside-region
// portion carries the flags and the outside portion doesn't.
// md2spans's `code` regions don't trigger this because the
// inside-region runs differ in style (family=code) and the
// spanStore preserves the boundary; default-styled blockquote
// regions DO need the split.
func (w *Window) buildStyledContent() rich.Content {
	if w.spanStore == nil || w.spanStore.TotalLen() == 0 {
		return rich.Plain(w.body.file.String())
	}

	var content []rich.Span
	offset := 0
	w.spanStore.ForEachRun(func(run StyleRun) {
		if run.Len == 0 {
			return
		}
		runEnd := offset + run.Len
		// Compute split points within this run from region
		// boundaries. If no regionStore, no splits.
		splits := []int{offset}
		if w.regionStore != nil {
			splits = append(splits, w.regionStore.BoundariesIn(offset, runEnd)...)
		}
		splits = append(splits, runEnd)
		for i := 0; i < len(splits)-1; i++ {
			subStart, subEnd := splits[i], splits[i+1]
			content = append(content, w.styleSubRun(run, subStart, subEnd))
		}
		offset = runEnd
	})
	return rich.Content(content)
}

// styleSubRun produces one rich.Span for the [subStart, subEnd)
// portion of a parent StyleRun. The base style comes from the
// run; region flags come from the regionStore's deepest enclosing
// region at subStart. Phase 3 round 6 (split-at-boundaries).
func (w *Window) styleSubRun(run StyleRun, subStart, subEnd int) rich.Span {
	buf := make([]rune, subEnd-subStart)
	w.body.file.Read(subStart, buf)
	text := string(buf)
	var style rich.Style
	if run.Style.IsBox {
		style = boxStyleToRichStyle(run.Style, text)
	} else {
		style = styleAttrsToRichStyle(run.Style)
	}
	if w.regionStore != nil {
		applyEnclosingRegions(&style, w.regionStore.EnclosingAt(subStart))
	}
	return rich.Span{Text: text, Style: style}
}

// applyEnclosingRegions walks from the outermost ancestor
// down to the deepest region, OR-ing each region's
// kind-specific flags into s. Order matters: the deepest
// region applies LAST so that for fields where two kinds
// could conflict (e.g., shared Bg), the innermost kind
// wins. Round 5 added the idempotent "code" kind; round 6
// added "blockquote" with a depth counter, where the
// outermost-first walk order is load-bearing (each ancestor
// independently bumps the depth).
//
// Run-alignment note: callers must invoke this with a run
// whose [start, end) does NOT cross any region boundary —
// the function's per-run flag application is uniform over
// the run. buildStyledContent satisfies this by splitting
// each spanStore run at region boundaries via
// RegionStore.BoundariesIn before calling here. (Earlier
// rounds asked producers to emit boundary-aligned runs;
// round 6 moved that responsibility to the bridge so
// default-styled runs that span a blockquote boundary still
// render correctly.)
func applyEnclosingRegions(s *rich.Style, deepest *Region) {
	if deepest == nil {
		return
	}
	// Walk outermost-first so deeper kinds layer over outer
	// ones (last-write-wins on shared fields). Per-kind
	// composition rules live in the apply* functions; the
	// dispatch here is shape-only.
	for _, r := range ancestorsOuterFirst(deepest) {
		switch r.Kind {
		case "code":
			applyCodeRegion(s, r)
		case "blockquote":
			applyBlockquoteRegion(s, r)
		case "listitem":
			applyListitemRegion(s, r)
		case "table":
			applyTableRegion(s, r)
		case "tablerow":
			applyTableRowRegion(s, r)
		case "tablecell":
			applyTableCellRegion(s, r)
		}
	}
}

// ancestorsOuterFirst returns the chain from outermost to
// the supplied deepest region, inclusive on both ends.
// Extracted from applyEnclosingRegions in round 6.5 so
// future per-kind apply functions can walk the chain
// directly (e.g., round 7's listitem will need the
// nearest-of-kind ancestor, not every ancestor).
func ancestorsOuterFirst(deepest *Region) []*Region {
	var inner []*Region
	for r := deepest; r != nil; r = r.Parent {
		inner = append(inner, r)
	}
	out := make([]*Region, len(inner))
	for i, r := range inner {
		out[len(inner)-1-i] = r
	}
	return out
}

// applyCodeRegion sets the per-rune flags for a `code`
// region ancestor. Composition rule: idempotent — multiple
// code ancestors produce the same result as one. v1
// disallows code-inside-code via the protocol's begin/end
// nesting rules, but the apply remains idempotent for
// safety. Phase 3 round 5.
func applyCodeRegion(s *rich.Style, _ *Region) {
	s.Block = true
	s.Code = true
	s.Bg = rich.InlineCodeBg
}

// applyBlockquoteRegion sets the per-rune flags for a
// `blockquote` region ancestor. Composition rule: additive
// — each blockquote ancestor in the chain bumps the depth
// counter by one, so nested `>>` produces depth=2, etc.
// The outermost-first walk order in applyEnclosingRegions
// makes this compose naturally. Phase 3 round 6.
func applyBlockquoteRegion(s *rich.Style, _ *Region) {
	s.Blockquote = true
	s.BlockquoteDepth++
}

// applyListitemRegion sets the per-rune flags for a
// `listitem` region ancestor. Composition rule: additive
// for ListIndent (one bump per ancestor; v1 emits a single
// listitem ancestor per rune, so depth is always 1, but
// the mechanism is ready for round 7.x's nesting). For
// per-instance payload (marker/number), per-call overwrite
// in the outermost-first walk gives nearest-of-kind
// semantics — the innermost listitem's marker/number wins
// when ancestors disagree. Phase 3 round 7.
func applyListitemRegion(s *rich.Style, r *Region) {
	s.ListItem = true
	s.ListIndent++
	if number, ok := r.Params["number"]; ok && number != "" {
		s.ListOrdered = true
		s.ListNumber = parseListNumber(number)
		return
	}
	// Unordered (marker= present, or neither — defensive).
	s.ListOrdered = false
	s.ListNumber = 0
}

// parseListNumber converts a `number=N` param value to an
// int. Returns 0 on parse error (the protocol parser
// rejects malformed numbers upstream, but be defensive at
// the bridge boundary).
func parseListNumber(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// applyTableRegion sets the per-rune flags for a `table`
// region ancestor. Composition rule: idempotent — the
// outermost table is the only one that matters (v1
// disallows table-in-table). The full set:
//
//   - Table: triggers the existing rich.Frame BlockTable
//     handling (gutter indent + block-level layout).
//   - Block: redundant for layout purposes (Table alone
//     triggers the same gutterIndent path) but kept for
//     consistency with code-block flagging and any
//     downstream consumers checking the Block boolean.
//   - Code: forces monospace font selection for EVERY
//     rune in the table — including the `|` markers
//     between cells, not just the cell content.
//     Without Code on the markers, columns would render
//     in proportional font and `|` wouldn't align even
//     when cell content does.
//
// Phase 3 round 8.
func applyTableRegion(s *rich.Style, _ *Region) {
	s.Table = true
	s.Block = true
	s.Code = true
}

// applyTableRowRegion sets the per-rune flags for a
// `tablerow` region ancestor. Composition rule:
// idempotent (no shared field with table). The
// `header=true` param promotes the row's runes to
// `Style.TableHeader=true`, which the existing layout
// uses to render bold + a separator line. Phase 3
// round 8.
func applyTableRowRegion(s *rich.Style, r *Region) {
	if r.Params["header"] == "true" {
		s.TableHeader = true
	}
}

// applyTableCellRegion sets the per-rune flags for a
// `tablecell` region ancestor. The `align=` param maps
// to `Style.TableAlign` (left, right, center; left is
// the default and matches the rich.AlignLeft zero
// value). Composition rule: nearest-of-kind — cells
// don't nest in v1, so per-call overwrite is moot, but
// the pattern is consistent with listitem's marker/
// number payload. Phase 3 round 8.
func applyTableCellRegion(s *rich.Style, r *Region) {
	switch r.Params["align"] {
	case "right":
		s.TableAlign = rich.AlignRight
	case "center":
		s.TableAlign = rich.AlignCenter
	default: // "left", "", or unrecognized
		s.TableAlign = rich.AlignLeft
	}
}

// styleAttrsToRichStyle maps StyleAttrs (from span protocol) to rich.Style (for rendering).
//
// Scale: StyleAttrs.Scale==0 is the "unset" sentinel and maps
// to rich.Style.Scale=1.0 (body baseline). A positive Scale is
// passed through directly. Per the spans-protocol round 1 design,
// the parser rejects negative / zero / non-finite Scale values,
// so this branch never sees them.
//
// Family: "code" maps to rich.Style.Code=true (rich.Frame's
// fontForStyle returns the registered codeFont). Empty Family
// leaves Code=false. Other values are no-ops here — the parser
// rejects unknown family names upstream, so this branch never
// sees them in production; the defensive ignore prevents a
// stale span store from breaking the rendering.
//
// HRule: passes through directly. The renderer keeps the span's
// text visible (source markers `---`/`***`/`___` render
// normally) and rich/mdrender's paintHorizontalRules draws a 1px
// line across the frame on the same row. Added in Phase 3 round
// 3; the original "suppress text" behavior was reverted in the
// round-3 follow-up.
func styleAttrsToRichStyle(sa StyleAttrs) rich.Style {
	s := rich.Style{
		Scale: 1.0,
	}
	if sa.Scale > 0 {
		s.Scale = sa.Scale
	}
	s.Fg = sa.Fg
	s.Bg = sa.Bg
	s.Bold = sa.Bold
	s.Italic = sa.Italic
	if sa.Family == "code" {
		s.Code = true
	}
	s.HRule = sa.HRule
	return s
}

// boxStyleToRichStyle maps a box StyleAttrs (IsBox=true) to rich.Style for rendering.
// Boxes without an image: payload are fixed-dimension colored rectangles.
// Boxes with an image: payload also enter the image rendering pipeline.
func boxStyleToRichStyle(sa StyleAttrs, altText string) rich.Style {
	s := rich.Style{
		Scale:       1.0,
		FixedBox:    true,
		ImageWidth:  sa.BoxWidth,
		ImageHeight: sa.BoxHeight,
		ImageAlt:    altText,
	}
	if sa.Scale > 0 {
		s.Scale = sa.Scale
	}
	s.Fg = sa.Fg
	s.Bg = sa.Bg
	s.Bold = sa.Bold
	s.Italic = sa.Italic
	if sa.Family == "code" {
		s.Code = true
	}
	s.HRule = sa.HRule
	// BoxPlacement="below" → render image anchored to the
	// line, painted below the line text (Phase 3 round 4).
	// "" and "replace" both denote the existing replacing
	// semantic.
	if sa.BoxPlacement == "below" {
		s.ImageBelow = true
	}

	// Parse payload. v1 convention: first token is `image:URL`;
	// subsequent space-separated tokens are key=value params
	// interpreted by the consumer (this function), not by the
	// wire-format parser. Unknown params are silently ignored
	// for forward-compat. Phase 3 round 4.
	applyImagePayload(&s, sa.BoxPayload)

	return s
}

// applyImagePayload parses a box's payload string and applies
// the recognized parts to the rich.Style:
//   - First token `image:URL` enables image rendering and sets
//     ImageURL to URL (without the `image:` prefix).
//   - Subsequent `key=value` tokens are recognized for v1's
//     small set:
//   - `width=N` sets ImageWidth to N (overrides any prior
//     value, including a wire-format BoxWidth).
//
// Anything else (unknown prefix on the first token, unknown
// param names, malformed values like `width=abc`) is silently
// ignored. Phase 3 round 4.
func applyImagePayload(s *rich.Style, payload string) {
	if payload == "" {
		return
	}
	tokens := strings.Fields(payload)
	if len(tokens) == 0 {
		return
	}
	first := tokens[0]
	if !strings.HasPrefix(first, "image:") {
		return
	}
	s.Image = true
	s.ImageURL = strings.TrimPrefix(first, "image:")
	for _, tok := range tokens[1:] {
		eq := strings.IndexByte(tok, '=')
		if eq <= 0 {
			continue
		}
		key, val := tok[:eq], tok[eq+1:]
		switch key {
		case "width":
			n, err := strconv.Atoi(val)
			if err == nil && n > 0 {
				s.ImageWidth = n
			}
		}
	}
}

// HandleStyledMouse handles mouse events when the window is in styled rendering
// mode. Returns true if the event was handled, false otherwise. Rune
// positions in rich.Frame are 1:1 with the body buffer (no source-map
// indirection), so click/selection translation is the identity.
func (w *Window) HandleStyledMouse(m *draw.Mouse, mc *draw.Mousectl) bool {
	if !w.styledMode || w.richBody == nil {
		return false
	}
	if !m.Point.In(w.body.all) {
		return false
	}

	rt := w.richBody

	// Scroll wheel (buttons 4 and 5). When the cursor is over a
	// horizontally-scrollable block region (table, code block,
	// inline-replacing image), redirect vertical scroll to
	// horizontal scrolling.
	if m.Buttons&8 != 0 || m.Buttons&16 != 0 {
		if regionIndex, ok := rt.Frame().PointInBlockRegion(m.Point); ok {
			delta := 40 // pixels per scroll tick
			if m.Buttons&8 != 0 {
				delta = -delta
			}
			rt.Frame().HScrollWheel(delta, regionIndex)
		} else {
			up := m.Buttons&8 != 0
			rt.ScrollWheel(up)
		}
		w.Draw()
		if w.display != nil {
			w.display.Flush()
		}
		return true
	}

	// Scrollbar clicks (buttons 1, 2, 3 in scrollbar area).
	scrRect := rt.ScrollRect()
	if m.Point.In(scrRect) {
		button := 0
		if m.Buttons&1 != 0 {
			button = 1
		} else if m.Buttons&2 != 0 {
			button = 2
		} else if m.Buttons&4 != 0 {
			button = 3
		}
		if button != 0 && mc != nil && rt.scrollbar != nil {
			rt.scrollbar.HandleClick(button)
			w.Draw()
			if w.display != nil {
				w.display.Flush()
			}
			return true
		}
	}

	// Horizontal scrollbar clicks for overflowing block regions
	// (tables, code blocks, wide images).
	if regionIndex, ok := rt.Frame().HScrollBarAt(m.Point); ok {
		button := 0
		if m.Buttons&1 != 0 {
			button = 1
		} else if m.Buttons&2 != 0 {
			button = 2
		} else if m.Buttons&4 != 0 {
			button = 3
		}
		if button != 0 && mc != nil {
			w.hscrollLatch(rt, mc, button, regionIndex)
			return true
		}
	}

	frameRect := rt.Frame().Rect()

	// B1: selection with chording.
	if m.Point.In(frameRect) && m.Buttons&1 != 0 && mc != nil {
		var p0, p1 int
		var lastButtons int

		selectq := rt.Frame().Charofpt(m.Point)
		b := m.Buttons
		fr := rt.Frame()

		// Double-click detection.
		prevP0, prevP1 := rt.Selection()
		if w.richClickRT == rt &&
			m.Msec-w.richClickMsec < 500 &&
			prevP0 == prevP1 && selectq == prevP0 {

			p0, p1 = fr.ExpandAtPos(selectq)
			rt.SetSelection(p0, p1)
			fr.Redraw()
			if w.display != nil {
				w.display.Flush()
			}
			w.richClickRT = nil

			x, y := m.Point.X, m.Point.Y
			for {
				me := <-mc.C
				lastButtons = me.Buttons
				if !(me.Buttons == b &&
					util.Abs(me.Point.X-x) < 3 &&
					util.Abs(me.Point.Y-y) < 3) {
					break
				}
			}
		} else {
			// Normal click/drag selection.
			anchor := selectq
			for {
				me := <-mc.C
				lastButtons = me.Buttons
				current := fr.Charofpt(me.Point)
				if anchor <= current {
					p0, p1 = anchor, current
				} else {
					p0, p1 = current, anchor
				}
				rt.SetSelection(p0, p1)
				fr.Redraw()
				if w.display != nil {
					w.display.Flush()
				}
				if me.Buttons != b || me.Buttons == 0 {
					break
				}
			}

			if p0 == p1 {
				w.richClickRT = rt
				w.richClickPos = p0
				w.richClickMsec = m.Msec
			} else {
				w.richClickRT = nil
			}
		}

		// Sync selection to body (identity map).
		w.body.q0 = p0
		w.body.q1 = p1
		q0 := p0

		// Chord processing loop.
		const (
			chordNone = iota
			chordCut
			chordPaste
			chordSnarf
		)
		state := chordNone
		for lastButtons != 0 {
			if lastButtons == 7 && state == chordNone {
				cut(&w.body, &w.body, nil, true, false, "")
				state = chordSnarf
			} else if (lastButtons&1) != 0 && (lastButtons&6) != 0 && state != chordSnarf {
				if state == chordNone {
					w.body.TypeCommit()
					global.seq++
					w.body.file.Mark(global.seq)
				}
				if lastButtons&2 != 0 {
					if state == chordPaste {
						w.Undo(true)
						w.body.SetSelect(q0, w.body.q1)
						state = chordNone
					} else if state != chordCut {
						cut(&w.body, &w.body, nil, true, true, "")
						state = chordCut
					}
				} else {
					if state == chordCut {
						w.Undo(true)
						w.body.SetSelect(q0, w.body.q1)
						state = chordNone
					} else if state != chordPaste {
						paste(&w.body, &w.body, nil, true, false, "")
						state = chordPaste
					}
				}
				w.UpdateStyledView()
				rt.SetSelection(w.body.q0, w.body.q1)
				clearmouse()
			}
			prev := lastButtons
			for lastButtons == prev {
				me := <-mc.C
				lastButtons = me.Buttons
			}
			w.richClickRT = nil
		}

		w.Draw()
		if w.display != nil {
			w.display.Flush()
		}
		return true
	}

	// B2: execute.
	if m.Point.In(frameRect) && m.Buttons&2 != 0 {
		priorP0, priorP1 := rt.Selection()

		var p0, p1 int
		if mc != nil {
			p0, p1 = rt.Frame().SelectWithColor(mc, m, global.but2col)
			rt.SetSelection(p0, p1)
		} else {
			charPos := rt.Frame().Charofpt(m.Point)
			p0, p1 = charPos, charPos
			rt.SetSelection(charPos, charPos)
		}
		if p0 == p1 {
			q0, q1 := rt.Frame().ExpandAtPos(p0)
			if q0 != q1 {
				rt.SetSelection(q0, q1)
				p0, p1 = q0, q1
			}
		}
		// Sync to body and execute.
		w.body.q0 = p0
		w.body.q1 = p1
		w.Draw()
		if w.display != nil {
			w.display.Flush()
		}

		// Get text from rich.Frame selection.
		var cmdText string
		if p0 != p1 {
			buf := make([]rune, p1-p0)
			w.body.file.Read(p0, buf)
			cmdText = string(buf)
		}
		if cmdText != "" {
			richExecute(&w.body, cmdText)
		}

		rt.SetSelection(priorP0, priorP1)
		w.Draw()
		if w.display != nil {
			w.display.Flush()
		}
		return true
	}

	// B3: look.
	if m.Point.In(frameRect) && m.Buttons&4 != 0 {
		var p0, p1 int
		if mc != nil {
			p0, p1 = rt.Frame().SelectWithColor(mc, m, global.but3col)
		} else {
			charPos := rt.Frame().Charofpt(m.Point)
			p0, p1 = charPos, charPos
		}

		// Sync click position to body so expand()'s inSelection()
		// and search()'s start position work correctly.
		w.body.q0 = p0
		w.body.q1 = p1

		// Delegate to full look3 handler: expand, file open,
		// address eval, plumbing, search fallback.
		look3(&w.body, p0, p1, false)

		// Sync rich text selection from whatever look3 set.
		rt.SetSelection(w.body.q0, w.body.q1)
		w.Draw()
		if w.display != nil {
			w.display.Flush()
		}
		return true
	}

	return false
}
