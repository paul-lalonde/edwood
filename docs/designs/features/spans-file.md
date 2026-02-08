# Spans File: External Styling via the 9P Filesystem

## Problem

Edwood has a rich text rendering engine (`rich.Frame`) built for the markdown
preview feature. It supports per-span foreground color, background color, bold,
italic, and other style attributes. However, this engine is only accessible
through the internal markdown parser. There is no way for external tools to
style the text in a window.

Plan 9 and Acme follow a philosophy where the editor provides mechanism and
external tools provide policy. Text windows expose synthetic files (`body`,
`ctl`, `addr`, `data`, `event`, `tag`) that tools read and write to interact
with the editor. Syntax coloring, lint highlighting, and other text decorations
should follow the same pattern: an external tool tokenizes the buffer content
and writes styling information back through the filesystem.

## Goals

- Expose a new per-window `spans` file in the 9P filesystem that external tools
  can write to in order to set style attributes (foreground color, background
  color, bold, italic) on ranges of the body text.
- Render styled text using the existing `rich.Frame` engine.
- Adjust span positions automatically when the user edits text, so styling
  remains approximately correct between external tool updates.
- Keep the design simple enough that a basic syntax coloring tool can be written
  as a standalone program that watches `event` and writes `spans`.

## Non-Goals

- Built-in syntax highlighting or language awareness. The editor provides
  the rendering mechanism; external tools decide what to color.
- Semantic token types or theme support. Tools write concrete colors. A
  theming layer can be added later on top of this mechanism.
- Span readback. We do not implement reads on the `spans` file initially.
- Hidden/collapsible spans. The `hidden` attribute is reserved in the style
  model but not rendered in the initial implementation. This is the path
  toward externalizing the markdown preview.
- Overlapping or layered spans from multiple tools. One span set per window;
  last writer wins.

---

## Architecture Overview

```
  External tool (e.g., syntax highlighter)
       │                          ▲
       │ write spans file         │ read event file
       ▼                          │
  ┌─────────────────────────────────────────┐
  │  9P filesystem (fsys/xfid)              │
  │                                         │
  │  /mnt/acme/<id>/spans  ◄── new file     │
  │  /mnt/acme/<id>/body                    │
  │  /mnt/acme/<id>/event                   │
  └────────────────┬────────────────────────┘
                   │
                   ▼
  ┌─────────────────────────────────────────┐
  │  Window                                 │
  │                                         │
  │  body buffer ([]rune)                   │
  │  spanStore (gap buffer of StyleRuns)    │
  │       │                                 │
  │       ▼                                 │
  │  buildStyledContent()                   │
  │       │                                 │
  │       ▼                                 │
  │  rich.Frame.SetContent() + Render()     │
  └─────────────────────────────────────────┘
```

### Workflow

1. An external tool opens `/mnt/acme/<id>/event` and `/mnt/acme/<id>/body`.
2. It reads the body text, tokenizes it, and writes span definitions to
   `/mnt/acme/<id>/spans`.
3. The editor receives the span write, updates the internal span store,
   builds styled `rich.Content` from the body text plus spans, and renders
   through `rich.Frame`.
4. When the user edits text, the editor adjusts span positions automatically
   (insertions extend the containing span and shift downstream spans;
   deletions shrink or remove overlapping spans).
5. The tool detects the edit via the `event` file (or by re-reading `body`),
   re-tokenizes the affected region, and writes updated spans for that region.

---

## Spans File Protocol

### File Properties

| Property   | Value |
|------------|-------|
| Name       | `spans` |
| QID type   | `plan9.QTFILE` |
| Permissions| `0200` (write-only initially; read can be added later) |

### Write Format

Each write to the `spans` file is a **region update**: a set of span
definitions that replace all existing spans within the covered region. The
region is defined implicitly by the first span's offset and the last span's
offset + length.

A write consists of one or more newline-separated span definitions:

```
<offset> <length> <fg-color> [<bg-color>] [<flags>...]
```

Fields:
- `offset` — rune offset in the body buffer (decimal integer).
- `length` — number of runes in this span (decimal integer).
- `fg-color` — foreground color as `#rrggbb` hex string, or `-` for default.
- `bg-color` — (optional) background color as `#rrggbb` hex string, or `-`
  for default. Omitted means default.
- `flags` — (optional) space-separated tokens: `bold`, `italic`, `hidden`.

**Spans within a single write must be contiguous and non-overlapping.** The
first span starts at offset S, and each subsequent span starts where the
previous one ended. The write replaces all existing style information in the
range [S, S+total_length).

Gaps between the write region and existing spans retain their existing style.

### Examples

Color Go keywords blue and strings green on a line `func main() {`:
```
0 4 #0000ff
4 1 -
5 4 #000000
9 1 -
10 1 #000000
11 1 -
```

Update just the string literal in a line after an edit:
```
15 12 #008000
```

### Special Commands

A write consisting of a single keyword (no span definitions):

| Command | Effect |
|---------|--------|
| `clear` | Remove all spans from the window. If the window auto-switched to styled rendering, it switches back to plain text mode. |

### Constraints

- Offsets must be within `[0, body.Nc()]`. Offsets beyond the buffer length
  are an error.
- Spans within a write must not overlap and must be in ascending offset order.
- A zero-length span is valid (used for hidden markers in future).
- Writing spans to a window that has no body text is a no-op.

---

## Internal Span Representation

### StyleRun

The internal representation is a run-length encoding of styles across the
buffer:

```go
// StyleAttrs holds the concrete styling for a span of text.
type StyleAttrs struct {
    Fg     color.Color // nil means default foreground
    Bg     color.Color // nil means default background
    Bold   bool
    Italic bool
    Hidden bool        // reserved for future use
}

// StyleRun is a contiguous run of runes sharing the same style.
type StyleRun struct {
    Len   int        // number of runes in this run
    Style StyleAttrs
}
```

A window's span state is a sequence of `StyleRun` values whose lengths sum to
the body buffer length. Unstyled text is represented by runs with zero-value
`StyleAttrs` (all nil/false).

### Gap Buffer Storage

The run sequence is stored in a gap buffer for efficient editing at the cursor
position:

```go
// SpanStore manages styled runs for a window's body text.
type SpanStore struct {
    runs []StyleRun // storage with gap
    gap0 int        // start of gap (index into runs)
    gap1 int        // end of gap (index into runs)
}
```

**Insert at position P** (rune offset in body):
1. Find the run containing P by scanning cumulative lengths from the gap
   edges. Move the gap to that run index — O(runs moved).
2. Split the containing run at P if the insertion is not at a run boundary.
3. Extend the run containing P by the insertion length (the new text inherits
   the style of the run it was inserted into).

**Delete from position P to P+N**:
1. Move the gap to the run containing P.
2. Shrink or remove runs that overlap [P, P+N). Runs fully inside the deleted
   range are absorbed into the gap. Runs partially overlapping have their
   lengths reduced.

**Region update** (from spans file write):
1. Find the runs covering [S, S+total_length).
2. Move the gap to that region.
3. Replace the covered runs with new runs parsed from the write.

### Cost Analysis

| Operation | Cost |
|-----------|------|
| Single character insert/delete at cursor | O(1) amortized (gap is already nearby) |
| Character insert/delete at distant position | O(k) where k = runs between old and new gap position |
| Region update from spans file | O(k) for gap move + O(m) for new runs, where m = number of new runs |
| Edit command affecting scattered positions | O(n) total for gap moves across all positions |

For typical editing (sequential typing), the gap buffer provides O(1) per
keystroke. For Edit commands on large files, the cost is proportional to the
number of spans traversed, which is acceptable given that Edit commands
themselves are O(n) on the text.

