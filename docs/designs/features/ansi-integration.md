# ANSI Integration Design

Wires the ANSI parser into the `rwin` pipeline, opens the spans file for
writing, changes the TERM variable, and removes the `label()` function.

**Package**: `main` (cmd/rwin)
**Primary file**: `cmd/rwin/rwin.go`

---

## Dependencies

From Phase 1 (`ansi_color.go`):
- `sgrState`, `styledRun` types

From Phase 2 (`ansi.go`):
- `ansiParser` struct
- `NewAnsiParser(titleFunc func(string)) *ansiParser`
- `(*ansiParser).Process(input []rune) (clean []rune, runs []styledRun)`

From Phase 3 (`ansi_spans.go`):
- `buildSpanWrite(baseOffset int, runs []styledRun) string`

From the 9P client library:
- `9fans.net/go/plan9` — `plan9.OWRITE`
- `9fans.net/go/plan9/client` — `client.MountService("acme")`, `(*Fsys).Open()`

---

## winWin Struct Additions

Add two fields to the `winWin` struct:

```go
type winWin struct {
    // ... existing fields ...
    parser   *ansiParser   // persistent ANSI parser (survives across reads)
    spansFid *client.Fid   // open handle to <winid>/spans (nil if open fails)
}
```

`parser` holds the state machine that strips escapes and tracks SGR state.
It persists across PTY reads to handle split sequences.

`spansFid` is the 9P file descriptor for writing span definitions. It is
kept open for the window lifetime to avoid repeated open/close overhead.
If the open fails (e.g., older edwood without spans support), it stays nil
and span writes are silently skipped.

---

## Initialization Changes

### NewWinWin / main()

After creating the acme window and before starting the process:

```go
// In main(), after NewWinWin() and win.W.Name():

// Mount the acme 9P service for spans file access.
fsys, err := client.MountService("acme")
if err != nil {
    fmt.Fprintf(os.Stderr, "rwin: mount acme (spans): %v\n", err)
    // Non-fatal: spans will be disabled (spansFid stays nil).
} else {
    fid, err := fsys.Open(fmt.Sprintf("%d/spans", win.W.ID()), plan9.OWRITE)
    if err != nil {
        fmt.Fprintf(os.Stderr, "rwin: open spans: %v\n", err)
        // Non-fatal: span writes silently skipped.
    } else {
        win.spansFid = fid
    }
}

// Create the ANSI parser with the title callback.
win.parser = NewAnsiParser(win.setWindowTitle)
```

**Ordering rationale**: The spans fid must be opened *after* the acme
window exists (so the window ID is valid) and *before* `stdoutproc()`
starts (so the fid is available for the first PTY read).

The parser is created *after* `NewWinWin()` because the `setWindowTitle`
method needs the `winWin` receiver. It must be created before
`startProcess()` because the first PTY output will pass through it.

### Cleanup

When the window exits (either from the `events()` goroutine calling
`os.Exit(0)` or from an error path), the spans fid should be closed:

```go
if win.spansFid != nil {
    win.spansFid.Close()
}
```

Since `rwin` currently calls `os.Exit(0)` directly from `events()`, the
OS will clean up the fid. But for correctness, add a deferred close in
`main()`:

```go
defer func() {
    if win.spansFid != nil {
        win.spansFid.Close()
    }
}()
```

---

## startProcess TERM/COLORTERM Changes

In `startProcess()`, change the environment setup from:

```go
cmd.Env = append(os.Environ(), []string{"TERM=dumb",
    fmt.Sprintf("winid=%d", w.W.ID())}...)
```

to:

```go
cmd.Env = append(os.Environ(), []string{
    "TERM=xterm-256color",
    "COLORTERM=truecolor",
    fmt.Sprintf("winid=%d", w.W.ID()),
}...)
```

**Rationale** (from ANSI_DESIGN.md section 8):
- `xterm-256color` is the most widely supported terminfo entry that
  enables colored output from programs like `ls`, `grep`, `git`, `bat`.
- `COLORTERM=truecolor` signals 24-bit color support to programs that
  check for it.
- The ANSI parser handles all sequences that `xterm-256color` programs
  emit. Unsupported sequences (cursor movement, alternate screen) are
  harmlessly consumed and discarded.

---

## stdoutproc Modification

The current pipeline is:

```
pty read -> UTF-8 partial handling -> echo.Cancel() -> dropcrnl() ->
  dropcr() -> squashnulls() -> label() -> password detection ->
  Q.Lock -> write to body -> Q.Unlock
```

The new pipeline:

```
pty read -> UTF-8 partial handling -> echo.Cancel() ->
  parser.Process() ->                         <- NEW: strips ANSI, emits runs
  dropcrnl() -> dropcr() -> squashnulls() ->
  password detection ->                       <- label() REMOVED
  Q.Lock -> write to body -> write spans -> Q.Unlock
```

### Full Modified stdoutproc

