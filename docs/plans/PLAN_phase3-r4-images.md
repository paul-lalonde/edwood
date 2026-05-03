# Phase 3 Round 4 — Inline Images — Plan

First protocol round of Phase 3 that touches BOTH the
spans-protocol surface AND the rich.Frame rendering pipeline.
Adds a `placement=NAME` flag namespace on `b` directives so
md2spans can render inline images without consuming the
source `![alt](url)` text. Follows the principle "protocol
expresses intent, not pixel placement" — layout decisions stay
in the renderer; future placements extend the value
vocabulary, not the wire-format flag set.

**Base design**: [`docs/designs/features/phase3-r4-images.md`](../designs/features/phase3-r4-images.md).

**Branch**: `phase3-r4-images`.

**Outcome**: edwood renders inline images below the line
containing their `![alt](url)` source. Source stays visible
alongside the image. Width from the user's title `width=Npx`
flows through as a payload parameter; absent that, the
renderer uses the image's intrinsic dimensions (probed via
the existing async cache). md2spans does no file IO.

**Files touched**:
- `spanstore.go` — `StyleAttrs.BoxPlacement string` field +
  `Equal()`.
- `spanparse.go` — parse `placement=NAME` flag on `b` lines;
  enforce `length=0` when set to `below`; recognize `0 0` W/H
  as legal.
- `rich/style.go` — new `Style.ImageBelow bool` field.
- `rich/layout.go` — line-height accounts for `ImageBelow`
  boxes additively (text + image stack heights).
- `rich/frame.go` — paint phase renders `ImageBelow` boxes
  below the line.
- `wind.go:boxStyleToRichStyle` — map `BoxPlacement="below"`
  → `Style.ImageBelow`; tokenize `BoxPayload` and apply
  `width=N` to `Style.ImageWidth`.
- `wind.go:initStyledMode` — wire `WithRichTextBasePath`
  (parity bug-fix with `previewcmd`).
- `cmd/md2spans/parser.go` — `![alt](url ...)` tokenizer.
- `cmd/md2spans/emit.go` — `Span.IsBox` discriminator;
  format `b` lines with `placement=below`.
- `docs/designs/spans-protocol.md` — document
  `placement=NAME`, the `0 0` W/H sentinel, and the payload
  parameter convention.
- `cmd/md2spans/README.md` — image entry in v1 scope table.
- Tests at every layer.

---

## Phase 3.4.0: Plan + design

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | phase3-r4-images.md drafted (revised) | [base doc] | Decisions: (a) `placement=NAME` flag namespace; (b) image renders below line, growing line height additively; (c) source stays visible; (d) WIDTH=0 HEIGHT=0 sentinel = "renderer probes"; (e) `width=N` flows via payload param; (f) md2spans does no file IO; (g) verbatim URL passthrough — consumer resolves via basePath; (h) `initStyledMode` basePath bug fix in scope. |
| [x] Tests | n/a (planning) | — | — |
| [x] Iterate | This plan + revised design | — | This file. |
| [ ] Commit | — | — | `Add Phase 3 round 4 design and plan: inline images` |

## Phase 3.4.1: Protocol — `BoxPlacement` on `StyleAttrs`

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Add `BoxPlacement string` field; Equal() includes it. Default "" = replacing semantic. | base doc § "StyleAttrs change" | String, not bool — extensible value space. |
| [x] Tests | Equal() with same/different BoxPlacement; default zero value | `spanstore_test.go` | — |
| [x] Iterate | Add field; Equal() includes it | `spanstore.go` | — |
| [x] Commit | — | — | `spans: add BoxPlacement field to StyleAttrs` |

## Phase 3.4.2: Parser — recognize `placement=NAME` flag and `0 0` W/H

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Single-token flag with namespaced value; unknown values rejected (mirrors `family=NAME`); WIDTH=0/HEIGHT=0 already legal but newly canonical for "renderer probes" — no parser change needed there. Length is unconstrained: placement=below covers source runes that render as text; placement=replace covers runes that render as the box. | base doc § "Wire-format change" | Mirrors round 2's `parseFamilyFlag` shape. The original draft required length=0 for placement=below; pivoted mid-round (commit dcb7323). |
| [x] Tests | placement=below over length-N round-trips; unknown placement= rejected; placement=replace explicit form OK; coexistence with bold/italic/scale/family; W=H=0 with placement=below; absent flag → BoxPlacement=""; payload-with-params round-trips | `spanparse_test.go` | — |
| [x] Iterate | Add `parsePlacementFlag` helper (validFamilies-style closed set); plumb into parseBoxLine flag switch | `spanparse.go` | — |
| [x] Commit | — | — | `spans: parse placement=NAME flag on b directives` |

