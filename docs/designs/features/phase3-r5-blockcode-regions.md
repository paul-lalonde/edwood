# Phase 3 Round 5 — Block Code (Regions) — Design

## Purpose

Fifth round of Phase 3 of the markdown-externalization plan
([markdown-externalization.md](markdown-externalization.md)).
Introduces the **first region directive** to the spans
protocol: `begin region` / `end region`. Round 5 uses the
simplest region kind (`code`) — full-line background, no
nesting, no parameters required (an optional `lang=NAME`
hint). Rounds 6-8 will extend the region machinery for
blockquote (nested), lists (per-item), and tables (cells).

Round 5's job is to:
1. Add the wire-format primitives (`begin region` /
   `end region`) — the smallest extension that admits
   future region kinds.
2. Add a sidecar region store on the consumer side
   alongside the existing `spanStore` — the storage shape
   that supports nesting from round 6+ on day one.
3. Wire the `wind.go` bridge so a `region=code` translates
   to per-rune `Style.Block=true, Code=true, Bg=InlineCodeBg`
   — round 5 reuses the existing rendering pipeline
   unchanged. Rounds 6+ may force a region-aware layout
   pass; v1 of regions does not.
4. md2spans recognizes fenced code blocks (` ``` `) and
   emits the new directives.

## Design principles inherited from rounds 1-4

The round-4 review codified three principles that round 5
inherits:

- **Protocol expresses intent, not pixel placement.** Region
  directives say *"this rune range is a code block"* and the
  renderer translates that to layout (gutter indent,
  full-width background, monospace font). Resize / theme /
  font changes don't touch the protocol.
- **Namespaced flag/directive values.** v1 region kinds:
  `code` (round 5), `blockquote` (round 6), `listitem`
  (round 7), `table` (round 8). New kinds extend the value
  vocabulary, not the wire format.
- **Payload-style key=value parameters.** Region directives
  carry optional space-separated `key=value` params after
  the kind: `begin region code lang=go`. Unknown params are
  silently ignored (forward-compat).

## Wire-format change

Two new directive prefixes:

```
begin region <kind> [param=value...]
end region
```

Examples:
```
begin region code                  ; bare; no lang hint
begin region code lang=go          ; with optional language
begin region blockquote indent=2   ; round 6 future
end region                         ; pops the most recent begin
```

The directives slot between `s`/`b` directives **without
advancing the per-write contiguity cursor**:

```
s 0 5 -                            ; "Here:"
begin region code lang=go
s 5 17 - family=code               ; "  fmt.Println(\"hi\")"
end region
s 22 4 -                           ; "...ok"
```

`begin region` records the current contiguity cursor as the
region's `Start` rune; `end region` records the cursor as the
region's `End`. The region thus spans `[Start, End)` in the
body, and the `s`/`b` directives between begin and end
constitute the region's content.

### Per-write rules

Extending the existing per-write rules:

1. **Region begin/end pair within a write.** Every `begin
   region` in a write must be matched by an `end region`
   within the same write. A `begin` without `end` is a
   protocol error; an `end` without prior `begin` is an
   error. Regions cannot span multiple Twrites — md2spans
   must buffer entire regions before writing (chunking on
   the 9P msize boundary respects region boundaries).
2. **Region nesting.** v1 (round 5) does not nest, but the
   parser maintains a stack so rounds 6+ work. The depth
   limit is the same as the spans protocol's reasonable
   bounds (no hard cap; 4-5 deep is realistic for nested
   blockquotes).
3. **Region directives don't break contiguity.** The cursor
   advances only on `s`/`b` directives. `begin/end region`
   anchor at the cursor but don't consume runes.
4. **`c` is exclusive (unchanged).** A `c` directive cannot
   be combined with any other directive in the same write —
   region directives included.

### Parser validation

- `begin region` requires a kind argument; `begin region`
  alone is an error.
- v1 valid kinds: `code`. Future rounds add `blockquote`,
  `listitem`, `table`. Unknown kinds are an error (mirrors
  `family=NAME` and `placement=NAME`).
- Param tokens have the form `key=value`; malformed tokens
  are silently ignored (forward-compat). Whitespace within
  a value is not allowed in v1; quoted values deferred.

### v1 recognized region kinds and params

| Kind | v1 params | Notes |
|---|---|---|
| `code` | `lang=NAME` (optional) | Fenced or indented code. NAME is the language hint (e.g., `go`, `python`); v1 ignores at render time. |

## Storage — sidecar region tree

The consumer adds a region store alongside the existing
`spanStore`:

```go
// Region represents a scoped layout region in the body.
// Regions form a tree: a Parent is the enclosing region, or
// nil for top-level regions. Children are the directly-nested
// regions inside this one.
type Region struct {
    Start, End int             // rune offsets in the body, half-open [Start, End)
    Kind       string          // "code" in v1; future: "blockquote", "listitem", "table"
    Params     map[string]string
    Parent     *Region          // nil at top level
    Children   []*Region
}

// RegionStore manages the set of regions for a window's
// body. Regions form a forest of trees (multiple top-level
// regions, each potentially with nested children).
type RegionStore struct {
    roots []*Region
}
```

Operations the store must support:
- `Add(r *Region)` — insert a new region; place it under
  the right parent based on `Start`/`End` containment, or
  at the top level if no enclosing region exists.
- `EnclosingAt(pos int) *Region` — find the deepest region
  containing `pos`, or nil. Used by buildStyledContent to
  expand regions into per-rune Style flags.
- `Insert(pos, length int)` / `Delete(pos, length int)` —
  shift region offsets when the body is edited. Same shape
  as spanStore's edit operations.
- `Clear()` — drop all regions.

Why a tree (forest) rather than a flat list:
- Round 6 (nested blockquotes) and round 7 (nested lists)
  need parent pointers for inherited indent.
- `EnclosingAt` is O(depth) on a tree, vs. O(N) on a flat
  list; nesting depth is small (≤5).

Why a sidecar rather than per-rune StyleAttrs:
- A rune can be inside MANY regions at once (code inside
  blockquote inside listitem). Per-rune fields would need
  to encode arbitrary nesting; a sidecar tree is the
  natural representation.
- The per-rune `Style.Block`/`Style.Code`/`Style.Blockquote*`
  fields the rich.Frame's layout reads remain — they're
  computed at content-build time from the region tree.

### Region-store edits

When the user inserts or deletes runes in the body, region
offsets shift. The store mirrors the spanStore's pattern:

- `Insert(pos, length)`: regions whose `Start >= pos` shift
  their `Start` by `length`; regions whose `End >= pos`
  shift their `End` by `length`. (A region containing
  `pos` grows; a region entirely after `pos` shifts.)
- `Delete(pos, length)`: regions whose `[Start, End)`
  intersects `[pos, pos+length)` get clipped or split. v1
  is conservative: any region whose body is touched by a
  delete is dropped (the next render rebuilds it). This
  matches the spanStore's behavior on edits to box runs.

## Consumer pipeline

### Parser (`spanparse.go`)

Parser changes:
1. New directive prefixes `begin` and `end` recognized by
   `parseSpanMessage`.
2. New return value: `regions []Region` alongside the
   existing `runs []StyleRun`. The caller (in xfid.go)
   merges these into `spanStore` and `regionStore`.
3. Validation: balanced begin/end within the write;
   recognized kinds; valid param syntax.

The parser maintains a region stack to handle nesting:
each `begin region` pushes a frame; each `end region` pops.
At end-of-write the stack must be empty.

### Bridge (`wind.go`)

`buildStyledContent` consumes spanStore + regionStore
together. For each StyleRun, look up the enclosing region
via `regionStore.EnclosingAt(offset)`. If the enclosing
region's kind is `code`:
- `Style.Block = true`
- `Style.Code = true`
- `Style.Bg = rich.InlineCodeBg`

These flags drive the existing rich.Frame layout (gutter
indent, non-wrap, full-line background) — no rich.Frame
changes for round 5.

For rounds 6+ (blockquote): the bridge similarly sets
`Style.Blockquote = true` and `Style.BlockquoteDepth = N`
based on parent traversal in the region tree. The
rich.Frame's existing blockquote handling kicks in.

### Storage (`xfid.go`)

`xfidspanswrite` already routes `parseSpanMessage`'s
output to `spanStore`. Round 5 extends it to also route
the parsed regions to `regionStore`.

For consistency with `c` (clear) semantics: a `c` write
also clears `regionStore` alongside `spanStore`.

## md2spans

### Parser

Add fenced-code-block detection to `scanParagraphs`:
- A line of three or more backticks (`\`\`\``) opens a fenced
  code block; the optional info string after the fence is
  the language hint.
