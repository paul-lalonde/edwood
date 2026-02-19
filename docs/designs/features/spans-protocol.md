# 9P Spans Protocol Design

This document describes the 9P filesystem registration and write protocol for
the `spans` file. It covers the new constant, directory table entry, xfid
dispatch logic, parse function, and validation rules.

**Package**: `main`

---

## Message Format

The `spans` file is write-only (`0200`). Programs write styled text
definitions to `/mnt/acme/<winid>/spans` to control the visual appearance of
text in a window's body. Each write is a complete message consisting of one
or more **prefixed message lines**.

### Prefixed Messages

Each line in a write begins with a single-letter prefix identifying the
message type:

| Prefix | Name | Format |
|--------|------|--------|
| `c` | Clear | `c` |
| `s` | Span | `s offset length fg-color [bg-color] [flags...]` |
| `b` | Box | `b offset length width height [fg-color] [bg-color] [flags...] [payload...]` |

A write may contain multiple `s` and `b` lines intermixed. They must obey
the contiguity rule (see below). A `c` line must be alone (the entire write
is a clear).

The legacy unprefixed format (bare `offset length color ...` lines and the
`clear` command) is still accepted for backward compatibility.

### Clear (`c`)

Clears all span styling and exits styled mode, returning the window to
plain text rendering. Must be the only line in a write.

### Span (`s`)

Defines the styling for a contiguous run of runes:

```
s offset length fg-color [bg-color] [flags...]
```

| Field | Required | Format | Description |
|-------|----------|--------|-------------|
| offset | yes | decimal integer | Rune offset into the body buffer where this span starts |
| length | yes | decimal integer | Number of runes in this span |
| fg-color | yes | `#rrggbb` or `-` | Foreground (text) color; `-` means use the default |
| bg-color | no | `#rrggbb` or `-` | Background color; `-` means use the default. Omit to use default. |
| flags | no | space-separated tokens | Style flags: `bold`, `italic`, `hidden` |

### Box (`b`)

Defines an inline replaced element — a fixed pixel region that covers (but
does not render as text) a span of buffer runes. Used for images and other
non-text visual elements.

```
b offset length width height [fg-color] [bg-color] [flags...] [payload...]
```

| Field | Required | Format | Description |
|-------|----------|--------|-------------|
| offset | yes | decimal int | Rune offset in body buffer |
| length | yes | decimal int | Runes covered (present in buffer, not rendered as text) |
| width | yes | decimal int | Pixel width of the box |
| height | yes | decimal int | Pixel height of the box |
| fg-color | no | `#rrggbb` or `-` | Foreground color |
| bg-color | no | `#rrggbb` or `-` | Background color |
| flags | no | `bold`, `italic`, `hidden` | Style flags |
| payload | no | rest of line | Opaque string for the layout engine |

**Payload parsing:** After consuming offset, length, width, height, optional
colors, and known flags, all remaining tokens are joined with spaces to form
the payload.

**Box invalidation:** Edits touching a box's rune region invalidate the box,
converting it back to a default text run. The tool must re-send the box after
the edit.

### Colors

Colors are specified as `#rrggbb` (a `#` followed by exactly 6 hex digits)
or `-` for the window's default color. Examples:

- `#ff0000` — red
- `#00ff00` — green
- `#000000` — black
- `-` — default (inherit from window theme)

### Flags

Optional trailing tokens after the color fields. Valid tokens:

| Token | Effect |
|-------|--------|
| `bold` | Render text in bold weight |
| `italic` | Render text in italic style |
| `hidden` | Reserved for future use |

Unknown tokens are an error. Duplicates are ignored.

### Distinguishing bg-color from flags

The color/flag field (after the required fields) is interpreted as a color
if it starts with `#` or equals `-`. Otherwise it is treated as a flag (for
spans) or the start of payload (for boxes). This is unambiguous because flag
tokens never start with `#` or equal `-`.

### Contiguity Rule

Spans and boxes within a single write must be **contiguous**: each line's
offset must equal the previous line's offset + length. There must be no gaps
or overlaps. The lines define a **region** starting at the first offset and
extending through the last.

### Examples

Prefixed format — styled text spans:
```
s 0 10 #0000cc
s 10 5 - bold
b 15 8 200 150 image:/tmp/diagram.png
s 23 12 #008000 italic
```

