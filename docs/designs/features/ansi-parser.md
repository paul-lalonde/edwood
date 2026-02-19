# ANSI Parser State Machine & SGR Dispatch

Design for `cmd/rwin/ansi.go` — the state machine that strips ANSI escape
sequences from PTY output and tracks cumulative SGR styling.

**Source**: Distilled from `cmd/rwin/ANSI_DESIGN.md` sections 1, 2, 10.

**Depends on**: `cmd/rwin/ansi_color.go` (types: `ansiColor`, `sgrState`,
`styledRun`; palette: `ansiPalette`).

---

## 1. State Enum

```go
type ansiState int

const (
    stateGround   ansiState = iota // Normal text — printable runes emitted
    stateEsc                        // Received ESC (0x1B), awaiting next byte
    stateCSI                        // CSI introducer received (ESC [)
    stateCSIParam                   // Collecting CSI numeric parameters
    stateOSC                        // OSC introducer received (ESC ])
    stateOSCString                  // Collecting OSC payload string
    stateIgnore                     // Consuming unsupported sequence bytes
)
```

Seven states total. OSC states are defined here but fully implemented in
Phase 2.2. During Phase 2.1, the parser transitions into `stateOSC` on
`ESC ]` and consumes bytes until `BEL` or `ST`, but `dispatchOSC()` is a
no-op stub.

---

## 2. Parser Struct

```go
type ansiParser struct {
    state    ansiState
    params   []int      // accumulated CSI parameters
    curParam int        // current parameter being built (-1 = no digit seen)
    hasParam bool       // true if at least one digit seen for curParam
    private  byte       // private marker ('?', '>', '!') or 0
    oscNum   int        // OSC command number
    oscBuf   []rune     // OSC string payload accumulator
    sgr      sgrState   // current cumulative style state

    // titleFunc is called when an OSC title sequence is complete.
    // Nil-safe: if nil, OSC titles are silently consumed.
    titleFunc func(title string)
}
```

### Constructor

```go
func NewAnsiParser(titleFunc func(string)) *ansiParser {
    return &ansiParser{
        state:     stateGround,
        params:    make([]int, 0, 16),
        curParam:  0,
        titleFunc: titleFunc,
    }
}
```

The params slice is pre-allocated with capacity 16 (typical CSI sequences
have 1-5 parameters; 16 covers extended color sequences with room to spare).

---

## 3. Process() Method

### Signature

```go
func (p *ansiParser) Process(input []rune) (clean []rune, runs []styledRun)
```

### Behavior

Single-pass O(n) scan over the input runes. For each rune, the action
depends on the current state:

- **Printable runes in ground state**: Appended to the current run's text.
  If the current style differs from the previous run's style (because an SGR
  was just processed), a new run is started.
- **ESC (0x1B) in any state**: Transitions to `stateEsc`. If encountered
  while already in a CSI/OSC sequence, the in-progress sequence is aborted
  (per ECMA-48, ESC restarts sequence parsing).
- **Escape sequence bytes**: Consumed without being appended to clean output.
- **C0 controls (0x00-0x1F except ESC)**: Passed through to clean output in
  ground state (they are handled by later pipeline stages). In CSI/OSC
  states, BEL (0x07) may terminate an OSC; other C0 controls are ignored
  within sequences.

### Run Management

The runs slice tracks contiguous styled regions:

```
currentRun = styledRun{style: p.sgr}

For each printable rune:
    if p.sgr != currentRun.style:
        if len(currentRun.text) > 0:
            append currentRun to runs
        currentRun = styledRun{style: p.sgr}
    append rune to currentRun.text

For each C0 control passed through:
    same logic as printable — append to currentRun.text
    (C0 controls occupy character positions in the output)

After processing all input:
    if len(currentRun.text) > 0:
        append currentRun to runs
```

The clean output is the concatenation of all run texts. For efficiency,
`clean` is built by appending runes directly (not by joining runs afterward).

### No Allocations in Hot Path