## Phase 3.4.3: rich — `Style.ImageBelow` field + layout/paint

This row is the meatiest. Splits into three sub-rows. Each
sub-row gets its own commit.

### 3.4.3a: Add the field

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Add `ImageBelow bool` to rich.Style; document the contract (anchored to line, paints below text, grows line height additively) | base doc § "Rendering" | One field, no behavior. |
| [x] Tests | DefaultStyle().ImageBelow=false; can compose with Image/ImageURL fields | `rich/style_test.go` | — |
| [x] Iterate | Add field + doc comment | `rich/style.go` | — |
| [x] Commit | — | — | `rich: add Style.ImageBelow field for non-replacing image boxes` |

### 3.4.3b: Layout — line height accounts for ImageBelow boxes

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | A line containing one or more `ImageBelow` boxes has effective `Height = textHeight + sum(image_heights)`. `ImageBelow` boxes contribute zero horizontal advance. Existing inline-replacing images keep `Height = max(text, image)` semantics. | base doc § "Rendering" item 1 | Layout loop accumulates separately; finalizes at every line-end. |
| [x] Tests | Line height with one ImageBelow; with two stacked; reset across newlines; no horizontal advance; coexistence with inline-replacing image on same line | `rich/layout_test.go` | — |
| [x] Iterate | Add `imagesBelowHeight` accumulator alongside existing `actualLineHeight`; ImageBelow boxes take an early-exit branch (Wid=0, append at xPos, accumulator +=); finalize line height as `actualLineHeight + imagesBelowHeight` at every close-out (newline, wrap-fits, wrap-split, end-of-loop) | `rich/layout.go` | — |
| [x] Commit | — | — | `rich: layout grows line height to fit ImageBelow boxes` |

### 3.4.3c: Paint — render ImageBelow boxes below the line

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Extension of the image paint phase: `paintLineImagesBelow` walks the line after inline-image painting and draws each ImageBelow at `(c.offset.X, line.Y + textHeight + cumulative)` via the existing `paintImageBox` helper with a synthesized Line | base doc § "Rendering" item 2 | Reuses `box.ImageData` + existing draw helpers. |
| [x] Tests | One ImageBelow paints below text; two stack with >= imageHeight spacing; ImageBelow on second line uses that line's Y | `rich/frame_test.go` | — |
| [x] Iterate | Add `paintLineImagesBelow` + `lineTextHeight` helper; route ImageBelow boxes out of the inline-image branch | `rich/frame.go` | — |
| [x] Commit | — | — | `rich: paint ImageBelow boxes below the line containing their offset` |

## Phase 3.4.4: `boxStyleToRichStyle` plumbs placement + payload params

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | `BoxPlacement="below"` → `Style.ImageBelow=true`. Payload tokenizer: split on whitespace; first token `image:URL` → `Style.ImageURL`; subsequent `key=value` tokens → field overrides (v1: `width=N` → `Style.ImageWidth`). Unknown params silently ignored. | base doc § "boxStyleToRichStyle change" + § "Payload parameters" | Tokenizer is small; encapsulated in helper. |
| [x] Tests | Placement passthrough; replace-explicit no-op; URL passthrough; width=N override; unknown params ignored; multiple params; payload-width-wins-over-wire-BoxWidth; invalid width=abc ignored | `wind_styled_test.go` | — |
| [x] Iterate | Add `applyImagePayload` helper; wire into boxStyleToRichStyle; map BoxPlacement="below" → ImageBelow | `wind.go` | — |
| [x] Commit | — | — | `wind: route BoxPlacement to ImageBelow; parse payload params` |

