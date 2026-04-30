# Phase 3 Round 1 — Font Scale (Headings) — Plan

First protocol-extension round of Phase 3. Add `scale=N.N` flag
to spans-protocol; teach md2spans to emit it for ATX headings.

**Base design**: [`docs/designs/features/phase3-r1-font-scale.md`](../designs/features/phase3-r1-font-scale.md).

**Branch**: `phase3-r1-font-scale`.

**Outcome after this round**: edwood renders `# Heading` /
`## Heading` / etc. when md2spans is the renderer. The in-tree
markdown path also produces scaled headings (already does);
md2spans now matches that capability.

**Files touched**:
- `spanstore.go` — `StyleAttrs.Scale` field + Equal().
- `spanparse.go` — parse `scale=N.N` flag in s/b lines.
- `wind.go:styleAttrsToRichStyle` — map Scale → rich.Style.Scale.
- `cmd/md2spans/parser.go` — paragraph-scanner gains heading
  detection; per-paragraph parse emits scaled spans.
- `cmd/md2spans/emit.go` — format Scale.
- `docs/designs/spans-protocol.md` — document the flag.
- Tests at every layer.

---

## Phase 3.1.0: Plan + design

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Draft phase3-r1-font-scale.md | [base doc] | Two design questions resolved: (a) Scale=0 means "unset → 1.0" (default-omit pattern), (b) Parse-time merge for emphasis-inside-heading. |
| [x] Tests | n/a (planning) | — | — |
| [x] Iterate | Write this plan + the design | — | This file. |
| [ ] Commit | — | — | `Add Phase 3 round 1 design and plan: font scale` |

## Phase 3.1.1: Protocol — `Scale` on `StyleAttrs`

Add the field, update Equal(), update the protocol spec doc.
No parser change yet; that lands in 3.1.2.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Confirm "0 = unset → 1.0" semantics | base doc § "StyleAttrs change" | Mirrors color (nil = default) pattern. |
| [ ] Tests | StyleAttrs.Equal() with Scale; including Scale=0 vs Scale=1.0 | `spanstore.go` adjacent | — |
| [ ] Iterate | Add field; Equal() includes it | `spanstore.go` | — |
| [ ] Commit | — | — | `spans: add Scale field to StyleAttrs` |

## Phase 3.1.2: Parser — recognize `scale=N.N`

`parseSpanLine` and `parseBoxLine` learn the `scale=N.N` flag.
Reject malformed values (negative, zero, NaN, Inf, > cap).

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Pin the cap at 10.0; reject zero | base doc § "Wire-format change" | Cap chosen as "1 OoM beyond reasonable headings" — 10× would render ridiculously. |
| [ ] Tests | parseSpanLine round-trip with scale; error cases | `spanparse_test.go` (or new file) | — |
| [ ] Iterate | Add parsing in parseSpanLine + parseBoxLine | `spanparse.go` | Single helper extracts scale from a flag list to dedup between s and b parsers. |
| [ ] Commit | — | — | `spans: parse scale flag on s/b directives` |

## Phase 3.1.3: Edwood renders Scale via `styleAttrsToRichStyle`

Map StyleAttrs.Scale → rich.Style.Scale. rich.Frame already
honors Scale; this just plumbs it through.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Confirm rich.Style.Scale's semantics match (1.0 = baseline) | `rich/style.go` | Scale field is float64; 1.0 is normal. Default 0 from spans → 1.0 in rich. |
| [ ] Tests | wind_styled_test or similar covering the mapping | — | — |
| [ ] Iterate | One-line plumb in styleAttrsToRichStyle | `wind.go:2636` | — |
| [ ] Commit | — | — | `wind: route StyleAttrs.Scale to rich.Style.Scale` |

## Phase 3.1.4: md2spans — heading detection

Paragraph scanner recognizes `# `, `## ` … `###### ` as
heading openers. Heading lines become single-line paragraphs;
they don't merge with adjacent paragraphs.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Pin ATX heading rules: 1-6 `#`s + space + content; trailing `#`s + whitespace stripped | base doc § "md2spans change" | Setext (`====`) deferred. |
| [ ] Tests | scanParagraphs and parseHeadingParagraph cases: H1-H6, no-space-after-#, mid-line #, trailing #s | `cmd/md2spans/parser_test.go` | — |
| [ ] Iterate | scanParagraphs returns paragraphs tagged with kind (Plain | Heading{level}); parseParagraph dispatches by kind | `cmd/md2spans/parser.go` | — |
| [ ] Commit | — | — | `md2spans: ATX heading detection` |

## Phase 3.1.5: md2spans — emit scale

Heading paragraphs emit a span over the heading text with the
correct Scale; emphasis inside heading text inherits the
heading's Scale per the Parse-time merge decision.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Confirm parse-time merge: emphasis spans inside headings carry both their flags AND Scale | base doc § "Bold-italic + scale interaction" | — |
| [ ] Tests | Heading produces Scale over the whole heading text; emphasis inside heading produces a span with both flags and Scale | `cmd/md2spans/parser_test.go` | — |
| [ ] Iterate | Add Scale field to Span; parseHeadingParagraph emits with Scale; emit.go formats `scale=N.N` flag (omit if Scale==0 or 1.0). | `cmd/md2spans/parser.go`, `emit.go` | — |
| [ ] Commit | — | — | `md2spans: emit scale flag for headings` |

## Phase 3.1.6: Protocol spec + README updates

Document the new flag in the authoritative spec; update
md2spans README to flip the heading row from `—` to `✓`.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | n/a (doc) | — | — |
| [ ] Tests | n/a (doc) | — | — |
| [ ] Iterate | Update `docs/designs/spans-protocol.md` § Directives + Future extensions; update `cmd/md2spans/README.md` v1 scope table | — | — |
| [ ] Commit | — | — | `docs: spans protocol gains scale flag; md2spans handles headings` |

---

## After this round

`md2spans` covers paragraphs + emphasis + links + headings. The
in-tree markdown path's heading rendering is now matched (for
ATX headings) by md2spans. Round 2 (font family for inline code
and code blocks) follows the same shape.

## Risks

(See base design doc; not duplicated here.) Main one is the
Parse-time merge logic for emphasis-inside-heading. Pinned by
tests at row 3.1.5.

## Status

Plan + design drafted. Awaiting review before any code.