- `clean` and `runs` are built by appending. The Go runtime amortizes
  append allocations.
- `params` is reused across sequences (cleared by `resetCSI()`).
- No string conversions during parsing — rune-level operations only.

---

## 4. State Transition Table

### Ground State

| Input | Action | Next State |
|-------|--------|------------|
| ESC (0x1B) | — | `stateEsc` |
| C0 control (0x00-0x1F, not ESC) | Emit to clean output | `stateGround` |
| Printable (>= 0x20) | Emit to clean output, extend current run | `stateGround` |

### Esc State

| Input | Action | Next State |
|-------|--------|------------|
| `[` (0x5B) | Reset CSI params | `stateCSI` |
| `]` (0x5D) | Reset OSC state | `stateOSC` |
| `(` `)` `*` `+` | — (charset designation, next byte consumed) | `stateIgnore` |
| `=` `>` | — (DECKPAM/DECKPNM, ignored) | `stateGround` |
| ESC (0x1B) | — (re-enter ESC) | `stateEsc` |
| Other | — (unknown, discarded) | `stateGround` |

### CSI State (initial, before any param digits)

| Input | Action | Next State |
|-------|--------|------------|
| `0`-`9` | Begin first parameter | `stateCSIParam` |
| `;` | Push 0 (default), begin next | `stateCSIParam` |
| `?` `>` `!` | Store as private marker | `stateCSIParam` |
| `m` | Push current param, dispatch SGR | `stateGround` |
| Other final byte (0x40-0x7E) | Dispatch/ignore non-SGR CSI | `stateGround` |
| ESC (0x1B) | Abort CSI | `stateEsc` |

### CSIParam State

| Input | Action | Next State |
|-------|--------|------------|
| `0`-`9` | `curParam = curParam*10 + digit` | `stateCSIParam` |
| `;` | Push curParam, reset curParam | `stateCSIParam` |
| `m` | Push curParam, dispatch SGR | `stateGround` |
| Other final byte (0x40-0x7E) | Push curParam, dispatch/ignore | `stateGround` |
| ESC (0x1B) | Abort CSI | `stateEsc` |

### OSC State (collecting numeric parameter)

| Input | Action | Next State |
|-------|--------|------------|
| `0`-`9` | `oscNum = oscNum*10 + digit` | `stateOSC` |
| `;` | — | `stateOSCString` |
| BEL (0x07) | Dispatch OSC (empty payload) | `stateGround` |
| ESC (0x1B) | Check for ST | (see below) |
| Other | — (malformed, discard) | `stateGround` |

### OSCString State (collecting payload)