- The block continues until a matching closing fence (same
  number of backticks).
- All content between fences is the code body.

The fences themselves stay visible (markup-stays-visible),
but they are NOT inside the region — only the body lines
are. The region begins after the opening fence's newline
and ends at the closing fence's start.

### Emit

For each fenced code block, md2spans emits:
1. `s` directives for any preceding plain text (default fill).
2. `s OFFSET LENGTH -` over the opening fence runes (default
   styling; markers visible).
3. `begin region code lang=NAME` if a language hint was
   present; bare `begin region code` otherwise.
4. `s OFFSET LENGTH - family=code` over the body runes.
5. `end region`.
6. `s` over the closing fence runes (default styling).

The body runes get `family=code` so they render in
monospace even when the region machinery isn't yet in play
(forward-compat in case the consumer doesn't yet handle
regions). The region wrapper adds the background.

### Indented code blocks

CommonMark also recognizes 4-space-indented blocks as code.
v1 of round 5 supports fenced only; indented is deferred to
a future round. The in-tree path supports both, so until
md2spans gains indented support there's a gap with the
in-tree path's coverage. Documented as a v1 limitation.

## Renderer

**No rich.Frame changes for round 5.** The existing
`Style.Block && Style.Code` per-rune handling produces:
- Gutter indent (`rich/layout.go:625`)
- Non-wrap layout (`rich/layout.go:642`)
- Full-width background (existing `drawBlockBackgroundTo`)
- Monospace font for content (`Style.Code` triggers
  `fontForStyle` to return the registered code font)

