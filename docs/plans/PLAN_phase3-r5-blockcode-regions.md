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
| [ ] Design | For each StyleRun, look up the enclosing region via regionStore.EnclosingAt; if kind=`code`, OR Style.Block=true, Code=true, Bg=InlineCodeBg into the resulting rich.Span's style | base doc § "Bridge (wind.go)" | "Translate regions to per-rune flags at consume time" — the X-toward-Y plan. |
| [ ] Tests | Region of kind `code` over body runes produces spans with Block/Code/Bg flags set; runes outside the region are unaffected; nested-region case (synthetic — round 5 doesn't produce nesting, but bridge must handle it for round 6+); empty region produces no spans | `wind_styled_test.go` | — |
| [ ] Iterate | Add region-expansion logic to buildStyledContent | `wind.go` | — |
| [ ] Commit | — | — | `wind: expand region=code into Block/Code/Bg per-rune Style flags` |

## Phase 3.5.6: md2spans — fenced-code-block parser

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | scanParagraphs detects ` ``` ` fences; paragraphRange gains `IsCodeBlock` and `CodeLang` fields (or new ranges struct); body runes between fences are the region's content; opening/closing fences stay visible (markup-stays-visible). | base doc § "md2spans Parser" | Indented code deferred. |
| [ ] Tests | Basic ` ```\nbody\n``` `; with language hint ` ```go\n...\n``` `; multi-line body; body containing inline-code backticks (single, doesn't conflict); unclosed fence (treat as code-to-EOF, matches CommonMark); fence inside a paragraph (must follow blank-line rule) | `cmd/md2spans/parser_test.go` | — |
| [ ] Iterate | Detect fences; emit Span with appropriate fields; design how regions are represented in the Span list (likely an additional sentinel-Span discriminator or a parallel region list — to settle here) | `cmd/md2spans/parser.go` | — |
| [ ] Commit | — | — | `md2spans: parse fenced code blocks` |

## Phase 3.5.7: md2spans — emit `begin region` / `end region`

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | FormatSpans emits `begin region code [lang=NAME]` before the body's `s` lines and `end region` after. Region directives slot in the wire output without breaking contiguity. | base doc § "md2spans Emit" | Mirrors row 3.4.7's writeBoxLine split — likely a new writeBeginRegionLine/writeEndRegionLine. |
| [ ] Tests | Single fenced block emits the expected wire output; with lang hint includes `lang=NAME`; body lines have `family=code` flag; surrounding plain text emitted normally; multiple fenced blocks emit independent regions | `cmd/md2spans/emit_test.go` | — |
| [ ] Iterate | FormatSpans branch for region begin/end | `cmd/md2spans/emit.go` | — |
| [ ] Commit | — | — | `md2spans: emit begin/end region directives for fenced code blocks` |

## Phase 3.5.8: md2spans — chunker honors region boundaries

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | writeChunked walks the formatted output and ensures no chunk boundary falls between a `begin region` and its matching `end region`. Within a region, lines may still be chunked at newline boundaries; only the begin/end pair must be in the same chunk. | base doc § "Risk #2 (atomicity)" | Bound on region length: ~16KB (large code blocks). 9P msize is typically 8KB-64KB. |
| [ ] Tests | A region whose body would naturally split across chunks gets bumped to the next chunk; chunk that just barely fits an entire region; multiple regions per chunk OK | `cmd/md2spans/main_test.go` | Tests likely require a configurable msize for determinism. |
| [ ] Iterate | Update writeChunked to track region depth and defer the chunk boundary | `cmd/md2spans/main.go` | — |
| [ ] Commit | — | — | `md2spans: writeChunked honors region boundaries` |

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
