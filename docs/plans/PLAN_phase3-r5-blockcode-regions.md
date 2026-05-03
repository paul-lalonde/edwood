# Phase 3 Round 5 — Block Code (Regions) — Plan

First **region** round of Phase 3. Adds `begin region` /
`end region` directives to the spans protocol, a sidecar
`RegionStore` on the consumer, and md2spans support for
fenced code blocks. The simplest region (`code`) — full-line
background, no nesting required by md2spans — used as the
test bed before the nested rounds (6 blockquote, 7 lists,
8 tables).

**Base design**: [`docs/designs/features/phase3-r5-blockcode-regions.md`](../designs/features/phase3-r5-blockcode-regions.md).

**Branch**: `phase3-r5-blockcode-regions`.

**Outcome**: edwood renders fenced code blocks (` ``` `) with
the existing in-tree-path visual (gutter indent, full-line
background, monospace font) when md2spans is the renderer.
The region machinery is the FIRST piece designed for nesting
on day one; rounds 6-8 extend the same machinery.

**Files touched**:
- `spanparse.go` — `parseSpanMessage` returns parsed regions
  alongside runs; new directive prefixes `begin` and `end`;
  region kind validation; param parsing.
- `region.go` (new) — `Region` and `RegionStore` types;
  forest construction; `EnclosingAt`; edit operations.
- `xfid.go:xfidspanswrite` — route parsed regions to the
  window's regionStore; clear regionStore on `c`.
- `wind.go` — `Window` gains `regionStore *RegionStore`;
  `buildStyledContent` consults regionStore to expand
  region kinds into per-rune Style flags.
- `cmd/md2spans/parser.go` — fenced-code-block detection
  in `scanParagraphs`; new `paragraphRange.IsCodeBlock`
  / `CodeLang` fields; new emit shape from `parseCodeBlockParagraph`.
- `cmd/md2spans/emit.go` — `Span` gains `RegionBegin`/
  `RegionEnd` discriminator-style fields, OR (alternatively)
  a separate slice of region directives interleaved with
  spans. To be settled at row 3.5.6's design step.
- `cmd/md2spans/main.go:writeChunked` — chunker honors region
  boundaries (don't split between a begin and its matching end).
- `docs/designs/spans-protocol.md` — document `begin region`
  / `end region`; describe region kinds and params.
- `cmd/md2spans/README.md` — flip "Block code with bg" to ✓
  and "Fenced / indented code blocks" to ✓ (fenced) /
  pending (indented).
- Tests at every layer.

---

## Phase 3.5.0: Plan + design

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | phase3-r5-blockcode-regions.md drafted | [base doc] | Decisions: (a) `begin region <kind> [params]` / `end region` as new wire-format directives; (b) region directives don't advance contiguity cursor; (c) sidecar tree-shaped RegionStore on consumer; (d) bridge translates regions to per-rune Style flags (X→Y plan); (e) v1 fenced code only; indented deferred. |
| [x] Tests | n/a (planning) | — | — |
| [x] Iterate | This plan + design | — | This file. |
| [ ] Commit | — | — | `Add Phase 3 round 5 design and plan: block code (regions)` |

## Phase 3.5.1: Region core types

Smallest first — define the data shape so subsequent rows
have something to integrate with.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | `Region` struct (Start, End, Kind, Params, Parent, Children); `RegionStore` with roots forest; basic ops (Add, EnclosingAt, Clear) | base doc § "Storage" | Forest, not flat list — required for round 6+ nesting. |
| [x] Tests | Region equality; store Add places regions correctly (top-level vs. nested vs. parent-after-child); deeply-nested case (round 6+); EnclosingAt finds deepest match + boundary cases; multiple-siblings disambiguation; Clear empties forest | `region_test.go` | — |
| [x] Iterate | Add region.go with types + ops; no integration yet | `region.go` (new) | — |
| [x] Commit | — | — | `regions: add Region and RegionStore types for sidecar region tree` |

## Phase 3.5.2: Region edits — Insert/Delete offset shift

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Insert(pos, length) shifts Start/End for regions at or after pos; Delete(pos, length) drops regions whose body is touched, with the exception that a parent shrinks instead of dropping when a child fully contained the delete (the child takes the hit). | base doc § "Region-store edits" | Mirror spanStore patterns; "proper body" rule for parent vs child. |
| [x] Tests | Insert at Start / inside / after / before; child shifts with parent; Delete before (shift) / after (untouched) / cross-boundary (drop) / fully-inside-body (drop) / drops-child-keeps-parent (the proper-body case) | `region_test.go` | — |
| [x] Iterate | Add Insert/Delete to RegionStore + applyInsert/applyDelete/filterDelete recursive helpers | `region.go` | — |
| [x] Commit | — | — | `regions: Insert/Delete operations for body edits` |

## Phase 3.5.3: Parser — `begin region` / `end region` directives

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | New directive prefixes `begin` and `end`; parseSpanMessage returns regions alongside runs; balanced begin/end within a write; kind validation; param parsing (key=value, malformed silently ignored) | base doc § "Wire-format change" + "Per-write rules" | Region directives don't advance the contiguity cursor. |
| [x] Tests | Balanced round-trip with span content; lang=NAME param; cursor invariant under wrapped spans; unmatched begin error; unmatched end error; unknown kind / missing kind / future kind error; malformed param silently ignored; nested begin/end; empty region (Start==End) | `spanparse_test.go` | — |
| [x] Iterate | Add region parsing to parseSpanMessage; parseBeginRegion / parseEndRegion helpers; validRegionKinds closed-set map; signature change adds `regions []*Region` return | `spanparse.go` | xfid.go's caller updated with `_` placeholder until row 3.5.4 wires regionStore. |
| [x] Commit | — | — | `spans: parse begin/end region directives` |

## Phase 3.5.4: Consumer integration — Window.regionStore

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Window gains regionStore field; xfidspanswrite routes parser-returned regions into regionStore via applyParsedSpans helper; `c` clears both stores via clearSpansAndRegions; text.go's Insert/Delete propagate to regionStore alongside spanStore | base doc § "Storage (xfid.go)" | applyParsedSpans / clearSpansAndRegions extracted as testable helpers. |
| [x] Tests | New window has nil regionStore; applyParsedSpans with regions populates it; applyParsedSpans without regions doesn't disturb it; clearSpansAndRegions empties both | `wind_styled_test.go` | — |
| [x] Iterate | Add regionStore field; applyParsedSpans + clearSpansAndRegions helpers; xfidspanswrite uses both helpers; text.go propagates Insert/Delete to regionStore | `wind.go`, `xfid.go`, `text.go` | — |
| [x] Commit | — | — | `wind: add regionStore for spans-protocol regions` |

## Phase 3.5.5: Bridge — buildStyledContent expands code regions

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | For each StyleRun, look up the enclosing region via regionStore.EnclosingAt; ancestor walk (deepest-up) ORs kind-specific flags. v1 case: `code` → Block + Code + Bg(InlineCodeBg). | base doc § "Bridge (wind.go)" | "Translate regions to per-rune flags at consume time" — the X-toward-Y plan. Ancestor walk lays groundwork for round 6+ stacked kinds. |
| [x] Tests | Run inside a code region picks up Block+Code+Bg; outer runs untouched; nil regionStore is a no-op; empty region (Start==End) doesn't affect any run | `wind_styled_test.go` | — |
| [x] Iterate | Add applyEnclosingRegions helper + EnclosingAt call in buildStyledContent's per-run loop | `wind.go` | Producer-responsibility contract documented: producer must emit separate s/b runs at region boundaries (md2spans satisfies naturally). |
| [x] Commit | — | — | `wind: expand region=code into Block/Code/Bg per-rune Style flags` |

## Phase 3.5.6: md2spans — fenced-code-block parser

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | scanParagraphs detects ` ``` ` fences; paragraphRange gains IsCodeBlock + CodeBodyRuneStart/End + CodeLang; body runes (incl. trailing \\n before closing fence) are the region's content; opening/closing fences stay outside the region for emit-time default fill | base doc § "md2spans Parser" | Indented code deferred. Span gains RegionBegin/RegionEnd/RegionParams sentinels for the wire-emission layer. |
| [x] Tests | Basic block; with language hint; multi-line body with rune counting; empty body (begin+end only); fenced block between paragraphs; unclosed fence runs to EOF | `cmd/md2spans/parser_test.go` | spansFieldEqual replaces == comparison (RegionParams is a map). |
| [x] Iterate | Add Span sentinel fields; paragraphRange code-block fields; isFenceLine + parseOpenFence helpers; scanParagraphs in-fence state machine; parseCodeBlockParagraph emits begin/body/end | `cmd/md2spans/parser.go` | — |
| [x] Commit | — | — | `md2spans: parse fenced code blocks` |

## Phase 3.5.7: md2spans — emit `begin region` / `end region`

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | FormatSpans separates region-directive sentinels from styled/box spans, anchors fillGaps to split default fills at directive offsets, merges by offset (directives emit immediately before same-offset styled spans). New writeBeginRegionLine + writeEndRegionLine. | base doc § "md2spans Emit" | Two-pointer offset-merge handles ordering; same-offset directives emit in input order (begin-before-end for empty regions). |
| [x] Tests | Basic block; with lang param; region at start (no leading fill); region at end (no trailing fill); empty region (begin+end at same offset) | `cmd/md2spans/emit_test.go` | — |
| [x] Iterate | uniqueDirectiveOffsets + fillGapsWithAnchors + isDefaultFill helpers; FormatSpans 2-way merge; writeBegin/EndRegionLine | `cmd/md2spans/emit.go` | — |
| [x] Commit | — | — | `md2spans: emit begin/end region directives for fenced code blocks` |

## Phase 3.5.8: md2spans — chunker honors region boundaries

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | writeChunked walks the payload tracking begin/end depth; the chunk ends at the latest depth-0 \\n at-or-before maxChunk; if no safe split exists, extend past maxChunk for a region (or error if simply line-too-long). | base doc § "Risk #2 (atomicity)" | Real fenced blocks << maxChunk; pathological case produces one big Twrite (still under typical 9P msize). |
| [x] Tests | Region straddling maxChunk lands balanced (begin/end count balanced per chunk); unclosed region at EOF errors; existing line-too-long error preserved | `cmd/md2spans/render_test.go` | — |
| [x] Iterate | Extract nextChunkEnd helper; track depth, lastSafe, newlineBeforeMax to distinguish "extend for region" vs "single line too long" | `cmd/md2spans/main.go` | — |
| [x] Commit | — | — | `md2spans: writeChunked honors region boundaries` |

## Phase 3.5.9: Spec + README

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | n/a (doc) | — | — |
| [ ] Tests | n/a (doc) | — | — |
| [ ] Iterate | spans-protocol.md gains `begin region` / `end region` directive sections; documents v1 kinds (code) and params (lang=NAME); md2spans README v1 scope flips "Block code with bg" to ✓ (fenced); notes indented as deferred | — | — |
| [ ] Commit | — | — | `docs: spans protocol gains region directives; md2spans handles fenced code` |

## Phase 3.5.10: Smoke test + merge prep

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | n/a (validation) | — | — |
| [ ] Tests | All packages green | `go test ./...` | — |
| [ ] Iterate | Build binaries; smoke-test with a markdown file containing 1-3 fenced code blocks of varying length and language; verify visual parity with in-tree path | — | User-driven. |
| [ ] Commit | — | — | n/a (no code change unless smoke surfaces something) |

---

## After this round

Round 5 establishes the region machinery. Rounds 6-8 extend it:
- **Round 6 (blockquote)**: nested regions; left-bar
  decoration; region-aware layout may become necessary.
- **Round 7 (lists)**: per-item regions; bullet/number
  prefixes; deeper nesting.
- **Round 8 (tables)**: regions with cells; needs the
  frame-dimension introspection 9P file.

The principles established here (intent-not-pixels,
namespaced kind values, payload-style param syntax, sidecar
forest storage) carry forward.

## Risks

(See base design doc.) The rendering bridge's
"per-rune flag expansion" approach is the major risk;
round 6 may force a region-aware layout pass. Round 5
commits to v1 of the bridge knowing this.

## Status

Plan + design drafted. Awaiting review before any code.
