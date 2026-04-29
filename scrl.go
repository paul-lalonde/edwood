package main

import (
	"fmt"
	"image"
	"time"

	"github.com/rjkroege/edwood/draw"
)

var scrtmp draw.Image

func ScrSleep(dt int) {
	timer := time.NewTimer(time.Duration(dt) * time.Millisecond)
	for {
		select {
		case <-timer.C:
			return
		case <-global.mousectl.C:
			timer.Stop()
			return
		}
	}
}

func scrpos(r image.Rectangle, p0, p1 int, tot int) image.Rectangle {
	var (
		q image.Rectangle
		h int
	)
	q = r
	h = q.Max.Y - q.Min.Y
	if tot == 0 {
		return q
	}
	if tot > 1024*1024 {
		tot >>= 10
		p0 >>= 10
		p1 >>= 10
	}
	if p0 > 0 {
		q.Min.Y += h * p0 / tot
	}
	if p1 < tot {
		q.Max.Y -= h * (tot - p1) / tot
	}
	if q.Max.Y < q.Min.Y+2 {
		if q.Max.Y+2 <= r.Max.Y {
			q.Max.Y = q.Min.Y + 2
		} else {
			q.Min.Y = q.Max.Y - 2
		}
	}
	return q
}

func ScrlResize(display draw.Display) {
	var err error
	scrtmp, err = display.AllocImage(image.Rect(0, 0, 32, display.ScreenImage().R().Max.Y), display.ScreenImage().Pix(), false, draw.Nofill)
	if err != nil {
		panic(fmt.Sprintf("scroll alloc: %v", err))
	}
}

// TODO(rjk): Don't pass in nchars. It's always the same.
//
// ScrDraw delegates to the shared Scrollbar widget. The widget owns
// drawing, dirty caching, and scratch-image lifecycle; this method
// keeps only the body-only and preview-mode guards from the legacy
// implementation. Both guards must remain at the call site (not in
// the widget) because the widget does not know which Text it is
// attached to or whether the window is in preview mode.
func (t *Text) ScrDraw(nchars int) {
	if t.w == nil || t != &t.w.body {
		return
	}
	if t.w.IsPreviewMode() {
		// In preview mode, the RichText handles its own scrollbar.
		return
	}
	if t.scrollbar == nil {
		return
	}
	t.scrollbar.SetRect(t.scrollr)
	t.scrollbar.Draw()
}

// Scroll delegates to the shared Scrollbar widget. The widget owns
// the click-and-hold latch loop, debounce timing, cursor warping,
// and post-release event drain — all of which used to live here in
// the legacy scrl.go implementation. The mode-specific arithmetic
// (BackNL / Charofpt / file.Nr) lives in textScrollModel.
func (t *Text) Scroll(but int) {
	if t.scrollbar == nil {
		return
	}
	t.scrollbar.HandleClick(but)
}