Clear all styling:
```
c
```

Box with colors and flags:
```
b 10 5 200 150 #ff0000 bold image:/path with spaces/img.png
```

Region update starting at offset 100:
```
s 100 20 #0000ff
s 120 15 -
s 135 10 #ff0000 bold
```

### Region Updates

A write does not need to cover the entire body buffer. The spans define a
region `[regionStart, regionStart + totalLength)` and only that region is
updated in the span store. Spans outside the region are preserved. This
allows incremental re-coloring — for example, coloring only the visible
viewport.

### Tolerance for Stale Data

The protocol tolerates writes based on a slightly stale body snapshot:

- Spans starting at or past the buffer end are silently discarded (the
  coloring tool may have read a stale body that was longer).
- If the total region extends past the buffer end, trailing runs are
  truncated (not rejected) to fit within the buffer.
- The span store re-syncs with the buffer length on each write if they
  have diverged.

The coloring tool will re-color on the next edit event, so transient
mismatches are self-correcting.

### Error Responses

A write returns a 9P error if any of these conditions are violated:

| Condition | Error message |
|-----------|---------------|
| Unknown prefix | `unknown span command: "..."` |
| `c` not alone in write | `clear must be the only command in a write` |
| Span: fewer than 3 fields | `bad span format: need at least offset length color` |
| Box: fewer than 4 fields | `bad box format: need at least offset length width height` |
| Offset not a valid integer | `bad span offset: "..."` / `bad box offset: "..."` |
| Length not a valid integer | `bad span length: "..."` / `bad box length: "..."` |
| Width/height not valid integer | `bad box width: "..."` / `bad box height: "..."` |
| Color not `#rrggbb` or `-` | `bad color value: "..."` |
| Unknown flag token | `unknown span flag: "..."` |
| Negative offset or length | `negative span offset or length` / `negative box offset or length` |
| Negative width or height | `negative box width or height` |
| Non-contiguous spans | `spans must be contiguous: expected offset N, got M` |
| Window is in preview mode | `cannot write spans to preview mode window` |

### Side Effects

- **First write** to a window automatically switches it to styled rendering
  mode (unless the user has explicitly toggled to plain mode via the Plain
  button).
- **`clear`** exits styled mode and returns to plain text.
- After applying spans, the window immediately re-renders.

---

## New Constant: QWspans

In `dat.go`, add `QWspans` immediately before `QMAX`:

```go
QWxdata
QWspans // window's spans file
QMAX
```

This gives `QWspans` the value 20 (after `QWxdata` = 19). The `nopen` array
in `Window` is sized `[QMAX]byte`, so adding before `QMAX` automatically
grows it.

---

## Directory Table Entry

In `fsys.go`, add to `dirtabw` (after the `xdata` entry):

```go
{"spans", plan9.QTFILE, QWspans, 0200},
```

The file is write-only (`0200`). The 9P `open` handler in `fsys.go` checks
`(f.dir.perm &^ (plan9.DMDIR | plan9.DMAPPEND)) & m` against the requested
mode. A `plan9.OREAD` open will be rejected because `0200 & 0400 == 0`.

---

## Lock Type

In `xfidwrite`, the lock type for `QWspans` must be `'E'` (exclusive), since
span writes modify rendering state (same rationale as body/tag writes).

Current code at `xfid.go:382-384`:

```go
c := 'F'
if qid == QWtag || qid == QWbody {
    c = 'E'
}
```

Change to:

```go
c := 'F'
if qid == QWtag || qid == QWbody || qid == QWspans {
    c = 'E'
}
```

---

## xfidopen Handler

Add a case for `QWspans` in `xfidopen` (inside the `w != nil` switch):

```go
case QWspans:
    w.nopen[q]++
```

No additional state initialization is needed at open time. The `SpanStore` is
lazily created on first write (in Phase 3 when rendering is wired up).

---

## xfidclose Handler

Add a case for `QWspans` in `xfidclose` (inside the `w != nil` switch):

```go
case QWspans:
    w.nopen[q]--
```

No cleanup is needed. The span store persists after the file is closed so
that styles remain visible.

---

## xfidread Default Case

The file is `0200` (write-only), so the 9P permission check rejects reads
before they reach `xfidread`. However, for clarity and defense in depth, add
an explicit case in `xfidread`'s window-file switch:

```go
case QWspans:
    x.respond(&fc, ErrPermission)
```

This returns a permission error if a read somehow reaches the handler.

---

## xfidwrite Handler

Add a case for `QWspans` in the `xfidwrite` switch:

```go
case QWspans:
    xfidspanswrite(x, w)
```

---

## xfidspanswrite Function

```go
func xfidspanswrite(x *Xfid, w *Window) {
    var fc plan9.Fcall

    data := strings.TrimRight(string(x.fcall.Data), "\n")
    if data == "" {
        fc.Count = x.fcall.Count
        x.respond(&fc, nil)
        return
    }

    bufLen := w.body.Nc()

    // Reject writes to preview mode windows.
    if w.IsPreviewMode() {
        x.respond(&fc, fmt.Errorf("cannot write spans to preview mode window"))
        return
    }

    // Handle special commands.
    if data == "clear" {
        if w.spanStore != nil {
            w.spanStore.Clear()
        }
        w.exitStyledMode()
        fc.Count = x.fcall.Count
        x.respond(&fc, nil)
        return
    }

    // Parse span definitions.
    runs, regionStart, err := parseSpanDefs(data, bufLen)
    if err != nil {
        x.respond(&fc, err)
        return
    }

    // No-op if body is empty.
    if bufLen == 0 {
        fc.Count = x.fcall.Count
        x.respond(&fc, nil)
        return
    }

    // Ensure the span store exists.
    if w.spanStore == nil {
        w.spanStore = NewSpanStore()
        // Initialize with a default run covering the full buffer.
        w.spanStore.Insert(0, bufLen)
    } else if w.spanStore.TotalLen() != bufLen {
        // Re-sync: the store length should match the buffer.
        // This can happen if the store was created but the buffer
        // changed without edit tracking (Phase 4 will fix this).
        w.spanStore.Clear()
        w.spanStore.Insert(0, bufLen)
    }

    // Apply region update.
    w.spanStore.RegionUpdate(regionStart, runs)

    // Auto-switch to styled mode on first span write.
    if !w.styledMode && !w.previewMode {
        w.initStyledMode()
    }

    // Build styled content and render.
    if w.styledMode && w.richBody != nil {
        content := w.buildStyledContent()
        w.richBody.SetContent(content)
        w.richBody.Render(w.body.all)
        if w.display != nil {
            w.display.Flush()
        }
    }

    fc.Count = x.fcall.Count
    x.respond(&fc, nil)
}
```

Note: `w.spanStore` is a `*SpanStore` field added to `Window` in Phase 3.
For Phase 2, we add the field early so the write handler can store results.
The field is added to `wind.go`:

```go
spanStore  *SpanStore // styled text runs (nil when no spans)
```

---

## Parse Function

### Signature

```go
func parseSpanDefs(data string, bufLen int) (runs []StyleRun, regionStart int, err error)
```

**Parameters**:
- `data` — the write payload with trailing newlines stripped, not "clear"
- `bufLen` — `w.body.Nc()`, the current body buffer length in runes

**Returns**:
- `runs` — parsed `StyleRun` values in order
- `regionStart` — the offset of the first span (start of the update region)
- `err` — validation error, or nil

### Parsing Logic

```
1. Split data on "\n" to get lines.
2. For each line:
   a. Split on whitespace (fields).
   b. Parse offset (field 0) as decimal int.
   c. Parse length (field 1) as decimal int.
   d. Parse fg-color (field 2) as "#rrggbb" or "-".
   e. Parse bg-color (field 3, optional) as "#rrggbb" or "-" or skip if
      it looks like a flag.
   f. Parse flags (remaining fields): "bold", "italic", "hidden".
   g. Construct StyleRun{Len: length, Style: StyleAttrs{...}}.
3. Validate:
   a. First span offset is regionStart.
   b. Each subsequent span offset == previous offset + previous length
      (contiguous, no gaps, no overlaps).
   c. All offsets in [0, bufLen].
   d. regionStart + sum(lengths) <= bufLen.
   e. At least 2 fields per line (offset and length are required;
      fg-color defaults to "-" if omitted for a zero-length span).
      Actually: at least 3 fields (offset, length, fg-color) for
      non-zero-length spans.
```

### Color Parsing

```go
func parseColor(s string) (color.Color, error)
```

