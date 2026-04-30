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
and merge separately. The first few rounds are flat (per-rune /
per-line scope); rounds from blockquote onward are *region-shaped*
(see "Nested layout: regions, not line decorations" below for the
underlying constraint). Likely sequence (cheapest first):

1. Font scale (headings) — flat.
2. Font family selector (inline code, code blocks) — flat.
3. Inline rule / horizontal rule — flat (line-level).
4. Slide-break / viewport-snap marker — flat (document-level).
5. Inline replaced images via the existing box mechanism (already
   reachable; just needs `md2spans` support) — flat.
6. Block code — region (`begin region code` ... `end region`):
   adds the simplest region primitive, with full-line background
   as the only decoration. A useful test bed before blockquote.
7. Blockquote — region (`begin region blockquote` ... `end
   region`): nested, with indent + left-edge bar. Validates the
   push/pop semantics.
8. Lists — region per list item, with indent + bullet/number
   prefix. Often nested inside blockquotes.
9. Tables — region with cells; the largest round, requires the
   layout-space-introspection mechanism (window dimensions
   exposed via 9P) and either two-pass cell measurement or
   externally-computed column widths.

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

Plus a set of layout primitives (Phase 3 additions, mix of
flat and region-shaped):

| Primitive | Shape | Purpose |
|---|---|---|
| Inline rule (line-spanning) | flat (per-line) | `<hr>` / `***` markers |
| Document-level slide-break marker | flat (per-document) | Slide preview viewport snap |
| `begin region` / `end region` (with kind + params) | region (push/pop stack) | Blockquote, code block, list item, table — anything with reduced content width and bounding-box semantics |
| Frame-dimension introspection | external read | Tables that need to know available width for column layout |

The line-level "indent / background / left-bar" primitives are
*emitted by the renderer* as side effects of region parameters,
not directly by the external tool. See "Nested layout: regions,
not line decorations" below.

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

| Primitive | Today | Phase 3 round | Shape | New protocol surface |
|---|---|---|---|---|
| Foreground / background | yes | — | flat | — |
| Bold / italic | yes | — | flat | — |
| Font scale | no | 1 | flat | `Scale float32` on StyleAttrs (or new prefix) |
| Font family | no | 2 | flat | `Family string` on StyleAttrs (`""`/`"code"`/...) |
| Inline replaced box | yes | — | flat | — |
| Inline image | yes (via box payload `image:...`) | — | flat | — |
| Inline rule | no | 3 | flat | New box payload kind (`rule:width:height`) or new prefix |
| Slide-break | no | 4 | flat | New document-level directive: `slide offset` |
| Inline image (md2spans support) | yes | 5 | flat | tool work only |
| Block code | no | 6 | **region** | `begin region code` ... `end region`; first region primitive |
| Blockquote | no | 7 | **region (nested)** | `begin region blockquote indent=N` with left-bar param |
| Lists | no | 8 | **region per item** | `begin region listitem indent=N marker=...` |
| Tables | no | 9 | **region with cells** | `begin region table` + cell sub-regions; needs frame-dimension introspection |
| Frame-dimension introspection | no | 9 (paired with tables) | external read | new 9P file e.g. `/mnt/acme/<winid>/dim` |

The exact wire format for each is a Phase-3-round design problem,
not this doc's. The region-shaped rounds (6-9) all share a common
`begin region <kind>` / `end region` mechanism with kind-specific
parameters; designing that mechanism is part of round 6 (the
simplest region) and reused in subsequent rounds. See the next
section for why those entries can't be expressed as flat
per-line primitives.

## Nested layout: regions, not line decorations

Most of the markdown rendering primitives map cleanly onto flat,
linear protocol additions: per-rune attributes (font scale, font
family), per-line directives (horizontal rule, slide-break
marker), or document-level markers. Bold, italic, color, code
font, headings, hrules, slide breaks, inline images — all flat.

Three features are NOT flat: **blockquote, code block, table**
(and arguably nested lists). They share a property the flat
primitives can't express:

- Their **content has a reduced layout width** — a blockquote at
  indent 20px wraps inside the remaining width, not the frame
  width.
- Their **bounding box matters** to outer concerns — scroll
  handles, hyperlink hit-tests on the region, alignment of
  outer content relative to the block's actual height.
- They **nest** — a blockquote inside a blockquote, a code
  block or table inside a blockquote.

A "per-line indent" decoration can't express any of these
correctly. The renderer needs layout state that pushes when the
block begins and pops when it ends, and the external tool emits
content sequentially without computing wrap itself.

