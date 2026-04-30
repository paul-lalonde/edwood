# Phase 3 Round 3 — Inline Horizontal Rule — Design

## Purpose

Third round of Phase 3 of the markdown-externalization plan
([markdown-externalization.md](markdown-externalization.md)).
Add an `hrule` flag to the spans protocol so external tools
(`md2spans`) can render markdown horizontal rules
(`---` / `***` / `___` on a line by themselves) via the same
external-tool pipeline.

This is the third (and final) **flat** round before round 5
introduces region directives. Following rounds 1 (scale) and 2
(family), round 3 reuses the same pattern: new `StyleAttrs`
field, new flag in the parser, new mapping in
`styleAttrsToRichStyle`, new tokenizer in `md2spans`.

## Wire-format change

```
s 5 3 - hrule
```

Adds a new boolean flag `hrule`. When set, the span's text
content is NOT drawn — instead, the `rich/mdrender` wrapper's
existing horizontal-rule painter draws a thin horizontal line
spanning the frame width on the line containing the span.
Position-in-flag-list doesn't matter; coexists with other
flags (though `hrule` plus `bold` is semantically odd; v1
doesn't reject the combination, just renders the rule).

Constraints (parser):
- `hrule` is a single token, no value.
- No conflict with existing flags; just adds to the recognized
  set.

## `StyleAttrs` change

```go
type StyleAttrs struct {
    Fg     color.Color
    Bg     color.Color
    Bold   bool
    Italic bool
    Hidden bool
    Scale  float64
    Family string
    HRule  bool

    // ... box fields ...
}
```

`Equal()` includes HRule (zero-value comparison). Mirrors how
existing bool flags (Bold, Italic) work.

## `styleAttrsToRichStyle` change

```go
if sa.HRule {
    s.HRule = true
}
```

`rich.Style.HRule` already exists (was used by the in-tree
markdown path; now exclusively consumed by `rich/mdrender`'s
post-paint phase). The existing `paintPhaseText` skip for
HRule-styled boxes (still in `rich/frame.go`) means the span's
TEXT is not drawn; only the rule line is drawn by the wrapper.

This is a clean reuse — no new rendering machinery; the
wrapper's existing HRule paint phase handles it.

## `md2spans` change

Recognize horizontal-rule lines: a line consisting of exactly
3 or more of the same marker character (`-`, `*`, or `_`),
optionally with trailing whitespace, with NO other content.
Examples:
```
---
***
___
- - -    ; CommonMark allows spaces between markers
```

For v1, accept the simple form: 3+ consecutive same-character
markers at line start, no internal spaces, optional trailing
whitespace. Internal-space form (`- - -`) deferred.

Emit: an `s` directive over the marker runes with the `hrule`
flag. The text-skip in `paintPhaseText` causes the markers
NOT to be drawn; the wrapper draws the rule. Visually: the
line shows only the horizontal rule, no `---` characters.

This is a slight divergence from v1's "markup remains visible"
stance — for HRule, hiding the markers IS the rendering. The
other markdown features (emphasis, code, headings, links) keep
their markers visible because the styled content needs to be
readable; an HRule has no styled content per se, just the rule.

## Detection rules (md2spans)

A line matches HRule if:
- The first non-whitespace character is `-`, `*`, or `_`.
- The line contains AT LEAST 3 of that character.
- The line contains NO other characters except trailing
  whitespace.
- ATX heading detection takes precedence: if the line starts
  with `#` followed by space, it's a heading regardless of
  pattern. (Trivially won't trigger since `#` isn't a
  marker character.)

Negative cases (v1 leaves as plain text):
- `- item` (list item — fewer markers, has content)
- `--` (only 2 markers)
- `--- title` (markers followed by content — defer to a
  later round if anyone actually writes this)
- `- - -` (internal spaces — v1 simplification; CommonMark
  allows; defer)
- `--*` (mixed markers)

## Paragraph scanning

HRule lines don't merge with surrounding paragraphs:
they're block-level (per CommonMark) and end any prior
plain paragraph. `scanParagraphs` extends to detect HRule
lines the same way it currently detects ATX headings.
Each HRule is its own one-line paragraph.

## Test plan

1. **`spanparse.go` tests**: `hrule` flag round-trips; coexists
   with other flags; absent → HRule=false.
2. **`StyleAttrs.Equal` tests**: HRule included in equality.
3. **`styleAttrsToRichStyle` tests**: HRule=true → `rich.Style.HRule=true`.
4. **`md2spans` parser tests**: `---` / `***` / `___` (3
   markers + 4 markers + with trailing whitespace), false
   positives (`--`, `- item`, `--- title`), HRule between
   plain paragraphs.
5. **`md2spans` emit tests**: `hrule` flag formatted; absent
   when HRule=false.

## md2spans markdown subset after round 3

| Feature | Pre-r3 | Post-r3 |
|---|---|---|
| Plain paragraph | ✓ | ✓ |
| Emphasis | ✓ | ✓ |
| Inline links | ✓ | ✓ |
| ATX headings | ✓ | ✓ |
| Inline code (`` `text` ``) | ✓ | ✓ |
| Horizontal rules (`---` / `***` / `___`) | — | ✓ |

## Non-goals

- Setext-heading underlines (`====` / `---` after a text line)
  — round 3's `---` detection would NOT fire for such lines
  because the prior line is text; the markdown package handles
  setext headings by joining the marker line with the prior
  text line, which v1 doesn't do.
  - **Edge case to handle**: a `---` line that's preceded by a
    text line and followed by non-blank content COULD be a
    setext heading. v1 just renders it as an HRule (visually
    correct in many cases since users often write `---` to
    separate sections, not to underline a heading). Setext is
    rare in practice.
- Spaced-marker form (`- - -`) — defer.
- Custom rule styles (color, thickness) — fixed at v1.
- Multiple rule kinds (e.g. dotted, dashed) — single thin line
  matches the in-tree markdown's existing rendering.

## Risks

1. **Setext-heading false positive.** A `---` line following
   a text line is an HRule in our v1 but a setext H2 in
   CommonMark. Users who write `---` between paragraphs
   intending a setext H2 will see an HRule. Document in
   the README.
2. **The Phase 1 Style.HRule field.** Round 3 reuses the
   existing rich.Style.HRule + paintPhaseHorizontalRules
   pipeline that landed in Phase 1.3 (mdrender wrapper).
   That pipeline expects the SPAN to have HRule=true; it
   iterates visible lines, finds HRule-styled boxes, draws
   the rule. Our span carries the flag through StyleAttrs →
   rich.Style → mdrender. Same path the in-tree markdown
   package uses today.
3. **No test for visually-clean rendering.** The "markers
   skipped, rule drawn" assertion is a function of two
   separate rendering paths (paintPhaseText's skip, mdrender's
   draw). Unit tests can pin StyleAttrs.HRule=true plumbs
   through; visual correctness is smoke-tested.

## Status

Design — drafted. Awaiting review.
