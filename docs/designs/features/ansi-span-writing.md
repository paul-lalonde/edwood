# ANSI Span Writing Design

Converts the ANSI parser's `[]styledRun` output into the prefixed span
protocol message format (`s offset length fg [bg] [flags...]`). Pure
function — no I/O, no state.

**Package**: `main` (cmd/rwin)
**File**: `cmd/rwin/ansi_spans.go`
**Test file**: `cmd/rwin/ansi_spans_test.go`

---

## Dependencies

From Phase 1 (`ansi_color.go`):
- `ansiColor` — RGB color with `set` flag
- `sgrState` — cumulative SGR attribute state
- `colorToHex(c ansiColor) string` — `"#rrggbb"` or `"-"`
- `resolveColors(s sgrState) (fg, bg ansiColor)` — applies inverse + dim
- `isDefaultStyle(s sgrState) bool` — true when no attributes are set

From Phase 2 (`ansi.go`):
- `styledRun` — `{text []rune, style sgrState}`

From the spans protocol (`docs/designs/features/spans-protocol.md`):
- Prefixed format: `s offset length fg-color [bg-color] [flags...]`
- Colors: `#rrggbb` or `-` for default
- Flags: `bold`, `italic`, `hidden`
- Contiguity rule: each span's offset = previous offset + previous length
- Region updates: a write covers `[regionStart, regionStart+totalLen)`

---

## Function Signature

```go
func buildSpanWrite(baseOffset int, runs []styledRun) string
```

**Parameters**:
- `baseOffset` — the rune offset in the body where the first run's text
  starts. This is `w.p` at the time the clean text was written, *before*
  `w.p` is incremented.
- `runs` — styled runs from `ansiParser.Process()`. These are contiguous
  by construction (the parser emits runs in order, each starting where
  the previous ended).

**Returns**:
- A string containing newline-separated span lines ready for writing to
  the `spans` file. Empty string if all runs use default styling (the
  optimization case).

---

## Algorithm

```
1. Default optimization check:
   Scan all runs. If every run has isDefaultStyle(run.style) == true,
   return "" immediately. This is the common case for programs that
   don't emit ANSI escapes.

2. Build span lines:
   offset := baseOffset
   var b strings.Builder
   for each run in runs:
       length := len(run.text)
       if length == 0:
           continue   // skip zero-length runs (can occur at style boundaries)
       fg, bg := resolveColors(run.style)
       fgHex := colorToHex(fg)

       // Start the span line
       fmt.Fprintf(&b, "s %d %d %s", offset, length, fgHex)

       // Determine if bg or flags are needed
       bgHex := colorToHex(bg)
       flags := buildFlags(run.style)

       if bgHex != "-" || len(flags) > 0:
           // bg-color is needed (either because it's set, or because
           // flags follow and the protocol requires bg before flags)
           fmt.Fprintf(&b, " %s", bgHex)

       for each flag in flags:
           fmt.Fprintf(&b, " %s", flag)

       b.WriteByte('\n')
       offset += length

3. Return b.String()
```

---

## Background Color Emission Rule

The spans protocol allows bg-color to be omitted (defaults to `-`). However,
flags follow bg-color positionally. If any flag is present, bg-color must be
emitted explicitly (even as `-`) so the parser doesn't misinterpret the flag
as a color. The rule:

- **No bg, no flags**: emit only `s offset length fg`
- **Bg set, no flags**: emit `s offset length fg bg`
- **No bg, has flags**: emit `s offset length fg - flags...`
- **Bg set, has flags**: emit `s offset length fg bg flags...`

This matches the edcolor reference implementation pattern.

---

## Flag Emission

```go
func buildFlags(s sgrState) []string
```

Returns a slice of flag tokens for the style. Only flags supported by
the spans protocol are emitted:

| SGR attribute | Span flag | Condition |
|---------------|-----------|-----------|
| `bold` | `"bold"` | `s.bold && !s.dim` — SGR 22 clears both; if dim is also active, bold is still emitted since dim is handled via color reduction |
| `italic` | `"italic"` | `s.italic` |
| `hidden` | `"hidden"` | `s.hidden` |

