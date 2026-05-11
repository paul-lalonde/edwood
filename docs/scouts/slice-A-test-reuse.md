# Slice A — test-reuse scout

Read-only survey of `/Users/paul/dev/edwood` (the prior styled-text
working copy) for tests we can borrow into the clean-room rewrite.
Scoped to Slice A (coloring + `edcolor`). Slice B and C scouts are
deferred until those slices are imminent.

Filter applied per design §13.1:
1. Test exercises **observable behavior** of public methods given
   inputs — not internal data layout.
2. Does not assume `rich.Frame`, `RichText`, `richBody`,
   `HandleStyledMouse`, `UpdateStyledView`, `IsStyledMode`,
   `IsPreviewMode`, `SetContent`, or any parallel-display
   construct.
3. Does not assume specific internal data structures (e.g.,
   `LinePixelHeights`, `LineStartRunes`, `TotalDocumentHeight`
   as public observables).

Classification: **REUSE** | **REUSE WITH EDITS** | **REJECT**.

Source state surveyed: `/Users/paul/dev/edwood`, branch
`simplification`, HEAD `a6b8846`, working tree had 43 untracked
files but no uncommitted code changes at scout time
(2026-05-11).

---

## Per-phase buckets

### Phase A1 — types (StyleRun, Style{Fg,Bg}, IsZero())

Prior repo has no `Style.IsZero()` (uses `Style.Equal(other)`
instead). The structural surface most directly applicable is
`spans/store_test.go` — but those tests largely belong in A3, not
A1. **Phase A1 tests should be written de novo**; the prior repo
doesn't have an analogous types-only contract.

For reference, prior color-equality tests (these would be redundant
under Go's struct equality but illustrate the surface):

- `spans/store_test.go:536`  TestStyleAttrs_BothNilColors           REUSE
- `spans/store_test.go:545`  TestStyleAttrs_OneNilOneNonNil         REUSE
- `spans/store_test.go:560`  TestStyleAttrs_SameRGBA                REUSE
- `spans/store_test.go:569`  TestStyleAttrs_DifferentRGBA           REUSE

### Phase A2 — frame styled methods (color-only)

Prior repo does not have public `frame.InsertWithStyle` or
`frame.SetStyleRange`. Its rendering bridge is `buildStyledContent`
(window-level, not frame-level), tested in `wind_styled_test.go`.
**Phase A2 tests should be written de novo** once the frame API
shape exists.

Reference (window-level, not directly applicable but instructive
for observable Fg/Bg assertions):

- `wind_styled_test.go:41`   TestBuildStyledContent_SingleRun       REUSE WITH EDITS
- `wind_styled_test.go:72`   TestBuildStyledContent_MultipleRuns    REUSE WITH EDITS
- `wind_styled_test.go:118`  TestBuildStyledContent_EmptySpanStore  REUSE WITH EDITS
- `wind_styled_test.go:149`  TestBuildStyledContent_MixedStyles     REUSE WITH EDITS
- `wind_styled_test.go:176`  TestBuildStyledContent_Unicode         REUSE WITH EDITS

All five touch `buildStyledContent`'s output (a list of styled
runs) rather than internal layout. To port: adapt to whatever
output `Frame.InsertWithStyle` exposes for assertions
(`GetStyleAt(p) Style`, or a debug-only dump method, etc.).

### Phase A3 — spans.Store

Strong reuse here. The prior `spans/store_test.go` covers exactly
the §6.2 / §6.3 surface.

- `spans/store_test.go:77`   TestStore_EmptyStore                   REUSE
- `spans/store_test.go:107`  TestStore_InsertIntoEmpty              REUSE
- `spans/store_test.go:115`  TestStore_InsertAtStart                REUSE
- `spans/store_test.go:123`  TestStore_InsertAtEnd                  REUSE
- `spans/store_test.go:131`  TestStore_InsertMidRun                 REUSE
- `spans/store_test.go:139`  TestStore_InsertAtRunBoundary          REUSE
- `spans/store_test.go:168`  TestStore_DeleteWithinOneRun           REUSE
- `spans/store_test.go:176`  TestStore_DeleteEntireSingleRun        REUSE
- `spans/store_test.go:184`  TestStore_DeleteFromStart              REUSE
- `spans/store_test.go:200`  TestStore_DeleteExactRun               REUSE
- `spans/store_test.go:220`  TestStore_DeleteMiddleMerge            REUSE
- `spans/store_test.go:270`  TestStore_ReplaceEntireStore           REUSE
- `spans/store_test.go:278`  TestStore_ReplaceAtStart               REUSE
- `spans/store_test.go:286`  TestStore_ReplaceAtEnd                 REUSE
- `spans/store_test.go:294`  TestStore_ReplaceMiddle                REUSE
- `spans/store_test.go:306`  TestStore_ReplaceSpanningRuns          REUSE
- `spans/store_test.go:318`  TestStore_ReplaceWithMergeLeft         REUSE
- `spans/store_test.go:327`  TestStore_ReplaceWithMergeRight        REUSE
- `spans/store_test.go:381`  TestStore_ForEachRunSingle             REUSE
- `spans/store_test.go:394`  TestStore_ForEachRunMultiple           REUSE
- `spans/store_test.go:418`  TestStore_ForEachRunAfterGapMove       REUSE
- `spans/store_test.go:450`  TestStore_ClearNonEmpty                REUSE
- `spans/store_test.go:458`  TestStore_ClearThenInsert              REUSE
- `spans/store_test.go:486`  TestStore_TotalLenAfterInsertSequence  REUSE

