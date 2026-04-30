# Phase 3 Round 1 — Font Scale (Headings) — Design

## Purpose

First round of Phase 3 of the markdown-externalization plan
([markdown-externalization.md](markdown-externalization.md)).
Add a font-scale primitive to the spans protocol so external
tools (`md2spans`) can render markdown headings (`# `, `## `,
…) at larger sizes via the same external-tool pipeline that
covers paragraphs / emphasis / links today.

This round is **flat** — it adds a per-rune attribute, not a
region. Heading lines have a per-line scope but emphasis-style
flat-span semantics are sufficient: the heading content is one
contiguous styled run with `scale=N`. No nesting; no push/pop
state machine.

This is the first protocol extension; design choices set the
template for subsequent flat rounds (round 2 font family,
round 3 inline rule, round 4 inline image support).

## What changes

| Layer | Change |
|---|---|
| Protocol | New flag `scale=N.N` on `s` and `b` directives. Default 1.0 if absent. |
| `spanstore.StyleAttrs` | Add `Scale float64` field. Equal() includes Scale. |
| `spanparse.go` | parseSpanLine / parseBoxLine recognize `scale=N.N`. |
| `wind.go:styleAttrsToRichStyle` | Map Scale → `rich.Style.Scale`. |
| `cmd/md2spans/parser.go` | Recognize ATX headings (`# h1`, `## h2`, …) and emit scaled spans. |
| `cmd/md2spans/emit.go` | Format `scale=N.N` flag in `s` lines. |
| `docs/designs/spans-protocol.md` | Document the new flag. |
| Tests | New: parse round-trip; md2spans heading parsing; integration through styled mode. |

## Wire-format change

```
s 0 12 - scale=2.0          ; H1: scale 2.0 over the heading text
s 0 8 - bold scale=1.5      ; bold H2 (when md2spans wants both)
s 0 5 - scale=1.0           ; equivalent to no scale flag
```

Format: lowercase `scale=` immediately followed by a positive
float in standard Go `strconv.ParseFloat` syntax. Examples:
`scale=2.0`, `scale=1.5`, `scale=1.25`, `scale=0.875`.

Constraints (parser):
- Scale must parse as a finite positive float. Negative, zero,
  NaN, and Inf are errors.
- A reasonable upper bound (e.g. 10.0) avoids degenerate
  rendering. Above the cap → error.
- Absent flag means scale=1.0 (current behavior, no styling
  change).

Position in the flag list: same position rules as `bold` /
`italic` — appears after the optional `<bg>` color and before
or after other flags. Order doesn't matter; the parser is
flag-name-driven, not positional.

## `StyleAttrs` change

```go
// spanstore.go
type StyleAttrs struct {
    Fg     color.Color
    Bg     color.Color
    Bold   bool
    Italic bool
    Hidden bool
    Scale  float64 // 0 means "unset → 1.0"; explicit 1.0 also fine

    IsBox      bool
    BoxWidth   int
    BoxHeight  int
    BoxPayload string
}
```

Equal() must compare Scale exactly (no float epsilon —
producers emit the same string each time, so byte equality
implies float equality after parsing). The Equal() change
slightly increases the dedup cost in the span store, but it's
one extra float compare per call; negligible.

Default-value question: is 0.0 or 1.0 the "no scaling" value?
Two readings:

- **0.0 = unset.** Producer-side: omit the `scale=` flag entirely
  when scale is 1.0. Parser-side: default field to 0.0 means
  "no styling applied"; downstream code treats 0.0 as "use
  default scale (1.0)". This keeps `Equal()` natural for
  default-valued StyleAttrs.
- **1.0 = explicit no-scale.** Always set Scale to 1.0 by
  default. Producers always emit `scale=1.0` (verbose) or omit
  it (parser fills in 1.0).

**Decision: 0.0 = unset.** Reason: matches the existing pattern
for color (nil = default, no override). Keeps the wire format
minimal — producers emit `scale=N.N` only when N != 1.0.
`styleAttrsToRichStyle` translates 0.0 → rich.Style.Scale = 1.0.

## `md2spans` change

