# Markdown Externalization

## Goal

Move markdown rendering out of edwood and into an external tool
(`md2spans`) that produces a layout description over the existing
spans protocol. Edwood becomes a renderer of styled spans plus a small
set of layout primitives; it stops parsing or interpreting markdown
itself.

This mirrors the existing pattern for syntax coloring: an external
tool (`gocolor`, `pycolor`, etc.) computes the coloring and writes
spans; edwood renders. Markdown is currently the outlier — it lives
inside edwood as a special path because the spans protocol can't yet
express the layout it needs. The end state of this work is that
markdown is no longer a special path.

## Why

The internal markdown path has grown substantial cruft:

- `rich.Style` carries 17 markdown-specific fields
  (`Code`, `Block`, `Link`, `HRule`, `ParaBreak`, `ListItem`,
  `ListBullet`, `ListIndent`, `ListOrdered`, `ListNumber`, `Table`,
  `TableHeader`, `TableAlign`, `Blockquote`, `BlockquoteDepth`,
  `SlideBreak`, plus image fields) on top of the universal
  styling primitives. The styled-spans path uses none of them; only
  the markdown preview path does.
- `rich.Frame.drawTextTo` has dedicated paint phases for blockquote
  borders, horizontal rules, block backgrounds with markdown
  semantics, and slide-break fills. Each phase is unreachable from
  the spans protocol.
- The `markdown/` package is non-trivial (parser, source map, link
  map, incremental preview), and every markdown feature added drops
  more state into `rich.Frame`. The architect review (April 2026)
  flagged this as the largest long-term risk in `rich/`.

Moving markdown handling external means:

- `rich.Frame` shrinks back to a clean styled-text engine. No
  markdown semantics; only styling primitives that any renderer
  needs.
- The spans protocol grows just enough capability to express the
  layout that markdown needs — and any other renderer can use those
  primitives too.
- The `markdown/` package and the in-tree paint paths for
  markdown-specific features get deleted.
- Iterating on markdown rendering (or supporting another markup
  language) becomes an external-tool change, not an edwood release.

## End state

```
+--------------+        spans protocol        +--------+
| md2spans     | -----------------------> ... |        |
+--------------+                              |        |
+--------------+                              |        |
| gocolor      | -----------------------> ... | edwood |  --> screen
+--------------+                              |        |
+--------------+                              |        |
| pycolor      | -----------------------> ... |        |
+--------------+                              +--------+
```

- `rich.Frame` understands styled spans, inline replaced boxes, and
  a small set of line/region decorations (indent, line background,
  left-edge bar, slide-break). No markdown awareness.
- The spans protocol can express everything the lean frame
  understands. External tools emit it; edwood renders it.
- `markdown/` package and `rich.Style`'s markdown-specific fields
  are deleted.
- Markdown preview becomes "run `md2spans <file>` and pipe its
  output to the spans file" — same shape as syntax coloring.

## Phased path

**Phase 1 — Lean `rich.Frame` (in-tree refactor).**
Move markdown-specific paint logic out of `rich.Frame` into a new
in-tree wrapper (`rich/mdrender` or similar). The wrapper is a
*transitional* layer: it consumes markdown content the same way the
existing markdown path does and produces lean-style content for the
frame to render. After Phase 1, `rich.Frame` is the frame that
will be in the end state; `rich/mdrender` is throwaway code with
its days numbered. Spans protocol unchanged. Internal markdown
preview still works through the wrapper.

**Phase 2 — `md2spans` v1 (minimal external tool).**
Build the external `md2spans` tool against the *current* spans
protocol. Its v1 capability is whatever the protocol can express
today: paragraph text with bold/italic/colored runs, inline code
(via `Code` styling once added), maybe inline links (via colored
runs). It deliberately does NOT cover headings, lists, tables,
blockquotes, block code, horizontal rules, images, or slides — those
require protocol additions. Even with this restricted capability,
`md2spans` is useful: simple markdown documents render through it
end-to-end. It establishes the toolchain shape (invocation,
command-line interface, error reporting) and proves the round-trip.