Note: prior API names (`RegionUpdate`, `Runs()`, `ForEachRun`,
`TotalLen`, `NumRuns`) differ from our design (`SetRegion`,
`GetStyleRuns`, `Snapshot`). Port by retargeting calls, not by
copying.

### Phase A4 — Text wiring (Inserted, fill, setorigin)

Sparse. Most prior Text-styled tests sit in `wind_styled_test.go`
and depend on `IsStyledMode` / `initStyledMode` / `richBody`
parallel-display constructs.

- `wind_styled_test.go:446`  TestStyledShowSendsSelectionEvent      REUSE WITH EDITS

This one is the 'S' event but also exercises the styled `Show()`
path; adapt by swapping `IsStyledMode` for `t.spans != nil`.

### Phase A5 — 9P spans file (directive parser, color-only)

Strong reuse. Prior `spans/parse_test.go` covers the directive
shape we want (`s` / `c` with `fg=` / `bg=`).

- `spans/parse_test.go:13`   TestParseColor                         REUSE
- `spans/parse_test.go:115`  TestParseSpanDefs_SingleSpan           REUSE
- `spans/parse_test.go:143`  TestParseSpanDefs_MultiSpanContiguous  REUSE
- `spans/parse_test.go:174`  TestParseSpanDefs_OptionalBgColor      REUSE
- `spans/parse_test.go:193`  TestParseSpanDefs_DashBgColor          REUSE
- `spans/parse_test.go:606`  TestParseSpanMessage_Clear             REUSE
- `spans/parse_test.go:616`  TestParseSpanMessage_SingleSpan        REUSE
- `spans/parse_test.go:642`  TestParseSpanMessage_MultiSpanContiguous REUSE

### Phase A6 — 'S' event

- `wind_styled_test.go:446`  TestStyledShowSendsSelectionEvent      REUSE WITH EDITS

Same test as A4 row. Port once.

### Phase A7 — `edcolor`

- `cmd/edcolor/go_test.go:56`     TestColorizeGo        REUSE
- `cmd/edcolor/python_test.go:142`  TestColorizePython  REUSE
- `cmd/edcolor/rust_test.go:234`  TestColorizeRust      REUSE
- `cmd/edcolor/latex_test.go:197`  TestColorizeLatex    REUSE

These are colorizer-output golden tests — observable directive
output for a given input file. Implementation-independent.

---

## Phase A1 — recommended new tests (Stage 2 input)

The Stage-2 test list for A1, written *de novo*:

1. **`TestStyleIsZero_ZeroValue`** — `Style{}.IsZero()` is true.
2. **`TestStyleIsZero_FgSet`** — `Style{Fg: anyImage}.IsZero()`
   is false.
3. **`TestStyleIsZero_BgSet`** — `Style{Bg: anyImage}.IsZero()`
   is false.
4. **`TestStyleIsZero_BothSet`** —
   `Style{Fg: a, Bg: b}.IsZero()` is false.
5. **`TestReplacedKind_ZeroIsNone`** — `ReplacedKind(0)` equals
   `ReplacedNone`. (Defensive: ensures zero-value `Style` plays
   well with the enum when the Replaced fields are added in
   Slice C.)
6. **`TestStyleRun_ZeroLenIsLegalConstructionOnly`** —
   `StyleRun{Len: 0, Style: Style{}}` is a valid Go value. (We
   *don't* assert anything about how `Frame` or `Spans` treat
   zero-Len runs — that's their concern in later phases.)

Intentionally **not** in A1 (deferred to later phases):

- Color equality semantics — handled by Go struct/pointer
  equality of `draw.Image`. No test needed at the types layer.
- `StyleRun` slice composition / Lens-sum invariants — that's
  `InsertWithStyle` contract (A2) and `GetStyleRuns` contract
  (A3), tested there.
- `ReplacedKind` non-zero enum values — Slice C concern.

---

## Notes / surprises

- Prior repo's `simplification` branch is the canonical
  styled-text integration. ~80 tests in `richtext_test.go` are
  **all REJECT** under §13.1 (parallel `RichText`/`richBody`,
  `SetContent`, image cache). Skip.
- Prior `wind_styled_test.go` has ~65 tests; ~15 are A4/A6
  candidates after stripping the `IsStyledMode` coupling. Most
  others are B/C territory or reject outright.
- Prior `spans/store_test.go` is the highest-yield file for
  Slice A reuse: ~24 directly applicable to A3 (filter out the
  Box/Scale/Family/HRule tests at lines 619–725 which are B/C).
- Prior `spans/parse_test.go` is similarly clean for A5 once
  we strip directives we don't support yet (`b` for boxes,
  font keys).
- `frame/` package in prior repo has **no** styled-frame tests
  worth porting — color-only frame behavior was driven through
  the window bridge, not unit-tested at the frame layer. We
  write Phase A2 frame tests fresh.
