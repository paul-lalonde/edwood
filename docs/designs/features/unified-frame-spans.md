# Unified Frame + Spans: Design

**Audience.** A staff-level engineer implementing a clean-room re-do
of the styled-text work on top of upstream `rjkroege/edwood`. You
should read this as a spec, not a recipe. Where the spec is silent on
internal structure, exercise judgment.

**Source of truth.** Start from `upstream/master`. Behavior tests
from the current branch are reusable on review — *only after
confirming each test exercises observable behavior rather than the
incumbent implementation's internal data layout*. Do not port
helpers, types, or files wholesale.

---

## 1. Goals

1. **One frame implementation** that renders both plain text and
   styled text. Plain is the degenerate case (empty style metadata),
   not a separate code path on the caller side.
2. **Spans as a separate concern from the buffer.** A
   `spans.Store` tracks per-rune styling, observes the buffer for
   index alignment, and notifies dependents when styling changes
   without a buffer mutation.
3. **Text remains the buffer-and-UI owner.** It gains thin
   coordination logic: when filling the frame, it reads runes from
   the buffer *and* styles from spans, and emits them together.
4. **Viewport-only frame semantics.** The frame never sees runes
   outside its visible window. This is the property that makes
   plain-text performance equal to upstream's.
5. **Replaced elements (images, code blocks, tables) live in the
   spans-driven path.** A replaced element is a single buffer rune
   with a style attribute that tells the frame to render it as a
   non-character block.
6. **Sub-element y-offset for tall elements** is supported (a tall
   image can be scrolled through), and *only* for tall elements.
   Ordinary lines retain line-granular scroll.
7. **Outboard producers** (`md2spans`, `edcolor`, `dirthumb`, future
   tools) write styling via a 9P `spans` file. They do not modify
   the buffer to convey styling.
8. **Minimal upstream diff.** The diff `upstream/master..feature` is
   readable as: new packages (`spans`, possibly an enlarged
   `frame`), additive method surface on `frame.Frame`, a small
   field and a few-line callback shift in `Text`, a 9P qid in
   `xfid.go`, no mode flags in `wind.go`/`acme.go`/`exec.go`/
   `edit.go`/`look.go`.

## 2. Non-Goals

