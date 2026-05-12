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
// spans.Store mutation.
//
//   - OpClearAll wipes every styled region; the body buffer
//     remains as a single plain run.
//
//   - OpSetStyle resolves color.Color values to draw.Image via
//     w.display, tags the Style with KindColored when at least
//     one color is non-default, and calls SetRegion on the
//     range. Out-of-range tolerance is left to the spans.Store:
//     directives whose range exceeds the buffer length will
//     panic with the current store implementation. The published
//     protocol's "silently drop / clamp" rule is enforced
//     upstream in writeSpansToStore.
func applySpansDirective(w *Window, d spans.Directive) error {
	switch d.Op {
	case spans.OpClearAll:
		// Clear all styled spans for the window. Under the
		// dense store, this is "set the whole buffer to plain."
		n := w.body.file.Nr()
		if n > 0 {
			w.body.spans.ClearRegion(0, n)
		}
		return nil
	case spans.OpSetStyle:
		// Out-of-range tolerance per the protocol: silently
		// drop directives whose offset exceeds the body length;
		// clamp directives whose end exceeds it.
		n := w.body.file.Nr()
		if d.Off >= n {
			return nil
		}
		end := d.Off + d.Len
		if end > n {
			end = n
		}
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
		w.body.spans.SetRegion(d.Off, end, style)
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
