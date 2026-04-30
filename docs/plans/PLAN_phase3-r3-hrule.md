# Phase 3 Round 3 — Inline Horizontal Rule — Plan

Third (and last flat) protocol-extension round of Phase 3. Add
`hrule` flag to spans-protocol; teach md2spans to recognize and
emit it for `---` / `***` / `___` lines.

**Base design**: [`docs/designs/features/phase3-r3-hrule.md`](../designs/features/phase3-r3-hrule.md).

**Branch**: `phase3-r3-hrule`.

**Outcome**: edwood renders horizontal rules when md2spans is
the renderer, reusing Phase 1.3's existing
paintPhaseHorizontalRules pipeline.

**Files touched**:
- `spanstore.go` — `StyleAttrs.HRule` field + Equal().
- `spanparse.go` — parse `hrule` flag.
- `wind.go:styleAttrsToRichStyle` — map HRule → `rich.Style.HRule`.
- `cmd/md2spans/parser.go` — HRule line detection;
  paragraphRange gains a sentinel.
- `cmd/md2spans/emit.go` — format `hrule` flag.
- `docs/designs/spans-protocol.md` — document the flag.
- Tests at every layer.

---

## Phase 3.3.0: Plan + design

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | phase3-r3-hrule.md drafted | [base doc] | (a) hrule is a flag on s lines (not new directive); (b) reuses existing rich.Style.HRule + mdrender pipeline; (c) HRule lines split paragraphs same as ATX headings. |
| [x] Tests | n/a (planning) | — | — |
| [x] Iterate | This plan + design | — | This file. |
| [x] Commit | — | — | `Add Phase 3 round 3 design and plan: inline horizontal rule` |

## Phase 3.3.1: Protocol — `HRule` on `StyleAttrs`

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Confirm bool field; Equal() includes it | base doc § "StyleAttrs change" | Mirrors Bold/Italic. |
| [x] Tests | Equal() with HRule | `spanstore_test.go` | — |
| [x] Iterate | Add `HRule bool`; Equal() includes it | `spanstore.go` | — |
| [x] Commit | — | — | `spans: add HRule field to StyleAttrs` |

## Phase 3.3.2: Parser — recognize `hrule` flag

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Single-token flag, no value | base doc § "Wire-format change" | Mirrors bold/italic exactly. |
| [x] Tests | Round-trip; coexistence with other flags; absent → HRule=false | `spanparse_test.go` | — |
| [x] Iterate | Add to parseSpanLine + parseBoxLine flag switch | `spanparse.go` | — |
| [x] Commit | — | — | `spans: parse hrule flag on s/b directives` |

## Phase 3.3.3: `styleAttrsToRichStyle` plumbs HRule

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | StyleAttrs.HRule → rich.Style.HRule (existing field) | base doc § "styleAttrsToRichStyle change" | One-line addition. |
| [x] Tests | HRule=true → rich.Style.HRule=true | `wind_styled_test.go` | — |
| [x] Iterate | Add the mapping | `wind.go:styleAttrsToRichStyle` | Box path likewise. |
| [x] Commit | — | — | `wind: route StyleAttrs.HRule to rich.Style.HRule` |

## Phase 3.3.4: md2spans — HRule line detection

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Detect `---`/`***`/`___` (3+ same char, no other content, optional trailing whitespace) | base doc § "Detection rules" | Spaced-marker form deferred. |
| [x] Tests | Basic three forms; 4+ markers; trailing whitespace; negatives (`--`, `- item`, `--- title`); HRule between paragraphs | `cmd/md2spans/parser_test.go` | — |
| [x] Iterate | paragraphRange gains a sentinel (e.g., HeadingLevel could be reused with -1, or a new field IsHRule); scanParagraphs detects; new parseHRuleParagraph emits the span | `cmd/md2spans/parser.go` | Probably IsHRule bool — semantically distinct from heading. |
| [x] Commit | — | — | `md2spans: detect horizontal-rule lines` |

## Phase 3.3.5: md2spans — emit hrule flag

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | `hrule` formatted in flag list; absent when HRule=false | base doc § "Wire-format change" | — |
| [x] Tests | hrule in output; absent when not set | `cmd/md2spans/emit_test.go` | — |
| [x] Iterate | Add HRule field to Span; emit.go formats; fillGaps copies through | `cmd/md2spans/emit.go`, `parser.go` | — |
| [x] Commit | — | — | `md2spans: emit hrule flag for horizontal rules` |

## Phase 3.3.6: Spec + README

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | n/a (doc) | — | — |
| [x] Tests | n/a (doc) | — | — |
| [x] Iterate | Update spans-protocol.md; update md2spans README v1 scope table | — | — |
| [x] Commit | — | — | `docs: spans protocol gains hrule flag; md2spans handles horizontal rules` |

---

## After this round

Round 3 closes the flat extensions. md2spans covers paragraphs +
emphasis + links + headings + inline code + horizontal rules. The
remaining markdown features (block code, blockquote, lists,
tables) are region-shaped and land in rounds 5-8 with the
region-protocol design.

## Risks

(See base design doc.) Setext-heading false positive is the main
concern; documented in the README.

## Status

Round complete. All six rows landed on `phase3-r3-hrule`.

**Smoke-test follow-up (April 2026)**: initial render hid the
source markers (`---`/`***`/`___`) under the rule line, which
made HRule visually inconsistent with every other v1 feature
(emphasis, code, headings keep markers visible). Fix: removed
the HRule skip in `rich/frame.go:paintPhaseText`; markers now
render alongside the rule line. Test renamed
`TestHRuleNotRenderedAsText` → `TestHRuleRenderedAsText` to pin
the new contract. Spec and README updated.
