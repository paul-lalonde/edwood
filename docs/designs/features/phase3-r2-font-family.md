# Phase 3 Round 2 — Font Family (Inline Code) — Design

## Purpose

Second round of Phase 3 of the markdown-externalization plan
([markdown-externalization.md](markdown-externalization.md)).
Add a font-family selector to the spans protocol so external
tools (`md2spans`) can render inline markdown code (`` `code` ``)
in monospace via the same external-tool pipeline that covers
paragraphs / emphasis / links / headings today.

This round is **flat** — it adds a per-rune attribute (a string
flag), not a region. Inline code spans are intra-paragraph
styled runs; the existing `[]Span` shape carries them naturally.

**Deferred to round 5**: fenced and indented code BLOCKS, which
need full-line backgrounds and the region-protocol machinery
(begin/end push/pop). Round 2 is just the font-family flag,
applied to inline code by md2spans.

## Family names are semantic, not literal Plan 9 paths

Plan 9 fonts are referenced by paths like
`/mnt/font/GoRegular/16a/font`. Edwood's `tryLoadCodeFont` and
`tryLoadFontVariant` helpers construct those paths by swapping
family / style components based on the user's *current* font
choice. The mapping from "I want monospace" to "this specific
Plan 9 font path" is edwood's responsibility.

The spans protocol's `family=NAME` flag is therefore a
**semantic key**, not a literal font path. A producer emits
`family=code` to mean "render this run in whatever the user's
monospace font is." Edwood looks up its registered code font
(loaded at styled-mode init) and uses it.

This decouples external tools from the user's font
configuration and keeps the protocol portable across systems
with different fonts available.

## What changes

| Layer | Change |
|---|---|
| Protocol | New flag `family=NAME` on `s` and `b` directives. v1 recognizes `family=code` only; other names are errors. |
| `spanstore.StyleAttrs` | Add `Family string` field. Equal() includes Family. |
| `spanparse.go` | parseSpanLine / parseBoxLine recognize `family=NAME`. |
| `wind.go:styleAttrsToRichStyle` | Map `Family == "code"` → `rich.Style.Code = true`. Other Family values are no-ops at v1 (parser rejects them, but the mapping is defensive). |
| `cmd/md2spans/parser.go` | Recognize inline code (`` `text` `` between backticks) and emit a span with Family="code". |
| `cmd/md2spans/emit.go` | Format `family=NAME` flag in `s` lines. |
| `docs/designs/spans-protocol.md` | Document the new flag and its v1-recognized value. |
| Tests | New: parse round-trip for family flag; md2spans backtick detection; integration through styled mode. |

## Wire-format change

```
s 0 5 - family=code               ; inline code "hello" rendered monospace
s 0 5 - bold family=code          ; bold monospace
s 0 5 - family=code scale=1.5     ; monospace at 1.5× (rare; H2 with code in title?)
```

Format: lowercase `family=` immediately followed by a name from
the v1-recognized set (`code` only). Future rounds may extend
the set. Order in the flag list doesn't matter; the parser is
flag-name-driven.

Constraints (parser):
- Empty value (`family=`) is an error.
- Unknown family name is an error: e.g., `family=serif`,
  `family=GoMono`, `family=fancy` all reject in v1.
- Absent flag means "use default font" (current behavior).

## `StyleAttrs` change

```go
type StyleAttrs struct {
    Fg     color.Color
    Bg     color.Color
    Bold   bool
    Italic bool
    Hidden bool
    Scale  float64

    // Family is the semantic font-family name (e.g., "code"
    // for monospace). Empty string = default font. Recognized
    // names live in the parser (rejected if unknown). v1
    // accepts "code" only; future rounds may add others.
    // Added in Phase 3 round 2 of the markdown-externalization
    // plan.
    Family string

    // Box fields (zero values = not a box)
    IsBox      bool
    BoxWidth   int
    BoxHeight  int
    BoxPayload string
}
```

Default-value question: same shape as Scale. Empty string =
unset, downstream maps to "use default font." Equal() uses
direct string comparison; "" != "code" naturally.

## `styleAttrsToRichStyle` change