- **No replacement of Text** as the buffer + UI owner. Text-vs-
  styled-display duality (today's `richBody`/`body`) does not exist
  in this design.
- **No rich rendering of tag bars.** Tags use the unified frame in
  its plain-styles mode. Pinning this down keeps the tag path free
  of replaced-element complications.
- **No sub-line scroll for ordinary text.** Line-granular only,
  except inside tall replaced elements.
- **No new editing semantics in core.** Insert/Delete/Type/Undo
  behave exactly as upstream. The only thing producers do that
  upstream doesn't is write styling to a separate file.
- **No async layout.** Layout runs on the goroutine that calls the
  frame method.
- **No mode flags** (`IsStyledMode`, `IsPreviewMode`, `previewMode`,
  etc.) on Window. Whether a body has styling is a property of its
  spans store, not of the window.
- **No `HandleStyledMouse` or any parallel mouse-input loop.** All
  mouse input goes through `Text.Select`.
- **No second scrollbar implementation.** Upstream's `scrollbar.go`
  serves both modes.

## 3. Background

Upstream's `frame.Frame` is a viewport: it holds the visible
chunk of a larger buffer, fills itself via `Insert`/`Delete`, and
its public methods all operate on rune offsets within that visible
window. Layout is cheap because it never sees the rest of the
document.

Our work on this branch built a second, parallel frame
(`rich.Frame`) that takes a full styled document via `SetContent`
and renders a viewport on top. It has its own mouse loop,
scrollbar adapter, content cache, and tall-element scrolling. It
also necessitated a second display object (`RichText`/`richBody`)
maintained in sync with the editing `body` via
`UpdateStyledView`, plus `IsStyledMode`/`IsPreviewMode` flags
checked in core.

This document specifies a clean-room target that keeps the
viewport-only frame property and the upstream Text/Window
architecture, while adding styling and replaced-element rendering
in a way that's mostly invisible to core.

## 4. Architecture Overview

```
+----------------------------------+
|              Buffer              |   file.ObservableEditableBuffer
|        (runes + Undo/Redo)       |   (upstream, unchanged)
+----------------+-----------------+
                 | observer chain
        +--------+--------+
        | (1) spans.Store |              spans/ (new package)
        |  - GetStyleRuns |
        |  - Observe      |
        +--------+--------+
                 |
                 | observer chain
        +--------+--------+
        |    (2) Text     |              text.go (upstream + small additions)
        |  - reads buffer |
        |  - reads spans  |
        |  - drives frame |
        +--------+--------+
                 |
                 |
        +--------+--------+
        |  frame.Frame    |              frame/ (upstream + additive methods)
        |  - viewport     |
        |  - styled runs  |
        |  - replaced els |
        +-----------------+
```

Numbers `(1)` and `(2)` denote registration order on the buffer's
observer chain. Spans must update its rune ranges before Text
queries them.

**Outboard producers** write the spans file via 9P:

```
md2spans, edcolor, dirthumb  ──9P──>  /mnt/wsys/<id>/spans
                                            │
                                            ▼
                                       spans.Store
                                            │
                                            ▼
                                       observer fires
                                            │
                                            ▼
                                       Text.SpansChanged
                                            │
                                            ▼
                                  Frame.SetStyleRange (visible)
```

## 5. The Frame Interface

### 5.1 Public surface

The unified `frame.Frame` interface is *upstream's frame.Frame plus
two methods and two getters/setters*. Plain `Insert` is preserved
and continues to mean "insert with default styling." All upstream
behavior remains.

```go
package frame

type Frame interface {
    SelectScrollUpdater   // see §5.2

    Maxtab(int)
    GetMaxtab() int
    Init(image.Rectangle, ...OptionClosure)
    Clear(bool)
    Redraw(enclosing image.Rectangle)

    Ptofchar(int) image.Point
    GetSelectionExtent() (int, int)

    Select(*draw.Mousectl, *draw.Mouse,
           func(SelectScrollUpdater, int)) (int, int)
    SelectOpt(*draw.Mousectl, *draw.Mouse,
              func(SelectScrollUpdater, int),
              draw.Image, draw.Image) (int, int)

    DrawSel(pt image.Point, p0, p1 int, highlighted bool)
    DrawSel0(pt image.Point, p0, p1 int, back, text draw.Image)

    // ── styled additions ────────────────────────────────────

    // InsertWithStyle inserts r at frame-relative position p0
    // with parallel styling. styles, if non-nil, is a sequence
    // of (Len, Style) pairs whose Lens sum to len(r). Plain
    // Insert is the case styles==nil; semantically identical
    // to upstream Insert in that case.
    InsertWithStyle(r []rune, p0 int, styles []StyleRun) bool

    // SetStyleRange replaces the style of runes already in the
    // frame, range [p0, p1). The caller clips p0/p1 to the
    // visible window before calling.
    SetStyleRange(p0, p1 int, styles []StyleRun)

    // SetOriginYOffset clips the top of the frame's first
    // visible line by yPx pixels. Meaningful only when the
    // first visible rune is a "tall" replaced element. For
    // line-granular scrolling Text passes 0.
    SetOriginYOffset(yPx int)
    GetOriginYOffset() int
}
```

### 5.2 SelectScrollUpdater additions

`InsertWithStyle` is also needed on `SelectScrollUpdater` so the
drag-scroll callback (`Text.FrameScroll`) can refill while the
frame lock is held.

```go
type SelectScrollUpdater interface {
    GetFrameFillStatus() FrameFillStatus
    Charofpt(image.Point) int
    DefaultFontHeight() int
    Delete(int, int) int

    Insert([]rune, int) bool                       // upstream
    InsertByte([]byte, int) bool                   // upstream
    InsertWithStyle([]rune, int, []StyleRun) bool  // new

    IsLastLineFull() bool
    Rect() image.Rectangle
    TextOccupiedHeight(image.Rectangle) int
}
```

The plain `Insert` and `InsertByte` are retained as shims that call
`InsertWithStyle` with `nil` styles. They are kept on the interface
to minimize call-site churn in upstream Text.

### 5.3 Data types

The `Style` struct and `Kind` bitmask shown below are the final v1
surface. Both grow across the three implementation slices in §12:
Slice A adds `Kind`/`Fg`/`Bg` and the `KindColored` bit; Slice B
adds `FontIdx` and the bold/italic/underline/font-index bits;
Slice C adds the replaced-element fields, block-context bits, and
horizontal-scroll field. Each slice ships only the bits and fields
it consumes; later slices grow both.

`Kind` is the primary discriminator. `KindPlain` (the zero value)
means "upstream defaults" — runes with a plain `Style` render
identically to upstream `Insert`, and `IsPlain()` exists exactly to
let the frame and producers detect that fast path. Non-plain bits
identify which other `Style` fields the frame should consult.
Producers are responsible for keeping `Kind` consistent with the
data fields (e.g., setting `Fg` must come with `Kind |= KindColored`;
a `Style` with `Fg != nil` but `Kind == KindPlain` is treated as
plain and its colors are ignored).

```go
package frame

// StyleRun is a contiguous run of Len runes that share a Style.
// A slice of StyleRuns whose Lens sum to K applies to K runes.
type StyleRun struct {
    Len   int
    Style Style
}

// Style is the per-run attribute bundle the frame consumes during
// layout and rendering. Kind is the bitmask discriminator; the
// data fields are meaningful iff their corresponding Kind bit is
// set. The implementer should keep this lean — only fields the
// frame consumes belong here.
type Style struct {
    // Kind is a bitmask of which attributes are active. The zero
    // value (KindPlain) means "upstream defaults"; non-zero bits
    // gate the data fields below.
    Kind Kind

    // Colors. Meaningful iff Kind & KindColored != 0.
    // nil means use the frame's default for that role.
    Fg draw.Image
    Bg draw.Image

    // Font index. Meaningful iff Kind & KindFontIdx != 0.
    // 0 = default; rest are caller-defined.
    FontIdx int

    // Replaced element. Meaningful iff Kind & KindReplaced != 0.
    // The buffer still contains exactly one rune at this
    // position; the frame renders it as a single fat box of the
    // given dimensions instead of as a glyph. Tab and newline
    // are NOT replaced elements — they remain regular characters
    // whose layout is handled by the line-breaker.
    ReplacedWidth  int           // px; 0 = use intrinsic from ReplacedRef
    ReplacedHeight int           // px; 0 = use intrinsic
    ReplacedKind   ReplacedKind  // subtype classifier; see below
    ReplacedRef    string        // image URL, code-block id, etc.

    // Per-element horizontal scroll for wide replaced elements
    // (tables, code blocks, oversized images) whose intrinsic
    // width exceeds the frame width. Meaningful iff Kind &
    // KindReplaced != 0. Wheel events over the element update
    // it; the optional per-element horizontal scrollbar (§10.2)
    // updates it too.
    HOffset int

    // Block context: layout indentation, not glyph styling.
    // BlockquoteDepth meaningful iff Kind & KindBlockquote != 0.
    // KindInCodeBlock and KindInTable are bool-equivalents
    // expressed entirely via the bitmask.
    BlockquoteDepth int
}

// Kind is a bitmask of active style attributes. KindPlain is the
// zero value and means "upstream defaults" — IsPlain() returns
// true for any Style whose Kind is KindPlain. The bits are
// introduced across slices; bit positions are stable.
type Kind uint

// KindPlain is the zero value; it sits in its own const so the
// bit-position iota counter in the block below starts at 0.
const KindPlain Kind = 0

const (
    // Slice A
    KindColored Kind = 1 << iota  // = 1, Fg / Bg meaningful

    // Slice B (typographic variation that doesn't change line
    // height — all three are bare flag tokens in the published
    // spans protocol).
    KindBold     // = 2, bold weight
    KindItalic   // = 4, italic angle
    KindHidden   // = 8, glyph is not painted (frame still paints bg)

    // Slice B.4 (md2spans compatibility — no line-height change;
    // §12 Phase B4).
    KindHRule       // = 16, draw horizontal rule on the row containing this run
    KindCodeFamily  // = 32, render with the code (monospace) font variant

    // Slice C (replaced elements + block context — line height
    // and layout may vary).
    KindFontIdx     // = 64,  FontIdx field meaningful
    KindUnderline   // = 128, underline decoration
    KindReplaced    // = 256, Replaced* and HOffset meaningful
    KindBlockquote  // = 512, BlockquoteDepth meaningful
    KindInCodeBlock // = 1024
    KindInTable     // = 2048
)

// IsPlain reports whether s carries no styling — i.e., a frame
// asked to render this Style will produce output identical to
// upstream's plain Insert. Equivalent to s.Kind == KindPlain.
// Callers use this to take the fast path.
func (s Style) IsPlain() bool { return s.Kind == KindPlain }

// ReplacedKind classifies a replaced element. The Replaced*
// fields are gated by Kind & KindReplaced; this enum names the
// subtype.
type ReplacedKind int
const (
    ReplacedNone ReplacedKind = iota
    ReplacedImage
    ReplacedCodeBlock
    ReplacedTable
    ReplacedFixedBox  // colored box of explicit dimensions
)
```

The implementer should resist the urge to add convenience fields
(timestamps, layout caches, source coordinates) to `Style`. The
frame uses `Style` to render; nothing else should touch it.

### 5.4 Semantics

#### InsertWithStyle

- **Contract.** Insert `len(r)` runes at frame-relative offset `p0`,
  associating each rune with a style determined by the parallel
  `styles` slice. If `styles == nil` *or* every `StyleRun` in
  `styles` has `Style.IsPlain()`, the implementation takes the
  fast path: no per-rune style storage, no replaced-element
  check, equivalent to upstream `Insert`. Under the §5.3 Kind
  bitmask model, `IsPlain()` is exactly `Kind == KindPlain`, which
  means "upstream defaults" — so plain runs by construction need
  no styling work.
- **Style assignment.** `styles[0]` applies to runes `[0,
  styles[0].Len)`; `styles[1]` to `[styles[0].Len, styles[0].Len +
  styles[1].Len)`; etc. The sum of `Len` fields must equal `len(r)`
  — the frame may panic on mismatch (developer error).
- **Replaced element runes.** If a rune has `Style.Replaced ==
  true`, the frame stores that rune as a single box of dimensions
  `(Style.ReplacedWidth, Style.ReplacedHeight)`. Line breaking
  treats it as one unbreakable character. If the replaced element
  is too wide for the line, the line is forced to that width and
  the element is the only visible glyph on it.
- **Line breaking.** As upstream: by character or word, subject to
  frame width. Style attributes do not change line-break rules
  except for replaced elements (single-character indivisible) and
  blockquote/table context (which may shift line start by a
  computed indent).
- **Return.** `true` if all runes fit in the frame; `false`
  otherwise. Matches upstream `Insert`.

#### SetStyleRange

- **Contract.** Re-style the runes already at frame-relative range
  `[p0, p1)` using `styles`. Sum of `Len` in `styles` must equal
  `p1 - p0`. The frame must:
  1. Update its per-rune style storage for the range.
  2. Recompute layout for affected lines (line heights may change
     if a replaced element appears/disappears, otherwise widths
     don't change).
  3. Repaint the affected region. Repaint is synchronous; the
     caller calls `display.Flush()` after.
- **Caller responsibility.** Clip to the visible window before
  calling. The frame may reject out-of-range arguments (debug
  panic) — it never silently extends.
- **No effect on selection.** SetStyleRange does not move `p0`/`p1`
  of the selection.

#### SetOriginYOffset

- **Contract.** Render the frame's first visible line clipped at
  the top by `yPx` pixels. Meaningful only when the first visible
  rune is a replaced element whose height exceeds the line's
  natural height. For ordinary lines, the implementation must
  treat any `yPx != 0` as `yPx = 0` (clamped) — i.e., it is a no-op
  outside the tall-element case.
- **Persistence.** The y-offset persists until SetOriginYOffset is
  called again or the frame is re-filled (origin changes). After
  any `Delete(0, *)` that drops the first visible rune, the
  implementation must reset y-offset to 0.

## 6. The Spans Package

### 6.1 Public surface

```go
package spans

// Store maintains per-rune styling for one buffer. It is
// expected to be installed as an observer on
// file.ObservableEditableBuffer *before* any UI observers
// (Text), so that UI callbacks see post-update spans.
type Store interface {
    // Empty reports whether any non-plain region exists (any
    // region whose Style.IsPlain() is false). Callers can
    // short-circuit style-query work entirely when Empty() is
    // true (most files in plain mode).
    Empty() bool

    // GetStyleRuns returns the styling for rune range [p0, p1).
    // The returned slice covers the full range — runes not
    // explicitly styled appear in runs of Style{}. The sum of
    // Lens equals p1-p0. The slice is owned by the caller;
    // implementations may reuse internal buffers via a pool but
    // must return a stable slice for the duration of the call.
    GetStyleRuns(p0, p1 int) []frame.StyleRun

    // Observe registers fn for style-only updates (changes that
    // do not stem from a buffer mutation). fn receives the rune
    // range that was re-styled. Calls are made on the goroutine
    // that triggered the change.
    Observe(fn func(p0, p1 int))

    // SetRegion replaces (or creates) a styled region covering
    // rune range [p0, p1) with style s. Used by 9P producers
    // and by edit commands. Triggers all Observe callbacks.
    SetRegion(p0, p1 int, s frame.Style)

    // ClearRegion removes any styling in [p0, p1), restoring
    // the runes to plain style. Triggers Observe callbacks.
    ClearRegion(p0, p1 int)

    // Snapshot returns a copy of the current store state, for
    // debugging / serialization.
    Snapshot() []Region
}

// Region is one styled extent in the store.
type Region struct {
    Start  int          // rune offset (document-absolute)
    Length int
    Style  frame.Style
}

// NewStore creates a Store. attach hooks it onto buf's observer
// chain. Must be called before Text registers its observer.
func NewStore(buf file.ObservableEditableBuffer) Store
```

### 6.2 Buffer observer behavior

Spans implements `file.BufferObserver` (or whatever upstream's
observer interface is called). The handlers:

- **Inserted(q0, b, nr).** For each region `[s, e)`, apply the
  trailing-edge extension rule:
  - If `q0 < s`: the region shifts forward by `+nr` (length
    unchanged). Inserted runes precede the region; they are
    unstyled.
  - If `q0 == s`: the region shifts by `+nr` (length unchanged).
    Inserted runes precede the region. *Leading-edge insertion
    does not extend.*
  - If `s < q0 ≤ e`: the region grows by `+nr` (the inserted runes
    join the region). *Trailing-edge insertion extends.*
  - If `q0 > e`: the region is untouched by this insertion in
    isolation; downstream regions still shift per the rules
    above.

  Rationale. The asymmetric trailing-edge rule matches text-editor
  convention: typing inside a heading (or at the cursor at end of
  the heading) extends the heading; clicking *just before* a
  styled region and typing produces plain text that precedes the
  region. Producers re-span on save/refresh, but the live-edit
  experience stays right in the meantime.
- **Deleted(q0, q1).** For each region that intersects `[q0, q1)`:
  if entirely contained, drop it; if straddles either edge, clip
  the surviving portion; if the region wraps the deletion, shrink
  by `q1 - q0`. After shifting, regions whose Start ≥ q1 shift by
  `-(q1 - q0)`.

Both `Inserted` and `Deleted` are *internal book-keeping only* —
they do not trigger `Observe` callbacks. The buffer observer
chain already notifies Text directly; Text's handlers query
spans for the updated state.

### 6.3 GetStyleRuns

Implementation must be O(log n + result-size) where n is the
number of regions. A sorted slice with binary search is fine; a
skip list or balanced tree is overkill for the expected sizes (a
few hundred regions for a typical markdown document).

**Output guarantee.** For input `[p0, p1)`:
- Sum of `Len` fields equals `p1 - p0`.
- No `Len == 0` runs.
- Adjacent runs with identical `Style` *may* be coalesced or may
  not be — callers must not depend on coalescing.

### 6.4 Persistence and the 9P spans file

Each window exposes a `spans` file in its `wsys` directory (new
qid `QWspans`). The wire format is **published externally** in
`docs/designs/spans-protocol.md` of the upstream repository; this
section summarizes only the Slice A subset our parser accepts.
Slices B and C extend the parser to cover the rest.

**Slice A directives:**

```
c                                  # clear all spans for the window
s <off> <len> <fg> [<bg>]          # styled run; positional colors
```

- `<fg>`, `<bg>` are each `#rrggbb` (lowercase 6-hex) or `-`
  (default). `<bg>` is optional.
- Producers fill gaps in the styled region with default-styled
  lines (`s <off> <len> -`).
- The published protocol's flag tokens (`bold`, `italic`,
  `hidden`, `hrule`, `scale=N.N`, `family=NAME`) are
  **silently accepted** in Slice A — the parser recognises and
  discards them so producers like the prior `edcolor` work
  unmodified, but the styling they convey does not yet apply.
  Slice B translates `bold` / `italic` / `hidden` into real
  rendering; Slice B.4 (§12 Phase B4) translates `hrule` and
  `family=code`. Unknown flag spellings remain errors.
- The `b` directive (replaced elements) and `begin region` /
  `end region` directives are rejected entirely in Slice A. In
  Phase B4 (§12) the parser **silently accepts** them — it
  consumes the line and emits `OpNoOp` so md2spans output
  flows through without errors — but the directives are
  inert until Slice C wires them up.

**Per-write rules** (the parser enforces these):

1. **`c` is exclusive.** A `c` directive must be the only line in
   its 9P write.
2. **`s` contiguity.** Within a write, each `s` directive's
   `<off>` must equal the previous directive's `<off> + <len>`.
   The first `s` sets the region's start; gaps inside that
   region must be filled by the producer with default-styled
   lines.
3. **Out-of-range tolerance.** A directive whose `<off>` ≥
   buffer rune count is silently dropped; a directive whose
   `<off> + <len>` exceeds the buffer bound is clamped.
   Producers should not rely on these behaviors; they exist as
   defensive clamps.

**Read protocol.** Reading the spans file is supposed to dump
the current `Snapshot()` in the same line format. Slice A
returns an empty body — the dense store loses the original
`color.Color` once it's resolved into a `draw.Image`, so
faithful round-trip serialization needs a side-channel that
hasn't been added yet. Defer until needed (Slice B will likely
add it alongside the font-aware producers).