### Proposed shape: region operations

The protocol gains region directives:

```
begin region <kind> [params...]    (e.g.  begin region blockquote indent=20)
... spans, boxes, nested begins ...
end region
```

`begin region` pushes a layout context onto a stack:

- The renderer reduces the available content width by the
  region's left-indent.
- The renderer remembers the region's decoration spec (left-bar
  color/width for blockquote, full-line background for code
  block, cell layout for table).
- Subsequent spans inside the region are laid out inside the
  pushed context — wrap honors the reduced width, font scale
  remains in effect from outer state, etc.

`end region` pops:

- The renderer records the region's bounding box (start_y,
  end_y, left_x, right_x) for any consumer that needs it
  (scroll handles, region-level hyperlinks, table-column
  alignment).
- Layout state returns to the outer context.

Regions nest. Two depths of blockquote → two pushes → indent
adds up. A code block inside a blockquote → outer push for
blockquote's indent + bar, inner push for code block's
background.

The external tool emits region begin/end semantically (markdown
syntax already groups by region: `>` runs, ` ``` ` fences, `|`
table cells). It does NOT compute wrap or pixel positions — that
remains the renderer's job, which is the renderer's strength.

### Layout-space introspection

For tables specifically, the external tool benefits from knowing
the available width — to decide column proportions, or to drop
columns entirely on narrow displays. Expose window/frame
dimensions via a new 9P file (e.g.,
`/mnt/acme/<winid>/dim` returning `<width> <height>` in pixels,
or extending the existing `info` file). Read-only, cheap,
optional. Tables that don't need it (e.g., tables where md2spans
trusts the renderer's two-pass layout) ignore it.

### Implications for the gap analysis table above

The line-level entries for "Per-line indent", "Per-line
background", and "Left-edge bar" in the gap analysis table are
*placeholders*. The actual Phase 3 design for blockquote / code
block / lists / tables will introduce region operations
instead. A region's per-line side effects (indent, bar, bg) are
emitted by the renderer based on the region's parameters, not
by the external tool per line.

The table is honest about being placeholder; the underlying
work is "what region kinds, what params, what wire format" —
deferred to per-round Phase 3 design docs starting with code
block (round 6).

### Implications for Phase 1

Phase 1 does NOT design the region protocol. The wrapper at
Phase 1 still consumes the existing `Style.Blockquote*`,
`Style.Block`, `Style.Table*`, `Style.ListItem`, `Style.HRule`
fields the same way the in-tree path does today — those fields
drive `rich/layout.go`'s width / indent / wrap decisions
unchanged. Phase 1's blockquote-bar painting (row 1.2) is just
a paint move; the blockquote layout (indent + reduced width)
already works because `layout.go` reads those Style fields.

Region protocol design begins at Phase 3 round 6 (block code,
the simplest region) and is refined at rounds 7-9 as the
abstraction proves itself.

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
2. **Region protocol design.** Blockquote, code block, lists,
   and tables all need region (push/pop) semantics — see "Nested
   layout: regions, not line decorations". The region protocol
   is designed at round 6 (block code, the simplest region) and
   refined through rounds 7-9. If round 6's design proves
   insufficient when round 7 (blockquote, with nesting) lands, we
   either revise the protocol or accept that early round 6
   consumers see a small breaking change. Mitigation: design
   round 6 with nesting and parameter extensibility in mind from
   day one even though block code itself doesn't nest. Open
   question: how parameter passing works on `begin region`
   (positional vs. key-value) — settle at round 6.
3. **Tables don't fit the region model trivially.** Real table
   layout needs column-width measurement that depends on
   rendering metrics edwood owns. The region model handles cells
   as nested regions, but column widths must be agreed between
   `md2spans` and edwood. Either `md2spans` pre-computes widths
   using the frame-dimension introspection file (assuming it has
   font metrics or queries them), or edwood does a two-pass cell
   measurement during table region close. This is the highest-
   risk Phase 3 round and will need its own design doc.
4. **Source map / link map.** The current preview tracks markdown
   source ↔ rendered-rune mapping for click-to-source navigation.
   `md2spans` will need to emit this mapping (probably to an
   adjacent file or via a new protocol directive). Design TBD;
   Phase 3 may need an extra round for this if Look navigation
   regresses.
5. **External tool installation friction.** `md2spans` needs to be
   on the user's `$PATH` for preview to work post-Phase-4. The
   syntax-coloring story has this same constraint and it's
   accepted; markdown will follow the same pattern.
6. **Round ordering.** The dependency graph between Phase 3 rounds
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
