# Spans Protocol — Specification

The spans protocol is the wire contract between edwood and external
tools that style a window's body text (currently `cmd/edcolor` for
syntax coloring and `cmd/md2spans` for markdown rendering).

External tools write a styling description to the window's
`spans` file (a 9P file at `/mnt/acme/<winid>/spans`). Edwood
parses the description and applies the styling to the body in
place — the body text itself is not modified by the protocol.

This document is the authoritative specification. The producer
(`cmd/edcolor`, `cmd/md2spans`) and the consumer
(`spanparse.go:parseSpanMessage`, `xfid.go:xfidspanswrite`) MUST
agree on every rule here. Drift between them is a bug.

## Status

This spec was extracted from existing code in April 2026 as part
of the markdown-externalization work (Phase 2.5). The protocol
is otherwise unchanged from its previous shape. Phase 3 rounds
will extend the protocol with new directives (font scale, font
family, region operations, etc.) — each round updates this spec
in lockstep with parser and tool changes.

## Wire format

The protocol is line-oriented UTF-8. Each non-empty line begins
with a single-character prefix identifying the directive. Empty
lines are ignored. Trailing whitespace on a line is ignored.

The protocol's unit of atomicity is **one Twrite to the spans
file**. Each Twrite is parsed independently
(`parseSpanMessage`). Multiple Twrites within a single open of
the spans file accumulate into the window's `spanStore` according
to the per-write ordering rules below.

## Directives

### `c` — Clear

```
c
```

Clears all spans for the window. The body text is unchanged.

**Constraints**:
- A `c` directive must be the only line in its Twrite. Mixing
  `c` with `s` or `b` lines in one write is an error
  (`spanparse.go:53`).

**Side effects** (consumer-side, `xfid.go:606-614`):
- The window's `spanStore` is cleared.
- The window exits styled mode if it was in it
  (`exitStyledMode`).
- The window's `styledSuppressed` flag is reset to `false`,
  re-enabling auto-switch into styled mode on the next `s` /
  `b` write.

The auto-switch and reset behavior matters for tools that want
to re-style a window the user previously took out of styled
mode via the "Markdown" tag command (which sets
`styledSuppressed = true`). Such tools issue a `c` write before
their styled-span writes; the clear resets suppression so the
subsequent write re-enters styled mode.

### `s` — Span

```
s <offset> <length> <fg> [<bg>] [<flag>...]
```

Defines a styled run of text. Fields:

- `<offset>`: integer rune index into the window body. Must be
  >= 0. (Rune-indexed, NOT byte-indexed; multi-byte UTF-8 runes
  count as one unit.)
- `<length>`: integer rune count. Must be >= 0.
- `<fg>`: foreground color. Either `#rrggbb` (lowercase hex,
  exactly 6 digits) or `-` for "default foreground" (no
  override). Alpha is fixed at 0xff.
- `<bg>`: optional background color. Same format as `<fg>`. If
  the field starts with `#` or is `-`, it's parsed as `<bg>`;
  any other token is the first flag. (Discriminated by
  appearance, not by position.)
- `<flag>...`: zero or more of `bold`, `italic`, `hidden`,
  `scale=N.N`. Order doesn't matter; each is a single token.
  Unknown flags are an error.

**`scale=N.N`** (added Phase 3 round 1):
- Value is a positive float in standard `strconv.ParseFloat`
  syntax (e.g. `scale=2.0`, `scale=1.5`, `scale=1.25`,
  `scale=0.875`, `scale=2`). Renders the run at N× the body
  font size.
- Validation: must parse as finite, > 0, and ≤ 10.0. Negative,
  zero, NaN, Inf, and values above the cap are errors.
- Absent flag = unset = renders at 1.0 (body baseline). The
  unset case is a distinct StyleAttrs from explicit
  `scale=1.0` (the span store treats them as different runs);
  consumers should prefer omitting the flag when scale is 1.0.

**Examples**:
```
s 0 5 #0000cc
s 0 5 - bold
s 0 5 - - italic            ; explicit default fg, default bg, italic
s 5 3 #ff0000 #ffff00 bold  ; red on yellow bold
s 7 4 - bold italic
s 0 12 - scale=2.0          ; H1 heading text
s 13 6 - bold scale=1.5     ; bold H2 content
```

### `b` — Box

```
b <offset> <length> <width> <height> [<fg>] [<bg>] [<flag>...] [<payload>...]
```

Defines a styled inline replaced element ("box") — a
fixed-dimension box whose visual is either a colored rectangle
or an image. Used for inline images and other replaced
elements that occupy a specific pixel-rectangle in the
rendered layout.

Fields:

- `<offset>`, `<length>`: as for `s`. The body's runes in
  `[offset, offset+length)` are replaced by the box at render
  time.
- `<width>`, `<height>`: integer pixels. Must be >= 0.
- `<fg>`, `<bg>`: optional, same format as `s`.
- `<flag>...`: same as `s`.
- `<payload>...`: optional trailing tokens preserved verbatim
  as the box's payload string. Currently used for image
  references: a payload starting with `image:` followed by a
  path (e.g. `image:/path/to/img.png`) renders the image; any
  other payload is currently ignored at render time.

**Examples**:
```
b 0 1 100 50 - - image:/path/to/img.png
b 5 1 20 20 #ff0000           ; 20×20 red box, no image
```

