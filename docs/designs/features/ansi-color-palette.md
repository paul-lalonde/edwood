# ANSI Color Palette, Types & Utilities

Design for `cmd/rwin/ansi_color.go` — the color foundation for ANSI escape
sequence processing.

**Source**: Distilled from `cmd/rwin/ANSI_DESIGN.md` sections 2, 3, 6.

---

## 1. Types

### ansiColor

Represents an RGB color that may or may not be explicitly set. When `set` is
false, the color is "default" — meaning the span should use `-` (inherit the
window's default foreground or background).

```go
type ansiColor struct {
    set     bool
    r, g, b uint8
}
```

Zero value: `ansiColor{}` — unset, default color.

### sgrState

Tracks the cumulative SGR (Select Graphic Rendition) attribute state. Each
SGR command modifies the current state; it does not replace it (except reset).
The zero value represents all defaults (no colors, no attributes).

```go
type sgrState struct {
    fg        ansiColor
    bg        ansiColor
    bold      bool
    dim       bool
    italic    bool
    underline bool
    blink     bool
    inverse   bool
    hidden    bool
    strike    bool
}
```

Reset clears everything to zero value:

```go
func (s *sgrState) reset() {
    *s = sgrState{}
}
```

### styledRun

A contiguous run of text sharing the same style. The parser emits a slice of
these. Defined here because it references `sgrState`, but used primarily by
the span writer (Phase 3).

```go
type styledRun struct {
    text  []rune
    style sgrState
}
```

---

## 2. Palette

### Data Structure

A package-level lookup table mapping 256-color indices to RGB values:

```go
var ansiPalette [256][3]uint8
```

Uses `[3]uint8` (R, G, B) rather than `image/color.RGBA` to avoid importing
`image/color` — the alpha channel is always 0xFF and not useful here.

### Standard 16 Colors (indices 0-15)

Traditional CGA/VGA palette. The first 8 are standard colors (SGR 30-37 /
40-47), the next 8 are bright variants (SGR 90-97 / 100-107).

| Index | Name | R | G | B | Hex |
|-------|------|---|---|---|-----|
| 0 | Black | 0x00 | 0x00 | 0x00 | `#000000` |
| 1 | Red | 0xaa | 0x00 | 0x00 | `#aa0000` |
| 2 | Green | 0x00 | 0xaa | 0x00 | `#00aa00` |
| 3 | Yellow (dark) | 0xaa | 0x55 | 0x00 | `#aa5500` |
| 4 | Blue | 0x00 | 0x00 | 0xaa | `#0000aa` |
| 5 | Magenta | 0xaa | 0x00 | 0xaa | `#aa00aa` |
| 6 | Cyan | 0x00 | 0xaa | 0xaa | `#00aaaa` |
| 7 | White (light gray) | 0xaa | 0xaa | 0xaa | `#aaaaaa` |
| 8 | Bright black (dark gray) | 0x55 | 0x55 | 0x55 | `#555555` |
| 9 | Bright red | 0xff | 0x55 | 0x55 | `#ff5555` |
| 10 | Bright green | 0x55 | 0xff | 0x55 | `#55ff55` |
| 11 | Bright yellow | 0xff | 0xff | 0x55 | `#ffff55` |
| 12 | Bright blue | 0x55 | 0x55 | 0xff | `#5555ff` |
| 13 | Bright magenta | 0xff | 0x55 | 0xff | `#ff55ff` |
| 14 | Bright cyan | 0x55 | 0xff | 0xff | `#55ffff` |
| 15 | Bright white | 0xff | 0xff | 0xff | `#ffffff` |

Stored as a Go array literal (not computed).

### 216-Color Cube (indices 16-231)

Computed at init time. For index N in 16..231:

```
N' = N - 16
r_level = N' / 36          (0-5)
g_level = (N' % 36) / 6    (0-5)
b_level = N' % 6           (0-5)
```

Each level maps to an intensity value:

| Level | Value |
|-------|-------|
| 0 | 0x00 |
| 1 | 0x5f |
| 2 | 0x87 |
| 3 | 0xaf |
| 4 | 0xd7 |
| 5 | 0xff |

### 24 Grayscale (indices 232-255)

Computed at init time. For index N in 232..255:

```
v = (N - 232) * 10 + 8
rgb = (v, v, v)
```

Range: `#080808` (index 232) to `#eeeeee` (index 255).

### Initialization

```go
func init() {
    // Standard 16: copy from literal array
    copy(ansiPalette[:16], standard16)

    // 216 cube
    levels := [6]uint8{0x00, 0x5f, 0x87, 0xaf, 0xd7, 0xff}
    for i := 0; i < 216; i++ {
        r := levels[i/36]
        g := levels[(i%36)/6]
        b := levels[i%6]
        ansiPalette[16+i] = [3]uint8{r, g, b}
    }

    // 24 grayscale
    for i := 0; i < 24; i++ {
        v := uint8(i*10 + 8)
        ansiPalette[232+i] = [3]uint8{v, v, v}
    }
}
```

Complexity: O(1) — fixed 256 entries computed once.

---

## 3. Utility Functions

### colorToHex

Converts an `ansiColor` to a `#rrggbb` hex string for the spans protocol, or
`"-"` if the color is default (unset).

```go
func colorToHex(c ansiColor) string {
    if !c.set {
        return "-"
    }
    return fmt.Sprintf("#%02x%02x%02x", c.r, c.g, c.b)
}
```

Complexity: O(1).

### applyDim

Approximates SGR dim (faint) by halving the brightness of each RGB component.
If the color is default (unset), returns a mid-gray `#808080` to approximate
dimmed default text.

```go
func applyDim(c ansiColor) ansiColor {
    if !c.set {
        return ansiColor{set: true, r: 0x80, g: 0x80, b: 0x80}
    }
    return ansiColor{set: true, r: c.r / 2, g: c.g / 2, b: c.b / 2}
}
```

Complexity: O(1).

### resolveColors

Applies inverse and dim transformations to produce the final foreground and
background colors for a span. Called at span emission time, not during SGR
parsing.

Inverse handling: swap fg and bg. When the swapped color was default (unset),
substitute an explicit approximation of the window default:
- Default bg as fg: `#fffff0` (edwood's default background)
- Default fg as bg: `#000000` (black)

Dim handling: applied after inverse, affects foreground only.

```go
func resolveColors(s sgrState) (fg, bg ansiColor) {
    fg, bg = s.fg, s.bg
    if s.inverse {
        fg, bg = bg, fg
        if !fg.set {
            fg = ansiColor{set: true, r: 0xff, g: 0xff, b: 0xf0}
        }
        if !bg.set {
            bg = ansiColor{set: true, r: 0x00, g: 0x00, b: 0x00}
        }
    }
    if s.dim {
        fg = applyDim(fg)
    }
    return fg, bg
}
```

Complexity: O(1).

### isDefaultStyle

Returns true if the style has no attributes set — no colors, no flags. Used
for the default optimization: if every run in a chunk is default-styled, skip
the span write entirely.

```go
func isDefaultStyle(s sgrState) bool {
    return !s.fg.set && !s.bg.set &&
        !s.bold && !s.dim && !s.italic && !s.underline &&
        !s.inverse && !s.hidden && !s.strike && !s.blink
}
```

Complexity: O(1).

---

## 4. File Organization

All types, palette data, and utility functions go in `cmd/rwin/ansi_color.go`.
Tests go in `cmd/rwin/ansi_color_test.go`.

The `styledRun` type is defined here (since it embeds `sgrState`) but is
primarily consumed by span generation in `cmd/rwin/ansi_spans.go` (Phase 3).

---

## 5. Design Notes

- **No `image/color` dependency**: Using `[3]uint8` for palette entries avoids
  pulling in `image/color` for a single use. The `ansiColor` type already
  carries R, G, B directly.

- **Palette is not configurable**: These are standard ANSI colors. User
  customization (e.g., solarized themes) is a future concern.

- **Inverse default colors**: The fallback values `#fffff0` and `#000000`
  match edwood's default acme color scheme. If edwood's theme changes, these
  would need updating. Acceptable for now.

- **Dim on default foreground**: Returns `#808080` (mid-gray) which is a
  reasonable visual approximation of "dimmed black text on light background."

- **Attributes tracked but not emitted**: `underline`, `blink`, and `strike`
  are tracked in `sgrState` (so SGR on/off codes work correctly) but have no
  corresponding span flag. They are not emitted. `bold`, `italic`, and
  `hidden` are emitted as span flags.
