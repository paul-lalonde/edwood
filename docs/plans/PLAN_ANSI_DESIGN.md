# ANSI Escape Sequence Processing for rwin

Parse ANSI escape sequences from PTY output in `rwin`, strip them from body
text, track SGR styling state, and write span definitions to the edwood spans
file so shell output renders with proper colors and styles.

**Base design doc**: `cmd/rwin/ANSI_DESIGN.md`

## Key Design Decisions

1. **Parser placement**: After `echo.Cancel()`, before `dropcrnl()`. Echo
   cancellation needs raw runes; line-ending normalization needs escape-free
   text.

2. **Parser persistence**: The `ansiParser` struct lives on `winWin` and
   survives across PTY reads. Split escape sequences are handled by resuming
   state on the next read.

3. **Span file access**: Use `9fans.net/go/plan9/client.MountService("acme")`
   and `fsys.Open(fmt.Sprintf("%d/spans", id), plan9.OWRITE)` to open the
   spans file — same pattern as `cmd/edcolor`. Keep the fid open for the
   window lifetime.

4. **Prefixed span format**: Use `s offset length fg [bg] [flags...]` (the
   prefixed message format from the spans protocol).

5. **Default optimization**: Skip span writes entirely when all styled runs
   use default attributes (common case for non-ANSI programs).

6. **label() removal**: OSC title handling moves into the parser's
   `dispatchOSC()`, eliminating the fragile backward-scanning `label()`.

---

## Phase 1: Color Foundation

Establishes the type system and color palette that all later phases depend on.
Pure data types and pure functions — no I/O, no state machines, no external
dependencies. This phase is risk-free and provides a solid base for testing
everything else.

### 1.1 Color Types, Palette & Utilities

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Distill color types, palette init, and utility functions from base doc sections 2, 3, 6 | `cmd/rwin/ANSI_DESIGN.md` | Output: `docs/designs/features/ansi-color-palette.md` |
| [x] Tests | Write tests for palette values, colorToHex, applyDim, resolveColors, isDefaultStyle | `docs/designs/features/ansi-color-palette.md` | Test file: `cmd/rwin/ansi_color_test.go`. Cover: 16 standard colors, 216-cube spot checks, 24 grayscale spot checks, truecolor hex formatting, dim halving, inverse swap, default detection |
| [x] Iterate | Red/green/review until all color tests pass | `docs/designs/features/ansi-color-palette.md` | Impl file: `cmd/rwin/ansi_color.go`. Palette init is O(1) (fixed 256 entries). All utilities are O(1). |
| [x] Commit | Commit color palette and utilities | — | Message: `Add ANSI color palette, types, and utility functions for rwin` |

**What this establishes**: `ansiColor`, `sgrState`, `styledRun` types;
`ansiPalette[256]` lookup table; `colorToHex()`, `applyDim()`,
`resolveColors()`, `isDefaultStyle()` functions. All later phases import
these types.

---

## Phase 2: ANSI Parser

Builds the state machine that strips escape sequences from PTY output and
tracks cumulative SGR styling. This is the core complexity of the project.
Split into two sub-features: the parser+SGR (which are tightly coupled since
SGR dispatch is triggered by CSI 'm'), then OSC handling (separate states,
separate dispatch, can be added incrementally).

### 2.1 Parser State Machine + SGR Dispatch

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Distill parser states, transitions, SGR dispatch, extended colors, split-sequence handling from base doc sections 1, 2 | `cmd/rwin/ANSI_DESIGN.md` | Output: `docs/designs/features/ansi-parser.md`. Must include: state enum, transition table, ansiParser struct, Process() signature, dispatchSGR() logic, all SGR codes, extended color sub-parameter parsing |
| [x] Tests | Write comprehensive parser and SGR tests | `docs/designs/features/ansi-parser.md` | Test file: `cmd/rwin/ansi_test.go`. Categories: (1) plain text passthrough, (2) escape stripping (CSI, unknown Esc), (3) state transitions, (4) SGR codes 0-9/21-29, (5) standard colors 30-37/40-47/90-97/100-107, (6) extended colors 38;5;N and 38;2;R;G;B, (7) multiple params in one sequence, (8) split sequences across reads, (9) C0 controls passthrough, (10) styled runs output with correct styles, (11) private marker sequences ignored |
| [x] Iterate | Red/green/review until all parser tests pass | `docs/designs/features/ansi-parser.md` | Impl file: `cmd/rwin/ansi.go`. Process() is O(n) single pass. dispatchSGR() is O(k) where k = number of params. No allocations in hot path except appending to runs slice. |
| [x] Commit | Commit ANSI parser and SGR dispatch | — | Message: `Add ANSI parser state machine with SGR color/attribute dispatch` |