**Phase 3 — Iterative enrichment (rounds 1..N).**
Each round picks one markdown feature `md2spans` can't yet handle,
adds the protocol primitive(s) needed, implements rendering in the
lean frame, and ships `md2spans` support. Rounds are independent
and merge separately. Likely sequence (cheapest first):

1. Font scale (headings).
2. Font family selector (inline code, code blocks).
3. Inline rule / horizontal rule.
4. Line-level indent (lists, blockquotes).
5. Line-level background (block code).
6. Left-edge decoration (blockquote bars).
7. Slide-break / viewport-snap marker.
8. Inline replaced images via the existing box mechanism (already
   reachable; just needs `md2spans` support).
9. Tables (largest; needs column-width handshake or pre-computed
   layout).

Each round closes the gap between `md2spans` capability and the
in-tree wrapper's capability. The internal markdown path keeps
working through the wrapper for users; only the boundary
between "wrapper produces" and "external tool produces" moves.

**Phase 4 — Migration and deletion.**
Once `md2spans` covers everything the in-tree wrapper handles,
flip the default: edwood preview mode invokes `md2spans` instead
of the internal path. Run side-by-side for one release cycle.
Then delete the `markdown/` package and the `rich/mdrender`
wrapper. Delete the markdown-specific fields from `rich.Style`.
Delete the markdown-specific paint phases from `rich.Frame`.

## Lean `rich.Frame` contract

What `rich.Frame` understands in the end state:

