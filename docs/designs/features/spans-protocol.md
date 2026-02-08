# 9P Spans Protocol Design

This document describes the 9P filesystem registration and write protocol for
the `spans` file. It covers the new constant, directory table entry, xfid
dispatch logic, parse function, and validation rules.

**Package**: `main`

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

    // Handle special commands.
    if data == "clear" {
        if w.spanStore != nil {
            w.spanStore.Clear()
        }
        // Phase 3 will add: exitStyledMode(w) if in styled mode.
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

    // Phase 3 will add: trigger styled rendering here.

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