**Risk note**: Split-sequence handling is the trickiest part. A CSI sequence
like `ESC[1;31m` could be split as `ESC[1;3` / `1m` across two PTY reads.
The parser must resume from saved state. Test this thoroughly.

### 2.2 OSC Handling

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Distill OSC parsing (states, dispatch, title extraction) from base doc section 7 | `cmd/rwin/ANSI_DESIGN.md` | Output: `docs/designs/features/ansi-osc.md`. Must include: OSC state transitions, BEL and ST terminators, oscNum/oscBuf accumulation, titleFunc callback interface, setWindowTitle() logic, list of ignored OSC numbers |
| [x] Tests | Write OSC parsing and title extraction tests | `docs/designs/features/ansi-osc.md` | Add to `cmd/rwin/ansi_test.go`. Categories: (1) OSC 0/1/2 title with BEL terminator, (2) OSC with ST terminator (ESC \\), (3) title callback invoked with payload, (4) unknown OSC numbers stripped silently, (5) OSC split across reads, (6) OSC interleaved with styled text, (7) empty OSC payload |
| [x] Iterate | Red/green/review until all OSC tests pass | `docs/designs/features/ansi-osc.md` | Modify `cmd/rwin/ansi.go` to add OSC states and dispatch. O(n) — no complexity change. |
| [x] Commit | Commit OSC handling | — | Message: `Add OSC sequence handling to ANSI parser for window title support` |

---

## Phase 3: Span Writing

Converts the parser's styled runs into the spans protocol text format. Pure
function with no I/O — takes styled runs and a base offset, returns a string.
Depends on color types from Phase 1 and styledRun from Phase 2.

### 3.1 Span Generation

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Distill span generation logic from base doc section 5 | `cmd/rwin/ANSI_DESIGN.md` | Output: `docs/designs/features/ansi-span-writing.md`. Must include: buildSpanWrite() signature, span line format, offset arithmetic (baseOffset + cumulative run lengths), flag emission (bold/italic/hidden), default optimization, color resolution integration, contiguity guarantee |
| [x] Tests | Write span generation tests | `docs/designs/features/ansi-span-writing.md` | Test file: `cmd/rwin/ansi_spans_test.go`. Categories: (1) single colored run, (2) multiple contiguous runs, (3) default-style-only returns empty string, (4) mixed default and styled runs, (5) bold/italic/hidden flags, (6) foreground only vs fg+bg, (7) inverse color swap, (8) dim brightness reduction, (9) offset arithmetic correctness, (10) all-default except one run still emits full block |
| [x] Iterate | Red/green/review until all span tests pass | `docs/designs/features/ansi-span-writing.md` | Impl file: `cmd/rwin/ansi_spans.go`. O(n) where n = number of runs. String building via strings.Builder. |
| [x] Commit | Commit span generation | — | Message: `Add span protocol message generation from ANSI styled runs` |

---

## Phase 4: Integration

Wires everything together: parser into the `stdoutproc()` pipeline, spans
file access, TERM variable change, and `label()` removal. This phase modifies
existing code (`rwin.go`) and requires careful attention to ordering and
synchronization.