**Atomicity.** Each 9P Twrite is parsed independently. The two-
write idiom (`c\n` followed by a contiguous `s` block in a
separate write) is the recommended way to fully replace styling.

## 7. Text Changes

The set of edits to upstream's `text.go` is intentionally small.

### 7.1 New field

```go
type Text struct {
    // ... upstream fields ...

    spans spans.Store // nil means plain text; no styling work
                      // is done for this Text.
}
```

### 7.2 Init / construction

A Text is configured with its spans store (or nil) at construction.
For bodies this happens via Window construction (§ 8). For tags
it is always nil.

### 7.3 Buffer observer: Inserted

Upstream's `Text.Inserted` is roughly:

```go
func (t *Text) Inserted(q0 OffsetTuple, b []byte, nr int) {
    // ... compute visibility, decide if frame needs update ...
    if visible {
        t.fr.Insert(runes, framePos)
    }
}
```

The change is to read styles when visible and call
`InsertWithStyle`:

```go
func (t *Text) Inserted(q0 OffsetTuple, b []byte, nr int) {
    // ... compute visibility, decide if frame needs update ...
    if visible {
        var styles []frame.StyleRun
        if t.spans != nil && !t.spans.Empty() {
            styles = t.spans.GetStyleRuns(q0.R, q0.R+nr)
        }
        t.fr.InsertWithStyle(runes, framePos, styles)
    }
}
```