```go
func (w *winWin) stdoutproc() {
    buf := make([]byte, 8192)

    var partialRune []byte
    for {
        n, err := w.rcpty.Read(buf[len(partialRune):])
        if err == io.EOF {
            continue
        }
        if err != nil {
            fmt.Fprintf(os.Stderr, "win: error reading rcw: %v\n", err)
            panic(err)
        }

        r, m := utf8.DecodeLastRune(buf[:n+len(partialRune)])
        if m != 0 && r == utf8.RuneError {
            partialRune = buf[n-m : n]
        }

        if n > 0 {
            input := []rune(string(buf[0 : n-len(partialRune)]))

            input = w.echo.Cancel(input)

            // ANSI processing: strip escapes, extract styled runs.
            // OSC titles are handled inside the parser (replaces label()).
            clean, runs := w.parser.Process(input)

            clean = dropcrnl(clean)
            clean = dropcr(clean)
            clean = squashnulls(clean)

            // label() call REMOVED — subsumed by parser's OSC dispatch.

            w.password = false
            istring := string(clean)
            if strings.Contains(strings.ToLower(istring), "password") ||
                strings.Contains(strings.ToLower(istring), "passphrase") {
                istring = strings.TrimRight(istring, " ")
                w.password = len(istring) > 0 && istring[len(istring)-1] == ':'
                debugf("password elision: %v\n", w.password)
                clean = []rune(istring)
            }

            w.Q.Lock()
            err := w.Printf("addr", "$")
            if err != nil {
                fmt.Fprintf(os.Stderr, "TODO(PAL): reset addr: %v", err)
                w.Printf("addr", "$")
            }

            n, err := w.W.Write("data", []byte(string(clean)))
            if err != nil || n != len([]byte(string(clean))) {
                fmt.Fprintf(os.Stderr, "Problem flushing body")
            }

            // Write spans if any run has non-default styling.
            // w.p is the offset where clean text was inserted (before increment).
            spanData := buildSpanWrite(w.p, runs)
            if spanData != "" && w.spansFid != nil {
                _, err := w.spansFid.Write([]byte(spanData))
                if err != nil {
                    debugf("spans write error: %v\n", err)
                }
            }

            w.p += len(clean)
            debugf("w.p == %d\n", w.p)

            copy(buf, partialRune)
            partialRune = partialRune[0:0]
            w.Q.Unlock()
        }
    }
}
```

### Key Changes from Current Code

1. **`w.parser.Process(input)` inserted** after `echo.Cancel()`, before
   `dropcrnl()`. Returns `clean` (ANSI-stripped text) and `runs` (styled
   runs for span generation).

2. **`label()` call removed**. OSC title sequences are handled inside
   the parser's `dispatchOSC()`, which calls `w.setWindowTitle()`.

3. **Span write added** after the body data write, before `w.p` is
   incremented. `buildSpanWrite(w.p, runs)` uses the current `w.p` as
   the base offset (where the text was just inserted). The span write
   happens inside `Q.Lock()` to ensure atomicity with the body write.

4. **`dropcrnl()` / `dropcr()` / `squashnulls()`** operate on `clean`
   (the ANSI-stripped output) instead of `input`.

---

## label() Removal

The `label()` method on `winWin` (rwin.go lines 595-624) is deleted
entirely. Its functionality is replaced by:

1. **OSC parsing** in `ansiParser.Process()` — the parser's state machine
   recognizes `ESC ] 0;...BEL` and `ESC ] 2;...BEL` sequences, strips
   them from the output, and dispatches via `dispatchOSC()`.

2. **Title callback** — `dispatchOSC()` calls `p.titleFunc(title)` for
   OSC 0/1/2, which is wired to `w.setWindowTitle()`.

---

## setWindowTitle Method

New method on `winWin` that replaces the inline logic from `label()`:

```go
func (w *winWin) setWindowTitle(title string) {
    windowname := title
    if !strings.Contains(windowname, "-") {
        windowname = windowname + "/-" + w.sysname
    }
    w.Printf("ctl", "name %s\n", windowname)
}
```

This matches the existing `label()` behavior:
- If the title already contains `-`, use it as-is (it likely includes a
  directory component like `/home/user/project/-rc`).
- If not, append `/-` and the system name to form a proper acme window
  name with a dash separator.

The title callback is invoked from within `Process()`, which runs outside
the `Q.Lock()` section. The `Printf("ctl", ...)` call writes to the ctl
file, which is safe to call outside the lock (ctl writes are independent
of body/addr state).

---

## Spans File Access via 9P Client

Following the pattern established by `cmd/edcolor`:

```go
import (
    "9fans.net/go/plan9"
    "9fans.net/go/plan9/client"
)

// Mount the acme 9P service.
fsys, err := client.MountService("acme")

// Open the window's spans file for writing.
fid, err := fsys.Open(fmt.Sprintf("%d/spans", win.W.ID()), plan9.OWRITE)
```

### Why 9P client instead of os.OpenFile

The design doc section 5 mentions `os.OpenFile("/mnt/acme/<id>/spans", ...)`,
but edcolor uses the 9P client library. The 9P client approach is
preferred because:

1. **Portability**: Works regardless of whether acme is mounted at
   `/mnt/acme` (on Plan 9) or served as a 9P service (on Unix).
2. **Consistency**: Matches the existing acme library pattern (`acme.New()`
   already uses the 9P client internally).