### 4.1 Pipeline Wiring, TERM Change & Spans File Access

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | Distill integration points from base doc sections 4, 5, 8, 10 | `cmd/rwin/ANSI_DESIGN.md` | Output: `docs/designs/features/ansi-integration.md`. Must include: exact pipeline order, winWin struct additions (parser, spansFid), NewWinWin/main changes, startProcess TERM/COLORTERM changes, stdoutproc modification (full code), label() removal, spans file open/write via plan9 client, Q.Lock synchronization, w.p offset usage, setWindowTitle method |
| [x] Tests | Write integration-level tests where feasible | `docs/designs/features/ansi-integration.md` | Add to `cmd/rwin/ansi_test.go`. Test: (1) end-to-end Process→buildSpanWrite pipeline with sample ANSI input produces correct clean text and span output, (2) setWindowTitle formats name correctly (with and without `-`), (3) pipeline ordering verification (ANSI before dropcrnl/dropcr). Note: full PTY integration requires manual verification (see below) |
| [x] Iterate | Red/green/review, then manual verification | `docs/designs/features/ansi-integration.md` | Modify `cmd/rwin/rwin.go`. Manual verify: `ls --color=auto`, `git diff`, `grep --color`, `printf '\e[1;31mred bold\e[0m'`. Check that escapes are stripped and colors render. |
| [x] Commit | Commit pipeline integration | — | Message: `Wire ANSI parser into rwin pipeline, enable xterm-256color, write spans` |

**Integration checklist** (verified during Iterate stage):
- [ ] `TERM=xterm-256color` and `COLORTERM=truecolor` set in `startProcess()`
- [ ] `ansiParser` created in `NewWinWin()` with `setWindowTitle` callback
- [ ] `spans` fid opened after window creation via `client.MountService`
- [ ] `w.parser.Process(input)` called after `echo.Cancel()` in `stdoutproc()`
- [ ] `label()` call removed from pipeline
- [ ] Clean text passed through `dropcrnl()` → `dropcr()` → `squashnulls()`
- [ ] `buildSpanWrite(w.p, runs)` called after body write, before `w.p` increment
- [ ] Span data written inside `Q.Lock()` section
- [ ] `spansFid.Close()` on window cleanup/exit

---

## Open Questions

1. **9P message size limits**: edcolor chunks span writes to stay under 4000
   bytes. A PTY read of 8KB with frequent style changes could produce span
   data exceeding this limit. Should `buildSpanWrite()` include chunking
   logic, or is the typical PTY read small enough that this is a future
   concern?

   *Recommendation*: Start without chunking. PTY reads for line-oriented
   output (the primary use case) rarely exceed a few hundred bytes per read.
   Add chunking if we observe `EMSGSIZE` errors in practice.

2. **Spans fid lifetime**: The design doc suggests keeping the file open for
   the window lifetime. If the edwood window is closed and re-opened (or the
   9P connection drops), the stale fid will cause write errors. Should we add
   reconnection logic, or is it safe to assume the fid lives as long as the
   window?

   *Recommendation*: Match the fid lifetime to the window. If writes fail,
   log and skip (degraded to uncolored output). This is simple and robust.

3. **Bold rendering**: The spans protocol supports `bold` as a flag, but does
   edwood's `rich.Frame` rendering actually render bold text differently?
   If not, emitting `bold` is harmless but visually no-op.

   *Recommendation*: Emit the flag regardless. If rendering support is added
   later, it will just work.

4. **dropcr() interaction with styled runs**: `dropcr()` can delete or
   reposition runes (e.g., `\r` causes line restart). The styled runs from
   the parser correspond to the pre-dropcr text positions. After dropcr
   modifies the text, the run boundaries may not align. Should styled runs
   be adjusted after dropcr, or is the mismatch acceptable?

   *Recommendation*: This is a real issue. Programs that use `\r` for
   progress bars (like `curl`, `wget`) will have misaligned spans. Two
   options:
   - (a) Accept misalignment — progress bar lines are overwritten rapidly and
     the visual glitch is transient.
   - (b) Make `dropcr()` return a position mapping so spans can be adjusted.

   Start with (a). The primary use case (colored `ls`, `grep`, `git diff`)
   does not use `\r` carriage returns. Revisit if users report issues.