When `styles == nil`, the call is semantically equivalent to
upstream's `t.fr.Insert(runes, framePos)`.

### 7.4 Buffer observer: Deleted

No change. Style metadata for deleted runes is cleared by the
frame's own `Delete` (per-rune style array shrinks alongside the
rune array).

### 7.5 setorigin / fill

Wherever `Text.setorigin` or `Text.fill` reads runes from the
buffer to push into the frame, it must also query spans for the
same range:

```go
func (t *Text) fill(fr SelectScrollUpdater) error {
    for /* each chunk to insert */ {
        // ... read runes from buffer ...
        var styles []frame.StyleRun
        if t.spans != nil && !t.spans.Empty() {
            styles = t.spans.GetStyleRuns(chunkStart, chunkEnd)
        }
        fr.InsertWithStyle(chunkRunes, framePos, styles)
        // ... existing fill-loop bookkeeping ...
    }
}
```

`setorigin` additionally must drive `SetOriginYOffset`:

```go
func (t *Text) setorigin(fr SelectScrollUpdater, org int,
                         exact, calledfromscroll bool) {
    // ... existing logic ...
    yPx := t.computeTallElementYOffset(org)
    fr.SetOriginYOffset(yPx)  // 0 in the common case
}
```

`computeTallElementYOffset` is a small helper on Text:
- Look up the spans run at rune `org`.
- If it represents a replaced element AND its height >
  `2 * fr.DefaultFontHeight()`, compute the sub-element pixel
  offset from Text's separate `tallY` state.
- Otherwise return 0.

The `tallY` state is updated by scroll inputs (mouse wheel, B1
scrollbar click, programmatic Show) when those inputs would
otherwise scroll past a replaced element's interior. See § 9.

### 7.6 attachSpans helper

```go
func (t *Text) attachSpans(s spans.Store) {
    t.spans = s
    s.Observe(func(p0, p1 int) {
        v0, v1 := t.org, t.org+t.fr.GetFrameFillStatus().Nchars
        if p1 <= v0 || p0 >= v1 {
            return
        }
        if p0 < v0 { p0 = v0 }
        if p1 > v1 { p1 = v1 }
        runs := s.GetStyleRuns(p0, p1)
        t.fr.SetStyleRange(p0-t.org, p1-t.org, runs)
        t.fr.Redraw(t.fr.Rect())  // or display.Flush(), per
                                  // upstream's pattern
    })
}
```

## 8. Window Changes

### 8.1 Construction

Window construction follows upstream until it adds the spans store
and registers it on the buffer's observer chain *before* adding
Text as an observer:

```go
func makenewwindow(...) *Window {
    // ... upstream window setup ...
    buf := w.body.file
    spans := spans.NewStore(buf)  // registers itself first
    w.body.attachSpans(spans)     // Text registers second
    // ... rest of upstream setup ...
    return w
}
```

The ordering is the single architectural invariant the design
places on outside code. Implementers should add an assertion (or
a test) that catches reversed ordering.

### 8.2 No mode flags

Window does **not** gain `IsStyledMode`, `IsPreviewMode`,
`SetPreviewMode`, `UpdateStyledView`, `richBody`, or related
state. The presence of styling in a body is reflected entirely in
its `spans.Store.Empty()` answer; there is no separate "I am in
rich mode now" boolean.

### 8.3 9P spans file

`xfid.go` gains a new qid `QWspans`. Opens write-only or read-
only. Writes parse directives and apply to the window's
`spans.Store`. Reads dump the store's snapshot in the same line
format. Multiple concurrent writers are not supported (last
writer wins per directive).

### 8.4 Tag bars

Tag bars use the unified frame in its plain-styles mode: their
Text has `t.spans == nil`. No spans store is allocated for tags.
Any attempt by external producers to direct styling at a tag is
silently dropped (the spans file does not exist for tags).

## 9. Mouse Handling

All mouse handling for body Text goes through upstream's
`Text.Select`. This is the single mouse loop. There is no
`HandleStyledMouse`, no `richBody`-specific path, no parallel
chord-detection state machine.

The unified frame's `Select` matches upstream's signature:
`Select(mc, m, getmorelines)`. The drag-scroll callback
(`Text.FrameScroll` → `Text.setorigin`) runs as in upstream, the
only change being that `setorigin` emits `InsertWithStyle`
instead of `Insert` and may set a non-zero y-offset.

### 9.1 Replaced elements during selection

A click landing inside a tall replaced element resolves to the
element's rune via `Charofpt`. The frame computes this from the
visible-window layout, taking the y-offset into account. There
is no special-case selection logic in Text.

### 9.2 Drag-scroll across replaced elements

