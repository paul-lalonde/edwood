# ANSI OSC Handling

Design for OSC (Operating System Command) sequence parsing and dispatch in
`cmd/rwin/ansi.go`. Extends the parser from Phase 2.1 to handle OSC title
sequences and silently consume all other OSC numbers.

**Source**: Distilled from `cmd/rwin/ANSI_DESIGN.md` section 7.

**Depends on**: `cmd/rwin/ansi.go` (parser state machine from Phase 2.1),
`cmd/rwin/ansi_color.go` (types).

---

## 1. OSC Sequence Format

OSC sequences have two forms:

```
ESC ] <number> ; <payload> BEL          (BEL-terminated)
ESC ] <number> ; <payload> ESC \        (ST-terminated)
```

- `<number>` is a decimal integer identifying the OSC command.
- `<payload>` is an arbitrary string (the meaning depends on the number).
- `BEL` is byte 0x07.
- `ESC \` is the two-byte String Terminator (ST).

The payload may be empty. Some OSC sequences omit the `;` and payload
entirely (e.g., `ESC ] 0 BEL` sets an empty title).

---

## 2. OSC State Transitions (Already Implemented)

The parser already has OSC states from Phase 2.1. They are fully wired in
`Process()`:

### stateOSC (collecting the numeric parameter)

| Input | Action | Next State |
|-------|--------|------------|
| `0`-`9` | `oscNum = oscNum*10 + digit` | `stateOSC` |
| `;` | — | `stateOSCString` |
| BEL (0x07) | Dispatch OSC (empty payload) | `stateGround` |
| ESC (0x1B) | Set `prevOSC = true` | `stateEsc` |
| Other | — (malformed, discard) | `stateGround` |

### stateOSCString (collecting the payload)

| Input | Action | Next State |
|-------|--------|------------|
| BEL (0x07) | Dispatch OSC with payload | `stateGround` |
| ESC (0x1B) | Set `prevOSC = true` | `stateEsc` |
| Other | Append rune to `oscBuf` | `stateOSCString` |

### ST Detection (in stateEsc)

When `prevOSC` is true and the next byte is `\` (0x5C), dispatch the OSC
and return to ground. Otherwise, clear `prevOSC` and handle the byte as a
normal ESC sequence continuation.

This is already implemented in `Process()` at the top of the `stateEsc`
case.

---

## 3. Parser Fields Used by OSC

These fields already exist on `ansiParser` from Phase 2.1:

```go
type ansiParser struct {
    // ... other fields ...
    oscNum    int       // OSC command number (accumulated from digits)
    oscBuf    []rune    // OSC payload string accumulator
    prevOSC   bool      // true when transitioning from OSC to stateEsc (for ST)

    titleFunc func(title string)  // callback for title changes
}
```

No new fields are needed. The `oscBuf` is reset (sliced to zero) when
entering `stateOSC` from `stateEsc` (on `]`).

---

## 4. dispatchOSC() Implementation

Replace the Phase 2.1 no-op stub with actual dispatch logic.

### Signature

```go
func (p *ansiParser) dispatchOSC()
```

### Behavior

```go
func (p *ansiParser) dispatchOSC() {
    switch p.oscNum {
    case 0, 1, 2:
        // Set window title.
        title := string(p.oscBuf)
        if p.titleFunc != nil {
            p.titleFunc(title)
        }
    // All other OSC numbers are silently consumed.
    }
}
```

### Dispatched OSC Numbers

| OSC Number | Purpose | Action |
|------------|---------|--------|
| 0 | Set icon name and window title | Call `titleFunc(payload)` |
| 1 | Set icon name | Call `titleFunc(payload)` (same as 0) |
| 2 | Set window title | Call `titleFunc(payload)` (same as 0) |

All three are treated identically: extract the payload as a string and
invoke `titleFunc`. The distinction between icon name and window title is
irrelevant for edwood/acme — there is only one name.

### Ignored OSC Numbers

| OSC Number | Purpose | Handling |
|------------|---------|----------|
| 7 | Current working directory | Silently consumed |
| 8 | Hyperlinks | Silently consumed |
| 10-19 | Color queries/changes | Silently consumed |
| 52 | Clipboard | Silently consumed |
| 133 | Shell integration (prompt marking) | Silently consumed |
| All others | Various | Silently consumed |

These sequences are fully consumed by the parser (the entire escape
sequence including payload is stripped from output). No action is taken.

---

## 5. titleFunc Callback Interface

The `titleFunc` field on `ansiParser` is set at construction time via
`NewAnsiParser(titleFunc)`. It receives the raw OSC payload string.

### Nil Safety

If `titleFunc` is nil, OSC title sequences are silently consumed with no
side effect. This is useful for testing the parser in isolation.

### Integration Target

In the final integration (Phase 4), `titleFunc` will be wired to
`winWin.setWindowTitle()`:

```go
func (w *winWin) setWindowTitle(title string) {
    windowname := title
    if !strings.Contains(windowname, "-") {
        windowname = windowname + "/-" + w.sysname
    }
    w.Printf("ctl", "name %s\n", windowname)
}
```

This replaces the existing `label()` function. The `setWindowTitle` logic
is implemented and tested in Phase 4, not in this phase. In Phase 2.2, we
only test that `dispatchOSC()` correctly invokes `titleFunc` with the
payload.

---

## 6. Empty Payload Handling

An OSC sequence may have no payload:

- `ESC ] 0 BEL` — BEL arrives while still in `stateOSC` (before `;`).
  `oscBuf` is empty. `dispatchOSC()` calls `titleFunc("")`.
- `ESC ] 0 ; BEL` — Semicolon transitions to `stateOSCString`, then BEL
  immediately. `oscBuf` is empty. `dispatchOSC()` calls `titleFunc("")`.

Both cases are valid. The callback receives an empty string, which is a
legitimate title (clears the window title).

---

## 7. Split-Sequence Handling

OSC sequences can be split across PTY reads, just like CSI sequences. The
parser's persistent state handles this:

- **Split in numeric parameter**: Parser is in `stateOSC`, `oscNum` has
  the partial number. Next read continues accumulating digits.
- **Split in payload**: Parser is in `stateOSCString`, `oscBuf` has the
  partial payload. Next read continues appending runes.
- **Split at ESC of ST**: Parser transitions to `stateEsc` with
  `prevOSC = true`. Next read's first byte determines whether it's `\`
  (complete ST, dispatch OSC) or something else (OSC was unterminated,
  continue as normal ESC sequence).

Example: `ESC ] 0 ; m y t i` | `t l e BEL`
- First read: ESC → stateEsc, `]` → stateOSC (reset oscNum/oscBuf),
  `0` → oscNum=0, `;` → stateOSCString, `m`, `y`, `t`, `i` → oscBuf
  accumulates. Read ends in stateOSCString.
- Second read: `t`, `l`, `e` → oscBuf continues. BEL → dispatchOSC
  (titleFunc("mytitle")), stateGround.

---

## 8. OSC Interleaved with Styled Text

OSC sequences can appear between styled text runs. The parser handles this
naturally:

```
ESC[1;31m red text ESC]0;My Title BEL more red text ESC[0m normal
```

Processing:
1. `ESC[1;31m` → SGR dispatch: bold + red foreground.
2. ` red text ` → Emitted as styled run (bold, red fg).
3. `ESC]0;My Title BEL` → OSC consumed, titleFunc("My Title") called.
   No output emitted. Style state unchanged.
4. ` more red text ` → Emitted as styled run (still bold, red fg).
5. `ESC[0m` → SGR reset.
6. ` normal` → Emitted as styled run (default style).

Key property: OSC sequences do not affect SGR state. They are invisible
to the text stream and styling.

---

## 9. Subsuming label()

The existing `label()` function in `rwin.go` scans backward through rune
buffers looking for `\033];...\007` patterns to extract window titles. It
has several limitations:

- Only handles the last occurrence in the buffer.
- Scans backward with heuristic matching.
- Uses fragile `-` character detection for path formatting.
- Is a separate pass over the data.

With OSC handling in the parser, `label()` is no longer needed:

- The parser processes OSC sequences in-line during its single forward pass.
- Multiple OSC title sequences in one read are all dispatched (last one
  wins, matching terminal behavior).
- The `titleFunc` callback handles the name formatting.
- The escape sequences are stripped from output as a natural consequence
  of the parser not emitting escape bytes.

`label()` removal happens in Phase 4 (integration). This phase establishes
the dispatch mechanism that replaces it.

---

## 10. Changes to ansi.go

The only code change in this phase is replacing the `dispatchOSC()` stub:

**Before** (Phase 2.1):
```go
func (p *ansiParser) dispatchOSC() {
    // OSC sequences are consumed and discarded for now.
}
```

**After** (Phase 2.2):
```go
func (p *ansiParser) dispatchOSC() {
    switch p.oscNum {
    case 0, 1, 2:
        title := string(p.oscBuf)
        if p.titleFunc != nil {
            p.titleFunc(title)
        }
    }
}
```

No state machine changes. No new fields. No new methods. The OSC state
transitions and byte accumulation are already correct from Phase 2.1.

---

## 11. Test Categories

Tests to add to `cmd/rwin/ansi_test.go`:

1. **OSC 0/1/2 with BEL terminator**: Verify titleFunc is called with the
   payload string. Test each of OSC 0, 1, and 2.

2. **OSC with ST terminator (ESC \\)**: Verify the two-byte ST terminator
   works identically to BEL.

3. **Title callback invoked with payload**: Verify the exact string passed
   to titleFunc matches the OSC payload.

4. **Unknown OSC numbers stripped silently**: OSC 7, 8, 52, 133, and
   arbitrary numbers produce no callback and no output.

5. **OSC split across reads**: Split an OSC sequence at various points
   (in numeric param, in payload, at ESC of ST) and verify correct
   dispatch after the second read.

6. **OSC interleaved with styled text**: Verify that OSC sequences between
   styled runs don't affect style state and that clean text is correct.

7. **Empty OSC payload**: Both `ESC]0;BEL` (empty after semicolon) and
   `ESC]0BEL` (no semicolon) produce titleFunc("").

---

## 12. Design Notes

- **O(n) unchanged**: dispatchOSC() is O(1) — a switch on oscNum and a
  string conversion. No change to the overall O(n) complexity of Process().

- **No allocations added to hot path**: `string(p.oscBuf)` allocates only
  when a title OSC is dispatched, which is infrequent (typically once per
  command, when the shell updates the title). Non-title OSC numbers don't
  allocate at all.

- **Backward compatible**: Existing tests pass unchanged. The only
  behavioral difference is that titleFunc is now called instead of being
  ignored. Tests that use a nil titleFunc see no change.

- **sgrState unaffected by OSC**: OSC dispatch does not modify `p.sgr`.
  Style state is orthogonal to OSC handling.
