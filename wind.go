package main

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"9fans.net/go/plumb"
	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/file"
	"github.com/rjkroege/edwood/frame"
	"github.com/rjkroege/edwood/internal/ui"
	"github.com/rjkroege/edwood/markdown"
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

	// Preview mode fields for rich text rendering
	previewMode        bool                // true when showing rendered markdown preview
	richBody           *RichText           // rich text renderer for preview mode
	previewSourceMap   *markdown.SourceMap // maps rendered positions to source positions
	previewLinkMap     *markdown.LinkMap   // maps rendered positions to link URLs
	imageCache *rich.ImageCache // cache for loaded images in preview mode
	selectionContext   *SelectionContext   // context metadata for the current preview selection

	// Preview double-click state (mirrors clicktext/clickmsec in text.go)
	previewClickPos  int       // rune position of last B1 null-click
	previewClickMsec uint32    // timestamp of last B1 null-click
	previewClickRT   *RichText // which richtext received the last click

	// Incremental preview update state
	prevBlockIndex *markdown.BlockIndex  // block boundaries from last parse
	pendingEdits   []markdown.EditRecord // edits since last UpdatePreview

	// Preview render debounce state
	previewLastRender    time.Time   // timestamp of last preview render
	previewRenderPending bool        // a render was skipped due to throttle
	previewDebounceTimer *time.Timer // trailing-edge timer for deferred render

	spanStore        *SpanStore // styled text runs (nil when no spans)
	styledMode       bool       // true when showing span-styled text via rich.Frame
	styledSuppressed bool       // true when user explicitly chose Plain; suppresses auto-enable

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
	w.body.ScrDraw(w.body.fr.GetFrameFillStatus().Nchars)
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
		// Pass noredraw=true if in preview or styled mode (we'll render ourselves)
		y = w.body.Resize(r1, keepextra, w.previewMode || w.styledMode /* noredraw */)
		w.r = r
		w.r.Max.Y = y
		w.body.all.Min.Y = oy

		// Render the appropriate view
		if (w.previewMode || w.styledMode) && w.richBody != nil {
			w.richBody.Render(w.body.all)
		} else {
			w.body.ScrDraw(w.body.fr.GetFrameFillStatus().Nchars)
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
		w.cancelPreviewDebounce()
		w.previewRenderPending = false
		w.previewMode = false
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
	// from the previous typing session and HandlePreviewType (and Text.Type)
	// skips Mark(), leaving seq at the value returned by the undo — which is
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
	// update the preview directly.
	if w.IsPreviewMode() {
		w.UpdatePreview()
	}
	if w.IsStyledMode() {
		w.UpdateStyledView()
	}
}

func (w *Window) SetName(name string) {
	t := &w.body
	t.file.SetName(name)
}