Drag-scroll is line-granular (one or more lines per tick,
matching upstream's `(distance/fontHeight)+1` formula). When the
scroll origin moves onto a replaced element, the y-offset is
reset to 0 and the element is shown from the top. To scroll
*through* a tall element (revealing its interior), the user uses
the scrollbar or mouse wheel, which can target a specific
y-offset within the element. Drag-scroll deliberately does not
scroll mid-element; this matches upstream acme's line-granular
feel.

### 9.3 The 'S' event for external producers

This design adds *exactly one* new event-file character to
upstream's vocabulary: `S`, body-only. There is no tag-side `s`.

#### Why a new event is needed

Upstream's event vocabulary is `D`/`d`, `I`/`i` (delete/insert),
`X`/`x` (B2 execute), `L`/`l` (B3 look). None of these fires on
*selection change without buffer mutation*: `L` only fires on
B3, missing B1 sweep selection, programmatic `Show`, cursor
moves from typing. Producers like `edcolor` that highlight all
matches of the current selection as the user moves the cursor
need a way to learn about selection changes.

The alternatives are:

1. **Polling the `addr` file** every few hundred ms. Chatty over
   9P, laggy in the user-visible feel.
2. **Reusing `L`.** Doesn't cover B1 sweep / programmatic
   selection.
3. **A new event char `S`.** Single byte of new vocabulary, gated
   on opt-in conditions so it never fires unless a producer is
   actually present and the body is styled.

The design chooses option 3.

#### Format

Identical shape to upstream event-file messages, no text payload:

```
S<q0> <q1> 0 0 \n
```

`q0` and `q1` are the new selection bounds in document-absolute
rune offsets. The trailing `0 0` are the flag and text-length
fields (no text payload), consistent with upstream's event
format.

There is no lowercase tag variant. Tag bars deliberately do not
emit selection events — tags are the editor's command line, and
allowing external apps to react to tag selection invites
behavior the user can't predict from the tag's appearance.

#### Emission conditions

`Text.SetSelect` emits the `S` event when *all* of the following
hold:

- The Text is a body (`t.what == Body`), not a tag.
- The window has an active event-file handler (`nopen[QWevent] >
  0`). This is upstream's standard "is anyone listening" gate
  for any event emission.
- The Text's spans store is non-nil. The presence of a spans
  store is the design's signal that "this body is styled and a
  producer is involved." Avoids `S` events on plain bodies that
  no producer cares about.
- The selection actually changed (`q0`/`q1` differ from prior
  values stored on the Text).

When any of these is false, no event fires.

#### B3 plumb override is a separate concern

Producers that want to override what B3-look plumbs for styled
runes (markdown link → URL, image marker → image path, etc.) do
so via the existing upstream `L` event — unchanged from upstream:

1. Producer receives the `L` event for a B3 click in the body.
2. Producer looks up the appropriate plumb target from its own
   state (e.g., the markdown AST it built when generating the
   spans).
3. Producer sends a plumb message directly (via the plumber's
   send port) with the desired target.

This flow uses no new event vocabulary and no in-process frame
hooks. The producer is the authority on what styled text means
and so is also the authority on what B3 on styled text should
plumb.

#### What `S` does *not* do

- It does not carry any text payload. Producers that need the
  selected text read it from the body via existing data-file
  mechanisms.
- It does not fire on tag selection.
- It does not fire on plain bodies (no spans store attached).
- It does not subsume `L`. Producers that only care about
  explicit B3 should continue to watch `L`; `S` is the
  finer-grained signal.

## 10. Scrollbars

### 10.1 Vertical scrollbar

Upstream's `scrollbar.go` is the single implementation. Its
`model` interface (`Geometry`, `OriginAtPixel`, etc.) is
satisfied by Text's existing adapter. The unified frame provides
whatever low-level operations the adapter needs to compute model
values; for tall-element sub-scrolling, the adapter consults
`GetOriginYOffset` and Text's `tallY` state when computing
scrollbar thumb position.

There is no separate `richScrollbarModel`.

### 10.2 Horizontal scrollbar (per-replaced-element)

Replaced elements whose intrinsic width exceeds the frame width
(wide code blocks, wide tables, oversized images) carry a
horizontal scroll state in their `Style.HOffset` field. The
frame consults `HOffset` during render and clips/translates the
element's contents accordingly.

**Input routing.** When a mouse wheel event arrives over a
replaced element with `ReplacedWidth > frameRect.Dx()`, the
wheel delta updates `HOffset` (clamped to `[0, ReplacedWidth -
frameRect.Dx()]`) instead of scrolling the frame vertically.
The frame exposes `HScrollAt(pt) (q, ok bool)` for hit-testing
the cursor position against any wide replaced element; Text's
mouse path uses this to decide whether wheel input goes to
horizontal-block-scroll or to vertical-frame-scroll.

**Optional widget.** The frame may render a thin horizontal
scrollbar at the bottom edge of each wide replaced element.
Click/drag on this widget updates `HOffset` directly. The
widget is purely a render-time affordance — it shares no code
with the vertical scrollbar.

**No global horizontal scroll.** The frame's top-level
horizontal scroll state remains as in upstream (none). Lines
wrap at frame width; only wide replaced elements have an
internal horizontal axis.

**HOffset persistence.** `HOffset` lives in the spans store
(it's a field of the rune's `Style`). Edits to the spans store
that update the replaced element's region (e.g., a producer
re-emits the element after a buffer change) reset `HOffset` to
0 unless the producer preserves it. Treat `HOffset` as
view-state that the user controls; producers should not write
non-zero values unprompted.

## 11. Outboard Producers

Three reference producers exist on the current branch and should
be re-implemented:

- **md2spans.** Reads a markdown body, writes spans directives for
  headings, emphasis, lists, code blocks, images, tables, etc.
- **edcolor.** Reads a source code body, writes spans directives
  for syntax coloring. Per-language.
- **dirthumb.** Reads a directory listing body, writes spans
  directives that turn each entry into a clickable thumbnail
  (using `b` directives for image-replaced elements).

All three should be clean-room rewrites against the spans 9P
file. Their *protocol* (which directives they emit, what fields
of `Style` they use) is part of the upstream-able contract; their
*implementation* is unconstrained.

## 12. Implementation Plan (Three Vertical Slices)

The implementation lands in three vertical slices. Each slice is
shippable end-to-end: at the close of a slice, the editor works,
the regression suite is green, and a real producer can exercise
the slice's surface. Every prior slice must keep working after the
next slice lands.

- **Slice A — Coloring.** Minimum wiring to make a clean-room
  `edcolor` work. Same-height lines, no font variation, no
  replaced elements. End state: `edcolor` syntax-colors a source
  file and highlights matches of the current selection via the
  `S` event.

- **Slice B — Typographic variation.** Adds the font/emphasis
  fields (`Bold`, `Italic`, `Underline`, `FontIdx`) and the
  variable line-height recompute that varied-size text demands.
  End state: a producer can emit body text with mixed bold,
  italic, underline, and size, and the frame lays it out
  correctly.

- **Slice C — Replaced elements and block context.** Adds the
  rest of the §5.3 `Style` surface: replaced-element fields,
  block-context fields (`BlockquoteDepth`, `InCodeBlock`,
  `InTable`), and the horizontal-scroll field (`HOffset`). End
  state: `md2spans` and `dirthumb` ship; full markdown rendering
  works.

The `Style` struct grows monotonically across slices. Interface
signatures (`InsertWithStyle`, `SetStyleRange`,
`SetOriginYOffset`) are final from Slice A; their implementations
grow.

### Phase 0 — Setup

- Branch from `upstream/master`. Confirm all upstream tests pass.
- Set up CI (or local test runner) to keep upstream's plain-text
  test suite as a regression baseline. Every subsequent phase
  must keep it green.