Recognize ATX-style markdown headings: lines that start with
1-6 `#` characters followed by a space, with the rest of the
line being the heading text. Trailing `#` characters and
trailing whitespace are stripped per CommonMark.

Mapping:

| Markdown | Scale | Rationale |
|---|---|---|
| `# H1` | 2.0 | Largest |
| `## H2` | 1.5 | |
| `### H3` | 1.25 | |
| `#### H4` | 1.1 | |
| `##### H5` | 1.05 | |
| `###### H6` | 1.0 | Visually similar to body, distinct via bold |

(Mirrors the existing in-tree markdown package's heading
sizes per `rich/style.go:82-84` for H1/H2/H3.)

Heading detection happens at the paragraph-scanner level —
ATX headings are *block-level*, so a line starting with `# ` is
its own paragraph regardless of whether blank lines surround
it. v1's paragraph scanner already groups consecutive non-blank
lines; we extend it to recognize a heading line as ending the
prior paragraph and starting (and ending) a one-line heading
paragraph.

Paragraph emit: heading lines emit one styled span over the
heading text (excluding the `## ` prefix and any trailing `#`s)
with `Fg=""`, `Bold=false`, `Italic=false`, `Scale=N.N`. The
markup characters (`#` and the space) remain in the body
unstyled (consistent with v1's "markup visible" stance).

## md2spans markdown subset after round 1

| Feature | Pre-r1 | Post-r1 |
|---|---|---|
| Plain paragraph | ✓ | ✓ |
| Emphasis (italic / bold / bold-italic) | ✓ | ✓ |
| Inline links | ✓ | ✓ |
| ATX headings (`# … `) | — | ✓ |
| Setext headings (`====` underline) | — | — (deferred; rarely used) |
| Code blocks | — | — |
| Lists | — | — |
| Blockquotes | — | — |
| Tables | — | — |

Setext headings (`Heading\n=====`) are not commonly used in
practice and add detection complexity; defer to a future polish
round.

## Integration considerations

- **Bold-italic + scale interaction.** Heading text in markdown
  can contain emphasis: `## **bold** title`. v1's emphasis
  matcher runs over the heading content; the bold span and
  the heading-scale span overlap. Today the `Span` shape can't
  carry both Scale and Bold simultaneously. **Mitigation**:
  `Span` already has Bold/Italic; add Scale alongside — same
  flat shape, no IR change. The emit layer combines flags into
  one `s` line (`s 0 5 - bold scale=1.5`). Handles the overlap
  cleanly.

- **Heading mixing with emphasis matcher.** A heading line
  `# *emphasized* title` should produce: `Scale=2.0` over
  the entire heading content, plus an italic span over
  "emphasized". Two spans. The emphasis matcher runs inside
  parseHeadingParagraph, same as inside parseParagraph. Each
  span gets Scale=2.0 (or the heading's scale); the emphasis
  span ALSO has Italic=true. Emit produces:

  ```
  s 0 1 -                          ; the leading "# "
  s 2 4 - scale=2.0                ; "embed" (or whatever) up to *
  s 6 11 - italic scale=2.0        ; "emphasized" — both flags
  s 17 6 - scale=2.0               ; " title"
  s 23 ... -                       ; rest of body, no scale
  ```

  The fillGaps logic stays unchanged — it inserts default
  spans (Scale=0) between scaled spans. Scaled spans don't
  break contiguity.

- **Selection / cursor placement on scaled lines.** Edwood's
  rich.Frame already handles variable line heights; scaled
  lines just have larger Height. No new code in edwood — round
  1 only touches the protocol path.

- **Bold-italic and heading overlap math.** The fillGaps
  default-fill spans inside a heading line need Scale=N too,
  not Scale=0, otherwise the line's blank space (markup runes,
  whitespace) renders at body scale and the line's overall
  height computation becomes inconsistent. **Decision**: emit
  the heading's Scale on EVERY span within the heading line,
  including default-styled fill spans for the markup runes.
  This requires md2spans to know "what scale to use for fill
  spans within paragraph X" — pass a per-paragraph default
  Scale to fillGaps.

  Actually wait — fillGaps is in emit.go, downstream of Parse.
  Parse produces []Span; fillGaps fills gaps with default-
  styled spans. The "default" scale for those spans must come
  from somewhere.

  Cleanest answer: Parse emits a span over the entire heading
  line with `Scale=N` and no other flags. The emphasis-inside-
  heading case becomes overlapping styled spans (heading-
  level + emphasis). fillGaps handles overlap with the
  earlier-wins clip rule we already have. The line-level
  heading span covers the whole line including markup runes;
  emphasis spans within it clip the heading span at their
  start/end.

  This requires Parse to emit OVERLAPPING spans, which
  fillGaps now must merge. The existing "earlier wins on
  overlap" behavior is wrong for this case — we want the
  heading's scale on the whole line PLUS the emphasis flags
  on the inner range.

  This is the first round-3 design wrinkle. Two options:

  - (a) **Merge at Parse time.** Parse emits non-overlapping
    spans where overlapping styled regions are pre-merged
    into one Span carrying the union of styling. emit.go
    stays simple. Parse gets more complex.
  - (b) **Stack at fillGaps time.** Parse emits overlapping
    spans (a heading-line span + emphasis spans). fillGaps
    builds a flat sequence by stacking and merging. Parse
    stays simple. fillGaps becomes a small layout engine.

  **Decision: (a) merge at Parse time.** Smaller blast
  radius in v1; emit.go contract stays "non-overlapping
  contiguous". Parse internally handles "this span is inside
  a heading, give it Scale=N too". The merge is mechanical:
  when emitting an emphasis span inside a heading
  paragraph, set its Scale to the heading's scale; when
  emitting the rest of the heading line, generate a single
  span covering the non-emphasis text with `Scale=N` only.

## Test plan

Per CODING-PROCESS:

1. **`spanparse.go` parser tests** (in main package): round-trip
   `scale=N.N` parsing, error cases (negative, zero, NaN, malformed).
2. **`StyleAttrs.Equal` tests**: Scale included in equality.
3. **`md2spans` parser tests**: H1-H6 detection, no-space-after-#
   not a heading, mid-line `#` not a heading, emphasis inside
   heading produces correctly-scaled spans.
4. **`md2spans` emit tests**: `scale=N.N` flag formatted
   correctly; default scale (1.0 or 0.0) omitted.
5. **Integration**: smoke test in edwood — open a markdown file
   with headings, run md2spans, verify rendering matches the
   in-tree markdown path (visually).

## Non-goals

- Setext headings (`Heading\n====`).
- Heading levels beyond 6 (CommonMark caps at 6).
- Configurable scale-per-level ratios (mapping is fixed in v1).
- ATX-close-form `# Heading #` — trailing `#`s are stripped
  per CommonMark but not visually replicated; that's a future
  polish.
- Animated scale transitions, scale interpolation, etc.

## Risks

1. **Float comparison in Equal().** Producer emits the same
   string each time → byte equality of the wire format. Parsed
   floats are byte-exact too. But if two producers emit
   `scale=2` vs `scale=2.0`, `strconv.ParseFloat` produces the
   same float (2.0) and Equal() compares 2.0 == 2.0 (true). No
   issue.
2. **Storage size of StyleAttrs.** Adds 8 bytes per StyleAttrs
   (float64). The span store holds these as runs; for large
   styled documents the overhead is one float per styled run,
   negligible.
3. **Overlap at Parse time.** Decision (a) above — merging
   emphasis-inside-heading at Parse time is a small parser
   complication. Tests pin the cases. If the merge logic gets
   gnarly later (when more flags are added), revisit — but
   round 1's bool flags + Scale are still tractable.
4. **md2spans heading detection across paragraph boundaries.**
   v1's scanParagraphs groups consecutive non-blank lines into
   one paragraph. After round 1, a non-blank line starting
   with `# ` ends the prior paragraph and is its own one-line
   paragraph regardless of whether blank lines surround it.
   The scanner change is small but non-zero.

## Status

Design — drafted. Awaiting review before implementation.