func (w *Window) Type(t *Text, r rune) {
	// In preview mode, route body key events through HandlePreviewKey
	if t.what == Body && w.IsPreviewMode() {
		if w.HandlePreviewKey(r) {
			return
		}
		w.HandlePreviewType(t, r)
		return
	}
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
// window body. In styled/preview mode it queries the rich.Frame; otherwise
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

// SelectionContentType identifies the markdown formatting type of a selection.
type SelectionContentType int

const (
	ContentPlain      SelectionContentType = iota // Plain unformatted text
	ContentHeading                                // Heading (# ... ######)
	ContentBold                                   // Bold (**text**)
	ContentItalic                                 // Italic (*text*)
	ContentBoldItalic                             // Bold+Italic (***text***)
	ContentCode                                   // Inline code (`text`)
	ContentCodeBlock                              // Fenced code block (```...```)
	ContentLink                                   // Link ([text](url))
	ContentImage                                  // Image (![alt](url))
	ContentMixed                                  // Selection spans multiple formatting types
)

// SelectionContext holds metadata about the current selection in preview mode,
// including source and rendered positions, content type, and formatting style.
// This is used for context-aware paste operations that adapt formatting based
// on the source and destination context.
type SelectionContext struct {
	SourceStart   int                  // Start offset in source markdown text
	SourceEnd     int                  // End offset in source markdown text
	RenderedStart int                  // Start offset in rendered text
	RenderedEnd   int                  // End offset in rendered text
	ContentType   SelectionContentType // Type of content selected
	PrimaryStyle  rich.Style           // Dominant style of the selection
	CodeLanguage  string               // Language tag for code blocks (e.g., "go")

	IncludesOpenMarker  bool // Selection includes the opening formatting marker
	IncludesCloseMarker bool // Selection includes the closing formatting marker
}

// classifyStyle maps a rich.Style to its SelectionContentType.
func classifyStyle(s rich.Style) SelectionContentType {
	switch {
	case s.Image:
		return ContentImage
	case s.Link:
		return ContentLink
	case s.Code && s.Block:
		return ContentCodeBlock
	case s.Code:
		return ContentCode
	case s.Bold && s.Scale > 1.0:
		return ContentHeading
	case s.Bold && s.Italic:
		return ContentBoldItalic
	case s.Bold:
		return ContentBold
	case s.Italic:
		return ContentItalic
	default:
		return ContentPlain
	}
}

// analyzeSelectionContent examines the spans in the rendered RichText content
// within the given rendered-position range [rStart, rEnd) and determines the
// SelectionContentType. This is used during selection context updates to
// classify what kind of markdown content the user has selected.
func (w *Window) analyzeSelectionContent(rStart, rEnd int) SelectionContentType {
	if w.richBody == nil || rStart >= rEnd {
		return ContentPlain
	}

	content := w.richBody.Content()
	if len(content) == 0 {
		return ContentPlain
	}

	var foundType SelectionContentType
	found := false
	pos := 0

	for _, span := range content {
		runeLen := len([]rune(span.Text))
		spanEnd := pos + runeLen

		// Check if this span overlaps [rStart, rEnd)
		if spanEnd > rStart && pos < rEnd {
			ct := classifyStyle(span.Style)
			if !found {
				foundType = ct
				found = true
			} else if ct != foundType {
				return ContentMixed
			}
		}

		pos = spanEnd
		if pos >= rEnd {
			break
		}
	}

	if !found {
		return ContentPlain
	}
	return foundType
}

// updateSelectionContext reads the current selection from richBody, translates
// the rendered positions to source positions via the previewSourceMap, analyzes
// the content type, and stores the result in w.selectionContext. This should be
// called after each selection change in preview mode.
func (w *Window) updateSelectionContext() {
	if !w.previewMode || w.richBody == nil || w.previewSourceMap == nil {
		w.selectionContext = nil
		return
	}

	p0, p1 := w.richBody.Selection()
	contentType := w.analyzeSelectionContent(p0, p1)

	// Translate rendered positions to source positions.
	srcStart, srcEnd := w.previewSourceMap.ToSource(p0, p1)

	// Determine the primary style from the first overlapping span.
	var primaryStyle rich.Style
	content := w.richBody.Content()
	pos := 0
	for _, span := range content {
		runeLen := len([]rune(span.Text))
		spanEnd := pos + runeLen
		if spanEnd > p0 && pos < p1 {
			primaryStyle = span.Style
			break
		}
		pos = spanEnd
	}

	w.selectionContext = &SelectionContext{
		SourceStart:   srcStart,
		SourceEnd:     srcEnd,
		RenderedStart: p0,
		RenderedEnd:   p1,
		ContentType:   contentType,
		PrimaryStyle:  primaryStyle,
	}
}

// transformForPaste adapts the pasted text based on source and destination
// context. It applies formatting rules: re-wraps formatted text for plain
// destinations, strips markers when destination already has the same format,
// and handles structural elements (headings, code blocks) based on whether
// the text includes a trailing newline.
func transformForPaste(text []byte, sourceCtx, destCtx *SelectionContext) []byte {
	// Pass through when context is missing or text is empty.
	if sourceCtx == nil || destCtx == nil || len(text) == 0 {
		return text
	}

	srcType := sourceCtx.ContentType
	dstType := destCtx.ContentType

	// If source and destination are the same formatting type, strip markers
	// (the destination context already provides the formatting).
	if srcType == dstType {
		return stripMarkers(text, srcType)
	}

	// Handle structural elements (headings, code blocks).
	switch srcType {
	case ContentHeading:
		// Trailing newline means structural paste — preserve as-is.
		if len(text) > 0 && text[len(text)-1] == '\n' {
			return text
		}
		// No trailing newline — strip the heading prefix, treat as text.
		return stripHeadingPrefix(text)

	case ContentCodeBlock:
		// Trailing newline means structural paste — preserve fences.
		if len(text) > 0 && text[len(text)-1] == '\n' {
			return text
		}
		// No trailing newline — just the code text, no fences.
		return text

	case ContentBold:
		if dstType == ContentPlain {
			return wrapWith(text, "**")
		}
		return text

	case ContentItalic:
		if dstType == ContentPlain {
			return wrapWith(text, "*")
		}
		return text

	case ContentBoldItalic:
		if dstType == ContentPlain {
			return wrapWith(text, "***")
		}
		return text

	case ContentCode:
		if dstType == ContentPlain {
			return wrapWith(text, "`")
		}
		return text
	}

	// Plain text or unrecognized — pass through.
	return text
}

// stripMarkers removes formatting markers for same-type paste (e.g., heading prefix).
func stripMarkers(text []byte, ct SelectionContentType) []byte {
	switch ct {
	case ContentHeading:
		return stripHeadingPrefix(text)
	default:
		return text
	}
}

// stripHeadingPrefix removes leading # characters and the following space.
func stripHeadingPrefix(text []byte) []byte {
	i := 0
	for i < len(text) && text[i] == '#' {
		i++
	}
	if i > 0 && i < len(text) && text[i] == ' ' {
		i++
	}
	return text[i:]
}

// wrapWith wraps text in the given marker string (e.g., "**", "*", "`").
func wrapWith(text []byte, marker string) []byte {
	m := []byte(marker)
	result := make([]byte, 0, len(m)+len(text)+len(m))
	result = append(result, m...)
	result = append(result, text...)
	result = append(result, m...)
	return result
}

// IsPreviewMode returns true if the window is in preview mode (showing rendered markdown).
func (w *Window) IsPreviewMode() bool {
	return w.previewMode
}

// SetPreviewMode enables or disables preview mode.
// When disabling preview mode, triggers a full redraw of the body.
// The image cache is kept alive so re-entering preview is fast.
func (w *Window) SetPreviewMode(enabled bool) {
	wasPreview := w.previewMode
	w.previewMode = enabled

	// When exiting preview mode, cancel any pending debounce render
	// and refresh the body.
	// Keep the image cache alive so re-entering preview is fast.
	if wasPreview && !enabled {
		w.cancelPreviewDebounce()
		w.previewRenderPending = false
		// Force a full redraw of the body by resizing it
		if w.display != nil {
			w.body.Resize(w.body.all, true, false)
			w.body.ScrDraw(w.body.fr.GetFrameFillStatus().Nchars)
			w.display.Flush()
		}
	}
}

// TogglePreviewMode toggles the preview mode state.
func (w *Window) TogglePreviewMode() {
	w.SetPreviewMode(!w.previewMode)
}

// RichBody returns the rich text renderer for preview mode, or nil if not initialized.
func (w *Window) RichBody() *RichText {
	return w.richBody
}

// Draw renders the window. In preview mode, it renders the richBody;
// otherwise, it uses the normal body rendering.
func (w *Window) Draw() {
	if (w.previewMode || w.styledMode) && w.richBody != nil {
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

// HandlePreviewMouse handles mouse events when the window is in preview mode.
// Returns true if the event was handled by the preview mode, false otherwise.
// When false is returned, the caller should handle the event normally.
func (w *Window) HandlePreviewMouse(m *draw.Mouse, mc *draw.Mousectl) bool {
	if !w.previewMode || w.richBody == nil {
		return false
	}

	// Check if the mouse is in the body area
	if !m.Point.In(w.body.all) {
		return false
	}

	rt := w.richBody

	// Handle scroll wheel (buttons 4 and 5).
	// When the cursor is over a horizontally-scrollable block region,
	// redirect vertical scroll to horizontal scrolling.
	if m.Buttons&8 != 0 || m.Buttons&16 != 0 {
		if regionIndex, ok := rt.Frame().PointInBlockRegion(m.Point); ok {
			// Horizontal scroll: button 4 = left, button 5 = right.
			delta := 40 // pixels per scroll tick
			if m.Buttons&8 != 0 {
				delta = -delta
			}
			rt.Frame().HScrollWheel(delta, regionIndex)
		} else {
			// Normal vertical scroll.
			up := m.Buttons&8 != 0
			rt.ScrollWheel(up)
		}
		w.Draw()
		if w.display != nil {
			w.display.Flush()
		}
		return true
	}

	// Handle scrollbar clicks (buttons 1, 2, 3 in scrollbar area).
	// Uses latching: once pressed, the scroll tracks the mouse until release.
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
		if button != 0 && mc != nil {
			if rt.scrollbar != nil {
				rt.scrollbar.HandleClick(button)
				w.Draw()
				if w.display != nil {
					w.display.Flush()
				}
			} else {
				// Fallback path: should not happen in practice
				// because Init constructs scrollbar whenever bg/thumb
				// colors are configured.
				w.previewVScrollLatch(rt, mc, button, scrRect)
			}
			return true
		}
	}

	// Handle horizontal scrollbar clicks (buttons 1, 2, 3 on h-scrollbar).
	// Uses latching: same pattern as vertical.
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
			w.previewHScrollLatch(rt, mc, button, regionIndex)
			return true
		}
	}

	// Handle button 1 in frame area for text selection and chording.
	// Chord processing follows the same pattern as text.go: chords are
	// handled inline in a loop while B1 is held, so that sequential
	// B1+B2 (cut) then B1+B3 (paste) works correctly.
	frameRect := rt.Frame().Rect()
	if m.Point.In(frameRect) && m.Buttons&1 != 0 && mc != nil {
		var p0, p1 int
		var lastButtons int // track button state for chord loop

		selectq := rt.Frame().Charofpt(m.Point)
		b := m.Buttons
		fr := rt.Frame()

		// Check for double-click: same richtext, same position, within 500ms.
		prevP0, prevP1 := rt.Selection()
		if w.previewClickRT == rt &&
			m.Msec-w.previewClickMsec < 500 &&
			prevP0 == prevP1 && selectq == prevP0 {

			// Double-click: expand selection
			p0, p1 = fr.ExpandAtPos(selectq)
			rt.SetSelection(p0, p1)
			fr.Redraw()
			if w.display != nil {
				w.display.Flush()
			}
			w.previewClickRT = nil

			// Wait for mouse state change (jitter tolerance), then
			// fall through to the chord processing loop below.
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
			// Normal click/drag selection: track drag until a chord
			// is detected or all buttons are released.
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
				// Chord detected or all buttons released: exit drag.
				if me.Buttons != b || me.Buttons == 0 {
					break
				}
			}

			// Record double-click state
			if p0 == p1 {
				w.previewClickRT = rt
				w.previewClickPos = p0
				w.previewClickMsec = m.Msec
			} else {
				w.previewClickRT = nil
			}
		}

		// Sync the preview selection to the source body buffer
		w.syncSourceSelection()
		q0 := w.body.q0

		// Chord processing loop: handle B2/B3 chords while B1 is held,
		// matching the text.go pattern with undo/redo toggle semantics.
		const (
			chordNone = iota
			chordCut
			chordPaste
			chordSnarf
		)
		state := chordNone
		for lastButtons != 0 {
			if lastButtons == 7 && state == chordNone {
				// B1+B2+B3 simultaneous: snarf only (copy, no delete)
				cut(&w.body, &w.body, nil, true, false, "")
				global.snarfContext = w.selectionContext
				state = chordSnarf
			} else if (lastButtons&1) != 0 && (lastButtons&6) != 0 && state != chordSnarf {
				if state == chordNone {
					w.body.TypeCommit()
					global.seq++
					w.body.file.Mark(global.seq)
				}
				if lastButtons&2 != 0 {
					// B2 chord: cut (or undo a previous paste)
					if state == chordPaste {
						w.Undo(true)
						w.body.SetSelect(q0, w.body.q1)
						state = chordNone
					} else if state != chordCut {
						cut(&w.body, &w.body, nil, true, true, "")
						global.snarfContext = w.selectionContext
						state = chordCut
					}
				} else {
					// B3 chord: paste (or undo a previous cut)
					if state == chordCut {
						w.Undo(true)
						w.body.SetSelect(q0, w.body.q1)
						state = chordNone
					} else if state != chordPaste {
						paste(&w.body, &w.body, nil, true, false, "")
						state = chordPaste
					}
				}
				// Collapse the rich frame's selection before re-rendering
				// so UpdatePreview doesn't draw a stale highlight.
				mq0, mq1 := w.previewSourceMap.ToRendered(w.body.q0, w.body.q1)
				rt.SetSelection(mq0, mq1)
				w.UpdatePreview()
				// Now use the new source map to set the correct selection.
				if w.previewSourceMap != nil {
					rendStart, rendEnd := w.previewSourceMap.ToRendered(w.body.q0, w.body.q1)
					if rendStart >= 0 {
						rt.SetSelection(rendStart, rendEnd)
					}
				}
				clearmouse()
			}
			// Wait for button state to change
			prev := lastButtons
			for lastButtons == prev {
				me := <-mc.C
				lastButtons = me.Buttons
			}
			w.previewClickRT = nil
		}

		w.Draw()
		if w.display != nil {
			w.display.Flush()
		}
		return true
	}

	// Handle button 2 (B2/middle-click) in frame area for Execute action
	if m.Point.In(frameRect) && m.Buttons&2 != 0 {
		// Save the prior selection to restore after B2 execute
		priorP0, priorP1 := rt.Selection()

		var p0, p1 int
		if mc != nil {
			// Use Frame.SelectWithColor() for proper drag selection with B2
			// Pass global.but2col (red) for colored sweep during drag
			p0, p1 = rt.Frame().SelectWithColor(mc, m, global.but2col)
			rt.SetSelection(p0, p1)
		} else {
			// Fallback: just set point selection if no Mousectl available
			charPos := rt.Frame().Charofpt(m.Point)
			p0, p1 = charPos, charPos
			rt.SetSelection(charPos, charPos)
		}
		// If null click (no sweep), expand selection. In a code block,
		// expands to the full block; otherwise expands to word.
		if p0 == p1 {
			q0, q1 := rt.Frame().ExpandAtPos(p0)
			if q0 != q1 {
				rt.SetSelection(q0, q1)
				p0, p1 = q0, q1
			}
		}
		// Sync the preview selection to the source body buffer
		w.syncSourceSelection()
		w.Draw()
		if w.display != nil {
			w.display.Flush()
		}
		// Execute the rendered text as a command
		cmdText := w.PreviewExecText()
		if cmdText != "" {
			previewExecute(&w.body, cmdText)
		}
		// Restore prior selection after B2 execute action
		rt.SetSelection(priorP0, priorP1)
		w.Draw()
		if w.display != nil {
			w.display.Flush()
		}
		return true
	}

	// Handle button 3 (B3/right-click) in frame area for Look action
	if m.Point.In(frameRect) && m.Buttons&4 != 0 {
		// Save prior selection before the sweep overwrites it.
		// Needed so a B3 null-click inside an existing selection uses
		// that selection rather than expanding to a word.
		priorQ0, priorQ1 := rt.Selection()

		// First, perform sweep selection (like B1/B2)
		var p0, p1 int
		if mc != nil {
			// Use Frame.SelectWithColor() for proper drag selection with B3
			// Pass global.but3col (green) for colored sweep during drag
			p0, p1 = rt.Frame().SelectWithColor(mc, m, global.but3col)
			rt.SetSelection(p0, p1)
		} else {
			charPos := rt.Frame().Charofpt(m.Point)
			p0, p1 = charPos, charPos
			rt.SetSelection(charPos, charPos)
		}

		// Determine the character position for link/image checks
		charPos := p0

		// If null click (no sweep), check for existing selection or expand.
		// Use word-level expansion only (not code block expansion).
		if p0 == p1 {
			// Check if click is inside the prior selection
			if priorQ0 != priorQ1 && charPos >= priorQ0 && charPos < priorQ1 {
				// Click inside existing selection - use it as-is
				p0, p1 = priorQ0, priorQ1
				rt.SetSelection(priorQ0, priorQ1)
			} else {
				q0, q1 := rt.Frame().ExpandWordAtPos(p0)
				if q0 != q1 {
					rt.SetSelection(q0, q1)
					p0, p1 = q0, q1
				}
			}
		}

		// Check if this position is within a link
		url := w.PreviewLookLinkURL(charPos)

		// For swept selections, also check if the range overlaps a single link
		if url == "" && p0 != p1 && w.previewLinkMap != nil {
			url = w.previewLinkMap.URLForRange(p0, p1)
		}

		if url != "" {
			// Save body selection before any modifications - we'll restore it before returning
			// so that preview link operations don't leave selections in the text buffer
			savedBodyQ0 := w.body.q0
			savedBodyQ1 := w.body.q1

			// Sync source selection so body.q1 is set as search start
			w.syncSourceSelection()

			// Check if URL is an address expression (like :/^# Index or ?word)
			if len(url) > 0 && (url[0] == ':' || url[0] == '/' || url[0] == '?') {
				// Parse as Acme address expression
				urlRunes := []rune(url)
				found := false

				// Create a getc function for the URL string
				getc := func(q int) rune {
					if q < len(urlRunes) {
						return urlRunes[q]
					}
					return 0
				}

				// Set search start to after the current selection, so the address
				// expression searches forward from the current position.
				// Save original selection in case address evaluation fails.
				origQ0 := w.body.q0
				origQ1 := w.body.q1
				w.body.q0 = origQ1
				w.body.q1 = origQ1

				// Skip the leading ':' prefix — it's our convention marker meaning
				// "this is an address expression", not part of Acme's address syntax.
				// Without this, the address parser treats ':' as a compound operator,
				// turning /regexp/ into a range instead of a point search.
				addrStart := 0
				if urlRunes[0] == ':' {
					addrStart = 1
				}

				// Evaluate the address expression
				r, eval, _ := address(false, &w.body, Range{-1, -1}, Range{w.body.q0, w.body.q1}, addrStart, len(urlRunes), getc, true)
				if eval && r.q0 <= r.q1 {
					w.body.q0 = r.q0
					w.body.q1 = r.q1
					found = true
				}

				// Wrap around: if forward search didn't find from current
				// position, retry from the start of the file. For backward
				// search, retry from the end. This matches Acme's Look behavior.
				if !found {
					nr := w.body.file.Nr()
					w.body.q0 = 0
					w.body.q1 = 0
					r, eval, _ = address(false, &w.body, Range{-1, -1}, Range{0, 0}, addrStart, len(urlRunes), getc, true)
					if eval && r.q0 <= r.q1 {
						w.body.q0 = r.q0
						w.body.q1 = r.q1
						found = true
					} else {
						// Also try from end for backward searches
						w.body.q0 = nr
						w.body.q1 = nr
						r, eval, _ = address(false, &w.body, Range{-1, -1}, Range{nr, nr}, addrStart, len(urlRunes), getc, true)
						if eval && r.q0 <= r.q1 {
							w.body.q0 = r.q0
							w.body.q1 = r.q1
							found = true
						}
					}
				}

				if !found {
					w.body.q0 = origQ0
					w.body.q1 = origQ1
				}

				if found {
					// Use ShowInPreview to map source positions to rendered,
					// set selection, scroll, draw, and flush — all in one call.
					rendStart := w.ShowInPreview(w.body.q0, w.body.q1)
					if rendStart >= 0 {
						// Warp cursor to found text
						if w.display != nil {
							warpPt := rt.Frame().Ptofchar(rendStart).Add(
								image.Pt(4, rt.Frame().DefaultFontHeight()-4))
							w.display.MoveTo(warpPt)
						}
						// Restore original body selection so toggling preview mode doesn't show erroneous selection
						w.body.q0 = savedBodyQ0
						w.body.q1 = savedBodyQ1
						return true
					}
				}
			}

			// Not an address expression, or address evaluation failed — plumb the URL.
			// We don't search the body for the URL text because search() wraps around
			// and matches the URL inside the link's own markdown syntax [text](url).
			w.body.q0 = savedBodyQ0
			w.body.q1 = savedBodyQ1

			if plumbsendfid != nil {
				pm := &plumb.Message{
					Src:  "acme",
					Dst:  "",
					Dir:  w.body.AbsDirName(""),
					Type: "text",
					Data: []byte(url),
				}
				if err := pm.Send(plumbsendfid); err != nil {
					warning(nil, "Markdown B3: plumb failed: %v\n", err)
				}
			} else {
				warning(nil, "Markdown B3: plumber not running\n")
			}
			return true
		}

		// Check if this position is within an image
		imageURL := rt.Frame().ImageURLAt(charPos)
		if imageURL != "" {
			// Plumb the image path
			if plumbsendfid != nil {
				pm := &plumb.Message{
					Src:  "acme",
					Dst:  "",
					Dir:  w.body.AbsDirName(""),
					Type: "text",
					Data: []byte(imageURL),
				}
				if err := pm.Send(plumbsendfid); err != nil {
					warning(nil, "Markdown B3 image: plumb failed: %v\n", err)
				}
			} else {
				warning(nil, "Markdown B3 image: plumber not running\n")
			}
			return true
		}

		// Not a link or image - use rendered text for Look (search in body)
		// Get the rendered (plain) text to search for, not the source markdown
		lookText := w.PreviewLookText()

		// Still sync the source selection so body.q1 is set as the search start position
		w.syncSourceSelection()

		if len(lookText) > 0 {
			// Search source buffer for the rendered text (no markup)
			if search(&w.body, []rune(lookText)) {
				// Map the search result (body.q0/q1) back to rendered positions
				if w.previewSourceMap != nil {
					rendStart, rendEnd := w.previewSourceMap.ToRendered(w.body.q0, w.body.q1)
					if rendStart >= 0 && rendEnd >= 0 {
						rt.SetSelection(rendStart, rendEnd)
						w.scrollPreviewToMatch(rt, rendStart)
						// Warp cursor to found text, matching normal Acme's look3() behavior
						if w.display != nil {
							warpPt := rt.Frame().Ptofchar(rendStart).Add(
								image.Pt(4, rt.Frame().DefaultFontHeight()-4))
							w.display.MoveTo(warpPt)
						}
					}
				}
			}
		}

		w.Draw()
		if w.display != nil {
			w.display.Flush()
		}
		return true
	}

	return false
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