---

### Slice A — Coloring

#### Phase A1 — Frame data types (color-only)

- Add `frame.StyleRun`, `frame.Style{Kind, Fg, Bg}`, `frame.Kind`
  (bitmask with `KindPlain = 0` in its own const, and
  `KindColored = 1 << iota = 1` opening the bit-position block),
  `frame.ReplacedKind` (declared, no consumer yet).
- Add `Style.IsPlain()` (`return s.Kind == KindPlain`) so callers
  can detect upstream-defaults styles in a single integer compare.
- No interface changes yet. Just types.

#### Phase A2 — Frame styled methods (color-only impl)

- Add `Frame.InsertWithStyle`. Fast path when `styles == nil` or
  every run is `Style{}` matches upstream `Insert`. Styled path
  stores per-rune Fg/Bg and applies them at render time.
- Add `Frame.SetStyleRange`. Color-only behavior: update per-rune
  Fg/Bg storage, repaint affected region. *No line-height
  recompute* (line height is invariant across Slice A).
- Add `Frame.SetOriginYOffset` / `GetOriginYOffset` as stubs
  (always 0, no-op). Real behavior arrives in Slice C.
- Plain-text behavior identical to upstream; regression suite
  stays green.

#### Phase A3 — Spans package

- Implement `spans.Store` against the buffer observer interface.
- `Inserted` / `Deleted` rules per §6.2, trailing-edge extension
  included from the start (the rule is the same across all
  slices; only the `Style` shape grows).
- Implement `GetStyleRuns`, `SetRegion`, `ClearRegion`,
  `Observe`, `Snapshot`.
- Unit tests cover the full §6.2 / §6.3 surface.

#### Phase A4 — Text wiring (no producers)

- Add `Text.spans` field, `attachSpans` helper.
- Modify `Text.Inserted`, `Text.fill`, `Text.setorigin` to query
  spans when present.
- Wire spans construction into Window setup; assert the §8.1
  observer-order invariant.
- With no producer writing to spans, every body is
  byte-identical to upstream.

#### Phase A5 — 9P spans file (color-only directives)

- Add `QWspans` qid in `xfid.go`.
- Directive parser/serializer for `s` (set style) and `c` (clear).
- Style encoding restricted to `fg=` and `bg=` for Slice A; `b`
  directives and font/replaced encodings come in B and C.
- Read path dumps Snapshot in the same line format.
- Manual integration test: hand-written producer writes directives;
  visible styling matches.

#### Phase A6 — 'S' event

- Implement emission in `Text.SetSelect` per §9.3 (body-only;
  spans attached; event listener present; selection actually
  changed).
- Tests cover all four gating conditions: spans nil suppresses,
  no event listener suppresses, tag emission suppresses,
  unchanged selection suppresses.

#### Phase A7 — `edcolor` clean-room rewrite

- 9P client of the `spans` file. Per-language syntax coloring.
- Watches `S` events to highlight matches of the current
  selection.
- Golden-output tests on representative source files.

**Exit criterion for Slice A.** `edcolor` syntax-colors a Go (or
other supported-language) file in an acme window, and
highlights-on-selection behave correctly. Plain-text bodies and
tag bars are byte-identical to upstream. Regression suite green.

---

### Slice B — Typographic variation

Builds on Slice A. Same `frame.Frame` interface signatures; the
`Style` struct grows; `SetStyleRange` and the frame's line breaker
learn to recompute line height when font flags change.

#### Phase B1 — Font fields on Style

- Extend `frame.Kind` with `KindBold`, `KindItalic`,
  `KindUnderline`, `KindFontIdx` bits.
- Extend `frame.Style` with the `FontIdx` value field (gated by
  `KindFontIdx`); bold/italic/underline are expressed entirely
  through the bitmask, no bool fields.
- `IsPlain()` requires no change — it already covers any
  non-`KindPlain` Kind value, including the new bits.
- Update the 9P directive parser to recognize `bold=`, `italic=`,
  `underline=`, `font=` keys; producers set the corresponding
  Kind bits.
- No consumer of the new bits yet; producers can emit them but
  no shipping producer does in this phase.

#### Phase B2 — Frame variable-height line breaking

- Frame's line-breaker consults per-rune font flags to compute
  glyph advances and per-line height.
- `SetStyleRange` recomputes line height for affected lines when
  font-flag deltas change glyph metrics; repaint includes the
  reflowed lines.
- Tests: range flips bold/non-bold (same height); range flips to
  larger `FontIdx` (height increases); wrap behavior with mixed
  fonts on a line; scroll math correctness when line heights
  vary.

#### Phase B3 — (optional) heading-only `md2spans` — **SKIPPED**

- A minimal `md2spans` that emits heading and emphasis directives
  only. Enough to demonstrate Slice B end-to-end on a markdown
  file before Slice C lands the full producer.
- Skipped: we reuse the existing external `md2spans` (from
  `/Users/paul/dev/edwood/cmd/md2spans`) once Phase B4 lands the
  parser surface and the small render wins that bring its output
  to a visually meaningful subset.

#### Phase B4 — md2spans compatibility (parser surface + small render wins)

The external `md2spans` emits the published-protocol's full surface
including directives Slice A rejects (`b`, `begin region`,
`end region`) and flag tokens Slice A/B silently accept but don't
render (`hrule`, `family=NAME`, `scale=N.N`). Until Slice C lands the
heavy work — `b` rendering, block-context layout, variable line
height for `scale=` — md2spans output crashes the parser the moment
it reaches a `begin region` line, so nothing renders.

This phase brings md2spans **end-to-end** by (a) widening the
parser so its full output is accepted, and (b) translating two
remaining flag tokens — `hrule` and `family=code` — into real
rendering. Both are typographic decorations that do not change
line height, so they fit the Slice B exit invariants. Three
remaining tokens (`scale=N.N`, `family=NAME` for non-code, the
`b`/region directives themselves) stay inert here and are picked up
in Slice C.

Before the new bits land, a small refactor consolidates layout
machinery so this slice and Slice C don't keep multiplying paint
sites. See R-B4.12 / R-B4.13 below.

Numbered requirements:

- **R-B4.1** (parser).  `ParseDirective` returns
  `Directive{Op: OpNoOp}` (a new op) for `b <args>`,
  `begin region <args>`, and `end region [<args>]` lines instead
  of an error. Malformed `b` lines that are not parseable as the
  published spec (missing required fields) still error — silent
  accept only covers well-formed lines.
- **R-B4.2** (parser).  Contiguity validation in `ParseAll` finds
  the previous `OpSetStyle` directive across intervening `OpNoOp`
  directives. Two `s` directives separated by `begin region` must
  still be contiguous; the parser does not silently allow a gap.
- **R-B4.3** (parser).  The `hrule` flag token on an `s` directive
  sets `Directive.Kind |= frame.KindHRule`.
- **R-B4.4** (parser).  The `family=code` flag token on an `s`
  directive sets `Directive.Kind |= frame.KindCodeFamily`. Other
  `family=NAME` values remain silent-accept-no-bit (unchanged from
  Slice A).
- **R-B4.5** (frame data types).  `frame.Kind` adds `KindHRule`
  and `KindCodeFamily` bits at the positions defined in §5.3.
  `IsPlain()` is unchanged — any non-zero `Kind` is non-plain.
