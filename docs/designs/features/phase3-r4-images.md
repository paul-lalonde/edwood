# Phase 3 Round 4 — Inline Images — Design

## Purpose

Fourth round of Phase 3 of the markdown-externalization plan
([markdown-externalization.md](markdown-externalization.md)).
Add md2spans support for CommonMark inline images
(`![alt](url)` and `![alt](url "title")`) by extending the
spans protocol with a **`placement=NAME` flag namespace** on
`b` directives, plus rendering support for "image laid out
below the source line."

The original round 4 plan called this "tool work only via the
existing box mechanism." That framing did not survive design:
the existing `b` directive replaces source runes by spec, and
the user's markup-stays-visible preference (established
through rounds 1-3) requires that the source `![alt](url)` text
remain readable alongside the rendered image. Round 4 therefore
becomes a small protocol round: one new flag namespace, one
consumer rendering extension, one md2spans tokenizer.

## Design principle: protocol expresses intent, not pixel placement

A central goal of round 4's protocol shape is to keep
**layout decisions in the renderer**, not in the wire format.
Window resizes, font changes, theme changes, and accessibility
adjustments must NOT require producer involvement. The
producer (md2spans) emits semantic intent ("render this image
near this offset, below the source line"); the renderer
translates intent into pixels using its knowledge of font
metrics, frame dimensions, image intrinsics, and user
preferences.

Three concrete consequences:

1. **Use a namespaced `placement=NAME` flag**, not a binary
   bool flag like `noreplace`. New layout modes in future
   rounds extend the value vocabulary, not the wire-format
   flag set. Same shape as round 2's `family=NAME` and round
   1's `scale=N.N`.
2. **`WIDTH=0 HEIGHT=0` is the canonical "renderer probes"
   sentinel.** Producers that don't know the image's
   intrinsic dimensions emit `0 0`; the renderer loads the
   image and uses intrinsic dims. Existing producers that
   want pixel-exact dims (e.g., a future tool emitting
   replaced-content boxes) keep using positive W/H.
3. **Image-attribute hints flow via payload parameters**, not
   wire-format flags. A user's `![cat](pic.png "width=200px")`
   becomes `b ... image:./pic.png width=200` — the W=200
   parameter sits in the box's payload. Future params
   (`alt=...`, `caption=...`, `align=...`) extend the
   payload, not the wire format.

These three rules together mean: future image features extend
the payload's parameter vocabulary; future placements extend
the `placement=` value vocabulary; the `b` directive's
positional fields stay stable.

## Background — what the in-tree path does

`markdown/inline.go:78-126` parses `![alt](url)` and produces a
single `rich.Span` whose Text is replaced with `[Image: alt]`
(or `[Image]` when alt is empty), styled as a link, with
`Style.Image=true`, `Style.ImageURL=url`,
`Style.ImageWidth=parseImageWidth(title)` (parsing
`width=Npx` from the title attribute, `markdown/parse.go:1171`).
The source text is conceptually replaced by the placeholder
string at parse time.

In md2spans we cannot replace text in the buffer (the body is
the user's typed source). We have two protocol primitives:
`s` (style this rune range) and `b` (replace this rune range
with a fixed-pixel box). To keep the source visible AND
render an image, we extend `b` with a non-replacing placement
mode.

## Wire-format change

```
b 12 0 0 0 - - placement=below image:./pic.png width=200
```

Adds a single new flag — `placement=NAME` — to the `b`
directive. v1 recognized values:

- `replace` (the existing implicit default; the box's
  `length` runes are replaced by the box at render time)
- `below` (Phase 3 round 4; the box does not consume runes
  and renders below the line containing its `offset`)

When `placement=below`:
- `length` MUST be 0. A producer emitting `length>0` plus
  `placement=below` is a protocol error.
- The box does NOT consume source runes. The rune at
  `offset` is unaffected by the box's presence; `s`
  directives covering that range render normally.
- The renderer anchors the box to the LINE containing
  `offset`, paints below the line text, and grows line
  height by image height (see "Rendering" below).

When `placement=replace` (or no `placement=` flag):
- Existing `b` semantics apply unchanged. Round 4 does NOT
  modify replacing-box behavior.

Per-write contiguity: a `placement=below b` line with
`length=0` satisfies the contiguity rule trivially — the
next directive's offset must equal `offset + 0 = offset`,
i.e., continue from the same position. Two
`placement=below b` lines may share an offset (e.g., two
images anchored at the same paragraph end). The consumer
paints them stacked, top-down in emission order.

Unknown `placement=` values are an error (same as `family=`
unknown values). Future rounds extend the value vocabulary
through this spec; until then, the parser rejects.

### `WIDTH=0 HEIGHT=0` semantics

When `WIDTH` and `HEIGHT` are both 0:
- The renderer ignores them and uses the image's intrinsic
  dimensions (loaded via the existing `box.ImageData`
  pipeline).
- A producer that knows nothing about image dims (e.g.,
  md2spans v1) emits `0 0` and lets the renderer probe.

When either is positive:
- The existing renderer behavior applies (the dim is used as
  an override on top of the image's intrinsic size, scaled
  proportionally).

This rule is *additive*: existing producers that always emit
positive W/H continue to work. md2spans is the first
producer that defaults to `0 0`.

### Payload parameters

The `<payload>` field of `b` already accepts arbitrary
trailing tokens (`spans-protocol.md` §"`b` — Box":
"optional trailing tokens preserved verbatim as the box's
payload string"). Round 4 imposes a soft convention WITHOUT
changing the wire format:

- The first payload token is the URL spec, e.g.,
  `image:./pic.png`. Conventions established by existing
  consumers continue (`image:` prefix → image rendering).
- Subsequent payload tokens are `key=value` pairs interpreted
  by the consumer. v1 recognizes:
  - `width=N` — explicit pixel width override (parity with
    in-tree path's `width=Npx` title attr; `px` suffix
    dropped on the wire — pure integers).
  - Future: `alt=ENCODED`, `caption=ENCODED`, `align=NAME`,
    etc. (all opaque to the parser; consumer-interpreted.)
- The spec parser passes the full payload string verbatim
  into `StyleAttrs.BoxPayload`; the consumer
  (`wind.go:boxStyleToRichStyle`) tokenizes and applies.
- Unknown payload params are silently ignored by v1 (graceful
  forward-compat for future params on older renderers).

## `StyleAttrs` change

```go
type StyleAttrs struct {
    // ... existing fields ...
    BoxPlacement string  // "" (default = replace), "replace", "below"
}
```

`Equal()` includes `BoxPlacement`. Storage stays minimal —
empty string is the default; explicit `replace` is also
stored as `"replace"` (renderer treats both as the existing
replacing semantic).

## `boxStyleToRichStyle` change

```go
if sa.BoxPlacement == "below" {
    s.ImageBelow = true
}
// Parse payload params after the URL.
applyPayloadParams(&s, sa.BoxPayload)
```

A new `rich.Style.ImageBelow bool` field signals to the
layout/paint pipeline that this image renders below the line
rather than inline. The existing `s.Image`, `s.ImageURL`,
`s.ImageWidth`, `s.ImageHeight` fields all continue to apply.

`applyPayloadParams` tokenizes the payload, finds the first
`image:URL` token, then walks subsequent `key=value` tokens
and applies `width=N` to `Style.ImageWidth`. Unknown params
are silently ignored.

We use `bool` for the renderer-internal flag (matching the
`Bold`/`Italic`/`Code`/`HRule` pattern) rather than a string
discriminator. Future renderer-internal placements add
new bool fields or refactor to a single field at that time;
the protocol's `placement=NAME` extensibility is independent.

## Rendering — image-below-line layout

The non-replacing image is anchored at a rune offset on a
line. The renderer:

1. **Layout**: a line containing one or more `ImageBelow`
   boxes has its effective height computed as
   `text_height + sum(image_heights)`. `ImageBelow` boxes
   contribute zero horizontal advance.
2. **Paint**: the image-painting pass renders each
   `ImageBelow`-flagged box at:
   - `X = line.X` (left edge of the line; the offset within
     the line determines stacking order, not horizontal
     position).
   - `Y = line.Y + text_height + sum(prior image_heights on
     this line)`.
   - dimensions from `imageBoxDimensions` (existing helper
     at `rich/layout.go:181`) — uses
     `box.ImageData.Width/Height` (intrinsic) by default,
     overridden by `Style.ImageWidth`/`ImageHeight` when set.
3. **Multiple images on one line**: stacked top-to-bottom in
   emission order. v1 keeps it simple — no horizontal
   layout of images.
4. **Line wrap**: text wrapping ignores `ImageBelow` boxes;
   the line's text content wraps as if the image weren't
   there. The image is anchored to the LINE that contains
   its offset, not to the visual position of the offset
   within wrapped text.
5. **Image not yet loaded**: line height grows by the cached
   `box.ImageData.Height` if available, else by a
   per-Renderer fallback (e.g., 150px). Async loads that
   complete after layout trigger a re-layout via the
   existing `onImageLoaded` callback path
   (`rich/layout.go:880`).

This sits in `rich.Frame`, not `rich/mdrender`. Image handling
is in the lean `rich.Frame` contract per
[markdown-externalization.md](markdown-externalization.md#lean-richframe-contract).
The wrapper is not involved.

## md2spans parser change

Recognize image syntax in `parseParagraph`'s inline tokenizer.
Pattern (greedy, matches in-tree path's recognition):

```
![ALT](URL)
![ALT](URL "TITLE")
```

Where:
- ALT is any text not containing `]` (no nesting in v1).
- URL is any text not containing `)` or whitespace before a
  `"TITLE"` field.
- TITLE is the optional double-quoted title (used for
  `width=Npx`).

When detected, md2spans emits TWO records:
1. An `s` span over `![alt](url ...)` with default styling
   (Fg="", no flags). This keeps the source text visible.
2. A `b` "span" (extended Span shape — see below) with:
   - `Offset = end of the source `![alt](...)` syntax`
   - `Length = 0`
   - `BoxWidth = 0, BoxHeight = 0` — the renderer probes.
   - `BoxPlacement = "below"`
   - `BoxPayload = "image:" + url`, plus an optional
     ` width=N` token if the title attr contains
     `width=Npx`. URL passed through verbatim — relative
     paths stay relative; the consumer resolves against the
     window's basePath.

The image syntax is recognized BEFORE the link syntax (`[..](..)`)
in the inline tokenizer, since `!` is the discriminating
character.

If the parsing pattern doesn't match (unclosed brackets, no
`(URL)`), the `!` is treated as literal text — no image span,
no special handling. Same fallback as the in-tree path.

**md2spans does NOT probe image files in v1.** No file IO,
no image decoder imports. The renderer probes via its
existing async-cache pipeline.

## md2spans emit change

`Span` gains box fields and a discriminator:

```go
type Span struct {
    // ... existing fields ...
    IsBox        bool
    BoxWidth     int
    BoxHeight    int
    BoxPayload   string
    BoxPlacement string  // "below" or "" (default replace)
}
```

`FormatSpans` recognizes `IsBox=true` and emits:
```
b OFFSET LENGTH WIDTH HEIGHT FG BG flags... payload
```

instead of an `s` line. For round 4 image emission,
`WIDTH=0 HEIGHT=0` and `BoxPlacement="below"` give:
```
b 12 0 0 0 - - placement=below image:./pic.png
b 12 0 0 0 - - placement=below image:./pic.png width=200
```

`fillGaps` skips `IsBox` spans when computing default-fill
gaps — a box at `Offset=N, Length=0` does not bound any
text region, so it sits "between" two text spans without
splitting their coverage.

## Path resolution

md2spans passes the URL VERBATIM as part of the `image:`
payload — relative paths stay relative. The consumer
(`rich/layout.go:891` `resolveImagePath`) resolves them
against the window's `basePath` (the markdown file's path).

**Bug fix required**: `wind.go:initStyledMode` does NOT
currently set `WithRichTextBasePath` — only `previewcmd`
does. md2spans-styled images would fail to resolve relative
paths. Round 4 mirrors `previewcmd`'s basePath wiring
(`wind.go:2587-2595`) into `initStyledMode`.

## Failure modes

| Failure | Behavior |
|---|---|
| Path syntax doesn't match `![alt](url)` | `!` treated as literal text; no image, no `b` line |
| File missing / unreadable | Renderer falls back to a placeholder (existing image-loader behavior); line height grows by fallback height; alt text shows where the renderer chooses |
| File present but not a recognized image | Same as missing — renderer-side handling |
| Title has no `width=Npx` | Box emitted with no payload param; renderer uses intrinsic dims |
| URL is HTTP/HTTPS | Emit `b` with `image:URL`; consumer's loader handles HTTP per its existing logic (or renders broken-image; v1 doesn't change this) |
| Multiple images on one line | All emitted; consumer stacks below the line top-to-bottom |
| Image inside emphasis (`*![alt](url)*`) | v1: image takes precedence; emphasis around it produces no spans (emphasis tokenizer ignores `!` and `[`). Document as a v1 limitation. |

## Test plan

1. **`spanparse.go` tests**: `placement=below` flag round-trips on
   `b` lines; `length>0` plus `placement=below` is rejected;
   `length=0` plus `placement=replace` (or no flag) is the
   existing degenerate case (allowed); unknown `placement=`
   values rejected; `WIDTH=0 HEIGHT=0` parsed OK.
2. **`StyleAttrs.Equal` tests**: `BoxPlacement` participates
   in equality.
3. **`boxStyleToRichStyle` tests**: `BoxPlacement="below"` →
   `rich.Style.ImageBelow=true`; payload param `width=N`
   applied to `Style.ImageWidth`.
4. **rich.Frame layout tests**: a line containing an
   `ImageBelow` box has effective height grown by image
   height; horizontal advance unchanged; multiple stack.
5. **rich.Frame paint tests**: `ImageBelow` box renders at
   `(line.X, line.Y + textHeight)`; multiple stack
   top-to-bottom; the source `s` text on the same line still
   paints.
6. **md2spans parser tests**: image syntax with/without
   alt, with/without title, with `width=Npx`, multiple
   images per paragraph, image-not-at-start-of-line, image
   adjacent to other inline syntax (link, emphasis), unclosed
   brackets fall back to literal text.
7. **md2spans emit tests**: `b OFF 0 0 0 - - placement=below
   image:URL` format; `IsBox` spans skip `fillGaps`
   text-fill logic; `width=N` payload param emitted when
   title attr present.
8. **wind.go test**: `initStyledMode` sets `BasePath`
   matching `previewcmd`'s behavior.
9. **End-to-end smoke**: write a markdown with
   `![cat](images/cat.png)`, assert the image renders
   below the source line, source remains visible.

## Non-goals

- **Image inside emphasis** (e.g., `*![alt](url)*`): v1
  recognizes the image but not the surrounding emphasis.
- **Reference-style images** (`![alt][ref]`): defer with
  reference links to a future round.
- **Inline-replacing images** (the existing `b` semantic
  without `placement=below`): not emitted by md2spans v1;
  users who want the existing behavior keep using the
  in-tree path until Phase 4.
- **Horizontal layout of multiple images**: v1 stacks
  vertically only.
- **Image alignment** (`align=center` payload param): future
  payload param; v1 left-aligns to the line's content X.
- **Image scaling/clipping based on frame width**: v1 uses
  the renderer's existing `imageBoxDimensions` helper. An
  image wider than the frame clips at the frame's right
  edge (existing behavior for inline replaced images).
- **Image hover / link behavior**: existing
  `frame.ImageURLAt` machinery continues to work for
  inline-replacing images; `ImageBelow` boxes are not
  exposed via `ImageURLAt` in v1 (the rune offset they're
  attached to does NOT cover the image's visual extent).
- **Producer-side image dimension probe**: md2spans does
  not probe files. The renderer's existing async cache
  handles probing.

## Risks

1. **Layout impact: line height changes break vertical
   scroll math.** The frame's existing line-height accounting
   computes `max(textHeight, imageHeight)` for inline-
   replacing images. `ImageBelow` boxes need additive
   accounting (`textHeight + sum(imageHeights)`) — a
   different shape. The change touches the layout loop
   directly (no single helper to extend); refactor risk is
   moderate. Mitigation: layout (3.4.3b) lands before paint
   (3.4.3c) so we can verify line-height math holds before
   adding draw operations.
2. **Async image load timing.** When the renderer probes
   an image, the load completes asynchronously via
   `cache.LoadAsync`. Until the load completes, the image's
   intrinsic dimensions are unknown. Mitigation: existing
   `onImageLoaded` callback re-triggers layout when load
   completes; line height recomputes. Brief flicker on
   first render is acceptable.
3. **`initStyledMode` basePath fix uncovers other relative-
   path bugs.** The fix is a one-line addition in
   `initStyledMode`, mirrored on `previewcmd`. Risk that
   this exposes a downstream basePath-not-handled-correctly
   bug elsewhere is low; the existing `previewcmd` path is
   well-exercised.
4. **Multiple images on one line edge case.** Two images at
   the same offset on the same line should stack
   deterministically. Layout/paint code must walk boxes in
   emission order, not arbitrary order.
5. **Source-stays-visible may surprise users who expect
   "image-only" rendering.** Document in the README that
   v1 shows source AND image; pin the expectation.
6. **Payload-param convention is informal.** The `b`
   payload accepts free-form trailing tokens by spec. v1
   imposes the `image:URL [key=value...]` convention by
   reader's-side parsing in `wind.go`. A future round
   could formalize the parser side; v1 doesn't.

## Status

Design — drafted (revised after review). Awaiting plan refresh.