| Input | Action | Next State |
|-------|--------|------------|
| BEL (0x07) | Dispatch OSC with payload | `stateGround` |
| ESC (0x1B) | Check for ST (`\` next) | (see below) |
| Other | Append rune to `oscBuf` | `stateOSCString` |

**ST detection**: When ESC is received in OSC or OSCString state, the parser
transitions to `stateEsc`. On the *next* byte, if it is `\` (backslash,
0x5C), that completes the ST terminator — dispatch OSC and go to ground. If
it is `[`, treat as a new CSI (the OSC was unterminated). Otherwise, handle
as a normal Esc sequence.

Implementation detail: before transitioning to `stateEsc` from OSC states,
save a flag (`oscPending bool`) so that the Esc handler knows an OSC
dispatch may be needed if `\` follows. Alternatively, handle this inline:
when in `stateEsc` and the previous state was an OSC state, check for `\`.

Simpler approach chosen: Add a `prevOSC bool` field to the parser. Set it
true when transitioning from OSC/OSCString to stateEsc. In the stateEsc
handler, if `prevOSC && r == '\\'`, dispatch OSC and go to ground. Otherwise
clear `prevOSC` and handle normally.

### Ignore State

| Input | Action | Next State |
|-------|--------|------------|
| Any single byte | Consumed | `stateGround` |

Used for charset designation sequences (`ESC (`, `ESC )`, etc.) which take
exactly one byte after the introducer. For unknown multi-byte sequences, the
Ignore state consumes until a final byte in the 0x40-0x7E range (but in
practice, charset designation is the only case, and it's always one byte).

---

## 5. CSI Parameter Management

### resetCSI()

```go
func (p *ansiParser) resetCSI() {
    p.params = p.params[:0]
    p.curParam = 0
    p.hasParam = false
    p.private = 0
}
```

Called when entering `stateCSI` from `stateEsc`.

### Parameter Accumulation

Digit bytes: `p.curParam = p.curParam*10 + int(r - '0')`, set `p.hasParam = true`.

Semicolon: Push `p.curParam` to `p.params`, reset `p.curParam = 0` and
`p.hasParam = false`.

Final byte: Push `p.curParam` to `p.params` (if `p.hasParam` or if params
is non-empty after a trailing `;`), then dispatch.

### Edge Cases

- **No parameters** (`ESC[m`): params is empty after push. SGR dispatch
  treats empty params as `[0]` (reset). This is required by ECMA-48.
- **Trailing semicolon** (`ESC[1;m`): After the `;`, `curParam` is 0 and
  `hasParam` is false. The push before dispatch adds 0. Result: `[1, 0]`.
  SGR processes both (bold on, then reset). This matches terminal behavior.
- **Private marker** (`ESC[?25h`): `private` is set to `?`. When `private`
  is non-zero, dispatchSGR is skipped (private CSI sequences are not SGR).
  The sequence is silently consumed.

---

## 6. dispatchSGR()

Called when a CSI sequence ends with `m` (the SGR final byte) and `private`
is 0. Processes the accumulated `params` slice, updating `p.sgr`.

### Signature

```go
func (p *ansiParser) dispatchSGR()
```

### Algorithm

```go
func (p *ansiParser) dispatchSGR() {
    params := p.params
    if len(params) == 0 {
        params = []int{0} // ESC[m = reset
    }

    for i := 0; i < len(params); i++ {
        code := params[i]
        switch {
        case code == 0:
            p.sgr.reset()

        // Attribute on
        case code == 1:
            p.sgr.bold = true
        case code == 2:
            p.sgr.dim = true
        case code == 3:
            p.sgr.italic = true
        case code == 4:
            p.sgr.underline = true
        case code == 5 || code == 6:
            p.sgr.blink = true
        case code == 7:
            p.sgr.inverse = true
        case code == 8:
            p.sgr.hidden = true
        case code == 9:
            p.sgr.strike = true

        // Attribute off
        case code == 21:
            p.sgr.bold = false
        case code == 22:
            p.sgr.bold = false
            p.sgr.dim = false
        case code == 23:
            p.sgr.italic = false
        case code == 24:
            p.sgr.underline = false
        case code == 25:
            p.sgr.blink = false
        case code == 27:
            p.sgr.inverse = false
        case code == 28:
            p.sgr.hidden = false
        case code == 29:
            p.sgr.strike = false

        // Standard foreground colors (30-37)
        case code >= 30 && code <= 37:
            idx := code - 30
            c := ansiPalette[idx]
            p.sgr.fg = ansiColor{set: true, r: c[0], g: c[1], b: c[2]}

        // Extended foreground (38;5;N or 38;2;R;G;B)
        case code == 38:
            i = p.parseExtendedColor(params, i, &p.sgr.fg)

        // Default foreground
        case code == 39:
            p.sgr.fg = ansiColor{}

        // Standard background colors (40-47)
        case code >= 40 && code <= 47:
            idx := code - 40
            c := ansiPalette[idx]
            p.sgr.bg = ansiColor{set: true, r: c[0], g: c[1], b: c[2]}

        // Extended background (48;5;N or 48;2;R;G;B)
        case code == 48:
            i = p.parseExtendedColor(params, i, &p.sgr.bg)

        // Default background
        case code == 49:
            p.sgr.bg = ansiColor{}

        // Bright foreground colors (90-97)
        case code >= 90 && code <= 97:
            idx := code - 90 + 8
            c := ansiPalette[idx]
            p.sgr.fg = ansiColor{set: true, r: c[0], g: c[1], b: c[2]}

        // Bright background colors (100-107)
        case code >= 100 && code <= 107:
            idx := code - 100 + 8
            c := ansiPalette[idx]
            p.sgr.bg = ansiColor{set: true, r: c[0], g: c[1], b: c[2]}

        // Unknown codes: silently ignored
        }
    }
}
```

### Extended Color Sub-Parameter Parsing

```go
// parseExtendedColor handles 38;5;N (256-color) and 38;2;R;G;B (truecolor).
// i is the index of the 38 or 48 parameter. Returns the new index (advanced
// past consumed sub-parameters).
func (p *ansiParser) parseExtendedColor(params []int, i int, target *ansiColor) int {
    if i+1 >= len(params) {
        return i // malformed, ignore
    }
    switch params[i+1] {
    case 5: // 256-color: 38;5;N
        if i+2 >= len(params) {
            return i + 1 // malformed
        }
        idx := params[i+2]
        if idx >= 0 && idx <= 255 {
            c := ansiPalette[idx]
            *target = ansiColor{set: true, r: c[0], g: c[1], b: c[2]}
        }
        return i + 2

    case 2: // truecolor: 38;2;R;G;B
        if i+4 >= len(params) {
            return i + 1 // malformed
        }
        r := clampByte(params[i+2])
        g := clampByte(params[i+3])
        b := clampByte(params[i+4])
        *target = ansiColor{set: true, r: r, g: g, b: b}
        return i + 4

    default:
        return i + 1 // unknown sub-type, skip
    }
}

func clampByte(v int) uint8 {
    if v < 0 {
        return 0
    }
    if v > 255 {
        return 255
    }
    return uint8(v)
}
```

### Non-SGR CSI Dispatch

When the final byte is not `m`, or when `private` is non-zero, the CSI
sequence is silently consumed. No action is taken. This handles cursor
movement, erase, mode set/reset, and other CSI sequences that rwin does not
support.

---

## 7. Split-Sequence Handling

The parser struct persists between `Process()` calls. When a PTY read ends
mid-sequence:

- **Mid-ESC**: Parser is in `stateEsc`. Next read starts by interpreting the
  first byte as the ESC continuation.
- **Mid-CSI**: Parser is in `stateCSI` or `stateCSIParam`. `params`,
  `curParam`, `hasParam`, and `private` retain their values. Next read
  continues accumulating parameters or receives the final byte.
- **Mid-OSC**: Parser is in `stateOSC` or `stateOSCString`. `oscNum` and
  `oscBuf` retain their values.

Example: Input split as `\x1b[1;3` | `1m`
- First read: ESC → stateEsc, `[` → stateCSI (resetCSI), `1` → stateCSIParam
  (curParam=1), `;` → push 1, curParam=0, `3` → curParam=3. Read ends in
  stateCSIParam.
- Second read: `1` → curParam=31, `m` → push 31, dispatch SGR([1, 31]),
  stateGround. Result: bold + red foreground.

---

## 8. ESC Handling Within Sequences

Per ECMA-48, an ESC byte received while inside a CSI or OSC sequence aborts
the current sequence and starts a new escape sequence. The parser implements
this: any state that receives ESC transitions to `stateEsc` (the in-progress
CSI/OSC state is abandoned).

This is important for robustness — if a sequence is malformed or truncated
and followed by a new ESC, the parser recovers cleanly.

---

## 9. Process() Pseudocode

```
func (p *ansiParser) Process(input []rune) (clean []rune, runs []styledRun):
    clean = make([]rune, 0, len(input))
    var currentRun styledRun
    currentRun.style = p.sgr  // inherit style from previous call

    for each rune r in input:
        switch p.state:

        case stateGround:
            if r == 0x1B:
                p.state = stateEsc
            else:
                // Printable or C0 control — emit to output
                if p.sgr != currentRun.style:
                    if len(currentRun.text) > 0:
                        runs = append(runs, currentRun)
                    currentRun = styledRun{style: p.sgr}
                currentRun.text = append(currentRun.text, r)
                clean = append(clean, r)

        case stateEsc:
            p.prevOSC = false  // clear any pending OSC
            switch r:
            case '[':
                p.resetCSI()
                p.state = stateCSI
            case ']':
                p.oscNum = 0
                p.oscBuf = p.oscBuf[:0]
                p.state = stateOSC
            case '(', ')', '*', '+':
                p.state = stateIgnore
            case '=', '>':
                p.state = stateGround
            case 0x1B:
                p.state = stateEsc  // stay
            default:
                p.state = stateGround

        case stateCSI:
            switch:
            case r >= '0' && r <= '9':
                p.curParam = int(r - '0')
                p.hasParam = true
                p.state = stateCSIParam
            case r == ';':
                p.params = append(p.params, 0)
                p.state = stateCSIParam
            case r == '?' || r == '>' || r == '!':
                p.private = byte(r)
                p.state = stateCSIParam
            case r == 'm':
                p.pushParam()
                if p.private == 0:
                    p.dispatchSGR()
                p.state = stateGround
            case r >= 0x40 && r <= 0x7E:
                p.pushParam()
                // non-SGR CSI: silently consumed
                p.state = stateGround
            case r == 0x1B:
                p.state = stateEsc
            // intermediate bytes (0x20-0x2F): stay in CSI state

        case stateCSIParam:
            switch:
            case r >= '0' && r <= '9':
                p.curParam = p.curParam*10 + int(r - '0')
                p.hasParam = true
            case r == ';':
                p.params = append(p.params, p.curParam)
                p.curParam = 0
                p.hasParam = false
            case r == 'm':
                p.pushParam()
                if p.private == 0:
                    p.dispatchSGR()
                p.state = stateGround
            case r >= 0x40 && r <= 0x7E:
                p.pushParam()
                p.state = stateGround
            case r == 0x1B:
                p.state = stateEsc

        case stateOSC:
            // Collecting OSC number
            switch:
            case r >= '0' && r <= '9':
                p.oscNum = p.oscNum*10 + int(r - '0')
            case r == ';':
                p.state = stateOSCString
            case r == 0x07:  // BEL
                p.dispatchOSC()
                p.state = stateGround
            case r == 0x1B:
                p.prevOSC = true
                p.state = stateEsc
            default:
                p.state = stateGround  // malformed

        case stateOSCString:
            switch:
            case r == 0x07:  // BEL
                p.dispatchOSC()
                p.state = stateGround
            case r == 0x1B:
                p.prevOSC = true
                p.state = stateEsc
            default:
                p.oscBuf = append(p.oscBuf, r)

        case stateIgnore:
            // Consume one byte and return to ground
            p.state = stateGround

    // Flush remaining run
    if len(currentRun.text) > 0:
        runs = append(runs, currentRun)

    return clean, runs
```

### prevOSC / ST Detection in stateEsc

The stateEsc handler shown above needs a refinement for ST detection:

```
case stateEsc:
    if p.prevOSC && r == '\\':
        p.dispatchOSC()
        p.prevOSC = false
        p.state = stateGround
        continue
    p.prevOSC = false
    // ... normal Esc handling
```

---

## 10. pushParam() Helper

```go
func (p *ansiParser) pushParam() {
    p.params = append(p.params, p.curParam)
    p.curParam = 0
    p.hasParam = false
}
```

Called before dispatching any CSI final byte. Always pushes the current
parameter value (even if no digits were seen, in which case it pushes 0,
the ECMA-48 default).

---

## 11. dispatchOSC() Stub (Phase 2.1)

In Phase 2.1, `dispatchOSC()` is a no-op:

```go
func (p *ansiParser) dispatchOSC() {
    // OSC dispatch implemented in Phase 2.2.
    // For now, OSC sequences are consumed and discarded.
}
```

OSC states and transitions are fully implemented so that OSC sequences
are correctly stripped from output. The actual title callback logic is
added in Phase 2.2.

---

## 12. Parser Struct Additions for OSC/ST

The parser struct needs one additional field beyond what section 2 shows:

```go
type ansiParser struct {
    // ... (fields from section 2)
    prevOSC bool  // true if transitioning to stateEsc from an OSC state
}
```

This is used only for ST (String Terminator) detection.

---

## 13. SGR Code Summary

All SGR codes handled by `dispatchSGR()`:

| Code Range | Action |
|------------|--------|
| 0 | Reset all to defaults |
| 1 | `bold = true` |
| 2 | `dim = true` |
| 3 | `italic = true` |
| 4 | `underline = true` |
| 5-6 | `blink = true` |
| 7 | `inverse = true` |
| 8 | `hidden = true` |
| 9 | `strike = true` |
| 21 | `bold = false` |
| 22 | `bold = false, dim = false` |
| 23 | `italic = false` |
| 24 | `underline = false` |
| 25 | `blink = false` |
| 27 | `inverse = false` |
| 28 | `hidden = false` |
| 29 | `strike = false` |
| 30-37 | `fg = palette[code-30]` |
| 38;5;N | `fg = palette[N]` (256-color) |
| 38;2;R;G;B | `fg = {R, G, B}` (truecolor) |
| 39 | `fg = default` |
| 40-47 | `bg = palette[code-40]` |
| 48;5;N | `bg = palette[N]` (256-color) |
| 48;2;R;G;B | `bg = {R, G, B}` (truecolor) |
| 49 | `bg = default` |
| 90-97 | `fg = palette[code-90+8]` (bright) |
| 100-107 | `bg = palette[code-100+8]` (bright) |
| Other | Silently ignored |

---

## 14. File Organization

Implementation file: `cmd/rwin/ansi.go`

Contents:
- `ansiState` type and constants
- `ansiParser` struct
- `NewAnsiParser()` constructor
- `Process()` method
- `dispatchSGR()` method
- `parseExtendedColor()` method
- `clampByte()` utility
- `resetCSI()` method
- `pushParam()` method
- `dispatchOSC()` stub

Test file: `cmd/rwin/ansi_test.go`

---

## 15. Design Notes

- **O(n) single pass**: `Process()` iterates over input exactly once. Each
  rune triggers at most one state transition and one output action. No
  backtracking.

- **O(k) SGR dispatch**: `dispatchSGR()` iterates over the `k` accumulated
  parameters once. Extended color parsing consumes sub-parameters inline by
  advancing the loop index.

- **Style inheritance across calls**: The `sgr` field persists across
  `Process()` calls. The first run of each call inherits the style from the
  end of the previous call. This handles programs that set a color in one
  write and emit colored text across multiple reads.

- **Run coalescing**: Adjacent runes with the same style are combined into a
  single `styledRun`. A new run starts only when the style changes (after an
  SGR sequence). This minimizes the number of span lines generated.

- **C0 controls as output**: Control characters (LF, CR, TAB, NUL, etc.)
  are emitted to the clean output. They are part of styled runs — their
  positions count toward run lengths. Later pipeline stages (dropcrnl,
  dropcr, squashnulls) modify or remove them, but that happens after span
  generation. Per the plan's recommendation, span offset mismatch from CR
  handling is accepted for Phase 1.

- **ESC aborts in-progress sequences**: This ensures the parser never gets
  stuck in a state due to malformed input. Any ESC resets to the Esc state,
  and any truly unrecognized sequence falls through to ground.

- **Private CSI sequences skipped**: When `private` is non-zero (e.g.,
  `ESC[?25h` for cursor show), the sequence is consumed but `dispatchSGR()`
  is not called. This correctly ignores DEC private mode sequences.

- **sgrState comparison**: Run coalescing requires comparing `sgrState`
  values. Since `sgrState` is a plain struct with no pointers or slices,
  Go's `==` operator works correctly for comparison.
