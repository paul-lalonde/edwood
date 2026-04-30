# Phase 3 Round 2 — Font Family (Inline Code) — Plan

Second protocol-extension round of Phase 3. Add `family=NAME`
flag to spans-protocol; teach md2spans to emit `family=code`
for inline backtick-delimited code spans.

**Base design**: [`docs/designs/features/phase3-r2-font-family.md`](../designs/features/phase3-r2-font-family.md).

**Branch**: `phase3-r2-font-family`.

**Outcome**: edwood renders `` `inline code` `` in monospace
when md2spans is the renderer. Code BLOCKS (fenced or indented)
are NOT addressed in this round — those are regions and
land in round 5.

**Files touched**:
- `spanstore.go` — `StyleAttrs.Family` field + Equal().
- `spanparse.go` — parse `family=NAME` flag in s/b lines.
- `wind.go:styleAttrsToRichStyle` — map `Family == "code"` →
  `rich.Style.Code = true`.
- `cmd/md2spans/parser.go` — backtick tokenizer; emit Family in Span.
- `cmd/md2spans/emit.go` — format `family=NAME`.
- `docs/designs/spans-protocol.md` — document the flag.
- Tests at every layer.

---

## Phase 3.2.0: Plan + design

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | phase3-r2-font-family.md drafted | [base doc] | (a) Family is a semantic name (not Plan 9 path); (b) v1 accepts "code" only; (c) parse-time merge for inline-code-inside-heading carries scale+family. |
| [x] Tests | n/a (planning) | — | — |
| [x] Iterate | This plan + the design | — | This file. |
| [ ] Commit | — | — | `Add Phase 3 round 2 design and plan: font family` |

## Phase 3.2.1: Protocol — `Family` on `StyleAttrs`

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Confirm "" = unset; semantic name set | base doc § "StyleAttrs change" | Empty string = no override (use default font). |
| [ ] Tests | Equal() with Family; default zero-value | `spanstore_test.go` | — |
| [ ] Iterate | Add `Family string`; Equal() includes it | `spanstore.go` | — |
| [ ] Commit | — | — | `spans: add Family field to StyleAttrs` |

## Phase 3.2.2: Parser — recognize `family=NAME`

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Pin v1 valid set: `{code}`. Reject empty + unknown. | base doc § "Wire-format change" | parseFamilyFlag helper for both s and b parsers. |
| [ ] Tests | Round-trip; basic + with other flags + error cases | `spanparse_test.go` | — |
| [ ] Iterate | Add parsing in parseSpanLine + parseBoxLine via shared helper | `spanparse.go` | — |
| [ ] Commit | — | — | `spans: parse family flag on s/b directives` |

## Phase 3.2.3: Edwood renders Family via `styleAttrsToRichStyle`

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Map `Family=="code"` → `rich.Style.Code = true` | base doc § "styleAttrsToRichStyle change" | Other family values ignored (parser already rejected). |
| [ ] Tests | Family="code" → Code:true; Family="" → Code:false | `wind_styled_test.go` | — |
| [ ] Iterate | Add the mapping (one branch) | `wind.go:styleAttrsToRichStyle` | Box style gets the same mapping. |
| [ ] Commit | — | — | `wind: route StyleAttrs.Family to rich.Style.Code` |

## Phase 3.2.4: md2spans — backtick tokenizer

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | One backtick opens / closes; intra-paragraph; mismatched falls through | base doc § "md2spans change" | Double-backtick form deferred. |
| [ ] Tests | Basic inline code; in sentence; adjacent to emphasis; unclosed; inside heading (Scale + Family merge) | `cmd/md2spans/parser_test.go` | — |
| [ ] Iterate | Add tryCode helper similar to tryEmphasis/tryLink; parseInlineSpans dispatches on backtick | `cmd/md2spans/parser.go` | Span gains Family field. |
| [ ] Commit | — | — | `md2spans: inline code recognition` |

## Phase 3.2.5: md2spans — emit family flag

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | `family=NAME` formatting; omit if empty; coexist with other flags | base doc § "Wire-format change" | — |
| [ ] Tests | family=code in output; absent when empty; with bold/italic/scale | `cmd/md2spans/emit_test.go` | — |
| [ ] Iterate | emit.go formats Family; fillGaps copies Family through | `cmd/md2spans/emit.go` | — |
| [ ] Commit | — | — | `md2spans: emit family flag for inline code` |

## Phase 3.2.6: Spec + README

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | n/a (doc) | — | — |
| [ ] Tests | n/a (doc) | — | — |
| [ ] Iterate | Update spans-protocol.md (add family=NAME entry); update md2spans README v1 scope table (inline code → ✓) | — | — |
| [ ] Commit | — | — | `docs: spans protocol gains family flag; md2spans handles inline code` |

---

## After this round

`md2spans` covers paragraphs + emphasis + links + headings +
inline code. The in-tree markdown path's inline-code rendering
is now matched by md2spans. Round 3 (inline horizontal rule)
follows the same shape; round 5 introduces region operations
for code blocks.

## Risks

(See base design doc.) Main one: backtick interactions with
emphasis (e.g. `*\`code\`*`) — v1 produces approximate
behavior. Pinned by tests at row 3.2.4.

## Status

Plan + design drafted. Awaiting review before any code.