```go
func styleAttrsToRichStyle(sa StyleAttrs) rich.Style {
    s := rich.Style{Scale: 1.0}
    if sa.Scale > 0 {
        s.Scale = sa.Scale
    }
    s.Fg = sa.Fg
    s.Bg = sa.Bg
    s.Bold = sa.Bold
    s.Italic = sa.Italic
    if sa.Family == "code" {
        s.Code = true  // rich.Frame's fontForStyle returns codeFont
    }
    return s
}
```

Other `sa.Family` values are no-ops at this layer. The parser
rejects them upstream, so this branch never sees them in
production; the defensive ignore prevents a stale span store
(if a v1-protocol-aware tool wrote spans before edwood was
upgraded to round 2) from breaking the rendering.

## `md2spans` change

Recognize **inline code** spans: a backtick-delimited region
within a paragraph. v1 supports the simple form:

- One ASCII backtick opens, one ASCII backtick closes.
- Content between is the literal code text. Markdown rules
  like "double backticks for code containing single
  backticks" are deferred to a future polish.
- Mismatched backticks (no closing) fall through as literal.

Emit: a span over the inner text with `Family="code"`. The
backtick markup runes themselves remain in the body
(consistent with v1's "markup runes visible at body scale"
stance).

Extension: just like emphasis, inline code can appear inside
heading paragraphs. The Parse-time merge handles it:
`# This is \`code\`` produces:

- Heading default span over `This is ` with Scale=2.0.
- Inline code span over `code` with Family="code" AND
  Scale=2.0 (the heading's scale carries through).
- Heading default span over the trailing `` ` `` with Scale=2.0.

**Multiple `*`/`_`/`` ` ``-style runs in a single paragraph**
already work because parseInlineSpans dispatches on the first
delimiter character it sees. Adding `` ` `` to the dispatch is
a one-line addition.

## md2spans markdown subset after round 2

| Feature | Pre-r2 | Post-r2 |
|---|---|---|
| Plain paragraph | ✓ | ✓ |
| Emphasis | ✓ | ✓ |
| Inline links | ✓ | ✓ |
| ATX headings | ✓ | ✓ |
| Inline code (`` `text` ``) | — | ✓ |
| Code blocks (fenced / indented) | — | — (round 5, region) |

## Test plan

1. **`spanparse.go` tests**: round-trip `family=code` parsing,
   error cases (empty, unknown name, multiple family flags).
2. **`StyleAttrs.Equal` tests**: Family included.
3. **`styleAttrsToRichStyle` tests**: `Family="code"` → `Code: true`;
   `Family=""` → `Code: false`; unknown families ignored.
4. **`md2spans` parser tests**: basic inline code, code in
   sentence, code adjacent to emphasis, code inside heading
   (with merged Scale + Family), unclosed backtick falls
   through, mid-line backtick patterns.
5. **`md2spans` emit tests**: `family=code` flag formatted; absent
   for empty Family; coexists with bold/italic/scale.

## Non-goals

- Code BLOCKS (fenced ` ``` ` or 4-space indented) — round 5
  (regions).
- Double-backtick inline code (`` `` `code with ` backtick` `` ``).
- Other family names (`family=serif`, etc.) — future rounds.
- Backtick-as-literal (escaping) — markdown supports `\``;
  v1 doesn't bother.
- Configurable family-to-font mapping — edwood owns the
  registry.

## Risks

1. **Backtick parser interactions with emphasis.** `*\`code\`*`
   should produce italic over the backtick-delimited content.
   The dispatcher at parseInlineSpans's switch already handles
   this: `*` opens emphasis, then inside the emphasis content,
   backticks would need to be tokenized. Current parser
   doesn't do nested emphasis-inside-link recursion (R5
   divergence); same divergence applies here. v1 produces
   an italic span over `` `code` `` without the family flag,
   then if the user closes emphasis cleanly, `*` matches.
   The known-divergence list grows by one; tests pin it.
2. **Family field adds a string allocation per StyleRun.**
   Negligible — most StyleRuns will have empty Family. Same
   cost-shape as Fg color (which can also be unset).
3. **`Code` flag double-purpose in rich.Style.** The flag is
   used for both inline code and code blocks today. Round 5
   (block regions) will introduce a separate region marker,
   so this round 2 use of `Code` for inline code stays clean
   relative to the future region-based code-block.

## Status

Design — drafted. Awaiting review.
