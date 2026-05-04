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
  `scale=N.N`, `family=NAME`, `hrule`. Order doesn't matter;
  each is a single token. Unknown flags are an error.

**`hrule`** (added Phase 3 round 3):
- Single-token boolean flag. Indicates that the span is a
  horizontal-rule line. The renderer keeps the span's text
  visible (the source markers `---`/`***`/`___` render
  normally) and `rich/mdrender.Renderer.paintHorizontalRules`
  draws a thin line on the line containing the span — the
  user sees both the markers and the rule. This matches the
  "markup remains visible" stance of every other
  md2spans-emitted markdown feature in v1; a future WYSIWYG
  mode may hide markers globally.
- The rule spans the **full frame width** regardless of the
  span's offset or length. A short `hrule` span (e.g. one
  rune) and a long one produce identical rule geometry; only
  the marker text differs. Producers do not need to size the
  span to the rule's visual extent — sizing it to the marker
  characters is canonical.
- No coexistence restrictions; `hrule` plus `bold` /
  `italic` / `scale=` / `family=` is valid and the styling
  applies to the visible marker text. v1 does not pin the
  visual semantics of these combinations beyond "the markers
  inherit the styling."

**`family=NAME`** (added Phase 3 round 2):
- NAME is a semantic font-family name from the v1-recognized
  set: `{code}`. `family=code` says "render this run with the
  user's monospace font" — edwood maps the name to a loaded
  font via its existing font registry (e.g.
  `cmd/edcolor`-style `tryLoadCodeFont`). External tools
  emit semantic names; they don't see Plan 9 font paths.
- v1 valid values: `code` (lowercase, exact match).
- Validation: empty value (`family=`) is an error. Unknown
  names (e.g. `family=serif`, `family=GoMono`) are errors.
  Future rounds may extend the recognized set.
- Absent flag = unset = default font.

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

- `<offset>`, `<length>`: as for `s`. By default, the body's
  runes in `[offset, offset+length)` are replaced by the box
  at render time. The `placement=NAME` flag (below) can
  change this to a non-replacing layout where the runes
  render as text in the normal way and the box's visual
  (e.g., an image) renders alongside them — see the
  `placement=below` description for the round 4 details.
- `<width>`, `<height>`: integer pixels. Must be >= 0.
  **`0 0` means "renderer probes":** the consumer ignores the
  positional dimensions and uses the image's intrinsic
  dimensions (loaded via its image cache). Producers that
  don't know an image's size emit `0 0` and let the renderer
  decide. Producers that need pixel-exact placement emit
  positive values, which the renderer honors as overrides.
  Phase 3 round 4 added this canonical sentinel; existing
  producers continue to work unchanged.
- `<fg>`, `<bg>`: optional, same format as `s`.
- `<flag>...`: same as `s`, plus `placement=NAME` (round 4).
- `<payload>...`: optional trailing tokens. **First token is
  the URL spec** (currently `image:URL`). **Subsequent
  tokens are space-separated `key=value` parameters**
  interpreted by the consumer (not by the parser). v1
  recognizes:
  - `width=N` — pixel-width override (parity with the
    in-tree path's `width=Npx` title-attribute convention;
    `px` suffix dropped on the wire — pure integers).
  Future image params (`alt=ENCODED`, `caption=ENCODED`,
  `align=NAME`, etc.) extend the payload, NOT the wire
  format. Unknown params are silently ignored by older
  renderers (forward-compat). Anything before the first
  recognized URL prefix is preserved as-is for non-image
  box uses.

**`placement=NAME`** (added Phase 3 round 4):
- Single-token namespaced value. Selects the box's layout
  mode. v1 values:
  - `replace` — existing semantic. The box's `length` runes
    are replaced by the box at render time. This is also
    the default when the flag is absent.
  - `below` — the box covers the runes `[offset,
    offset+length)` but does NOT replace them. The
    renderer renders those source runes as text in the
    normal way (preserving `![alt](url)` markup
    visibility, consistent with rounds 1-3's
    markup-stays-visible stance), AND paints the image on
    the same line, anchored below the line's text. Line
    height grows additively by the image's height.
    `length` is the rune span the directive covers —
    typically the rune count of the source `![alt](url)`
    syntax. Multiple `placement=below b` lines on adjacent
    runes stack their images top-to-bottom in emission
    order.
- Future placements (`above`, `center`, `right`, etc.)
  extend the value vocabulary by way of their own Phase 3
  rounds, not by adding new flags. The parser deliberately
  rejects unknown values so producer mistakes surface
  loudly.

**Examples**:
```
b 0 1 100 50 - - image:/path/to/img.png
b 5 1 20 20 #ff0000                                ; 20×20 red box, no image
b 12 11 0 0 - - placement=below image:./pic.png    ; image rendered below source
b 12 11 0 0 - - placement=below image:./pic.png width=200
b 0 1 100 50 - - placement=replace image:./pic.png ; explicit form of the default
```