**Upgrade path**: If profiling shows the gap buffer is a bottleneck for Edit
commands on very large files, the `SpanStore` interface can be reimplemented
with a balanced tree (e.g., augmented B-tree) providing O(log n) for all
operations. The rest of the system interacts only through the `SpanStore`
methods, so this is a local change.

---

## Rendering Integration

### Auto-Switch to Styled Mode

When the first span write arrives on a window that has no existing spans, the
window transitions from plain text rendering (`frame.Frame`) to styled
rendering (`rich.Frame`). This mirrors how markdown preview mode works:

1. A `rich.Frame` (wrapped in `RichText`) is initialized for the window.
2. The body text is combined with span styles to produce `rich.Content`.
3. The `rich.Frame` renders the styled content.
4. Editing continues to target the body buffer; the styled view re-renders
   after each edit.

When spans are cleared (via `clear` command), the window reverts to plain text
rendering.

### The "Plain" Command

The `ctl` file accepts a new keyword:

```
Plain
```

Writing `Plain` to a window's `ctl` file toggles between styled and plain text
rendering:
- If the window is in styled mode (has spans), `Plain` switches to plain text
  rendering. Spans are preserved but not rendered.
- If the window is in plain text mode and has spans, `Plain` switches back to
  styled rendering.
- If the window has no spans, `Plain` is a no-op.

This lets tools or users temporarily see the unstyled text without losing span
data.

### Building Styled Content

When the styled view needs to render (after an edit or span update), the
window builds `rich.Content` from the body text and span store:

```go
func (w *Window) buildStyledContent() rich.Content {
    var content []rich.Span
    offset := 0
    w.spanStore.ForEachRun(func(run StyleRun) {
        text := w.body.file.ReadRuneSlice(offset, offset+run.Len)
        span := rich.Span{
            Text:  string(text),
            Style: styleAttrsToRichStyle(run.Style),
        }
        content = append(content, span)
        offset += run.Len
    })
    return rich.Content(content)
}
```

The `styleAttrsToRichStyle` function maps `StyleAttrs` to `rich.Style`,
setting `Fg`, `Bg`, `Bold`, `Italic`, and `Code` (for monospace).

### Source Map

Since styled editing renders the same text as the body (no content
transformation), the source map is an identity mapping: rendered position N
corresponds to source position N. This makes cursor remapping trivial compared
to markdown preview mode.

If hidden spans are implemented in the future, the source map becomes
non-trivial (rendered positions skip hidden regions), and the existing
`markdown.SourceMap` infrastructure can be reused.

### Interaction with Preview Mode

Styled rendering via spans and markdown preview mode are mutually exclusive.
A window is in one of three states:

| State | Rendering | Trigger |
|-------|-----------|---------|
| Plain | `frame.Frame` | Default; or `clear` spans; or `Plain` toggle |
| Styled | `rich.Frame` with spans | First span write |
| Preview | `rich.Frame` with markdown parser | `Markdeep` command |

Writing spans to a window in preview mode is an error. The tool should write
to a non-preview window.

---

## 9P Filesystem Registration

### New Constants

In `dat.go`, add before `QMAX`:

```go
QWspans // window's spans file
```

### Directory Table

In `fsys.go`, add to `dirtabw`:

```go
{"spans", plan9.QTFILE, QWspans, 0200},
```

Permissions are `0200` (write-only) initially. Read support can be added later
by changing to `0600` and implementing the read handler.

### Dispatch

In `xfid.go`:

**xfidopen**: Initialize span state tracking.
```go
case QWspans:
    w.nopen[q]++
```

**xfidclose**: Decrement open count.
```go
case QWspans:
    w.nopen[q]--
```

**xfidwrite**: Parse span definitions and update the span store.
```go
case QWspans:
    xfidspanswrite(x, w)
```

The lock type for spans writes should be `'E'` (exclusive), same as body
writes, since it modifies rendering state.