- `"-"` returns `nil, nil` (default color)
- `"#rrggbb"` returns `color.RGBA{R: rr, G: gg, B: bb, A: 0xff}, nil`
- Anything else returns `nil, error`

The `#` prefix is required. Exactly 6 hex digits follow it.

### Flag Parsing

Flags are optional trailing fields after the color fields. Valid tokens:
`bold`, `italic`, `hidden`. Unknown tokens are an error. Duplicate tokens
are ignored (idempotent).

### Distinguishing bg-color from flags

Field 3 (index 3) is bg-color if it starts with `#` or equals `"-"`.
Otherwise it's treated as a flag. This avoids ambiguity since flag tokens
(`bold`, `italic`, `hidden`) never start with `#` or equal `"-"`.

---

## Validation Rules and Error Cases

| Condition | Error message |
|-----------|---------------|
| Line has fewer than 3 fields | `"bad span format: need at least offset length color"` |
| Offset is not a valid integer | `"bad span offset: ..."` |
| Length is not a valid integer | `"bad span length: ..."` |
| Color is not `#rrggbb` or `-` | `"bad color value: ..."` |
| Unknown flag token | `"unknown span flag: ..."` |
| Offset < 0 or length < 0 | `"negative span offset or length"` |
| Spans not contiguous (gap) | `"spans must be contiguous: expected offset %d, got %d"` |
| Region extends past buffer | `"span region exceeds buffer length"` |
| Offset > bufLen | `"span offset beyond buffer"` |
| Write to window with no body | No error; no-op (handled before parse) |

---

## Error Variables

Add to the `var` block in `xfid.go`:

```go
ErrBadSpanFormat = fmt.Errorf("bad span format")
```

Individual parse errors include detail via `fmt.Errorf(...)`.

---

## Interaction with Preview Mode

Writing spans to a window in preview mode is an error. Check in
`xfidspanswrite` before processing:

```go
if w.IsPreviewMode() {
    x.respond(&fc, fmt.Errorf("cannot write spans to preview mode window"))
    return
}
```

---

## Summary of File Changes (Phase 2)

| File | Change |
|------|--------|
| `dat.go` | Add `QWspans` before `QMAX` |
| `fsys.go` | Add `{"spans", plan9.QTFILE, QWspans, 0200}` to `dirtabw` |
| `wind.go` | Add `spanStore *SpanStore` field to `Window` struct |
| `xfid.go` | Add `QWspans` to `'E'` lock condition; add open/close/read/write cases; implement `xfidspanswrite` |
| `spanparse.go` | **New file**: `parseSpanDefs`, `parseColor` functions |

---

## Test Plan (for Tests stage)

Tests go in `spanparse_test.go`:

1. **Single span parse**: `"0 10 #ff0000"` with bufLen=10 produces one run
   with Fg=red, default Bg, no flags.
2. **Multi-span contiguous**: `"0 4 #0000ff\n4 6 -"` with bufLen=10 produces
   two runs.
3. **Optional bg-color**: `"0 5 #ff0000 #00ff00"` parses both fg and bg.
4. **`#rrggbb` and `-` color parsing**: Verify `parseColor("#aabbcc")` and
   `parseColor("-")`.
5. **Flags**: `"0 5 #ff0000 bold italic"` produces run with Bold=true,
   Italic=true.
6. **Bg-color + flags**: `"0 5 #ff0000 #000000 bold"` parses bg and bold.
7. **`clear` command**: Handled in `xfidspanswrite`, not by `parseSpanDefs`.
   Test at integration level (or in a separate test for the write handler).
8. **Validation errors**:
   - Overlapping/non-contiguous spans: `"0 5 #ff0000\n7 3 #00ff00"` errors.
   - Gaps: `"0 3 #ff0000\n5 5 #00ff00"` errors.
   - Out-of-range offset: `"0 20 #ff0000"` with bufLen=10 errors.
   - Bad format: `"0 abc #ff0000"` errors.
   - Bad color: `"0 5 red"` errors.
   - Unknown flag: `"0 5 #ff0000 underline"` errors.
9. **Write to window with no body text**: bufLen=0, should be no-op (checked
   before parse in `xfidspanswrite`).
10. **Zero-length span**: `"5 0 #ff0000\n5 5 -"` is valid (zero-length run
    dropped by SpanStore).