| Feature | Mechanism |
|---|---|
| Per-run foreground / background color | `Style.Fg`, `Style.Bg` |
| Per-run weight / italic | `Style.Bold`, `Style.Italic` |
| Per-run font scale | `Style.Scale` (heading sizes) |
| Per-run font family | `Style.Code` (or generalized to a family selector) |
| Inline replaced boxes | `Style.FixedBox` + `ImageWidth`/`ImageHeight` |
| Inline images | `Style.Image` + `ImageURL`/`ImageAlt`/dimensions |
| Selection | `p0`, `p1`, sweep colors |
| Cursor / tick | unchanged |
| Vertical scrolling | unchanged |
| Horizontal block-region scrolling | unchanged (the mechanism stays; the markdown trigger doesn't) |

Plus a small set of *line/region decorations* (Phase 3 additions):

| Decoration | Purpose |
|---|---|
| Per-line indent (pixels) | Lists, blockquotes |
| Per-line background (color, width) | Block code, table rows |
| Per-line left-edge bar (color, width) | Blockquote depth |
| Document-level slide-break marker | Slide preview viewport snap |

What moves out of `rich.Frame` into `rich/mdrender` in Phase 1, and
out of edwood entirely in Phase 4:

- Markdown-specific style fields (`Code`, `Link`, `Block`, `HRule`,
  `ParaBreak`, `ListItem`, `ListBullet`, `ListIndent`,
  `ListOrdered`, `ListNumber`, `Table`, `TableHeader`,
  `TableAlign`, `Blockquote`, `BlockquoteDepth`, `SlideBreak`).
  Some collapse into the universal primitives above (`Code` →
  font-family selector; `HRule` → inline rule decoration); the
  rest stop existing.
- Paint phases that interpret those fields:
  `paintPhaseHorizontalRules`, `paintPhaseBlockquoteBorders`, the
  block-background branch in `paintPhaseBlockBackgrounds` (the
  *mechanism* stays; the markdown semantic in the trigger moves
  to the wrapper / external tool).
- Slide-region detection (`findSlideRegions`,
  `adjustLayoutForSlides`) — slide-break becomes a protocol-level
  marker, not a markdown-content discovery.
- The `markdown/` package itself, and the source-map / link-map /
  incremental-preview code paths.

## Spans protocol gap analysis

Today's `StyleAttrs` (`spanstore.go:7`):

```go
type StyleAttrs struct {
    Fg, Bg     color.Color
    Bold       bool
    Italic     bool
    Hidden     bool
    IsBox      bool
    BoxWidth   int
    BoxHeight  int
    BoxPayload string
}
```

To express the lean-frame contract above, the protocol needs:

| Primitive | Today | Phase 3 round | New protocol surface |
|---|---|---|---|
| Foreground / background | yes | — | — |
| Bold / italic | yes | — | — |
| Font scale | no | 1 | `Scale float32` on StyleAttrs (or new prefix) |
| Font family | no | 2 | `Family string` on StyleAttrs (`""`/`"code"`/...) |
| Inline replaced box | yes | — | — |
| Inline image | yes (via box payload `image:...`) | — | — |
| Inline rule | no | 3 | New box payload kind (`rule:width:height`) or new prefix |
| Per-line indent | no | 4 | New per-line directive: `i offset indent_pixels` |
| Per-line background | no | 5 | New per-line directive: `lbg offset bg_color [right_edge_pixels]` |
| Left-edge bar | no | 6 | New per-line directive: `bar offset color width` |
| Slide-break | no | 7 | New document-level directive: `slide offset` |
| Tables | no | 9 | Open question — see Risks |

The exact wire format for each is a Phase-3-round design problem,
not this doc's. But the *quantity* of new directives is small —
roughly five new prefix kinds plus two field additions to existing
prefixes. The protocol stays line-oriented and readable.

## `md2spans` v1 scope

What v1 must do:

- Read markdown from stdin or a file argument.
- Emit spans-protocol output to stdout (the test harness; in
  production the caller pipes it to the window's `spans` file).
- Handle plain paragraph text.
- Handle emphasis (`*italic*`, `_italic_`, `**bold**`).
- Handle inline color via standard CommonMark: not directly
  expressible, so v1 uses a no-color baseline plus `Bold`/`Italic`.
- Map link text (`[text](url)`) to a colored run with `Fg = LinkBlue`
  for visual indication. The URL is dropped at v1 (no protocol
  primitive for "this run's URL is X" yet — punted to a later round).
- Skip-or-pass-through what it can't express: leave headings as
  plain text (no scale change), code blocks as plain text (no font
  change), lists as plain text with their literal `- ` / `1. `
  prefix kept, etc. The output should be *something* for any
  input, not an error.

What v1 explicitly does NOT do:

- Headings (need protocol round 1).
- Inline code, code blocks (need round 2).
- Horizontal rules (need round 3).
- Lists with proper indent / bullets (need round 4 + render of
  bullet glyph).
- Block code with backgrounds (need round 5).
- Blockquotes with bars (need round 6).
- Slides (need round 7).
- Images (could work today via the box protocol; deferred to
  round 8 to keep v1 scope tight).
- Tables (round 9).

The point of v1 is to establish the toolchain. A markdown file
that's mostly paragraph text with emphasis and links should render
through v1 indistinguishably from the internal path. Anything
fancier degrades gracefully.

## Phase 1 detail: in-tree wrapper boundary

Phase 1 is the only phase that's a refactor of existing code (the
others add new code or delete old code). Some specifics for
when we cut the boundary:

- New package: `rich/mdrender` (or `mdrender/` at top level —
  bikeshed later). Imports `rich/`. Owns:
  - The paint phases that interpret markdown-specific style fields,
    moved verbatim from `rich/frame.go` (`paintPhaseHorizontalRules`,
    `paintPhaseBlockquoteBorders`, the markdown-semantic part of
    `paintPhaseBlockBackgrounds`, slide-break detection).
  - Conversion from markdown's `rich.Content` (with markdown-style
    fields) into lean `rich.Content` plus a list of line/region
    decorations the wrapper applies on top of the lean frame's
    paint pass.
  - Currently-internal helpers like `findSlideRegions`,
    `adjustLayoutForSlides`, `computeCodeBlockIndent`.
- The wrapper's API is roughly:
  ```go
  type Renderer struct { /* wraps a *rich.Frame */ }
  func New(frame rich.Frame) *Renderer
  func (r *Renderer) SetMarkdownContent(c rich.Content) // existing rich.Content shape
  func (r *Renderer) Redraw()
  ```
  The wrapper's `Redraw` calls the lean frame's `Redraw`, then
  applies the markdown-specific decoration phases on top.
- `richtext.go` (`RichText`) moves to the wrapper for preview
  mode. Styled mode keeps using the lean frame directly — that
  path is already lean.