---

## Span Adjustment on Buffer Edits

When the body text is modified (via typing, cut/paste, Edit commands, or 9P
writes), the span store must be adjusted to keep styles aligned with the text
they describe.

### Insert at Position P, Length N

The run containing position P is found. If P falls at a run boundary, the
preceding run is extended by N. If P falls within a run, the run's length
increases by N (the inserted text inherits the style of the surrounding run).
No other runs are affected — downstream runs are implicitly shifted because
positions are derived from cumulative lengths.

### Delete from Position P to P+N

Runs overlapping [P, P+N) are shrunk. A run fully inside the range is removed.
A run partially overlapping at the start or end has its length reduced. If a
deletion spans multiple runs, runs in the middle are removed entirely.

### Integration Points

Span adjustment hooks into the same observer callbacks that currently serve
the markdown preview (`Text.Inserted`, `Text.Deleted`). When a window is in
styled mode:

- `Text.Inserted` calls `w.spanStore.Insert(q0, nr)`
- `Text.Deleted` calls `w.spanStore.Delete(q0, q1)`

These adjust the gap buffer and then trigger a re-render.

---

## Future: Hidden Spans

The `hidden` flag in `StyleAttrs` is reserved for a future feature where spans
can be styled as invisible. This enables:

- **Markdown externalization**: An external markdown tool writes spans that
  mark syntax characters (`#`, `**`, `` ` ``, etc.) as hidden, while styling
  the visible text with heading sizes, bold, italic, etc. The body buffer
  retains the full source; the display elides markers.
- **Code folding**: Regions can be collapsed by marking them hidden.

Implementation considerations (deferred):
- Cursor movement must skip hidden spans.
- Selection across hidden regions should include the hidden text for
  copy/cut operations on the underlying source.
- The source map becomes non-trivial (rendered positions != source positions)
  and the existing `markdown.SourceMap` can be reused.
- Typing at the boundary of a hidden span requires policy decisions about
  whether new text inherits the hidden attribute.

---

## Files to Modify

| File | Change |
|------|--------|
| `dat.go` | Add `QWspans` constant |
| `fsys.go` | Add `spans` entry to `dirtabw` |
| `xfid.go` | Add open/close/write handlers for `QWspans`; add lock type |
| `wind.go` | Add `SpanStore` field to `Window`; add `buildStyledContent()` method; add styled rendering mode state |
| `text.go` | Hook span adjustment into `Inserted`/`Deleted` observers |
| `exec.go` | Add `Plain` ctl command |
| `spanstore.go` | **New file**: `SpanStore` gap buffer implementation |

---

## Implementation Phases

### Phase 1: SpanStore Data Structure
Implement the gap buffer of `StyleRun` values with insert, delete, and region
update operations. Unit test thoroughly with edge cases (insert at boundaries,
delete across multiple runs, region replacement).

### Phase 2: 9P Spans File
Register the `spans` file in the filesystem. Implement the write handler that
parses span definitions and calls `SpanStore` methods. Implement the `clear`
command.

### Phase 3: Styled Rendering
Wire up auto-switch to `rich.Frame` when spans are set. Implement
`buildStyledContent()` to produce `rich.Content` from body text + spans.
Handle re-rendering after edits and span updates.

### Phase 4: Span Adjustment on Edit
Hook `SpanStore.Insert`/`Delete` into the `Text.Inserted`/`Text.Deleted`
observer callbacks. Verify that styles track correctly during typing, cut/paste,
and undo/redo.

### Phase 5: Plain Toggle
Add `Plain` keyword to the `ctl` file write handler. Implement toggling between
styled and plain rendering modes.

### Phase 6: Integration Testing
Build a minimal external syntax coloring tool (e.g., for Go source) that
watches `event`, reads `body`, tokenizes, and writes `spans`. Verify the
end-to-end workflow.