## Per-write ordering rules

Within a single Twrite:

1. **`c` is exclusive.** A `c` directive cannot be combined
   with any other directive in the same write. The parser
   returns immediately on encountering a `c`, so any `s` or
   `b` lines following the `c` in the same write are silently
   discarded by `parseSpanMessage` — but that's a parser bug
   waiting to bite if anyone relies on it. Treat as an error
   condition; producers must isolate `c` to its own write.

2. **`s` and `b` directives must be contiguous.** Within a
   write, the offset of each `s` / `b` must equal the previous
   directive's `offset + length` (`spanparse.go:72`). Gaps in
   the styled regions must be filled with default-styled `s`
   directives (fg "-", no flags).

3. **Region start.** The first `s` / `b` directive in a write
   sets the *region start*: subsequent directives are applied
   as a contiguous replacement starting at that offset. The
   region need not begin at offset 0, but every rune in
   `[regionStart, regionStart + sumOfLengths)` must be covered
   by exactly one `s` or `b` line. The store's runs outside the
   region are preserved.

4. **Out-of-range tolerance.** A directive whose `offset`
   exceeds the body's rune count is silently dropped
   (`spanparse.go:64`). A directive whose
   `offset + length` exceeds the body bound is clamped via
   `clampRunsToBuffer`. Producers SHOULD NOT rely on these
   behaviors; they exist as defensive clamps.

## Side effects of `s` / `b` writes

- The first `s` / `b` write to a window with empty `spanStore`
  initializes the store with a default run covering the full
  buffer, then applies the region update.
- If the window is not yet in styled mode and not in preview
  mode and `styledSuppressed` is false, edwood automatically
  enters styled mode (`xfid.go:642`).
- If `styledSuppressed` is true (the user took the window out
  of styled mode via the Markdown tag command), the spans are
  applied to the store but styled mode is NOT re-entered —
  the user has explicitly opted out. Producers can override by
  writing a `c` first (which resets `styledSuppressed`).

## Legacy unprefixed format

The pre-prefix protocol is still accepted for backward
compatibility (`xfid.go:597`):

```
clear           ; same as `c\n`
<offset> <length> <fg> [<bg>] [<flag>...]   ; one or more lines, no `s` prefix
```

The prefix detection (`isPrefixedFormat`) checks whether the
write's first non-empty line starts with `c `, `s `, or `b `.
New tools should use the prefixed format.

## Atomicity and partial writes

Each Twrite is parsed independently. Mid-Twrite process death
on the producer leaves the window's `spanStore` containing
whatever was applied by completed writes. The next render
typically replaces stale state via a fresh `c` + `s` ... write
sequence.

The two-write idiom (`c\n` followed by a contiguous `s` block
in a separate write) is the recommended way to fully replace
styling. Between the two writes, the window is briefly in
"cleared, no spans" state. On a slow connection this can
flicker. Phase 3 may introduce a single-directive
"clear-and-replace" form to eliminate the flicker window;
until then, the two-write idiom is canonical.

## Producer responsibilities

A spans-protocol producer (e.g. `cmd/md2spans`) MUST:

- Emit rune-indexed offsets and lengths (NOT byte-indexed).
- Cover its target region contiguously — fill gaps with
  default-styled `s` directives.
- Issue `c` writes alone (never with `s`/`b` in the same
  write).
- Limit each Twrite's payload size to fit the 9P msize (chunk
  on newline boundaries; a per-line cap of 4000 bytes is
  conservative).
- Treat the protocol as append-only within a write — directives
  are processed in order; later directives in a write cannot
  override earlier ones.

A producer SHOULD:

- Issue a `c` before its first styled write if it cannot rely
  on the window's prior state. This ensures `styledSuppressed`
  is reset and that any prior runs are cleared.
- Re-render on body edits with debounce (200-300 ms; producers
  decide).

## Future protocol extensions (Phase 3 roadmap)

The following extensions are planned per
`docs/designs/features/markdown-externalization.md`. Each will
update this spec in lockstep:

- **Round 1 — font scale**: ✓ landed (April 2026). New `<flag>`
  `scale=N.N` (e.g. `scale=2.0` for H1). See above.
- **Round 2 — font family**: new `<flag>` `family=code` (etc.).
- **Round 3 — inline rule**: new directive (or new box payload
  kind `rule:width:height`).
- **Round 4 — slide / images**: tool-side only; protocol
  unchanged.
- **Round 5 — block code (region)**: `begin region` /
  `end region` directives; first region primitive.
- **Rounds 6-7 — blockquote, lists**: region kinds + parameters.
- **Round 8 — tables**: region with cells; frame-dimension
  introspection 9P file.

Each round's design will revise this spec; the rules in this
document apply only to today's protocol surface.

## Implementation references

- Producer: `cmd/edcolor/main.go:writeSpans` (line ~457),
  `cmd/md2spans/main.go:writeSpans` (line ~194).
- Consumer parser: `spanparse.go:parseSpanMessage`,
  `parseSpanLine`, `parseBoxLine`.
- Consumer write handler: `xfid.go:xfidspanswrite` (line 565).
- Side-effect flag: `wind.go:96` (`styledSuppressed`),
  `wind.go:2417` (set by `exitStyledMode`),
  `xfid.go:611` (reset on `c`).