- P1-6 (geometry ownership) collapses naturally: the wrapper /
  `RichText` is the geometry source-of-truth; `rich.Frame` becomes
  passive. `rich.Frame.Init` no longer takes a rectangle (or
  ignores it); `RichText.SetRect` is the only setter.
- Tests: `rich/frame_test.go` shrinks. Anything testing markdown-
  specific paint phases moves to `rich/mdrender/*_test.go`.

The wrapper has *no* future. It exists so we can move markdown
features out of `rich.Frame` without immediately rewriting them
against the spans protocol. As Phase 3 rounds land, each markdown
feature in the wrapper migrates to "ask `md2spans` to emit the
protocol primitive instead". The wrapper shrinks each round and
disappears at Phase 4.

## Non-goals

- **Deprecating the spans protocol.** This work makes it richer,
  not different. Existing protocol consumers (`gocolor`,
  `pycolor`, third-party tools) keep working with the additions
  optional.
- **Real-time / interactive markdown editing.** `md2spans` is
  invoked on demand the same way syntax colorers are; live
  re-render on every keystroke is a separate concern handled by
  edwood's existing preview-debounce machinery.
- **Markdown standardization.** `md2spans` is a tool; users can
  swap it for an alternative renderer if they have different
  markdown opinions. Edwood doesn't pick a markdown flavor.
- **Performance optimization of the lean frame.** Phase 1 keeps
  paint behavior identical; Phase 3 adds primitives. Optimization
  is a separate axis.
- **Removing the wrapper before `md2spans` covers everything it does.** Until coverage is full, the wrapper is the only path
  for markdown preview; deleting it prematurely loses features.

## Risks

1. **Wrapper becomes load-bearing.** If `md2spans` development
   stalls between Phase 2 and Phase 4, the wrapper stays in the
   tree indefinitely. Mitigation: each Phase 3 round must include
   the wrapper-side migration to use the protocol primitive, so
   the wrapper actively shrinks. If a round lands the protocol
   support but skips the wrapper migration, the cruft persists.
2. **Tables don't fit the spans model cleanly.** Real table
   layout needs column-width measurement that depends on
   rendering metrics edwood owns. Either `md2spans` pre-computes
   widths (assuming it knows the font metrics; possible if edwood
   exposes them via a 9P file), or the protocol needs a
   table-region directive that does layout server-side. This is
   the highest-risk Phase 3 round and may require its own design
   doc.
3. **Source map / link map.** The current preview tracks markdown
   source ↔ rendered-rune mapping for click-to-source navigation.
   `md2spans` will need to emit this mapping (probably to an
   adjacent file or via a new protocol directive). Design TBD;
   Phase 3 may need an extra round for this if Look navigation
   regresses.
4. **External tool installation friction.** `md2spans` needs to be
   on the user's `$PATH` for preview to work post-Phase-4. The
   syntax-coloring story has this same constraint and it's
   accepted; markdown will follow the same pattern.
5. **Round ordering.** The dependency graph between Phase 3 rounds
   isn't fully linear. Some rounds may unblock others (e.g., font
   scale is a prerequisite for proper heading rendering, which
   affects how lists-under-headings layout). Round ordering will
   be revisited per-round, not pinned here.

## Open questions

- **Wrapper package name:** `rich/mdrender` vs. `mdrender/` (top
  level) vs. something else. Defer to Phase 1 implementation.
- **Wire format for line/region directives:** new prefix letters
  vs. extending existing ones. Defer to per-round design.
- **`md2spans` source language:** Go (lives in `cmd/md2spans/`) or
  external? Go keeps it in this repo and shareable; external
  decouples release cycles. Go is the lower-friction default;
  revisit if a non-Go alternative becomes attractive.
- **Source-map / link-map protocol:** how `md2spans` communicates
  source ↔ rendered position mapping back to edwood. Almost
  certainly a new protocol directive; design lives in its own
  Phase 3 round.

## Status

- This document captures the direction agreed in April 2026
  following the architect review of `rich/` (P1-5, P1-6
  findings). Phase 1 is not yet started.
- Phases 1-4 will each produce their own design docs / plans
  under `docs/designs/features/` and `docs/plans/` per the project
  convention.