### `begin region` / `end region` — Region (added Phase 3 round 5)

```
begin region <kind> [param=value...]
end region
```

Defines a layout region — a scoped range of body runes
that share a kind-specific layout policy (full-line
background for code blocks, indent + bar for blockquotes
in round 6, etc.). Region directives slot between `s` /
`b` directives WITHOUT advancing the per-write contiguity
cursor: a `begin region` records the cursor's current
position as the region's `Start`; the matching `end
region` records `End`.

Fields:

- **`<kind>`** (required on `begin region`): namespaced
  layout-mode discriminator. v1 valid values: `code`,
  `blockquote`, `listitem`. Future rounds add `table`
  (round 8). Unknown kinds are an error.
- **`<param>=<value>`** (optional on `begin region`):
  zero or more space-separated key=value parameters
  carrying region attributes. v1 recognized params:
  - `lang=NAME` for `code` regions: optional language
    hint (e.g., `lang=go`, `lang=python`). Captured by
    the consumer for future use; v1 doesn't drive
    rendering off it.
  - `marker=X` for `listitem` regions (unordered): X is
    the bullet character (one of `-`, `*`, `+`).
  - `number=N` for `listitem` regions (ordered): N is
    the item's decimal number, ≥ 1. Implies ordered.
    The bridge sets `Style.ListOrdered=true` and
    `Style.ListNumber=N`.
  - For `listitem` regions, exactly one of `marker=`
    or `number=` is required. Producers that emit
    neither produce a wire-format error.
  - Malformed params (no `=` or empty value) are silently
    ignored (forward-compat).
- **`end region`** takes no kind / params.

**Blockquote depth (round 6)**: blockquote nesting depth
is COMPUTED, not declared. The protocol does NOT carry a
`depth=N` parameter on `begin region blockquote`. Producers
that emit nested blockquotes do so by emitting nested
`begin region blockquote` / `end region` pairs at the
appropriate offsets; the consumer's bridge counts
blockquote ancestors during the `applyEnclosingRegions`
walk. A producer that wants to emit `depth=N` as a hint is
free to (it's a recognized but unenforced parameter); the
bridge ignores it (the count is authoritative).

**Listitem depth (round 7 / round 7.x)**: each listitem
region is a SIBLING of its neighbors — the wire format
does NOT encode list nesting. Visual depth lives in the
source leading whitespace, which renders as plain text
per the markup-stays-visible stance. The bridge sets
`Style.ListIndent = 1` for every list-line first box
(one ancestor: the item's own region). The layout
applies one `ListIndentWidth` of indent uniformly; the
source whitespace adds `(N-1) × ListIndentWidth` for a
depth-N item, producing a marker-column of
`N × ListIndentWidth` overall.

Per-instance payload (`marker=` / `number=`) IS carried
— it varies per region. The bridge reads it from the
deepest listitem ancestor at a rune (nearest-of-kind via
the outermost-first walk + per-call overwrite); v1
typically has only one listitem ancestor per rune
(siblings, not nested).

Producers that want to convey list nesting structurally
(rather than via source whitespace) could add a `depth=N`
parameter; the v1 bridge ignores it.

**Constraints**:
- Every `begin region` in a Twrite must have a matching
  `end region` in the SAME Twrite (regions cannot span
  Twrites — producers chunk between regions, not within).
- `end region` without a matching `begin region` is an
  error.
- Regions nest. The parser maintains a stack; `end region`
  closes the most recent open region. v1 (round 5) does
  not produce nesting — `code` regions don't nest in
  CommonMark — but the parser accepts nested input for
  forward-compat with rounds 6+.
- Region directives don't break contiguity: the next
  `s` / `b` directive's offset must equal the cursor at
  the time of the directive.
- Empty regions (`begin region X` immediately followed by
  `end region` at the same cursor) are legal; the
  consumer records them with `Start == End`.

**Storage** (consumer-side): regions live in a sidecar
`RegionStore` (a forest of `Region{Start, End, Kind,
Params, Parent, Children}` trees). `Add` places a new
region by offset containment; nested regions form a tree.
The store is cleared by the protocol's `c` directive
alongside the `spanStore`.

**Rendering** (round 5 of the bridge): for each per-rune
StyleAttrs, the consumer's `buildStyledContent` consults
`regionStore.EnclosingAt(offset)` and ORs the enclosing
region's kind-specific flags into the per-rune
`rich.Style`. v1 case for `code`: `Style.Block=true,
Style.Code=true, Style.Bg=InlineCodeBg` — driving the
existing rich.Frame layout (gutter indent, full-line bg,
monospace font). Future kinds add cases without
rich.Frame changes.

**Examples**:
```
s 0 5 -
begin region code
s 5 12 - family=code
end region
s 17 5 -
```

```
begin region code lang=go
s 0 30 - family=code
end region
```

```
s 0 5 -
begin region code
end region                                            ; empty region
s 5 5 -
```

Blockquote (round 6) — single:
```
begin region blockquote
s 0 9 -                                               ; "> a quote"
end region
```

Nested blockquote — depth computed from ancestors:
```
begin region blockquote
s 0 8 -                                               ; "> outer\n"
begin region blockquote
s 8 9 -                                               ; ">> inner\n"
end region
s 17 13 -                                             ; "> outer again"
end region
```

Cross-kind — fenced code inside blockquote:
```
begin region blockquote
s 0 7 -                                               ; "> ```go\n"
begin region code lang=go
s 7 17 - family=code                                  ; "> fmt.Println()\n"
end region
s 24 5 -                                              ; "> ```\n"
end region
```

Single-line list items (round 7):
```
begin region listitem marker=-
s 0 5 -                                               ; "- foo"
end region
begin region listitem marker=-
s 5 5 -                                               ; "- bar"
end region
```

Ordered list:
```
begin region listitem number=1
s 0 6 -                                               ; "1. foo"
end region
begin region listitem number=2
s 6 6 -                                               ; "2. bar"
end region
```

List inside blockquote (cross-kind, round 7 inside round 6):
```
begin region blockquote
begin region listitem marker=-
s 0 7 -                                               ; "> - foo"
end region
end region
```

Nested list (round 7.x — sibling regions, depth via source
whitespace):
```
begin region listitem marker=-
s 0 3 -                                               ; "- a\n"
end region
begin region listitem marker=-
s 4 5 -                                               ; "  - b\n" (note 2-space indent)
end region
begin region listitem marker=-
s 10 3 -                                              ; "- c"
end region
```

The `  ` prefix on `  - b` is plain body text — the
markup-stays-visible stance preserves it, and its
rendered width (2 × char-width) is what visually indents
the inner item one ListIndentWidth deeper than `- a` and
`- c`.

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

5. **Regions are atomic.** `begin region` / `end region`
   pairs must be balanced within a single Twrite (Phase 3
   round 5). Producers must chunk on the BOUNDARY between
   regions, not within one. The chunker in
   `cmd/md2spans/main.go:writeChunked` enforces this on
   the producer side (extending past the nominal chunk
   size to enclose a region rather than breaking it). The
   consumer's parser rejects writes with unbalanced
   region directives.

6. **Directives at the same cursor apply in input order.**
   Multiple region directives may share a cursor offset
   (e.g., a parser-emitted `[begin@N, end@N]` empty region,
   or a closing `end region` immediately followed by an
   opening `begin region` between two adjacent fenced
   blocks). Producers MUST emit such directives in the
   order they should be applied; consumers MUST process
   them in input order. The two-pointer merge in
   `cmd/md2spans/emit.go:FormatSpans` and the stack-based
   parser in `spanparse.go:parseSpanMessage` both honor
   this contract.

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
- **Round 2 — font family**: ✓ landed (April 2026). New `<flag>`
  `family=NAME` with v1-recognized value `code`. See above.
- **Round 3 — inline rule**: ✓ landed (April 2026). New
  `<flag>` `hrule`. See above.
- **Round 4 — inline images**: ✓ landed (April 2026). New
  `<flag>` `placement=NAME` on `b` directives (v1 values:
  `replace`, `below`); canonical `0 0` W/H sentinel for
  "renderer probes"; payload-parameter convention (`width=N`
  recognized in v1). See above.
- **Round 5 — block code (regions)**: ✓ landed (May 2026).
  New `begin region <kind> [params]` / `end region`
  directives; first region primitive. v1 kind: `code`
  with optional `lang=NAME` param. See above.
- **Round 6 — blockquote (nested regions)**: ✓ landed (May 2026).
  Adds `blockquote` to v1 region kinds. First non-idempotent
  kind — depth is computed from ancestor count in the bridge.
  First nested region kind in production use; validates
  round-5 region machinery's claims about kind-vocabulary
  extension and ancestor-walk composition. See above.
- **Round 7 — lists (per-item regions)**: ✓ v1 landed
  (May 2026). Adds `listitem` to v1 region kinds. Carries
  per-region payload `marker=X` (unordered) or `number=N`
  (ordered) — the first kind with required per-instance
  params. Round 7 v1 covered column-0 single-line items;
  round 7.x added leading-whitespace nesting (2 spaces or
  1 tab per level — matches the in-tree path). Items at
  any depth are emitted as SIBLING regions; depth is
  conveyed via source leading whitespace, not the wire.
  Multi-line continuation deferred to round 7.y.
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
