package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"

	"9fans.net/go/plan9"
	"github.com/rjkroege/edwood/draw"
	"github.com/rjkroege/edwood/frame"
	"github.com/rjkroege/edwood/spans"
)

// writeSpansToStore parses the 9P spans-file write payload and
// applies each directive to w.body.spans. Color values are
// resolved to draw.Image via w.display before SetRegion / fires
// the spans store's Observe callbacks (so any attached Text
// repaints the visible window).
//
// Extracted from the xfid write path so tests can exercise the
// apply logic without building an Xfid scaffold.
func writeSpansToStore(w *Window, payload string) error {
	if w.body.spans == nil {
		return errors.New("xfid: body has no spans store")
	}
	directives, err := spans.ParseAll(payload)
	if err != nil {
		return err
	}
	for _, d := range directives {
		if err := applySpansDirective(w, d); err != nil {
			return err
		}
	}
	return nil
}

// applySpansDirective converts one parsed Directive into a
// spans.Store mutation. SetStyle directives resolve color.Color
// values to draw.Image via w.display and tag the Style with
// KindColored.
func applySpansDirective(w *Window, d spans.Directive) error {
	switch d.Op {
	case spans.OpClearStyle:
		w.body.spans.ClearRegion(d.Off, d.Off+d.Len)
		return nil
	case spans.OpSetStyle:
		style := frame.Style{}
		if d.Fg != nil {
			img, err := allocColorImage(w.display, d.Fg)
			if err != nil {
				return fmt.Errorf("xfid: resolve fg: %w", err)
			}
			style.Fg = img
			style.Kind |= frame.KindColored
		}
		if d.Bg != nil {
			img, err := allocColorImage(w.display, d.Bg)
			if err != nil {
				return fmt.Errorf("xfid: resolve bg: %w", err)
			}
			style.Bg = img
			style.Kind |= frame.KindColored
		}
		w.body.spans.SetRegion(d.Off, d.Off+d.Len, style)
		return nil
	default:
		return fmt.Errorf("xfid: invalid spans op %v", d.Op)
	}
}

// allocColorImage turns an arbitrary color.Color into a 1×1
// repeated draw.Image owned by display. Used to translate
// parsed `fg=#RRGGBB` / `bg=#RRGGBB` directives into the
// draw.Image values frame.Style holds.
func allocColorImage(display draw.Display, c color.Color) (draw.Image, error) {
	r, g, b, a := c.RGBA()
	dc := draw.Color(uint32(r>>8)<<24 | uint32(g>>8)<<16 | uint32(b>>8)<<8 | uint32(a>>8))
	return display.AllocImage(image.Rect(0, 0, 1, 1), display.ScreenImage().Pix(), true, dc)
}

// xfidspanswrite is the xfid write-path handler for QWspans. It
// drains the request data, dispatches to writeSpansToStore, and
// responds.
func xfidspanswrite(x *Xfid, w *Window) {
	var fc plan9.Fcall
	payload := string(x.fcall.Data)
	if err := writeSpansToStore(w, payload); err != nil {
		x.respond(&fc, err)
		return
	}
	fc.Count = x.fcall.Count
	x.respond(&fc, nil)
}