Actually, `bold` and `dim` are independent SGR attributes. `bold` is emitted
whenever `s.bold` is true, regardless of `dim`. The `dim` attribute is handled
by `resolveColors()` reducing the foreground color brightness — it doesn't
affect the `bold` flag.

Corrected logic:

| SGR attribute | Span flag | Condition |
|---------------|-----------|-----------|
| `bold` | `"bold"` | `s.bold` |
| `italic` | `"italic"` | `s.italic` |
| `hidden` | `"hidden"` | `s.hidden` |

Attributes tracked but not emitted as flags:
- `underline` — no span protocol support
- `blink` — no span protocol support
- `strike` — no span protocol support
- `dim` — approximated via `resolveColors()` color reduction
- `inverse` — handled via `resolveColors()` color swap

---

## Offset Arithmetic

The runs from `ansiParser.Process()` are contiguous: each run starts
exactly where the previous one ended in the clean text. The offset for
each span is computed as:

```
offset_0 = baseOffset
offset_1 = baseOffset + len(runs[0].text)
offset_2 = baseOffset + len(runs[0].text) + len(runs[1].text)
...
offset_i = baseOffset + sum(len(runs[0..i-1].text))
```

This is equivalent to maintaining a running `offset` variable that starts
at `baseOffset` and increments by `len(run.text)` after each run.

The contiguity guarantee of the spans protocol is automatically satisfied
because the runs are emitted in order from a linear scan.

---

## Default Optimization

The most common case in rwin is programs that don't emit ANSI escapes
(plain `ls`, `cat`, `echo`, etc.). In this case, the parser produces runs
where every `style` is the zero-value `sgrState` — `isDefaultStyle()` returns
true for all of them.

`buildSpanWrite()` detects this case and returns `""` (empty string),
avoiding:
- The `strings.Builder` allocation
- The `fmt.Sprintf` formatting work
- The 9P write to the spans file

The caller checks `spanData != ""` before writing.

---

## Color Resolution Integration

Each run's style goes through `resolveColors()` before being formatted.
This function (from `ansi_color.go`) handles:

1. **Inverse (SGR 7)**: Swaps fg and bg. If the swapped color was default,
   substitutes a concrete color (`#fffff0` for inverse-fg from default-bg,
   `#000000` for inverse-bg from default-fg).

2. **Dim (SGR 2)**: Halves the foreground RGB components. If fg was default,
   substitutes `#808080`.

The resolved colors are then passed to `colorToHex()` for formatting.

---

## Examples

### Single red foreground run

Input: `baseOffset=100`, one run of 5 runes with `fg={set:true, r:0xaa, g:0, b:0}`

Output:
```
s 100 5 #aa0000
```

### Multiple contiguous runs

Input: `baseOffset=50`, three runs:
- 3 runes, red fg
- 4 runes, default style
- 5 runes, green fg + bold

Output:
```
s 50 3 #aa0000
s 53 4 -
s 57 5 #00aa00 - bold
```

Note: the default-styled run (offset 53) still emits a span line because
the block as a whole contains non-default runs. The contiguity rule requires
all runs in the region to be present.

### Inverse video

Input: `baseOffset=0`, one run of 10 runes with `inverse=true`, default fg/bg

After `resolveColors()`: fg=`#fffff0` (default bg approximation), bg=`#000000`

Output:
```
s 0 10 #fffff0 #000000
```

### All default (optimization)

Input: `baseOffset=0`, two runs both with default style

Output: `""` (empty string, no spans written)

### Bold + italic with background

Input: `baseOffset=200`, one run of 8 runes: fg=blue, bg=yellow, bold, italic

Output:
```
s 200 8 #0000aa #aa5500 bold italic
```

---

## Complexity

- Time: O(n) where n = number of runs. Each run is processed once.
- Space: O(m) where m = total output string length. Built via
  `strings.Builder` with no intermediate allocations.
- The default optimization makes the common case O(n) with no allocation
  (just the scan to check `isDefaultStyle`).

---

## File Organization

`cmd/rwin/ansi_spans.go` contains:
- `buildSpanWrite(baseOffset int, runs []styledRun) string`
- `buildFlags(s sgrState) []string` (unexported helper)

Both functions are pure — no side effects, no I/O, easily testable.