## Phase 3.4.5: `initStyledMode` — basePath wiring (bug fix)

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Mirror previewcmd's basePath wiring (wind.go:2587-2595) into initStyledMode so images with relative paths resolve correctly | base doc § "Path resolution" + bug-fix note | Same class as the rounds 1/2 missing-font-load bug. |
| [x] Tests | basePath non-empty after initStyledMode; basePath is absolute form of body file's name | `wind_styled_test.go` | — |
| [x] Iterate | Add the option to initStyledMode | `wind.go:initStyledMode` | — |
| [x] Commit | — | — | `wind: initStyledMode wires basePath for relative image resolution` |

## Phase 3.4.6: md2spans — parser tokenizes image syntax

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Detect `![alt](url)` and `![alt](url "title")`; produce a single box record (length=N covering the source `![alt](url ...)` runes, W=H=0, placement=below, payload `image:URL [width=N]`). Source markers stay visible because the box's covered runes render as text. | base doc § "md2spans parser change" | Tokenizer slot before the link tokenizer. Originally drafted with length=0; pivoted mid-round (commit dcb7323). |
| [x] Tests | Basic image; with title; with alt empty; with `width=Npx`; image mid-paragraph; multiple per paragraph; image adjacent to link; URL with slashes; malformed forms fall to literal | `cmd/md2spans/parser_test.go` | — |
| [x] Iterate | Add Span.IsBox + box fields + BoxPlacement + BoxPayload; tryImage tokenizer + findInlineImage + parseImageWidthPx helpers | `cmd/md2spans/parser.go` | — |
| [x] Commit | — | — | `md2spans: tokenize image syntax and emit non-replacing box record` |

## Phase 3.4.7: md2spans — emit `b` lines with `placement=below`

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | FormatSpans branches on IsBox; emits `b OFF LEN 0 0 - - placement=below image:URL [width=N]`; fillGaps handles length-N boxes via the existing styled-span path | base doc § "md2spans emit change" | Shared style-flag formatter applies to both `s` and `b`. |
| [x] Tests | Box at start / middle of buffer; with width=N param; adjacent to styled span; explicit placement=replace round-trip; empty BoxPlacement omitted | `cmd/md2spans/emit_test.go` | Post-pivot: tests use length=N covering source runes. |
| [x] Iterate | Split FormatSpans into writeSpanLine + writeBoxLine + writeStyleFlags; fillGaps passes IsBox/Box* fields through (no length-0 special case) | `cmd/md2spans/emit.go` | — |
| [x] Commit | — | — | `md2spans: emit placement=below b directive for inline images` |

## Phase 3.4.8: Spec + README

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | n/a (doc) | — | — |
| [x] Tests | n/a (doc) | — | — |
| [x] Iterate | spans-protocol.md gains `placement=NAME` doc, the `0 0` W/H sentinel, and the payload parameter convention; md2spans README v1 scope table flips Images to ✓ with caveats | — | — |
| [x] Commit | — | — | `docs: spans protocol gains placement= flag, W=H=0 sentinel, payload params` |

## Phase 3.4.9: Smoke test + merge prep

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | n/a (validation) | — | — |
| [x] Tests | All 26 packages green | `go test ./...` | — |
| [x] Iterate | Build binaries; smoke-tested in real edwood with `test_images.md` containing multiple `![alt](url)` references. Initial result was no images rendered — prompted the length-0 → length-N pivot. Re-smoke-tested post-pivot: source markers visible above rendered images, as designed. | — | User-driven; pivot landed in commit dcb7323. |
| [x] Commit | — | — | Pivot commit + design/plan refresh. |

---

## After this round

Round 4 closes the flat-extension phase. Round 5 introduces
the FIRST region directive (block code) — a substantially
different protocol shape. md2spans's per-rune flag/box model
gets push/pop semantics with parameters.

The protocol-shape principles established in round 4
(intent-not-pixels, namespaced flag values, payload
parameters) should continue to guide rounds 5-8.

## Risks

(See base design doc.) Layout-height-grows-additively is the
main concern; landed before paint (3.4.3b before 3.4.3c) to
verify line math holds before drawing.

## Status

Round complete. All 9 plan rows landed plus a mid-round
pivot (length-0 anchor → length-N source-covering, commit
dcb7323) driven by smoke-test feedback. Source markers
stay visible above rendered images as designed. All 26
packages green; both binaries (./edwood, ./md2spans) build
clean. Ready for review + merge to master.