The bridge expands the region into these per-rune flags;
the renderer reuses the existing pipeline.

When rounds 6-8 introduce nested regions, the per-rune
expansion may not be expressive enough — particularly when
rendering needs to know "the visual extent of THIS specific
region" rather than "is this rune in some region."
Round 5's design accepts this; the eventual region-aware
layout pass is a downstream concern.

## Edge cases / failure modes

| Case | Behavior |
|---|---|
| `begin region code` with no matching `end region` in same write | Parser error; whole write rejected |
| `end region` with empty stack | Parser error; whole write rejected |
| `begin region UNKNOWN_KIND` | Parser error; whole write rejected |
| `begin region code badparam` (malformed key=value) | Param silently ignored; region accepted |
| Empty region (begin immediately followed by end) | Region recorded with `Start == End`; renders as a zero-width code block (no body); the bridge produces no Block&&Code spans |
| Region spanning Twrites | Producer error; the protocol does not allow it |
| Edit inside a region's body | Region body grows/shrinks; offset shifts (Insert/Delete) |
| Edit deleting the begin/end markers | Region is dropped (entire region invalidated; next render rebuilds) |
| md2spans encounters unclosed fence | Treat as code block continuing to end-of-buffer (matches CommonMark's behavior) |

## Test plan

1. **`spanparse.go` tests**: balanced begin/end round-trips;
   unbalanced begin or end is an error; unknown kind is an
   error; param syntax (`lang=go`, `lang=`); region
   directives don't advance the contiguity cursor.
2. **`Region.Equal` and `RegionStore` tests**: forest
   construction; `EnclosingAt`; `Insert`/`Delete` offset
   shifts; nested regions (synthetic — round 5 doesn't
   produce them, but the store must handle them).
3. **`buildStyledContent` tests**: a region=code over body
   runes produces spans with `Style.Block=true, Code=true,
   Bg=InlineCodeBg`; runes outside the region are unaffected.
4. **md2spans parser tests**: fenced blocks recognized;
   language hint captured; opening/closing fences emitted
   as default-styled spans; body runes emitted with
   `family=code` inside a region; nested fences (a triple
   backtick inside a quadruple-backtick fence) — defer or
   handle.
5. **md2spans emit tests**: round-trip of a fenced block;
   wire-format format matches the spec examples; region
   directives slot between `s` directives correctly.
6. **Spec doc tests** (manual): example wire format matches
   what md2spans produces and what spanparse accepts.
7. **End-to-end smoke**: open a markdown file with one or
   more fenced code blocks, verify rendering matches the
   in-tree path's appearance.

## Non-goals (v1 round 5)

- **Indented code blocks**: deferred to a future round.
  v1 fenced-only.
- **Syntax highlighting in the region**: the `lang=NAME`
  hint is captured but ignored at render time. A future
  round (or `cmd/edcolor`-integration) may use it.
- **Nested code blocks**: CommonMark doesn't allow them;
  v1 doesn't either.
- **Region-aware layout**: rich.Frame layout stays per-rune
  flag-driven for round 5. The region tree is held in the
  consumer-side store but not exposed to the renderer
  beyond what the per-rune Style flags express.
- **Region-level hyperlinks / Look targets**: deferred.
- **Region bounds metadata for scrollbar / table-column
  alignment**: round 8 territory.

## Risks

1. **Wire-format directive growth.** Two new directive
   prefixes (`begin`, `end`) extend the prefix vocabulary
   beyond `c`/`s`/`b`. The parser's `switch prefix` grows
   by two cases. Acceptable extension; mirrors how `b` was
   added previously.
2. **Atomicity: regions cannot span Twrites.** md2spans
   must buffer regions before writing. Existing chunked
   writes may need to honor region boundaries. The current
   chunker (`writeChunked` in `cmd/md2spans/main.go`) splits
   on newline boundaries; we extend it to ALSO not split
   between a `begin region` and its matching `end region`.
3. **Region store edit logic.** `Insert`/`Delete` need to
   shift region offsets correctly. The spanStore had
   subtle bugs in this area (length-0 box drops in round
   4); the regionStore is simpler (no length-0 corner case)
   but still needs careful tests.
4. **Region tree construction at parse time.** A flat list
   of regions from one Twrite may need to be merged into
   an existing forest from prior writes. The merge logic
   (place new regions under the right parents based on
   containment) is straightforward but worth testing.
5. **Round 6+ may force a redesign.** The "translate to
   per-rune flags" bridge is sufficient for round 5's
   non-nested code blocks. Round 6's nested blockquote may
   not fit cleanly. We accept the risk — round 5's design
   commits to v1 of the region machinery; round 6 may
   refine.
6. **Visual parity with in-tree path.** The in-tree
   markdown path has specific rendering for code blocks
   (gutter indent, bg color, font). Round 5 must produce
   the same visual via the per-rune flag bridge. The
   per-line bg painting goes through `drawBlockBackgroundTo`
   which reads `Style.Bg` from the line's boxes; the bridge
   must set `Style.Bg = rich.InlineCodeBg` on every box in
   the region.

## Status

Design — drafted. Awaiting review.