- **R-B4.6** (frame opts).  `frame.OptCodeFont(f draw.Font)`
  installs a code-family variant font on the frame, analogous to
  `OptBoldFont`. The frame stores it in a new `fontCode` field.
- **R-B4.7** (frame font selection).  `fontFor(Style)` returns the
  code font when `Kind & KindCodeFamily != 0` and the code font is
  configured; otherwise it falls back to the existing
  weight/italic resolution. A frame without `OptCodeFont`
  configured renders `KindCodeFamily` runs in the base font (graceful
  degradation, same pattern as bold/italic).
- **R-B4.8** (frame rendering).  After painting glyphs and
  background, a box whose `Style.Kind & KindHRule != 0` has a
  1-pixel horizontal line drawn across its rectangle at the row's
  vertical center. The line uses the box's effective foreground
  color (`Style.Fg` if set, else the frame text color). Both
  `drawtext` and `repaintBoxRange` honor this rule so the line
  appears on initial paint and on re-style.
- **R-B4.9** (frame rendering).  The marker glyphs themselves
  continue to render normally (the "markers stay visible" stance
  shared by every other v1 directive). The horizontal line is
  drawn over the same row but does not suppress the glyphs.
- **R-B4.10** (Text wiring).  `acme.tryLoadFontVariant` probes the
  base font's family for a code variant (the existing GoMono path
  it already probes for bold/italic falls under this) and threads
  the result through to `frame.Init` via `OptCodeFont`. If no
  variant is found, the frame is built without it and Slice B
  graceful-degrades.
- **R-B4.11** (regression).  Slice A and Slice B producers
  (`edcolor`, hand-issued color directives) continue to work with
  no behavior change. `./regression.sh` green.
- **R-B4.12** (refactor — `paintBox`).  A single method
  `(*frameimpl).paintBox(b *frbox, pt image.Point, text, back draw.Image)`
  consolidates per-box styled paint. It is the only function
  in `frame/draw.go` that resolves a box's effective font
  (`fontFor`), resolves a box's effective fg/bg from
  `KindColored`, paints the box's background rect, paints
  glyphs, applies `KindHidden`, or paints decorations
  (`KindHRule` after B4.2 lands; future `KindUnderline` from
  Slice C lands the same way). `drawtext` and
  `repaintBoxRange` reduce to walk-and-call loops over
  `paintBox`. The observable invariant: adding a new
  decoration that paints on every render path requires a
  one-site edit in `paintBox`.
- **R-B4.13** (refactor — `boxWid`).  A single helper
  `(*frameimpl).boxWid(b *frbox) int` returns the width a
  content box (`b.Nrune > 0`) should carry given its current
  `(Style, Ptr)`. Every site that previously inlined
  `f.fontFor(b.Style).BytesWidth(b.Ptr)` (or
  `StringWidth(string(b.Ptr))`) calls `boxWid` instead.
  `validateboxmodel` (under `-validateboxes`) asserts
  `b.Wid == f.boxWid(b)` for every content box; this
  structurally prevents the SetStyleRange-forgot-Wid bug
  class. Special boxes (tabs/newlines, `b.Nrune < 0`) are out
  of scope and retain their tabstop/metric-driven widths.

Not in scope for Phase B4:

- `scale=N.N` rendering (variable line height — Slice C).
- `b` directive *rendering* (the parser accepts but ignores; Slice
  C C1 lands the replaced-element render path).
- `begin region` / `end region` *semantics* (the parser accepts
  but the applier discards; Slice C C4 lands block-context layout).
- `family=NAME` for non-`code` values (no defined behavior; v1
  external `md2spans` only emits `family=code`).
- An hrule line that spans the full *row* width instead of just
  the marker box's `Wid`. v1 draws within the box; a full-row
  hrule is a polish item if visible ugliness motivates it.

**Exit criterion for Phase B4.** External `md2spans` runs against
the cleanroom edwood end-to-end. Body text renders bold, italic,
bold-italic, inline-link colors, **horizontal rules**, and inline
`family=code` spans (and fenced code-block bodies, which md2spans
also emits as `family=code` over the body runes). Headings,
images, code-block backgrounds, blockquote indents, and list
markers stay unstyled until Slice C lands their machinery.
Slice A/B producers unaffected.

**Exit criterion for Slice B.** Body text can carry mixed bold,
italic, underline, and font sizes; line heights adapt; selection,
cursor behavior, and Slice A producers (`edcolor`) all still
work. Regression suite green.

---

### Slice C — Replaced elements and block context

Builds on Slices A and B. Adds the remaining §5.3 `Style` fields
and the layout machinery they require.

#### Phase C1 — Replaced rendering

- Extend `Style` with `Replaced`, `ReplacedWidth`,
  `ReplacedHeight`, `ReplacedKind`, `ReplacedRef`.
- Frame renders `Replaced=true` runes as boxes of the given
  dimensions; line breaking treats them as unbreakable single
  characters.
- 9P directive parser learns the `b` directive.

#### Phase C2 — Tall-element y-offset

- Implement `SetOriginYOffset` real behavior (replaces the Slice A
  stub).
- Add `Text.computeTallElementYOffset` and `tallY` state per §7.5.
- Test fixtures: image inline; image taller than viewport; image
  at viewport boundary.

#### Phase C3 — Image cache

- Simple LRU cache (per-window or global; implementer's call per
  §15 item 4).
- Frame consults cache during Replaced render.
- Decoupled: cache injected via an Init option, not hard-coded.

#### Phase C4 — Block context

- Extend `Style` with `BlockquoteDepth`, `InCodeBlock`,
  `InTable`.
- Frame's line breaker honors blockquote/table indent on line
  start.
- Tests: blockquote nesting, code block continuation across
  lines, table layout.

#### Phase C5 — Horizontal scroll for wide replaced elements

- Add `HOffset` to `Style`.
- `Frame.HScrollAt(pt) (q, ok)` for hit-testing.
- Text routes wheel events over wide elements to update
  `HOffset` (§10.2).
- Optional thin widget at the element's bottom edge.

#### Phase C6 — Producer rewrites (`md2spans`, `dirthumb`)

- Full `md2spans`: headings, emphasis, lists, code blocks,
  images, tables, blockquotes.
- `dirthumb`: directory listings → clickable thumbnails via `b`
  directives.
- Both shipped with golden tests.

#### Phase C7 — Polish

- Drag-scroll past frame edge in styled mode (the work done on
  the prior branch's `unify-frame-interface` was rich-side; in
  the unified design it lives in Text and is shared with plain
  text).
- Sub-element drag scroll for very tall images (deliberately
  *not* in v1 per §9.2; reconsider only if real workflows demand
  it).
- Performance baselines from §13.3 measured and recorded.

**Exit criterion for Slice C.** Markdown bodies render with the
full §5.3 `Style` surface. `md2spans` and `dirthumb` ship with
golden tests. Slice A and B producers still work. Regression
suite green and §13.3 baselines met.

## 13. Test Strategy

### 13.1 Reusable tests from current branch

A test from the current branch may be borrowed if and only if:

1. It exercises **observable behavior** — what comes out of public
   methods given particular inputs.