// previewVScrollLatch implements acme-style latching for the vertical
// scrollbar in preview mode. Once a button is pressed in the scrollbar, the
// scroll action tracks the mouse until the button is released. The cursor is
// physically warped back into the scrollbar on each iteration, matching the
// acme pattern in scrl.go.
func (w *Window) previewVScrollLatch(rt *RichText, mc *draw.Mousectl, button int, scrRect image.Rectangle) {
	buttonBit := 1 << uint(button-1)
	centerX := (scrRect.Min.X + scrRect.Max.X) / 2

	// Initial scroll action.
	rt.ScrollClick(button, mc.Mouse.Point)
	w.Draw()
	if w.display != nil {
		w.display.Flush()
	}

	first := true
	for {
		if button == 2 {
			// B2: read per-event for live thumb drag.
			mc.Mouse = <-mc.C
		} else {
			// B1/B3: drain events for the debounce window. The
			// drain absorbs cursor jitter (a click physically
			// wiggles the cursor 1-2 px, generating spurious
			// move events between press and release). Without
			// the drain, each jitter event used to trigger a
			// per-iteration scroll-click — a single physical
			// click became N dispatches.
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

		// Clamp Y and lock X to center of scrollbar.
		my := mc.Mouse.Point.Y
		if my < scrRect.Min.Y {
			my = scrRect.Min.Y
		}
		if my >= scrRect.Max.Y {
			my = scrRect.Max.Y
		}
		warpPt := image.Pt(centerX, my)

		// Warp cursor back into scrollbar if it has moved away.
		if !mc.Mouse.Point.Eq(warpPt) {
			if w.display != nil {
				w.display.MoveTo(warpPt)
			}
			mc.Mouse = <-mc.C // absorb synthetic move event
		}

		rt.ScrollClick(button, warpPt)
		w.Draw()
		if w.display != nil {
			w.display.Flush()
		}
	}

	// Drain remaining mouse events until all buttons released.
	for mc.Mouse.Buttons != 0 {
		mc.Mouse = <-mc.C
	}
}

// previewHScrollLatch implements acme-style latching for horizontal
// scrollbars in preview mode. Same pattern as previewVScrollLatch but for the
// horizontal axis. The cursor is warped to stay within the scrollbar band.
func (w *Window) previewHScrollLatch(rt *RichText, mc *draw.Mousectl, button int, regionIndex int) {
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

// ShowInPreview maps source positions [q0, q1) to rendered positions,
// updates the preview selection, scrolls to make it visible, and redraws.
// Returns the rendered start position (for cursor warping), or -1 if
// mapping failed.
func (w *Window) ShowInPreview(q0, q1 int) int {
	if !w.previewMode || w.richBody == nil || w.previewSourceMap == nil {
		return -1
	}
	rt := w.richBody
	rendStart, rendEnd := w.previewSourceMap.ToRendered(q0, q1)
	if rendStart < 0 || rendEnd < 0 {
		return -1
	}
	rt.SetSelection(rendStart, rendEnd)
	w.scrollPreviewToMatch(rt, rendStart)
	w.Draw()
	if w.display != nil {
		w.display.Flush()
	}
	return rendStart
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
	w.scrollPreviewToMatch(rt, q0)
	w.body.org = rt.Origin() // Keep plain frame origin in sync
	w.Draw()
	if w.display != nil {
		w.display.Flush()
	}
}

// scrollPreviewToMatch scrolls the preview so that the match at rendStart
// is visible, placing it roughly 1/3 from the top of the frame (matching
// Acme's Show() scroll behavior). If the match is already visible, no
// scrolling occurs.
func (w *Window) scrollPreviewToMatch(rt *RichText, rendStart int) {
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

// SetPreviewSourceMap sets the source map used for mapping rendered positions
// to source positions when in preview mode.
func (w *Window) SetPreviewSourceMap(sm *markdown.SourceMap) {
	w.previewSourceMap = sm
}

// PreviewSourceMap returns the current source map, or nil if not set.
func (w *Window) PreviewSourceMap() *markdown.SourceMap {
	return w.previewSourceMap
}

// SetPreviewLinkMap sets the link map used for mapping rendered positions
// to link URLs when in preview mode.
func (w *Window) SetPreviewLinkMap(lm *markdown.LinkMap) {
	w.previewLinkMap = lm
}

// PreviewLinkMap returns the current link map, or nil if not set.
func (w *Window) PreviewLinkMap() *markdown.LinkMap {
	return w.previewLinkMap
}

// PreviewLookLinkURL returns the URL if the given position in the rendered preview
// falls within a link. Returns empty string if the position is not within a link,
// if not in preview mode, or if no link map is set.
// This is used by the Look handler to determine if a B3 click should open a URL.
func (w *Window) PreviewLookLinkURL(pos int) string {
	if !w.previewMode || w.previewLinkMap == nil {
		return ""
	}
	return w.previewLinkMap.URLAt(pos)
}

// recordEdit accumulates an edit record for the incremental preview path.
func (w *Window) recordEdit(e markdown.EditRecord) {
	w.pendingEdits = append(w.pendingEdits, e)
}

// updatePreviewModel performs the model-only portion of a preview update:
// incremental/full markdown parse, updating content + sourceMap + linkMap +
// blockIdx, and restoring the scroll position. It does NOT render or flush.
func (w *Window) updatePreviewModel() {
	if !w.previewMode || w.richBody == nil {
		return
	}

	// Get the current scroll position to preserve it
	currentOrigin := w.richBody.Origin()
	currentYOffset := w.richBody.GetOriginYOffset()

	// Determine if the user was scrolled near the end of the content.
	// If so, we'll follow the tail after updating (like a terminal).
	oldLen := 0
	if oldContent := w.richBody.Content(); oldContent != nil {
		oldLen = oldContent.Len()
	}
	followTail := isNearEnd(currentOrigin, oldLen)

	// Read the current body content
	bodyContent := w.body.file.String()

	var content rich.Content
	var sourceMap *markdown.SourceMap
	var linkMap *markdown.LinkMap
	var blockIdx *markdown.BlockIndex

	// Try incremental path when we have a previous block index and pending edits.
	if w.prevBlockIndex != nil && len(w.pendingEdits) > 0 {
		old := markdown.StitchResult{
			Content:  w.richBody.Content(),
			SM:       w.previewSourceMap,
			LM:       w.previewLinkMap,
			BlockIdx: w.prevBlockIndex,
		}
		result, ok := markdown.IncrementalUpdate(old, bodyContent, w.pendingEdits)
		if ok {
			content = result.Content
			sourceMap = result.SM
			linkMap = result.LM
			blockIdx = result.BlockIdx
		}
	}

	// Full re-parse fallback.
	if content == nil {
		content, sourceMap, linkMap, blockIdx = markdown.ParseWithSourceMapAndIndex(bodyContent)
	}

	// Clear pending edits.
	w.pendingEdits = w.pendingEdits[:0]

	// Update the rich body content
	w.richBody.SetContent(content)
	w.previewSourceMap = sourceMap
	w.previewLinkMap = linkMap
	w.prevBlockIndex = blockIdx

	newLen := content.Len()

	if followTail {
		// Scroll to the end of the new content so the latest output is visible.
		w.richBody.SetOrigin(newLen)
		w.richBody.SetOriginYOffset(0)
	} else {
		// Restore the previous scroll position, clamped to new content length.
		if currentOrigin > newLen {
			currentOrigin = newLen
		}
		w.richBody.SetOrigin(currentOrigin)
		w.richBody.SetOriginYOffset(currentYOffset)
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

// UpdatePreview updates the preview content from the body buffer.
// This should be called when the body buffer changes and the window is in preview mode.
// It re-parses the markdown and updates the richBody, preserving the scroll position.
// When edit position information is available (from pendingEdits), it attempts an
// incremental re-parse of only the affected blocks. Falls back to full re-parse
// when the incremental path cannot determine the affected region.
func (w *Window) UpdatePreview() {
	if !w.previewMode || w.richBody == nil {
		return
	}

	// Cancel any pending debounce render since we're doing a full update.
	w.cancelPreviewDebounce()

	w.updatePreviewModel()

	// Map the body's source selection to a rendered position so the
	// insertion point follows external writes (e.g. shell output via
	// the data pseudo-file).
	if w.previewSourceMap != nil {
		rendStart, rendEnd := w.previewSourceMap.ToRendered(w.body.q0, w.body.q1)
		if rendStart >= 0 {
			w.richBody.SetSelection(rendStart, rendEnd)
		} else {
			contentLen := w.richBody.Content().Len()
			w.richBody.SetSelection(contentLen, contentLen)
		}
	}

	// Render the preview using body.all as the canonical geometry
	w.richBody.Render(w.body.all)
	if w.display != nil {
		w.display.Flush()
	}
}

// PreviewSnarf returns the text that would be snarfed (copied) when in preview mode.
// It uses the source map to convert the selection in the rendered rich text back to
// positions in the source markdown, then extracts that range from the body buffer.
// Returns empty slice if not in preview mode, no rich body, no selection, or no source map.
func (w *Window) PreviewSnarf() []byte {
	if !w.previewMode || w.richBody == nil || w.previewSourceMap == nil {
		return nil
	}

	// Get selection from the rich text frame
	p0, p1 := w.richBody.Selection()
	if p0 == p1 {
		return nil // No selection
	}

	// Map rendered positions to source positions
	srcStart, srcEnd := w.previewSourceMap.ToSource(p0, p1)

	// Clamp to body buffer bounds
	srcStart, srcEnd = clampToBuffer(srcStart, srcEnd, w.body.file.Nr())
	if srcStart >= srcEnd {
		return nil
	}

	// Read the source text from the body buffer
	buf := make([]rune, srcEnd-srcStart)
	w.body.file.Read(srcStart, buf)

	return []byte(string(buf))
}

// PreviewLookText returns the selected text from the preview for a Look (B3) operation.
// In preview mode, this returns the rendered text (not the source markdown).
// Returns empty string if not in preview mode or no selection.
func (w *Window) PreviewLookText() string {
	if !w.previewMode || w.richBody == nil {
		return ""
	}

	// Get selection from the rich text frame
	p0, p1 := w.richBody.Selection()
	if p0 == p1 {
		return "" // No selection
	}

	// Get the plain text from the rendered content
	content := w.richBody.Content()
	if content == nil {
		return ""
	}

	plainText := content.Plain()
	if p0 < 0 || p1 < 0 || p0 > p1 || p1 > len(plainText) {
		return ""
	}

	return string(plainText[p0:p1])
}

// PreviewExecText returns the selected text from the preview for an Exec (B2) operation.
// In preview mode, this returns the rendered text (not the source markdown).
// Returns empty string if not in preview mode or no selection.
func (w *Window) PreviewExecText() string {
	// Exec and Look use the same text extraction logic
	return w.PreviewLookText()
}

// PreviewExpandWord expands a click position to the full word in preview mode.
// Given a position in the rendered text, returns the word containing that position
// along with its start and end positions. Used for B3 Look when there's no selection.
func (w *Window) PreviewExpandWord(pos int) (word string, start, end int) {
	if !w.previewMode || w.richBody == nil {
		return "", pos, pos
	}

	content := w.richBody.Content()
	if content == nil {
		return "", pos, pos
	}

	plainText := content.Plain()
	if pos < 0 || pos >= len(plainText) {
		return "", pos, pos
	}

	start, end = w.richBody.Frame().ExpandWordAtPos(pos)
	if start >= end {
		return "", pos, pos
	}

	return string(plainText[start:end]), start, end
}

// isWordChar returns true if the rune is part of a word (alphanumeric or underscore).
func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

// clampToBuffer clamps start and end positions to [0, bufLen].
// If clamping causes start > end, start is set to end.
func clampToBuffer(start, end, bufLen int) (int, int) {
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	if start > bufLen {
		start = bufLen
	}
	if end > bufLen {
		end = bufLen
	}
	if start > end {
		start = end
	}
	return start, end
}

// syncSourceSelection maps the current preview selection to the corresponding
// positions in the source body buffer. This keeps body.q0 and body.q1 in sync
// with the rendered preview selection, enabling Snarf and other Acme operations
// to work correctly in preview mode.
func (w *Window) syncSourceSelection() {
	if !w.previewMode || w.richBody == nil || w.previewSourceMap == nil {
		return
	}

	// Get selection from the rich text frame
	p0, p1 := w.richBody.Selection()

	// Map rendered positions to source positions
	srcStart, srcEnd := w.previewSourceMap.ToSource(p0, p1)

	// Clamp to body buffer bounds
	srcStart, srcEnd = clampToBuffer(srcStart, srcEnd, w.body.file.Nr())

	// Update the body's selection to match
	w.body.q0 = srcStart
	w.body.q1 = srcEnd
}

// HandlePreviewKey handles keyboard input when the window is in preview mode.
// Returns true if the key was handled (navigation keys), false otherwise (typing keys).
// Navigation keys (Page Up/Down, arrows, Home, End) scroll the preview.
// Escape exits preview mode.
// Typing keys are ignored in preview mode (returns false to indicate not handled).
func (w *Window) HandlePreviewKey(key rune) bool {
	if !w.previewMode || w.richBody == nil {
		return false
	}

	rt := w.richBody
	frame := rt.Frame()
	if frame == nil {
		return false
	}

	// Compute current pixel position for pixel-based scrolling.
	// Uses the frame's layout line data (which accounts for images and
	// other tall elements) rather than content newlines.
	lineHeights := frame.LinePixelHeights()
	lineStarts := frame.LineStartRunes()
	if len(lineHeights) == 0 || len(lineStarts) == 0 {
		return false
	}

	totalPixelHeight := frame.TotalDocumentHeight()
	frameHeight := frame.Rect().Dy()
	fontH := frame.DefaultFontHeight()
	if fontH <= 0 {
		fontH = 14
	}

	currentLine := findLineForOrigin(rt.Origin(), lineStarts)
	currentPixelY := lineOffsetToPixel(currentLine, rt.GetOriginYOffset(), lineHeights)

	maxPixelY := totalPixelHeight - frameHeight
	if maxPixelY < 0 {
		maxPixelY = 0
	}

	switch key {
	case draw.KeyPageDown:
		// Scroll down by a page worth of pixels
		step := frame.MaxLines() * fontH
		if step <= 0 {
			step = 10 * fontH
		}
		rt.ScrollToPixelY(currentPixelY + step)
		rt.Redraw()
		return true

	case draw.KeyPageUp:
		// Scroll up by a page worth of pixels
		step := frame.MaxLines() * fontH
		if step <= 0 {
			step = 10 * fontH
		}
		rt.ScrollToPixelY(currentPixelY - step)
		rt.Redraw()
		return true

	case draw.KeyDown:
		// Scroll down by one text line of pixels
		rt.ScrollToPixelY(currentPixelY + fontH)
		rt.Redraw()
		return true

	case draw.KeyUp:
		// Scroll up by one text line of pixels
		rt.ScrollToPixelY(currentPixelY - fontH)
		rt.Redraw()
		return true

	case draw.KeyHome:
		// Scroll to beginning
		rt.ScrollToPixelY(0)
		rt.Redraw()
		return true

	case draw.KeyEnd:
		// Scroll to end
		rt.ScrollToPixelY(maxPixelY)
		rt.Redraw()
		return true

	default:
		// Typing keys and other keys are not handled in preview mode
		return false
	}
}

// HandlePreviewType handles text editing keys in preview mode, inserting or
// deleting characters in the source buffer and immediately re-rendering the
// preview. It follows the same sync→edit→render→remap cycle used by the
// chord cut/paste handlers.
func (w *Window) HandlePreviewType(t *Text, r rune) {
	if !w.previewMode || w.richBody == nil || w.previewSourceMap == nil {
		return
	}

	// Only accept printable characters, newline, tab, and editing keys.
	switch {
	case r == '\n', r == '\t':
		// accepted
	case r == 0x08: // ^H: backspace
	case r == 0x7F: // Del: delete right
	case r == 0x15: // ^U: kill line
	case r == 0x17: // ^W: kill word
	case r >= 0x20 && r < KF: // printable, excluding draw key constants (0xF0xx, 0xF1xx)
		// accepted
	default:
		return
	}

	// 1. Map rendered cursor/selection to source positions.
	w.syncSourceSelection()

	// 2. Create undo points matching text mode behavior.
	// Deletion keys and newline always start a new undo group.
	// Regular characters only create an undo point at the start of
	// a typing sequence (eq0 == -1), so consecutive chars are grouped
	// into one Undo operation.
	switch r {
	case 0x08, 0x7F, 0x15, 0x17, '\n': // deletion keys and newline
		t.TypeCommit()
		global.seq++
		t.file.Mark(global.seq)
	default:
		if t.eq0 == -1 {
			t.TypeCommit()
			global.seq++
			t.file.Mark(global.seq)
		}
	}

	// 3. Handle deletion keys.
	switch r {
	case 0x08: // ^H: backspace
		if t.q0 != t.q1 {
			// Range selected: delete it.
			cut(t, t, nil, false, true, "")
		} else if t.q0 > 0 {
			t.q0--
			cut(t, t, nil, false, true, "")
		}
		w.previewTypeFinish(t)
		return

	case 0x7F: // Del: delete right
		if t.q0 != t.q1 {
			cut(t, t, nil, false, true, "")
		} else if t.q1 < t.file.Nr() {
			t.q1++
			cut(t, t, nil, false, true, "")
		}
		w.previewTypeFinish(t)
		return

	case 0x15: // ^U: kill line
		if t.q0 != t.q1 {
			cut(t, t, nil, false, true, "")
		} else if t.q0 > 0 {
			nnb := t.BsWidth(0x15)
			if nnb > 0 {
				t.q0 -= nnb
				cut(t, t, nil, false, true, "")
			}
		}
		w.previewTypeFinish(t)
		return

	case 0x17: // ^W: kill word
		if t.q0 != t.q1 {
			cut(t, t, nil, false, true, "")
		} else if t.q0 > 0 {
			nnb := t.BsWidth(0x17)
			if nnb > 0 {
				t.q0 -= nnb
				cut(t, t, nil, false, true, "")
			}
		}
		w.previewTypeFinish(t)
		return
	}

	// 4. If range selected, cut it first (like Text.Type).
	if t.q1 > t.q0 {
		cut(t, t, nil, false, true, "")
		t.eq0 = ^0
	}

	// 5. Insert the character into the source buffer.
	t.file.InsertAt(t.q0, []rune{r})
	t.q0++
	t.q1 = t.q0

	w.previewTypeFinish(t)
}

// previewRenderInterval is the minimum time between preview renders during
// typing. Renders are throttled to this interval; a trailing timer ensures
// the final keystroke in a burst always triggers a render.
const previewRenderInterval = 80 * time.Millisecond

// previewTypeFinish completes a preview-mode edit by updating the model
// synchronously and throttling the render.
func (w *Window) previewTypeFinish(t *Text) {
	// Always update the model synchronously (cheap).
	w.updatePreviewModel()

	// Remap source cursor to rendered position and update selection.
	if w.previewSourceMap != nil {
		rendStart, rendEnd := w.previewSourceMap.ToRendered(t.q0, t.q1)
		if rendStart >= 0 {
			w.richBody.SetSelection(rendStart, rendEnd)
		} else if t.q0 == t.q1 {
			// Cursor at end of content or in a gap between source map entries.
			// Fall back to the end of the rendered content.
			contentLen := w.richBody.Content().Len()
			w.richBody.SetSelection(contentLen, contentLen)
		}
	}

	// Throttle rendering.
	w.cancelPreviewDebounce()
	now := time.Now()
	if now.Sub(w.previewLastRender) >= previewRenderInterval {
		w.renderPreview()
	} else {
		w.previewRenderPending = true
		w.previewDebounceTimer = time.AfterFunc(previewRenderInterval, func() {
			global.previewRenderCh <- func() {
				global.row.lk.Lock()
				defer global.row.lk.Unlock()
				if w.previewRenderPending && w.previewMode {
					w.renderPreview()
				}
			}
		})
	}
}

// renderPreview performs the actual render+flush for preview mode.
func (w *Window) renderPreview() {
	w.Draw()
	if w.display != nil {
		w.display.Flush()
	}
	w.previewLastRender = time.Now()
	w.previewRenderPending = false
}

// cancelPreviewDebounce stops any pending debounce timer.
func (w *Window) cancelPreviewDebounce() {
	if w.previewDebounceTimer != nil {
		w.previewDebounceTimer.Stop()
		w.previewDebounceTimer = nil
	}
}

// flushPreviewDebounce cancels any pending timer and immediately renders
// if a render was deferred. Used by tests to synchronize.
func (w *Window) flushPreviewDebounce() {
	w.cancelPreviewDebounce()
	if w.previewRenderPending && w.previewMode {
		w.renderPreview()
	}
}

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
// in styled or preview mode.
func (w *Window) initStyledMode() {
	if w.styledMode || w.previewMode {
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

	// Create an image cache for box elements that reference images.
	if w.imageCache == nil {
		w.imageCache = rich.NewImageCache(0)
	}
	rtOpts = append(rtOpts, WithRichTextImageCache(w.imageCache))

	rt.Init(display, font, rtOpts...)

	w.richBody = rt
	w.styledMode = true
	w.styledSuppressed = false
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
		w.body.ScrDraw(w.body.fr.GetFrameFillStatus().Nchars)
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

// rebuildPreviewFont tears down and rebuilds the preview richBody with
// the current w.body.font, preserving scroll position and content.
// Called when the user changes the font while in preview mode.
func (w *Window) rebuildPreviewFont() {
	if !w.previewMode || w.richBody == nil {
		return
	}

	display := w.display
	if display == nil {
		display = global.row.display
	}
	if display == nil {
		return
	}

	// Save scroll position.
	savedOrigin := w.richBody.Origin()
	savedYOffset := w.richBody.GetOriginYOffset()

	// Resolve font path.
	fontPath := w.body.font
	if fontPath == "" {
		fontPath = global.tagfont
	}

	// Load base font and variants.
	font := fontget(fontPath, display)
	if font == nil {
		return
	}
	boldFont := tryLoadFontVariant(display, fontPath, "bold")
	italicFont := tryLoadFontVariant(display, fontPath, "italic")
	boldItalicFont := tryLoadFontVariant(display, fontPath, "bolditalic")
	codeFont := tryLoadCodeFont(display, fontPath)
	h1Font := tryLoadScaledFont(display, fontPath, 2.0)
	h2Font := tryLoadScaledFont(display, fontPath, 1.5)
	h3Font := tryLoadScaledFont(display, fontPath, 1.25)

	// Build new rich text renderer with preview colors.
	rt := NewRichText()

	bgImage, err := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0xFFFFFFFF)
	if err != nil {
		return
	}
	textImage, err := display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, 0x000000FF)
	if err != nil {
		return
	}

	rtOpts := []RichTextOption{
		WithRichTextBackground(bgImage),
		WithRichTextColor(textImage),
		WithRichTextSelectionColor(global.textcolors[frame.ColHigh]),
		WithScrollbarColors(global.textcolors[frame.ColBord], global.textcolors[frame.ColBack]),
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
	if codeFont != nil {
		rtOpts = append(rtOpts, WithRichTextCodeFont(codeFont))
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

	if w.imageCache != nil {
		rtOpts = append(rtOpts, WithRichTextImageCache(w.imageCache))
	}

	// Wire async image load callback.
	rtOpts = append(rtOpts, WithRichTextOnImageLoaded(func(path string) {
		go func() {
			global.row.lk.Lock()
			defer global.row.lk.Unlock()
			if !w.previewMode || w.richBody == nil {
				return
			}
			w.richBody.Render(w.body.all)
			if w.display != nil {
				w.display.Flush()
			}
		}()
	}))

	// Set base path for relative image resolution.
	name := w.body.file.Name()
	basePath := name
	if !filepath.IsAbs(basePath) {
		if abs, err := filepath.Abs(basePath); err == nil {
			basePath = abs
		}
	}
	rtOpts = append(rtOpts, WithRichTextBasePath(basePath))

	rt.Init(display, font, rtOpts...)

	// Re-parse markdown and set content.
	mdContent := w.body.file.String()
	content, sourceMap, linkMap := markdown.ParseWithSourceMap(mdContent)
	rt.SetContent(content)

	w.richBody = rt
	w.SetPreviewSourceMap(sourceMap)
	w.SetPreviewLinkMap(linkMap)

	// Restore scroll position.
	rt.SetOrigin(savedOrigin)
	rt.SetOriginYOffset(savedYOffset)
	rt.Render(w.body.all)
	if w.display != nil {
		w.display.Flush()
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
		buf := make([]rune, run.Len)
		w.body.file.Read(offset, buf)
		text := string(buf)

		var style rich.Style
		if run.Style.IsBox {
			style = boxStyleToRichStyle(run.Style, text)
		} else {
			style = styleAttrsToRichStyle(run.Style)
		}

		span := rich.Span{
			Text:  text,
			Style: style,
		}
		content = append(content, span)
		offset += run.Len
	})
	return rich.Content(content)
}

// styleAttrsToRichStyle maps StyleAttrs (from span protocol) to rich.Style (for rendering).
func styleAttrsToRichStyle(sa StyleAttrs) rich.Style {
	s := rich.Style{
		Scale: 1.0,
	}
	s.Fg = sa.Fg
	s.Bg = sa.Bg
	s.Bold = sa.Bold
	s.Italic = sa.Italic
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
	s.Fg = sa.Fg
	s.Bg = sa.Bg
	s.Bold = sa.Bold
	s.Italic = sa.Italic

	// Parse payload: if it starts with "image:", set Image + ImageURL.
	if strings.HasPrefix(sa.BoxPayload, "image:") {
		s.Image = true
		s.ImageURL = strings.TrimPrefix(sa.BoxPayload, "image:")
	}

	return s
}

// HandleStyledMouse handles mouse events when the window is in styled rendering
// mode. Returns true if the event was handled, false otherwise. This is the
// styled-mode analog of HandlePreviewMouse, simplified by the identity mapping
// between rich.Frame positions and body buffer positions.
func (w *Window) HandleStyledMouse(m *draw.Mouse, mc *draw.Mousectl) bool {
	if !w.styledMode || w.richBody == nil {
		return false
	}
	if !m.Point.In(w.body.all) {
		return false
	}

	rt := w.richBody

	// Scroll wheel (buttons 4 and 5).
	if m.Buttons&8 != 0 || m.Buttons&16 != 0 {
		up := m.Buttons&8 != 0
		rt.ScrollWheel(up)
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
		if button != 0 && mc != nil {
			if rt.scrollbar != nil {
				rt.scrollbar.HandleClick(button)
				w.Draw()
				if w.display != nil {
					w.display.Flush()
				}
			} else {
				// Fallback path: should not happen in practice
				// because Init constructs scrollbar whenever bg/thumb
				// colors are configured.
				w.previewVScrollLatch(rt, mc, button, scrRect)
			}
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
		if w.previewClickRT == rt &&
			m.Msec-w.previewClickMsec < 500 &&
			prevP0 == prevP1 && selectq == prevP0 {

			p0, p1 = fr.ExpandAtPos(selectq)
			rt.SetSelection(p0, p1)
			fr.Redraw()
			if w.display != nil {
				w.display.Flush()
			}
			w.previewClickRT = nil

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
				w.previewClickRT = rt
				w.previewClickPos = p0
				w.previewClickMsec = m.Msec
			} else {
				w.previewClickRT = nil
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
			w.previewClickRT = nil
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
			previewExecute(&w.body, cmdText)
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
