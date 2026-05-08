// Spans → rich.Content render bridge. Pure data transformation:
// given a body's runes plus a Store and (optional) RegionStore,
// produces the rich.Content the renderer consumes. No I/O,
// no Window state — the caller (typically Window.buildStyledContent)
// supplies the body and the stores.
//
// Phase 3 rounds 1–8 added per-kind region handling here; this
// file consolidates the kind dispatch (applyEnclosingRegions and
// the apply*Region family) plus the StyleAttrs ↔ rich.Style
// translation.

package spans

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/rjkroege/edwood/rich"
)

// Render produces the rich.Content view of a body, given its
// styled-span Store and its (optional) RegionStore.
//
// Body is the buffer's runes — passed in (rather than read
// inside Render) so the caller can control the read once and
// the function stays I/O-free. Store carries the per-rune
// styling; regions carries the structural overlays (code,
// blockquote, listitem, table, …) that compose with the per-
// rune style at render time.
//
// Returns rich.Plain if the store is nil or empty.
func Render(body []rune, store *Store, regions *RegionStore) rich.Content {
	if store == nil || store.TotalLen() == 0 {
		return rich.Plain(string(body))
	}

	var content []rich.Span
	offset := 0
	store.ForEachRun(func(run StyleRun) {
		if run.Len == 0 {
			return
		}
		runEnd := offset + run.Len
		// Compute split points within this run from region
		// boundaries. If no regions, no splits.
		splits := []int{offset}
		if regions != nil {
			splits = append(splits, regions.BoundariesIn(offset, runEnd)...)
		}
		splits = append(splits, runEnd)
		for i := 0; i < len(splits)-1; i++ {
			subStart, subEnd := splits[i], splits[i+1]
			content = append(content, subRun(run, body, subStart, subEnd, regions))
		}
		offset = runEnd
	})
	return rich.Content(content)
}

// subRun produces one rich.Span for the [subStart, subEnd)
// portion of a parent StyleRun. The base style comes from the
// run; region flags come from the regionStore's deepest enclosing
// region at subStart. Phase 3 round 6 (split-at-boundaries).
func subRun(run StyleRun, body []rune, subStart, subEnd int, regions *RegionStore) rich.Span {
	text := string(body[subStart:subEnd])
	var style rich.Style
	if run.Style.IsBox {
		style = boxStyleToRichStyle(run.Style, text)
	} else {
		style = styleAttrsToRichStyle(run.Style)
	}
	if regions != nil {
		applyEnclosingRegions(&style, regions.EnclosingAt(subStart))
	}
	return rich.Span{Text: text, Style: style}
}

// TryAddRegion calls regions.Add and converts any panic (the
// only error condition; partial overlap with an existing region)
// into a returned error. Used by main package's applyParsedSpans
// so that a producer bug doesn't take down the whole session.
func TryAddRegion(regions *RegionStore, r *Region) (err error) {
	defer func() {
		if x := recover(); x != nil {
			err = fmt.Errorf("%v", x)
		}
	}()
	regions.Add(r)
	return nil
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
// the run. Render satisfies this by splitting each Store
// run at region boundaries via RegionStore.BoundariesIn
// before calling here.
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

// applyCodeRegion sets the per-rune flags for a `code` region
// ancestor. Composition rule: idempotent — multiple code
// ancestors produce the same result as one. Phase 3 round 5.
func applyCodeRegion(s *rich.Style, _ *Region) {
	s.Block = true
	s.Code = true
	s.Bg = rich.InlineCodeBg
}

// applyBlockquoteRegion sets the per-rune flags for a
// `blockquote` region ancestor. Composition rule: additive —
// each blockquote ancestor bumps the depth counter by one,
// so nested `>>` produces depth=2. Phase 3 round 6.
func applyBlockquoteRegion(s *rich.Style, _ *Region) {
	s.Blockquote = true
	s.BlockquoteDepth++
}

// applyListitemRegion sets the per-rune flags for a `listitem`
// region ancestor. Composition rule: additive for ListIndent
// (one bump per ancestor); per-instance payload (marker /
// number) per-call overwrite gives nearest-of-kind semantics.
// Phase 3 round 7.
func applyListitemRegion(s *rich.Style, r *Region) {
	s.ListItem = true
	s.ListIndent++
	if number, ok := r.Params["number"]; ok && number != "" {
		s.ListOrdered = true
		s.ListNumber = parseListNumber(number)
		return
	}
	s.ListOrdered = false
	s.ListNumber = 0
}

// parseListNumber converts a `number=N` param value to an int.
// Returns 0 on parse error (the protocol parser rejects
// malformed numbers upstream; defensive at the bridge boundary).
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

// applyTableRegion sets the per-rune flags for a `table` region
// ancestor. Composition rule: idempotent (v1 disallows
// table-in-table). Sets Table (gutter indent + block layout),
// Block (consistency with code-block flagging), and Code
// (forces monospace for `|` markers between cells). Phase 3
// round 8.
func applyTableRegion(s *rich.Style, _ *Region) {
	s.Table = true
	s.Block = true
	s.Code = true
}

// applyTableRowRegion sets the per-rune flags for a `tablerow`
// region ancestor. The `header=true` param promotes the row's
// runes to TableHeader=true (bold + a separator line). Phase 3
// round 8.
func applyTableRowRegion(s *rich.Style, r *Region) {
	if r.Params["header"] == "true" {
		s.TableHeader = true
	}
}

// applyTableCellRegion sets the per-rune flags for a `tablecell`
// region ancestor. The `align=` param maps to TableAlign
// (left, right, center). Composition rule: nearest-of-kind.
// Phase 3 round 8.
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

// styleAttrsToRichStyle maps StyleAttrs (from span protocol) to
// rich.Style (for rendering).
//
// Scale: StyleAttrs.Scale==0 is the "unset" sentinel and maps
// to rich.Style.Scale=1.0 (body baseline). A positive Scale is
// passed through directly. The parser rejects negative / zero /
// non-finite Scale values, so this branch never sees them.
//
// Family: "code" maps to rich.Style.Code=true. Empty Family
// leaves Code=false. Other values are no-ops here — the parser
// rejects unknown family names upstream.
//
// HRule: passes through directly. The renderer keeps the span's
// text visible (source markers `---`/`***`/`___` render normally)
// and rich/mdrender's paintHorizontalRules draws a 1px line
// across the frame on the same row. Phase 3 round 3.
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

// boxStyleToRichStyle maps a box StyleAttrs (IsBox=true) to
// rich.Style. Boxes without an image: payload are fixed-
// dimension colored rectangles. Boxes with an image: payload
// also enter the image rendering pipeline.
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
	if sa.BoxPlacement == "below" {
		s.ImageBelow = true
	}
	applyImagePayload(&s, sa.BoxPayload)
	return s
}

// applyImagePayload parses a box's payload string and applies
// the recognized parts to the rich.Style:
//   - First token `image:URL` enables image rendering and sets
//     ImageURL to URL (without the `image:` prefix).
//   - `width=N` overrides ImageWidth (including any wire-format
//     BoxWidth).
//
// Anything else is silently ignored for forward-compat. Phase 3
// round 4.
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