2. It does not assume the existence of `rich.Frame`, `RichText`,
   `richBody`, `HandleStyledMouse`, `UpdateStyledView`,
   `IsStyledMode`, `IsPreviewMode`, `SetContent`, or any other
   parallel-display construct.
3. It does not assume specific internal data structures (e.g.,
   `LinePixelHeights`, `LineStartRunes`, `TotalDocumentHeight` as
   public observables).

Likely-reusable categories:

- Spans correctness: insertion/deletion shifting, region query.
  (Some current-branch tests in `spans/`.)
- Markdown rendering output (rune layout, line breaks) on
  golden documents — *if the test compares rendered output, not
  intermediate boxes*.
- Outboard producer output (md2spans correctness on sample
  markdown).

Likely-not-reusable:

- Anything that probes `Frame.boxes`, `Frame.lines`,
  `LinePixelHeights`, etc.
- `HandleStyledMouse` tests.
- `RichText.ScrollByLines` / `ScrollByPixels` tests — the
  scrolling primitive in the new design lives on Text.

### 13.2 New tests required

**Frame:**

- `InsertWithStyle` with various style layouts: empty styles,
  single style, multiple runs, run-lengths summing exactly to
  `len(r)`, mismatched lengths (expect panic in tests).
- `SetStyleRange` clipping and repaint correctness.
- Replaced-element rendering: width/height honored, line height
  bumped, click-to-charofpt correctness inside the element.
- `SetOriginYOffset`: non-zero offset clips top; reset to 0 on
  Delete(0, *).

**Spans:**

- All `GetStyleRuns` cases including empty store, single region,
  multiple non-overlapping, requests overlapping no/some/all
  regions.
- Buffer observer-driven shifting: insertion before/within/at-end
  of regions; deletion clipping/merging/erasing.
- `Observe` callback invocation order, parameter correctness.

**Text:**

- `Inserted` propagates styles to frame when spans is non-empty.
- `fill` reads styles for newly-revealed runes.
- `attachSpans` observer clips to visible range.
- Observer ordering: spans must update before Text reads.

**Window:**

- 9P spans file read/write roundtrip.
- Producer writes propagate to Text and onward to frame.
- 'S' event emitted on body selection change when spans is
  attached and event handler is open; *not* emitted when spans
  is nil; *not* emitted when no event handler is open; *not*
  emitted from tags regardless of attachment.

**Integration:**

- Open a markdown file, run `md2spans`, verify visible
  rendering matches expected styled output.
- Scroll through a long styled document; verify rune positions
  remain consistent across scroll.

### 13.3 Performance baselines

Maintain regression benchmarks:

- Plain-text Insert throughput (runes/sec) for a 10 MB file.
  Target: within 5% of upstream.
- Styled-text Insert throughput for the same file with a heavy
  spans load (one region per 100 runes). Target: within 25% of
  plain.
- `GetStyleRuns` p99 latency on a 10 K-region store. Target:
  < 50 µs.

## 14. Performance Budget

The viewport-only invariant is what guarantees plain-text
performance. The cost of styling is bounded by:

1. **At fill time:** one `GetStyleRuns(visible_start, visible_end)`
   call. With binary search over regions, O(log R) to locate the
   first region and O(K) to emit K runs in the visible window. K is
   bounded by the visible-line count × max runs per line.
2. **Per Insert/Delete:** spans observer does O(log R + affected
   regions) of work to shift indices. Text observer pays one
   `GetStyleRuns` on the inserted range.
3. **Per spans-only update:** O(log R + affected runs) in the spans
   store; O(visible-overlap) in Text's observer callback.

Memory: per-rune style arrays in the frame are bounded by the
visible window's rune count, *not* by the document size.

## 15. Open Decisions for the Implementer

The following are deliberately left to the implementer:

1. **Frame internal data layout.** Run-length-encoded styles vs.
   one Style pointer per rune. RLE is cheaper for typical styled
   text; pointer-per-rune is simpler.
2. **Style canonicalization.** Whether `Style` values are interned
   (so equality is pointer-compare). Recommendation: keep `Style`
   a plain struct value; let Go's compare handle equality. Intern
   only if profiling shows allocation pressure.
3. **Spans persistence format.** Section 6.4 sketches a line-
   oriented format. The implementer may choose JSON, a binary
   form, or refine the line format — provided producers and the
   read-back path stay consistent.
4. **Image cache scope.** Per-window, per-row, or global. Global
   is simplest; per-row is the upstream convention for image-
   related state.
5. **Replaced-element selection granularity.** Today selection is
   per-rune; a replaced element is one rune. Whether to expand
   "select" of a replaced element to highlight the whole element
   block visually (rather than a thin cursor) is a UX call.

## 16. Risks

| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| Plain-text performance regression | Medium | Phase 1's contract that `nil styles` is a no-op path. Benchmark in CI. |
| Observer ordering bugs | Medium | Document the invariant. Add a test that asserts it. Register spans in a constructor that does not expose ordering to callers. |
| Replaced-element edge cases (image taller than viewport, at edge) | High | Dedicated test fixtures in Phase 6. Hand-write three pathological documents. |
| Spans store data corruption under concurrent producers | Low | Document "last writer wins per directive"; do not promise concurrent producer safety. |
| Style type grows beyond what frame needs | High | Code review: every new Style field must justify its use during layout or rendering. |
| md2spans behavior drift vs. current branch | Medium | Use the current branch's golden-output tests as a reference (they qualify under § 13.1 reuse criteria). |

## 17. Glossary

- **Buffer**: `file.ObservableEditableBuffer`. The rune storage,
  Undo/Redo, observer dispatcher. Upstream, unchanged.
- **Spans**: `spans.Store`. Tracks per-rune styling. Observes the
  buffer for index alignment.
- **Frame**: `frame.Frame`. Viewport renderer. Holds a slice of
  visible runes + parallel style data.
- **Text**: orchestrator. Owns a buffer view, an optional spans
  store, a frame, and a scrollbar. Drives mouse input, fills the
  frame.
- **Replaced element**: a buffer rune that the frame renders as a
  non-character block (image, code block, table, fixed colored
  box). Identified by `Style.Replaced == true`.
- **Tall element**: a replaced element whose height exceeds
  `2 × DefaultFontHeight()`. Eligible for sub-element y-offset
  scrolling.
- **Producer**: a process that writes to a window's spans file.
  Reference producers: md2spans, edcolor, dirthumb.
- **Viewport-only**: the property that the frame holds runes only
  for the visible window, never the full document.
- **Plain style**: a `Style` with `Kind == KindPlain` (= 0).
  Renders with the frame's default font and colors, exactly as
  upstream `Insert`. The zero value `Style{}` is a plain style.
  See `Style.IsPlain()`.
- **Kind**: a `Style` field carrying a bitmask of which other
  `Style` fields are meaningful. `KindPlain = 0` is the zero
  value and means "no styling, render as upstream default";
  non-zero bits (`KindColored`, `KindBold`, `KindReplaced`, …)
  gate the data fields. See §5.3 for the full bit set and §12
  for the per-slice schedule.
- **'S' event**: the single new event-file character this
  design adds to upstream's vocabulary. Emitted by Text on body
  selection change when the body has spans attached and an
  event-file consumer is open. Body-only (no tag variant). See
  § 9.3.
- **HOffset**: a `Style` field carrying per-replaced-element
  horizontal scroll state. See § 10.2.

---

*End of design.*