3. **Reference implementation**: edcolor uses this exact pattern and is
   known to work correctly.

### Fid Lifetime

The fid is opened once at startup and kept open for the window lifetime:
- **No reconnection logic**: If the fid becomes stale (e.g., edwood
  restarts), writes will fail and be logged/skipped. This is acceptable
  because the window itself would also be gone in that scenario.
- **Graceful degradation**: If the open fails (older edwood without
  spans support), `spansFid` stays nil and all span writes are silently
  skipped. rwin still works correctly — output is just uncolored.

---

## Q.Lock Synchronization

The span write occurs inside the existing `Q.Lock()` / `Q.Unlock()`
critical section, immediately after the body data write and before
`w.p` is incremented. This ensures:

1. **Body-span atomicity**: The body text and its spans are always in
   sync. No event processing can interleave between the text write and
   span write.

2. **Correct offset**: `w.p` accurately reflects where the text landed
   in the body buffer. `buildSpanWrite(w.p, runs)` uses this as the
   base offset for span definitions.

3. **Sequential ordering**: The span protocol's contiguity requirement
   is satisfied because each PTY read produces one contiguous block of
   styled runs.

### w.p Offset Usage

`w.p` tracks the rune offset of the "output point" — where new text
is appended in the body. The sequence is:

```
1. Write clean text to body at $    (text lands at offset w.p)
2. Build span lines using w.p       (spans reference the just-written text)
3. Write spans to spansFid          (atomic under Q.Lock)
4. Increment w.p by len(clean)      (advance output point past new text)
```

This ordering is critical: spans must be written *before* `w.p` advances,
because the span offsets reference the current `w.p` value.

---

## Pipeline Ordering Rationale

### After echo.Cancel()

Echo cancellation operates on raw rune sequences. ANSI escapes are part
of PTY output but not part of typed input (the echo buffer records what
was typed). If ANSI stripping ran first, escape bytes would be missing
from the echo comparison and cancellation would break.

### Before dropcrnl()

ANSI processing must strip escape sequences before line-ending
normalization. Escape sequences themselves don't contain `\r\n` patterns,
but their presence could interfere with `dropcr()`'s line-start detection
if left in.

### Subsumes label()

OSC sequences (including window title) are handled by the ANSI parser's
state machine. The parser naturally strips the entire sequence from the
output stream and dispatches the title change as a side effect. The
existing `label()` function's backward-scanning approach is more fragile
and only handles the last occurrence.

---

## dropcr() Interaction with Styled Runs

Programs that use `\r` for progress bars (curl, wget) will have styled
runs that reference pre-dropcr text positions. After `dropcr()` modifies
the text, run boundaries may not align perfectly.

**Current approach**: Accept the misalignment. The primary use case
(colored `ls`, `grep`, `git diff`) does not use `\r` carriage returns.
Progress bar lines are overwritten rapidly and visual glitches are
transient.

**Future option**: Make `dropcr()` return a position mapping so spans
can be adjusted. Deferred until users report issues.

---

## New Import Requirements

`cmd/rwin/rwin.go` needs these new imports:

```go
import (
    "9fans.net/go/plan9"
    "9fans.net/go/plan9/client"
)
```

The existing imports (`fmt`, `io`, `os`, `os/exec`, `strings`, `sync`,
`unicode/utf8`, `9fans.net/go/acme`, `github.com/creack/pty`) are
unchanged.

---

## Summary of Changes to rwin.go

| Location | Change |
|----------|--------|
| Imports | Add `"9fans.net/go/plan9"`, `"9fans.net/go/plan9/client"` |
| `winWin` struct | Add `parser *ansiParser`, `spansFid *client.Fid` |
| `main()` | Mount acme 9P, open spans fid, create parser with title callback |
| `main()` | Add deferred `spansFid.Close()` |
| `startProcess()` | Change `TERM=dumb` to `TERM=xterm-256color`, add `COLORTERM=truecolor` |
| `stdoutproc()` | Insert `w.parser.Process(input)` after `echo.Cancel()` |
| `stdoutproc()` | Remove `label()` call |
| `stdoutproc()` | Add span write (`buildSpanWrite` + `spansFid.Write`) inside `Q.Lock()` |
| `stdoutproc()` | Change pipeline variable from `input` to `clean` after Process() |
| `label()` | Delete entire method |
| New method | Add `setWindowTitle(title string)` |

---

## Integration Checklist

Verified during the Iterate stage:

- [ ] `TERM=xterm-256color` and `COLORTERM=truecolor` set in `startProcess()`
- [ ] `ansiParser` created in `main()` with `setWindowTitle` callback
- [ ] `spans` fid opened after window creation via `client.MountService`
- [ ] `w.parser.Process(input)` called after `echo.Cancel()` in `stdoutproc()`
- [ ] `label()` call removed from pipeline
- [ ] Clean text passed through `dropcrnl()` -> `dropcr()` -> `squashnulls()`
- [ ] `buildSpanWrite(w.p, runs)` called after body write, before `w.p` increment
- [ ] Span data written inside `Q.Lock()` section
- [ ] `spansFid.Close()` on window cleanup/exit
