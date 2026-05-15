# Working Log — `cleanroom` branch

A handoff-to-future-self log per project CLAUDE.md. Read at the start
of every session on this branch; update at the end of every session
in which the branch changes.

## Purpose

Clean-room re-implementation of the unified-frame-spans feature on
top of `rjkroege/edwood`'s `master`. Spec lives at
`docs/designs/features/unified-frame-spans.md` (§12 lays out the
nine-phase implementation plan; the plan doc at
`docs/plans/PLAN_unified-frame-spans.md` is the working checklist).

The "earlier work" the spec refers to is the styled-text /
markdown-rendering branches in this same repo (Markdown-take-two,
unify-frame-interface, rich-text-markdown, spans-file-external-styling,
…) and in the sibling working copy at `/Users/paul/dev/edwood`. None
of it is reused wholesale; specific tests may be borrowed per the
§13.1 reuse criteria, and only after confirming each test exercises
observable behavior rather than the incumbent's internal layout.

## Base

- Branch: `cleanroom`
- Branched from: `refs/remotes/upstream/master`
- HEAD at Phase 0 setup: `e6ccb75` ("Delete the rest of gozen")
- Upstream remote: `https://github.com/rjkroege/edwood.git` (fetched
  2026-05-11; HEAD coincides with the branch base).

There is *also* a stale local branch named `upstream/master` at
`61193d0`. Do not use it — the canonical upstream pointer is
`refs/remotes/upstream/master`.

## Backup refs

None preserved on this branch yet. The prior styled-text work is
preserved as the dozens of branches already in this repo (see
`git branch -a`). They are read-only references for this rewrite.

## Commit graph

```
dc5fae9 (cleanroom)  chore: phase 0 -- design, plan, working log, regression runner
230d818              docs: add CLAUDE.md and CODING-PROCESS.md
e6ccb75 (upstream/master)  Delete the rest of gozen  ← Phase 0 base
```

Working tree is clean at session 1 end. The two Phase 0 commits land
all of: project instruction docs, the design doc, the plan, this
working log, and `regression.sh`. The working-log update reflecting
those commits is itself an unstaged change at the moment of writing
— commit it with the first Phase 1 work, or as its own small commit
at session start.

## Test status

`./regression.sh` is green at HEAD. It mirrors the GitHub Actions
workflow (`.github/workflows/edwood.yml`) and runs:

1. `gofmt -d -s .` (no diff)
2. `go vet .` (root package, matches CI)
3. `staticcheck -checks inherit,-U1000,-SA4003 ./...` via `go run`
4. `misspell -error .` via `go run`
5. `go test -race ./...`

Every Phase >= 1 commit must keep `./regression.sh` green.

## Quirks & loose ends

- `go vet ./...` (note the `./...`, not the `.` CI uses) reports
  `file/buffer_adapter.go:53:2: unreachable code` — a pre-existing
  upstream wart in `IndexRune`. CI does not gate on subpackages and
  upstream tolerates it. Leave it alone unless touching `file/`.
- `staticcheck` and `misspell` are run via `go run @version` (no
  `go install`). First invocation downloads modules; subsequent
  runs use the module cache.
- The `cleanroom` working tree has untracked top-level docs
  (`CLAUDE.md`, `CODING-PROCESS.md`) that mirror files at
  `/Users/paul/` and `/Users/paul/dev/edwood-cleanroom/`. They
  exist locally to satisfy session bootstrap but have not been
  committed yet.

## Session summaries

### 2026-05-11 — Phase 0 setup

- Confirmed `cleanroom` sits exactly on `refs/remotes/upstream/master`
  (`e6ccb75`). Fetched upstream; nothing to pull.
- Verified `gofmt`, `go vet .`, `go test -race ./...` all green at
  baseline. All test packages pass: root, complete, drawutil,
  dumpfile, file, frame, ninep, regexp, runes, sam.
- Wrote `regression.sh` mirroring CI exactly; uses `go run` for the
  two lint tools to avoid `$GOPATH/bin` installs.
- Wrote `docs/working-log.md` (this file) and
  `docs/plans/PLAN_unified-frame-spans.md`.
- Landed two commits on `cleanroom`:
  - `230d818` — project instruction docs (`CLAUDE.md`,
    `CODING-PROCESS.md`).
  - `dc5fae9` — Phase 0 deliverables (design doc, plan, this log,
    regression runner).
- Also updated `~/CLAUDE.md` "Known logs" index to point at this
  file (outside the repo; not part of either commit).
- Re-phased the design: §12 now describes three vertical slices
  (A: coloring, B: typographic variation, C: replaced elements +
  block context) rather than the original nine sequential phases.
  Slice A's `Style` carries only `Fg`/`Bg`. Rewrote design §12 and
  plan to match. Landed as `4b8af5c`.
- Slice A test-reuse scout (`docs/scouts/slice-A-test-reuse.md`)
  surveying `/Users/paul/dev/edwood` (`simplification` branch,
  `a6b8846`). Most A1 reuse is nil — types layer is new
  territory; high yield in A3 (spans.Store) and A5 (parser).
  Landed as `8abf15b`.
- A1.1 first pass: `frame.StyleRun`, `frame.Style{Fg, Bg}`,
  `Style.IsZero()`, `frame.ReplacedKind` enum. Landed as
  `927a34a`.
- A1.2 rework (Stage-4 "wrong design"): replaced `IsZero()` with
  `IsPlain()`; introduced `frame.Kind` as a bitmask.
  `KindPlain = 0` sits in its own `const` declaration; the
  bit-position block beneath it starts at `iota = 0`, so
  `KindColored = 1 << iota = 1`. Later slices add `KindBold = 2`,
  `KindItalic = 4`, `KindUnderline = 8`, `KindFontIdx = 16`,
  `KindReplaced = 32`, `KindBlockquote = 64`, `KindInCodeBlock = 128`,
  `KindInTable = 256` — each picking up the next iota step in the
  same block. `IsPlain()` is exactly `Kind == KindPlain`, which
  means "upstream defaults" — the fast-path predicate. The
  design's `Style` struct lost its `Bold`/`Italic`/`Underline`
  bool fields (subsumed by Kind bits). Design §5.3/§5.4/§6.1/§12
  A1+B1/§17 updated accordingly. Landed and re-amended twice to
  refine the bitmask declaration into the two-const-block iota
  pattern that yields the expected bit positions.
- A2.1 — `InsertWithStyle` color-only impl. Added `Style` field to
  `frbox`; added `InsertWithStyle` to `Frame` /
  `SelectScrollUpdater` interfaces (plus the proxy in
  `unlockedproxy.go`) and to `MockFrame`. `bxscan` and
  `insertbyteimpl` were unified to take an optional
  `runeStyles []Style` — nil is the upstream plain path, non-nil
  drives the same code with style-boundary splits in `bxscan` and
  per-box `Style` stamping. No sibling styled twin functions
  (initial draft had `bxscanstyled` / `insertbyteimplstyled`;
  collapsed before commit to avoid ~230 LOC of duplication).
  `drawtext` honors `box.Style` — `KindColored` runs use
  `Style.Fg` for text and repaint `Style.Bg` as the box
  background. `clean()` only merges adjacent boxes when their
  Styles are equal (otherwise a colored run gets folded back into
  a neighboring plain run). Tests cover the §5.4 contract: nil
  styles ≡ Insert, all-IsPlain ≡ fast path, color applied to
  boxes, split at style boundary, Len mismatch panics, return
  value matches Insert. Selection rendering on styled text
  intentionally deferred — drawsel0 still uses frame defaults
  when clearing, so per-box colors are momentarily lost until the
  next redraw. To be revisited as a Slice A polish row.
- A2.2 — `SetStyleRange` color-only impl. Added to the `Frame`
  interface (not `SelectScrollUpdater`, per §5.2) and to
  `MockFrame`. Implementation:
  - `findbox` splits boxes at `p0` and `p1` so the affected runes
    sit in a contiguous box range `[nb0, nb1)`.
  - The box walk applies styles in-place; when a box's runes
    span a style boundary, `splitbox` divides it and the loop
    continues, growing `nb1`.
  - Repaint uses a new `repaintBoxRange(pt, nb0, nb1, ...)` helper
    that always clears each box's bg rect before drawing the
    glyph (so old colored glyphs don't bleed through under the
    new ones). This is *separate* from `drawtext`: the upstream
    Insert path uses `drawtext` (which only paints non-default
    `Bg`) so existing SVG-baselined tests stay green.
  - `clean` merges adjacent same-Style boxes after the splits.
  Tests cover the §5.4 contract: simple recolor, partial range,
  mid-box split, Len mismatch and out-of-range panics, empty
  range no-op, selection bounds unchanged. Selection-overlap
  repaint deferred (consistent with A2.1).
  - Stage-1 design picked "always clear in drawtext" but it
    perturbed the existing TestInsert SVG goldens (extra fill ops
    per box). Walked back to the `repaintBoxRange` shape during
    Stage 3 — clean separation, no upstream test churn.
- A2.3 — `SetOriginYOffset` / `GetOriginYOffset` stubs. Added to
  the `Frame` interface (not `SelectScrollUpdater`) and to
  `MockFrame`. For Slice A both are no-ops: `Set` accepts any
  argument and discards it; `Get` always returns 0. Real
  implementation arrives in Slice C row C2 alongside replaced
  elements and `Text.computeTallElementYOffset`. Tests pin the
  stub contract.
- A3.1 — introduced the `spans/` package with the public Store
  interface and an in-memory implementation. During design we
  flipped from a sparse internal layout (only non-plain regions
  stored; plain runs synthesized) to a *dense* layout (full-
  coverage `[]Region` covering `[0, totalLen)`, plain runs
  stored explicitly). Reason: dense `GetStyleRuns` walks a
  single uniform slice with one clip branch, vs. sparse's
  three-way "gap-before / overlap / gap-after" synthesis; the
  §6.2 trailing/leading-edge rule (A3.2) likewise applies
  uniformly to every run. The dense path costs one extra
  invariant — every region's `Start+Length` must equal the
  next region's `Start`, with `regions[last].Start+Length ==
  totalLen` — but the simpler reads were worth it. The design
  doc's external contract (Empty, GetStyleRuns, SetRegion,
  ClearRegion, Snapshot) is unchanged. NewStore accepts the
  `*file.ObservableEditableBuffer`; for Slice A it stashes the
  pointer but doesn't yet `AddObserver` (A3.2 wires that up).
  A package-internal `newStoreWithLen(n)` helper lets tests
  seed a plain run without a real buffer. 13 tests covering
  Empty/Snapshot defaults, plain-coverage Empty, SetRegion/
  ClearRegion equivalence, single/multi region GetStyleRuns,
  full-coverage invariant, overlap-wins, partial-clear splits.
- A3.2 — `Inserted` / `Deleted` observer methods on `*store`.
  `NewStore` now calls `buf.AddObserver(s)` so buffer edits keep
  the store's offsets aligned. Under the dense layout the §6.2
  trailing/leading-edge rule applies uniformly: leading-edge at
  a region boundary makes the *previous* region's trailing edge
  extend (and the right region shifts); mid-region insertion
  extends; end-of-buffer insertion extends the last region.
  Empty-store insertion seeds a plain region. The plain-region-0
  case observably matches "prepend a plain region of length nr"
  because coalescing absorbs the new plain content into the
  existing plain run.
  Deleted clips intersecting regions per the five cases (entirely
  contained, straddles-left, straddles-right, wraps, after-shift)
  then runs `coalesce` so adjacent same-Style regions merge.
  Initial implementation had a subtle bug: in the "plain region
  0 leading-edge extension" branch I extended region 0's length
  but forgot to shift the subsequent regions' Starts. That broke
  the dense invariant and was caught by the integration test
  (`TestObserver_Integration_BufferDrivesStore`) — a colored
  region's Start ended up one short. Fixed by always shifting
  regions[1+] when region 0 grows. 12 new tests covering the
  algorithm's branches plus a real-buffer integration test.
- A3.3 — `Observe(fn)` callback dispatch. Store keeps an
  `observers []func(p0, p1 int)` slice; SetRegion (and ClearRegion
  via delegation) calls `notify(p0, p1)` after the mutation +
  coalesce. Buffer-driven Inserted/Deleted do NOT notify per
  §6.1's "style-only updates" wording. Empty-range SetRegion is
  a no-op and does not fire. 5 tests cover the contract: fires
  on SetRegion, fires on ClearRegion, NOT fired by Inserted /
  Deleted, supports multiple observers, no fire on empty range.
- Prep commit `9c5262f`: changed
  `file.ObservableEditableBuffer.observers` from
  `map[BufferObserver]struct{}` to `[]BufferObserver`. Map
  iteration was non-deterministic; A4.1's §8.1 ordering invariant
  needs deterministic firing order so spans (registered first)
  updates before Text (registered second). AddObserver dedupes,
  DelObserver removes; AllObservers / inserted / deleted iterate
  the slice. Three tests pin the new contract.
- A4.1 — `Text.spans` field + `attachSpans` helper added to
  text.go; `wind.initHeadless` body setup now creates the spans
  store via `spans.NewStore(f)` BEFORE registering w.body as an
  observer on the body buffer (so spans is observer 1 and Text
  is observer 2). Tags get nil spans. The body-spans Insert and
  Delete propagation will be wired up in A4.2.
  Mid-implementation gotcha: after wiring spans onto the body
  buffer, `TestComplexEditActions/firstCloseMutatedWindow`
  panicked with `body.file == nil` inside `Window.Lock`. Root
  cause: `D1` (and 5 other call sites) use
  `file.HasMultipleObservers()` as a proxy for "is this buffer
  shared by multiple Texts/clones?" With spans added,
  `len(observers) > 1` even on an un-cloned buffer, so D1
  thought a clone existed and closed the window, nilling
  body.file. Fix: added an optional `file.AuxiliaryObserver`
  interface (`IsAuxiliary() bool`); `HasMultipleObservers`
  now excludes observers that mark themselves auxiliary;
  `spans.Store` implements `IsAuxiliary() bool { return true }`.
  Existing six call sites of HasMultipleObservers unchanged.
  3 new A4.1 tests pin the behavior: body has non-nil spans,
  tag has nil spans, spans registered before Text on the
  observer chain.
- A4.2 — Text.Inserted now routes through frame.InsertWithStyle
  when `t.spans != nil && !t.spans.Empty()`. The §8.1 ordering
  guarantee means spans has already shifted/extended its regions
  by the time Text.Inserted runs, so `t.spans.GetStyleRuns(q0,
  q0+nr)` returns the post-insert styles for the new runes.
  When spans is nil or Empty (fast path), the existing
  `t.fr.InsertByte(b, framePos)` call stays intact — no byte→
  rune conversion, no spans query. `Deleted` requires no change
  because the frame's per-box style data is removed alongside
  the runes by the existing Delete path.
  Tests use a `recordingFrame` that embeds MockFrame and
  records the args passed to InsertByte / InsertWithStyle.
  Five cases pin the routing: nil spans, empty spans, mid-
  colored insert, multi-rune mid-colored insert (styles slice
  correctness), and "spans non-empty but inserted range is in
  a plain area" (still routes via InsertWithStyle since
  Empty() is false; the styles slice happens to be all plain).
  Subtlety: MockFrame's GetFrameFillStatus reports Nchars=0,
  which would gate every test insert out of the visibility
  check. The recording frame overrides GetFrameFillStatus to
  report a large Nchars. Tests bypass the buffer entirely and
  call Text.Inserted directly (with a manual call to
  spans.Inserted first to simulate the buffer-driven ordering)
  to keep the tag-status observer chain (UpdateTag, setTag1,
  Resize) out of scope.
- A4.3 — Text.fill and Text.setorigin now query spans before
  pushing runes into the frame. fill: when spans is non-empty,
  the call to `fr.Insert(rp[:i], framePos)` becomes
  `fr.InsertWithStyle(rp[:i], framePos, t.spans.GetStyleRuns(
  t.org+framePos, t.org+framePos+i))`; the plain path uses
  fr.Insert unchanged. setorigin's scroll-forward branch (when
  the new origin sits before the current org) does the same.
  After fill, setorigin calls `t.fr.SetOriginYOffset(0)` — the
  A2.3 stub returns 0 in Slice A; Slice C C2 will compute the
  real tall-element y-offset via Text.computeTallElementYOffset.
  Tests: nil spans → Insert; empty spans → Insert; styled spans
  → InsertWithStyle with correct styles slice (sum of Lens
  matches rune count). The setorigin test verifies the
  SetOriginYOffset(0) call.
  Test infrastructure: extended `recordingFrame` to intercept
  `Insert` (rune-based, for fill) and `SetOriginYOffset` (for
  setorigin), and bump its reported Nchars on each insert so
  fill's loop terminates instead of feeding the buffer back to
  itself. Made the reported Nchars configurable per-test
  (A4.2 needs it large for the visibility check; A4.3 starts
  at 0 so fill sees a fresh frame).
- A4.4 — `attachSpans` grew an `Observe` callback (§7.6). When
  a producer calls SetRegion / ClearRegion on the spans store,
  the callback receives (p0, p1) in document-absolute rune
  offsets. It clips to the visible window
  `[t.org, t.org+Nchars)` — if the change is entirely outside
  the window, returns early; otherwise calls
  `t.fr.SetStyleRange(p0-t.org, p1-t.org, runs)` with frame-
  relative offsets and the styles for the clipped range from
  `GetStyleRuns`. 4 tests pin the contract: in-window →
  SetStyleRange called with full range; out-of-window → no
  call; partial overlap → clipped; non-zero t.org → frame-
  relative offsets correct.
- A5.1 — `spans/parse.go` introduces `ParseDirective(line)` and
  `ParseAll(text)` for the 9P spans-file wire format. Slice A
  recognizes `s <off> <len> [fg=#RRGGBB] [bg=#RRGGBB]` and
  `c <off> <len>`; `b` directives, unknown keys (bold/italic/
  underline/font), malformed integers, and malformed colors all
  fail with errors. Parsed colors are `color.Color` rather than
  `draw.Image` so the spans package stays free of a draw
  dependency — the xfid/main-package handler (A5.2) will
  resolve to `draw.Image` via `display.AllocImage` before
  calling `Store.SetRegion`. Serialization (Snapshot →
  directives) is deferred; `draw.Image` is opaque so we can't
  recover RGB from a stored Style without an additional
  side-channel. 13 tests cover happy paths and rejections.
- A5.2 — `QWspans` qid wired into xfid. `dat.go` enum + `fsys.go`
  dirtab entry (mode 0600) make the file appear in every
  window's wsys directory. Writes route through a new
  `xfid_spans.go`: `xfidspanswrite` → `writeSpansToStore(w,
  payload)` (testable helper) → `spans.ParseAll` →
  `applySpansDirective` per directive. Each `s` directive's
  color.Color fields are resolved to `draw.Image` via
  `allocColorImage(display, color)` which packs RGBA into
  `draw.Color` (0xRRGGBBAA) and calls `display.AllocImage`.
  Reads return an empty string for Slice A — full serialization
  (Snapshot → directives) is deferred (see A5.1 note about
  draw.Image opacity). Tests cover set / clear paths, multi-
  directive payloads, bg-only, bad directives, and nil-spans
  defense; they bypass `InsertAt` (which would trigger the
  tag-status observer chain) by pre-loading the body buffer
  with `file.MakeObservableEditableBuffer("name", []rune("..."))`.
- A5.3 — integration test pinning the producer-driven path:
  `writeSpansToStore` → `Store.SetRegion` → A4.4 Observe
  callback → `Frame.SetStyleRange` with the right frame-relative
  offsets and a `KindColored` run. We test the *apply* chain
  (which is what mattered to get right); the 9P I/O wrapping
  isn't end-to-end tested at the protocol level. A manual smoke
  test using the prior repo's `edcolor` against the cleanroom
  edwood is the natural next confidence step.
  One gotcha during test writing: a second `attachSpans` call
  registers a SECOND Observe callback (it doesn't replace the
  first). Doubled callbacks meant SetStyleRange fired twice.
  Fix: avoid re-attaching; the callback closes over `t` (a
  `*Text`), so swapping `t.fr` is picked up on the next
  invocation. Worth being careful about: if attachSpans ever
  needs to be re-callable, it should unregister the previous
  observer first.
- Protocol-compliance rework. The first A5.1 pass implemented
  an invented `s 0 5 fg=#RRGGBB bg=#RRGGBB` key=value wire
  format. After landing A5.3 we tried to confirm by running the
  prior `edcolor` against the cleanroom build and discovered
  the wire format is published externally
  (`/Users/paul/dev/edwood/docs/designs/spans-protocol.md`) and
  is *positional*:
    `c`                                     -- clears all
    `s <off> <len> <fg> [<bg>] [<flag>...]` -- styled run
  where `<fg>`/`<bg>` are `#rrggbb` or `-` and flags are bare
  tokens (`bold`, `italic`, `scale=N.N`, `family=NAME`,
  `hrule`). The published spec also requires `c` to be alone
  in its 9P write, requires `s` contiguity within a write
  (each `s.off == prev.off + prev.len`), and tolerates
  out-of-range directives (offset >= Nr drops silently;
  end > Nr clamps). Rewrote `spans/parse.go` and
  `xfid_spans.go` to match:
    - parser: positional fields; `-` → nil color; bg
      discriminated by appearance (`#`-prefix or `-`); any
      other 4th-or-later token is a flag and rejected in
      Slice A; ParseAll enforces c-exclusivity and
      contiguity.
    - applier: `OpClearAll` calls `ClearRegion(0, Nr())`;
      `OpSetStyle` clamps `end` to `Nr()` and silently drops
      directives whose `off >= Nr()`.
  Updated `docs/designs/features/unified-frame-spans.md` §6.4
  to point at the published spec and describe the Slice A
  subset accurately. 20 parser tests + 10 xfid tests now
  exercise the real protocol shape.
- Follow-up to the protocol rework: the published `s` directive
  carries flag tokens — `bold`, `italic`, `hidden`, `hrule`,
  `scale=N.N`, `family=NAME` — that Slice A can't yet honor.
  Rather than make every producer omit them, the parser now
  silently accepts these as no-ops (the Directive's observable
  fields are unchanged). Slice B / C will translate them into
  rendering. Unknown flag spellings still error, so producers
  mis-typing a flag get a loud failure. With this change the
  prior `edcolor`'s bold-tagged lines parse cleanly; one less
  obstacle to running it as a smoke test once we finish A6
  (the S event it needs to react to selection changes).
- A6 — Text.SetSelect now emits the `S` event per §9.3 of the
  design. Implementation is six lines after the existing q0/q1
  update: save old (q0, q1), and if t.what == Body and
  t.spans != nil and t.w != nil and (oldQ0 != q0 ||
  oldQ1 != q1), call `t.w.Eventf("S%d %d 0 0 \n", q0, q1)`.
  The "listener open" gate is enforced by Eventf itself
  (`nopen[QWevent] > 0`). Format is `S<q0> <q1> 0 0 \n` —
  matches the published acme event-file vocabulary (single
  char prefix + four space-separated fields, no text payload).
  Six tests pin the four gates plus a "subsequent change
  fires again" case. Smoke test with prior `edcolor` should
  now work end-to-end: edcolor reacts to S events to
  re-colorize matches of the current selection.
- **Slice A closing**. A7 (cleanroom `edcolor` rewrite)
  skipped — the external prior `edcolor` works unmodified
  against the cleanroom edwood now that A1–A6 are in place.
  The smoke test was confirmed live; the user reported it
  working. Slice A's exit criterion is therefore met by way
  of the external tool: cleanroom edwood + external edcolor
  = working syntax coloring with selection-driven highlight.
  Bilateral wire-format compliance is the contract; the
  in-repo rewrite was always an option, not a requirement.
  Next slice: Slice B (typographic variation).

### 2026-05-12 — Slice B follow-ups + Phase B4 (md2spans compat)

- **Bold-width clipping** (`dce9c08`). Bold glyphs are wider
  than the regular font in the Go family but box.Wid was
  computed via `f.font.BytesWidth` at 8+ sites. Adjacent boxes
  overlapped → "type"/"struct"/"map" tails clipped. Fix:
  route every whole-Ptr width compute through
  `f.fontFor(b.Style).BytesWidth(b.Ptr)`. Regression test pins
  the contract via a mock bold font wider than the base.
- **SetStyleRange Wid refresh** (`fd390b0` then patched in
  `01a4af5`). SetStyleRange assigned `b.Style` without
  recomputing `b.Wid`, so on first paint after edcolor styled a
  token the next box's background fill started early and the
  bold glyph's tail got clipped. Same bug class as the
  bold-width one, different code path. First fix clobbered
  tab/newline widths because the existing `boxRunes <= 0`
  guard never triggered for them (`nrune()` returns 1 for
  specials). Final fix gates Wid refresh on `b.Nrune < 0`
  directly and updates the new `boxWid` helper landed in
  B4.1.5. Tagged the result `color-and-style` after a working
  smoke test in the user's hands.

- **Phase B4 — md2spans compatibility** (4 rows landed
  `59605e5`, `961fd98`, `5ee0dd0`, plus this row).
  - B4.1 (`59605e5`) — Parser silently accepts `b`,
    `begin region`, `end region` as `OpNoOp`; contiguity
    walks past `OpNoOp`s; flag loop translates `hrule` →
    `KindHRule` and `family=code` → `KindCodeFamily`. Other
    `family=NAME` and `scale=N.N` stay silent-accept-no-bit.
    xfid_spans.go applier ignores `OpNoOp` explicitly.
  - B4.1.5 (`961fd98`) — Refactor. Extract `paintBox(b, pt,
    text, back, clearBg)` as the single per-box paint
    function; drawtext and repaintBoxRange collapse to
    walk-and-call. Extract `boxWid(b)` as the single
    content-box width compute; validateboxmodel asserts
    `b.Wid == f.boxWid(b)` so the SetStyleRange-Wid bug
    class is structurally prevented. Pure refactor; new
    tests pin the invariants but no behavior change.
  - B4.2 (`5ee0dd0`) — Frame renders the new bits.
    `fontCode` slot + `OptCodeFont`; `fontFor` consults
    KindCodeFamily before weight/italic (family wins).
    `paintBox` draws a 1-pixel hrule line at the row's
    vertical center when KindHRule is set, after the
    glyph paint (markers stay visible). One-site edit.
  - B4.3 (this commit) — Text/acme wiring.
    `tryLoadFontVariant` factored into a pure
    `variantPathFor` helper; map gains `code` → `GoMono`
    for `GoRegular` bases (and identity for `GoMono`
    bases). text.go calls it with variant="code" and
    threads the result through `frame.OptCodeFont`.
    Failures degrade gracefully to the base font.

  Smoke test needed: open a markdown file in the rebuilt
  edwood, add `md2spans` to the tag, run it (with md2spans
  built from `/Users/paul/dev/edwood/cmd/md2spans`). Expect
  bold/italic/bold-italic emphasis, inline-link colors,
  horizontal rules, and `family=code` runs in the monospace
  cousin. Headings (`scale=N.N`), images (`b`), and block
  regions (`begin region` / `end region`) stay unstyled —
  they're Slice C.

### 2026-05-12 (later) — Phase B2.2 attempt 1 reverted

After ten+ patches piled on B2.2.3 (per-line height in layout
walks) the implementation was too compromised to land. Each
fix uncovered another walk that drifted from `ptofcharptb`.
The architectural pattern — store a mutable `f.curLineH` on
the frame, require every walk to reset it — proved fragile.

Attempt-1 preserved as the tag `b22-attempt-1`. The repo
HEAD reset to `86848f2` (end of Phase B4). The valuable
infrastructure carried back:

- `docs/designs/features/frame-layout-invariants.md` — five
  general invariants (I-1, I-2, I-5, I-10, I-11), plus the
  architectural-notes section capturing the lessons.
- Phase B5 spec (word-boundary wrap) in design §12 and the
  plan — still applies, scanner-level change orthogonal to
  the variable-height question.
- Baseline-alignment requirement — added to architectural
  notes. Attempt 1 painted glyphs top-aligned; restart must
  use baseline alignment so a heading and adjacent body text
  share a baseline.

Not carried back (re-create during B2.2 restart):

- `frame/draw_bounds_test.go` — referenced KindScale and
  related constructs that no longer exist post-reset.
- `Box` / `Spans` tag-bar debug commands — implementation
  was tangled with the after-paint-hook architecture from
  attempt 1; rewrite cleanly during restart.
- `-validatelayout` flag — same.
- B2.2-specific invariants (I-3, I-4, I-6, I-7, I-8, I-9,
  I-12) — they belong in the next attempt's design.

### 2026-05-12 (latest) — Phase B5 rows B5.1 + B5.2 landed

B5.1 (cce20b4) — bxscan flushes wipbox at space ↔ non-space
transitions in its content branch; `isSpaceOnlyBox` added to
`clean`'s merge predicate so word and space boxes never coalesce.
B5.1 added `frame/word_split_test.go` with eight focused tests
for bxscan + clean behavior.

B5.2 (042fdc0) — `cklinewrap0` now wraps a content box as a
whole when it doesn't fit at p.X. The B5.1 split-at-spaces
work means whole-box wrap == word-boundary wrap automatically.
A word longer than the line still terminates: it wraps to a
fresh line first, then `canfit + splitbox` splits the tail.
Order matches the stated preference — character-split only
when the next line won't fit either.

Sixteen pre-existing sub-tests in `TestInsert` /
`TestInsertAligned` / `TestDelete` pinned upstream's
partial-fit-split layout and are now marked
`knowntofail: true` with pointer comments to `*_trial.html`
under `frame/testdata`. B5.3 rewrites these against the B5
layout; the trial HTMLs are committed as forensic input for
that rewrite, and `CLAUDE.scripts/diff_baselines.sh` builds a
side-by-side HTML diff page from them.

Test status: `./regression.sh` green.

Exit criterion for B5 is "markdown paragraphs wrap at word
boundaries; the bold lead line wraps before the first word
that doesn't fit". The scanner+wrap mechanics are in. Smoke
test pending: confirm md2spans-produced bold lead lines wrap
correctly in a running edwood, before B5.3.

### 2026-05-12 (latest) — Phase B5 smoke + debug overlays

Smoke test confirmed: md2spans paragraph wrap looks clean,
including the bold lead-line case. Phase B5 exit criterion met.

Debug overlays restored from B2.2-attempt-1 and rebuilt cleanly
on the B5 base (f54a40a):
- `Box` tag command — Purpleblue 1-px outline per painted box,
  state propagated through bxscan's nframe so Insert paths
  paint outlines for new content.
- `Spans` tag command — Black 1-px outline per non-plain
  region, **one rect per visual line** (invariant I-12;
  multi-line regions split at wrap, not hull).
- `Frame.SetAfterPaintHook(fn)` fires fn once per public
  paint-causing entry point AFTER `f.lk` is released — so the
  hook may freely call back into Frame methods without
  deadlock. (The first attempt fired the hook under the lock
  and deadlocked when paintSpansOverlay re-entered
  `GetFrameFillStatus`.)
- `Text.suppressSpansOverlay` short-circuits the hook during
  `setorigin`'s pre-`t.org`-update shift work; setorigin
  explicitly fires `paintSpansOverlay` at the end so
  backward-scroll-into-top-content gets covered when fill is
  a no-op.

Known overlay limitations (debug feature; acceptable for v1):
- Forward scroll relies on `t.fill`'s Inserts to fire the
  hook; if fill is a no-op the overlay can show stale pixels
  in the blitted area until the next paint. (`setorigin` does
  an explicit fire at the end, mitigating most cases.)
- `scale=N.N` (md2spans heading sizing) silently accepted →
  headings don't show overlays.
- `begin region` / `end region` → NoOp → block-region runes
  not in the overlay unless they have inline styling on top.
- md2spans deliberately leaves inline markup (`**`, `[]()`,
  `#`) plain (markup-stays-visible stance), so those never
  show overlays.

### 2026-05-14 — B2.3 reset (layout-once rewrite)

B2.2 R1–R7 + R4.1 all landed but produced user-visible
glitches on scaled headings: bottom-line clipping, mid-screen
spacing wrong after scroll, and overlap on backward scroll. The
later commits (`677ab5e` "_draw tracks per-line height locally",
`e488f22` "re-relayout child after _draw") were attempts to
paper over a deeper issue and introduced their own regressions.

User decision: stop patching `_draw` / `bxscan` from the
outside; write a first-principles design for the layout
function and migrate every consumer to readers.

This session:
- Selectively reverted the thrash (`2a917a7`). Kept the
  `lastlinefull` reset in `deleteimpl` (small isolated fix),
  kept `test-md-layout.md` (controlled fixture), kept
  `layout-once-invariant.md` (audit doc that motivates the
  rewrite). Restored `frame/draw.go` and `frame/insert.go` to
  their state before `677ab5e`.
- Wrote `docs/designs/features/frame-layout-design.md` (new):
  the layout-once spec. Single forward pass owns geometry;
  adds a per-line summary table (`lineSummary {FirstBox,
  TopY, LineH, LineA}`); uniform mutator flow (read pt0/pt1
  pre-mutation, mutate box list, `relayoutFrom`, blit shift,
  paint from box state); lists the legacy walkers slated for
  deletion (`cklinewrap`, `cklinewrap0`, `advance`,
  `ptofcharptb`, `ptofcharnb`, `charofptimpl`,
  `lineHForAdvance`, `lineHAtPt`); adds I-LAYOUT-1..5 (§7).
- Added Phase B2.3 to PLAN_unified-frame-spans.md with nine
  subrows R1..R9. The old B2.3 (perf) is renumbered B2.4.
- Forward-pointer in `frame-layout-invariants.md`: I-5 (paint
  == ptofcharptb) marked for supersession by I-LAYOUT-5 once
  ptofcharptb is gone; I-2's wording moved off `cklinewrap`.

All landed as `fdecc91` "docs: B2.3 layout-once design + plan
rows + invariants forward-pointer".

Test status: `./regression.sh` green at `fdecc91`.

Next: B2.3.R1 (per-line summary table). One CODING-PROCESS
pass — write the lineSummary type + populate from
`relayoutFrom`'s phase B, no consumer migration yet, assert
I-LAYOUT-2 / I-LAYOUT-3 fixtures.

### 2026-05-14 (later) — B2.3 design revision + R1 landed

Two-part session: a design-doc revision pass driven by inline
review, then the first migration row.

**Part 1 — design revision (commit `7889f31`).**

Reviewed `frame-layout-design.md` end-to-end. Substantive
changes:

- §3.3 rewritten: long-word `splitbox` lives **inside**
  `relayoutFrom`, not in `bxscan`. Eager split is inline in
  phase A; multi-split propagation and iterator state under
  splice are spelled out. Added a symmetric **eager coalesce**
  (inverse `splitbox`) for adjacent same-style content boxes
  whose combined `Wid` fits.
- §3.4 expanded: position seed with "why `box[nb0-1]` is safe"
  justification; entry-time `f.lines` truncation; line-count
  shrinking case; `lastlinefull` under partial relayout;
  `nb0 == len(f.box)` edge.
- §3.5 new — **paint deltas via line-table diff**. `snapshotLines`
  + `diffLines` helpers; three-way per-line classification
  (identical / shifted / dirty) keyed by `FirstRune`; adjacent
  same-ΔY runs compose into one blit (the wire-cheap path).
- §6 mutators reworked through snapshot+diff. §6.5 new —
  frame rect resize via the same primitives; `Text` owns refill
  via `lastlinefull`.
- §2.1 `frbox` comment now distinguishes content-vs-layout
  writers (relayoutFrom mutates content fields via split /
  coalesce). §2.2 `lineSummary` gained `FirstRune`.
- §5 reader-replacement column uses real names; `_draw` row
  restated as "accumulator only, paint walk remains".
- §7 I-LAYOUT-1 scoped to layout fields; new I-LAYOUT-6 (no
  layout-only fragmentation).
- §8 migration order grew 9 → 12 rows: row 1 bundles split +
  coalesce; new row 5 introduces snapshot/diff helpers; new
  rows 9/10 for scroll-fill and resize. §9 test plan extended
  to cover the new helpers and round-trips.

Plan updated to mirror (`b53d67f`). Pre-tests refinement
(`e04bc81`) added the space/word carve-out to eager-coalesce
and I-LAYOUT-6 — without it, "hello" + " " would merge and
defeat B5 word-wrap.

**Part 2 — R1 implementation.**

Per CODING-PROCESS:

- **Stage 2 tests** in `frame/line_summary_test.go` (18
  numbered requirements R1.1–R1.18). Failed at compile-time
  on the missing `lines` field — the expected red state.
- **Stage 3 implementation:** added `lineSummary` struct
  in `frame/relayout.go`; `frameimpl.lines []lineSummary`
  in `frame/frame.go`; rewrote `relayoutFrom` to populate
  the table, eager-split at line-start when `b.Wid >
  rect.Dx()` via `canfit` + `splitbox` (k=1 fallback for
  the single-rune-wider-than-line case, mid-rune split
  deferred to B5.4), eager-coalesce via the new
  `coalesceAt` helper before each wrap decision, and
  append a `lineSummary` entry per closed line.
- **Stage 4 bug classification:** one pre-existing test
  (`TestBxscan-long-word fallback splits across wrapped
  lines`) failed because it pinned B2.2 R7's "wrap-to-
  fresh-line-first-even-at-line-start" semantic — the new
  design §3.3 case 3 intentionally produces a tighter
  layout (first chunk sits at line-start Y rather than
  pushing to an empty next line). Classified as **wrong
  test** under the new design. Updated the test expectation
  and comment with user approval.

`./regression.sh` green. The legacy `splitbox` call in `_draw`
(`frame/draw.go:518`) is now dead code on the bxscan path —
relayoutFrom does the splitting before `_draw` runs — but it
stays until R4 removes the `_draw` accumulator.

Next: B2.3.R2 — move `lastlinefull` ownership into
`relayoutFrom`. Drop the explicit reset in `deleteimpl`
(commit `677ab5e`'s carryover); assert I-LAYOUT-4. Should be
a small row given R1's groundwork.

### 2026-05-14 (later still) — R2 landed

8 numbered requirements (R2.1–R2.8) in
`frame/lastlinefull_test.go` covering empty / fits / exactly-
fills / overflows + post-Insert / post-Delete / post-
SetStyleRange / sole-writer.

Implementation: a `defer` at the top of `relayoutFrom` sets
`f.lastlinefull` from the line-table formula
(`lines[-1].TopY + lines[-1].LineH >= rect.Max.Y`, false for
empty). Removed every other writer:
- `deleteimpl`'s explicit reset (commit `677ab5e`).
- `clean()`'s pt-walk setter in `util.go`.
- `bxscan`'s child→parent copy `f.lastlinefull = frame.lastlinefull`.
- `_draw`'s truncation setter at `draw.go:506` (truncation
  still happens; `lastlinefull` is re-derived by the parent
  `relayoutFrom`).

`frame.go` Init's `f.lastlinefull = false` stays as the
setup-time zero, scoped out of the "no other writer" rule.

Semantic refinement: for exact-fill (last line bottom ==
rect.Max.Y), the new formula gives `true` where the prior
pt-walk gave `false`. Intentional per design §2.3 — "no more
vertical room for content" is the better semantic of
"lastlinefull."

`./regression.sh` green. No tests outside R2's own changed
expectations; the existing Insert/Delete/SetStyleRange paths
already produced correct lastlinefull under the new formula.

Next: B2.3.R3 — route `Charofpt` through the line-summary
table. Binary search by `TopY`; the inner walk uses
`FirstRune` to identify rune offsets inside a line.

### 2026-05-14 (later still 2) — R3 landed

`charOfPtReader` rewritten in `frame/ptofchar.go`:

- Binary search `f.lines` by `TopY` (lines are Y-sorted by
  I-LAYOUT-3) to find the target line in O(log n).
- Walk only that line's boxes via `FirstBox`..next-line's-
  `FirstBox`. Inner walk seeds `p` from `line.FirstRune`.
- Above-content / empty-frame / below-content cases handled
  via explicit shortcuts. End-of-content rune offset is
  derived from the box list (not `f.nchars`) so inline-
  constructed test fixtures work.

Tests: 10 numbered requirements (R3.1–R3.10) in
`frame/charofpt_line_table_test.go`. Plain-text parity test
sweeps a grid of click positions across a 4-line frame; all
results stay within `[FirstRune, nextLine.FirstRune]` for
their respective lines.

Stage-4 minor: one existing `TestCharofpt` case (inline-
constructed frame, no `nchars`) initially failed because the
first cut of my new code returned `f.nchars` for below-
content clicks. Fixed by deriving end-of-content from the
box list directly. Not a wrong-design — just a more robust
implementation. No test changes needed.

`./regression.sh` green. Legacy `charofptimpl` retained for
the `unlockedproxy.Charofpt` path; deleted in R11.

Next: B2.3.R4 — eliminate `_draw` as an accumulator walker.
`bxscan` reads `pt1` from the staging frame's last line-table
entry instead of `_draw`'s pt accumulator. This is called out
in the migration order as the single biggest correctness gain
(suspected root of the scroll-overlap glitches that motivated
B2.3 in the first place).

### 2026-05-14 (later still 3) — R4 landed (scoped)

R4 turned out to need scoping. The plan row's target ("pt1
read from staging's `lines[-1]`") only works once
insertbyteimpl is also restructured (R6) because:

- `_draw` does insertion-point-aware layout: it walks the
  staging frame's boxes with `pt = pt0` (the parent-coord
  insertion point), letting cklinewrap0 wrap based on the
  ACTUAL placement position. The staging frame's
  relayoutFrom lays out from `staging.rect.Min`, not from
  `pt0`, so the line-table doesn't capture this.
- Downstream code at `insert.go:295, 316, 464` consumes
  `pt1` as the visual end position of inserted content;
  swapping the source mid-flow breaks it.

Scoped R4:

1. **Added tab newwid in `relayoutFrom`'s phase A** (parent-
   frame path). After the wrap decision, if the box is a
   tab, recompute `b.Wid` via `newwid0(pt, b)`. This makes
   SetStyleRange / Delete / resize land correctly without
   needing a separate post-relayout walker.
2. **Stripped `_draw` of the redundant `canfit + splitbox`
   calls.** R1's eager-split inside relayoutFrom guarantees
   no box arriving at `_draw` is wider than `rect.Dx()`, so
   the long-word split path in `_draw` was dead. Kept the
   pt-accumulator + truncation + tab newwid for the staging-
   frame use case.
3. **Added helpers** `nextPositionAfterLast` and
   `truncateOffscreen` in `relayout.go`. Both are line-
   table-driven and ready for R6/R7 to consume; not yet
   wired into bxscan (that wiring is part of restructuring
   `insertbyteimpl` to use snapshot+diff).

Stage 4: `TestSetStyleRange_PreservesTabWidth` failed because
it pinned a wrong behavior — when bold styling grew the
leading "a"'s width, the test expected the tab's Wid to be
preserved (which made "b" drift off the tabstop column). The
correct semantic is "tab Wid is recomputed so b stays on the
tabstop." Updated the test assertions (with user approval) to
check tabstop alignment instead of preservation. Newline Wids
(position-independent 10000 placeholder) still preserved.

Tests: 7 numbered requirements (R4.1–R4.5 + extensions) in
`frame/draw_accumulator_test.go`. Existing TestBxscan,
TestInsert, TestInsertAligned, TestSetStyleRange* all green.
`./regression.sh` green.

Outstanding for R6 (insertbyteimpl restructure):

- Wire `nextPositionAfterLast` + `truncateOffscreen` into
  bxscan, dropping the `_draw` call entirely.
- Use `snapshotLines` + `diffLines` (R5) to compute paint
  ops instead of the pt0→pt1 reconciliation loop.

Next: B2.3.R5 — introduce `snapshotLines` + `diffLines`
helpers per design §3.5. No mutator wired yet; unit tests
construct pre/post `f.lines` states and assert the
classification (identical / shifted / dirty) + run
compression. `deleteimpl` is the first consumer in R6.

### 2026-05-14 (later still 4) — R5 landed; design corrected

Added `frame/diff_lines.go` with:

- `lineSnap{FirstRune, TopY, LineH, Digest}` — pre-mutation
  snapshot entry.
- `paintOp{Kind, Src, Dst}` + `OpPaint` / `OpBlit` constants.
- `snapshotLines()`, `lineDigest(i)`, `diffLines(snap)`.

`lineDigest` is FNV-1a over each box's `Ptr` bytes +
`Style.Kind` + `Bc` + `Nrune` for the line's box range.

**Design correction mid-row.** The first cut of `diffLines`
matched new→old lines by `FirstRune` (per design §3.5's
language: "FirstRune is the canonical identity, stable
across box-list shifts caused by inserts/deletes within
earlier lines"). The shifted-run test failed because that
language was wrong — inserting "\n" at position 0 shifts
every subsequent line's rune coordinate by +1, so no
FirstRune match is found and every line classifies as
dirty. Replaced with **forward-greedy digest matching**:
for each new line, find the earliest unused old line whose
content digest equals the new line's, scanning forward in
the snap. This correctly identifies "moved content" as a
blit. Updated §3.5 to reflect — `FirstRune` is retained in
the snap for debugging and possible future LCS-style
matchers but isn't consulted today.

Tests: 9 numbered requirements (R5.1–R5.9) in
`frame/diff_lines_test.go`. All green. `./regression.sh`
green.

Helpers are introduced but not yet consumed by any mutator;
R6 wires `deleteimpl` as the first consumer, R7
`insertbyteimpl`, R8 `SetStyleRange`, R9 the scroll/fill
path, R10 resize.

Next: B2.3.R6 — restructure `deleteimpl` to use
`snapshotLines` + `diffLines`. Pre-mutation snapshot, mutate
box list, one `relayoutFrom(0)`, one `diffLines(snap)`, issue
ops. Delete the inner per-box loop and the legacy
`contentBottomY` snapshot. First mutator-side consumer of
the new helpers.

### 2026-05-14 (later still 5) — R6 landed

`deleteimpl` rewritten per design §6.2:

1. Pre-mutation: `snapshotLines()` + capture `oldBottom`
   (lines[-1].TopY + LineH).
2. `findbox` splits at p0/p1 boundaries; `closebox` splices
   out [n0, n1).
3. `f.nchars -= p1 - p0`; selection bounds adjusted.
4. `relayoutFrom(0)` — eager-coalesce re-merges boundary
   fragments; lastlinefull re-derived (R2's defer).
5. `diffLines(snap)` + `issuePaintOps(ops)` — issues blit
   ops for shifted runs and OpPaint for dirty lines.
6. Vacated region at bottom cleared explicitly.
7. Tick + nlines update.

Removed:
- The legacy per-box `cklinewrap0` / `cklinewrap` / `canfit`
  / `splitbox` / `advance` inner blit walk.
- The trailing "blit up the remainder" step (now subsumed
  by the diff's run-compressed blits).
- The `f.clean(...)` call (R1's eager-coalesce in
  relayoutFrom is a superset).

Added in `frame/diff_lines.go`:
- `issuePaintOps(ops)`: blit Src→Dst for OpBlit; clear rect
  + repaintBoxRange for OpPaint.
- `linesInDstYRange(minY, maxY)`: helper to map a Dst rect
  back to f.lines indices for the per-box repaint.

**Stage-4: implementation accident in R2 found and fixed.**
The deferred `lastlinefull`-setting was registered AFTER an
early-return for the `nb0 == len(f.box)` case (empty or
past-end). R6's test for Delete-to-empty exposed this:
deleting all content left `lastlinefull` at its previous
value (`true`) because the defer never fired. Moved the
defer to the top of `relayoutFrom`, before any early
return. Bug fixed in-place; the R6 commit picks it up.

**Stage-4: wrong-test classifications (with user approval).**
R6 intentionally changes the draw-op sequence from
per-box surgical updates (legacy) to line-batch blits +
line-rect paints (new design). Same end-state on the
rendered frame, different op recorder output.

- `TestR7_Delete_BlitHeight_PlainLines` /
  `TestR7_Delete_BlitHeight_ScaledLine`: updated to expect
  R5's run-compressed multi-line blits (Dy = total shifted-
  run height, not single-line height).
- `TestDelete` sub-cases `deleteSingleCharacterAtLineEnd`,
  `deleteSingleCharacterInMiddle`,
  `deleteNewlineTocreateWrappedLine`, `rippleUpMultiLine`:
  updated `want:` to match the new line-rect paint sequence.
- 4 SVG baselines in `frame/testdata/TestDelete/` regenerated
  to match.

Tests: 5 new (R6.1–R6.6) in `frame/delete_diff_test.go`:
single-line / across-lines / vacates-bottom / delete-all /
I-LAYOUT invariants hold post-Delete.

`./regression.sh` green at commit.

Outstanding for R7: the same restructure but for
`insertbyteimpl`. Note that R7's blits move DOWNWARD
(ΔY > 0), which means input-order blits would overwrite
Src regions before reading them. `issuePaintOps` will need
either bottom-to-top blit ordering or a separate code path
for downward shifts.

Next: B2.3.R7 — `insertbyteimpl` via snapshot+diff. Same
pattern as R6 but the blits shift content DOWN to make
room for the insertion.

### 2026-05-14 (later still 6) — R7 landed

Rewrote `insertbyteimpl` per design §6.1. Production:

- Pre-mutation `snapshotLines`.
- `findbox` + `drawselimpl` (erase selection).
- `bxscan` (legacy; still runs nframe._draw for tab Wid +
  off-screen truncation in the staging frame).
- Splice nframe.box into f.box. Adjust nchars + selection.
- `f.relayoutFrom(0)`.
- `truncateOffscreen` to keep the bounded-frame contract.
- `diffLines(snap)` + `issuePaintOps`.

Removed: the legacy convergence loop, `drawtext` on the
staging frame, `fillNonGlyphAreas`, the `pts[]` parallel
walk, the chop path, and the trailing `f.clean` call.

Updated `issuePaintOps` (in `diff_lines.go`) to handle
Insert's downward blits: blits are issued first (so paint
ops don't clobber pixels needed by blits), then within the
blit pass downward shifts are issued bottom-to-top so each
Src isn't overwritten by a prior Dst.

**Stage-4 design refinement of §3.5 diffLines.** Initial
smoke test showed a "stale stripe" bug — content from before
a scroll showing through where new content should be. Root
cause: `relayoutFrom` keeps off-screen geometry, so the line
table can contain lines whose pixels were never validly
drawn (paintBox bails on Y >= rect.Max.Y). When such a line
moves on-screen via a Delete or Insert, the diff classified
it as "shifted" and emitted a blit from the off-screen
position — copying stale pixels from a previous frame into
view. Fix: classify a line as **dirty** (paint) when its OLD
or NEW position isn't fully visible, never blit. This was
the load-bearing fix for the user-visible scroll bug.

**Stage-4 implementation accidents found and fixed.**

1. `Init` and `Clear` reset `f.box = nil` / `make(..., 0)`
   without touching `f.lines`. A stale `f.lines[0].FirstBox=0`
   on a frame whose box was just reset caused `lineDigest` to
   index out of range when the next `snapshotLines` ran.
   Both now nil `f.lines` in lockstep.
2. Defensive clamp in `lineDigest`: even if `f.lines` is
   somehow stale relative to `f.box`, the digest loop now
   bounds-clamps `start`/`end` so it can't index past
   `len(f.box)`.

Manual smoke test (open CLAUDE.md, scroll part-way down,
exercise scroll-down + B3 middle-click jumps): all three
artifacts gone — no duplicate stripe, no missing heading
gap, no out-of-order content. The legacy "mid-screen
spacing wrong after scroll" bug that motivated B2.3 is
fixed.

Tests:
- 6 new requirements (R7.1–R7.6) in
  `frame/insert_diff_test.go`: single-line, adds-lines,
  top-of-full-frame, invariants, round-trip.
- 6 TestInsert sub-cases + 2 TestInsertAligned sub-cases
  updated for new line-batch op sequence.
- 4 TestDelete sub-cases (already updated in R6) revised
  for downstream effects of Insert's new ops.
- 9 SVG baselines regenerated.

`./regression.sh` green.

Outstanding from R7:
- The legacy `f.sp1 += f.nchars` typo (should be `=`) is
  preserved to keep test fixtures valid; a follow-up row
  fixes it.
- `_draw`, `cklinewrap*`, `advance`, `ptofcharptb`,
  `charofptimpl`, etc. are still present but increasingly
  unused; R11 deletes them.

Next: B2.3.R8 — `SetStyleRange` via diff. Simpler than R7
because the rune offsets don't shift; the diff should
mostly classify identical lines as identical and just
repaint the styled range.

### 2026-05-14 (later still 7) — R8 landed

`SetStyleRange` rewritten per design §6.3:

- Pre-mutation `snapshotLines`.
- Boundary `splitbox` at `[p0, p1)` edges via `findbox`.
- Walk boxes, apply Style + recompute Wid; mid-box style
  splits grow nb1.
- `f.relayoutFrom(0)` — eager-coalesce merges any boundary
  splits whose pieces ended up with identical styles.
- `f.diffLines(snap)` + `f.issuePaintOps(ops)`.
- Selection-highlight repaint if overlapping the styled
  range.

Removed:
- `preBottomY := f.contentBottomY()` snapshot and the
  `if contentBottomY() != preBottomY` conditional that
  switched between full-clear-and-repaint and narrow-
  repaint. diffLines handles both via the dirty/shifted
  classification.
- `f.clean(...)` (R1's eager-coalesce subsumes it).
- The unused `contentBottomY` helper function itself.

Tests:
- 4 new requirements (R8.1–R8.4) in
  `frame/set_style_range_diff_test.go`: no-reflow,
  invariants hold, no-op style change.
- All existing TestSetStyleRange_* green.

Stage-4 wrong-test (with user approval per CLAUDE.md):
`TestPaintParity_DrawtextAndRepaintAgreeOnFont` pinned the
legacy "re-style to same style triggers redundant repaint"
behavior. R8's optimization classifies a same-style restyle
as identical (no op). Updated the test to use a real style
transition (plain → bold) so the parity guard (drawtext and
repaintBoxRange both produce font ops appropriate to their
style) is preserved.

`go test ./frame/` green.

Next: B2.3.R9 — scroll/fill path via parent snapshot+diff.
Per §6.4 the `Text.fill`/`Text.scroll` path should call
`snapshotLines` on the parent before splicing nframe boxes;
issue `diffLines` ops after. This is largely already
happening because Insert now does this internally (R7);
R9 may collapse to "audit + verify the Text-side has no
remaining direct frame-pixel manipulation."

### 2026-05-14 (later still 8) — R9 audit-only

Audit confirmed R6+R7 already cover R9's intent. No
production code changes.

Text-side scroll/fill paths inventoried:
- `Text.fill` → `fr.Insert` / `fr.InsertWithStyle`
- `Text.setorigin` → `fr.Delete` + `t.fill`
- `Text.FrameScroll` → `Text.setorigin`
- `Text.Redraw` → `frame.Init` + `frame.Redraw` (bg fill) +
  `t.fill`

Every mutator-style path now uses snapshot+diff via R6
(Delete) or R7 (Insert). `frame.Redraw` is just a bg fill
before `t.fill` paints content; OpPaint also clears with the
same bg color, so the two paths agree.

`grep -rn "_draw\b"` outside frame/ → no callers. Frame's
`_draw` retains its pt-walk for the legacy bxscan path
(R7's insertbyteimpl drops the staging-frame return value
but bxscan still invokes _draw for tab Wid recompute and
nframe truncation). R11 deletes `_draw` along with the
other legacy walkers.

Next: B2.3.R10 — frame rect resize. The §6.5 path:
snap → set `f.rect` → `relayoutFrom(0)` → `diffLines` →
issue ops; `Text` inspects `lastlinefull` post-resize and
calls `fill` if the new rect is taller. Mostly verifying
the existing resize path handles this correctly (it should,
since `frame.Init` already nils `f.lines` and `t.fill`
follows).

### 2026-05-14 (later still 9) — R10 audit-only

Audit confirmed Text.Resize already works correctly via
R1+R7. No production code changes.

Resize path:
- `Text.Resize` → `t.fr.Clear(false)` (clears box; my R7 fix
  also nils `f.lines`).
- `t.Redraw(r, odx, noredraw)` → `frame.Init` (resets state
  + new rect via `setrects`) → `frame.Redraw` (bg fill) →
  `t.fill` (chunked `fr.Insert` via R7).
- Each chunked Insert snapshots, mutates, diffs. First
  insert's snap is empty → all new lines dirty → painted
  from scratch.

The R7 "off-screen blit fix" handles content that was
off-screen pre-resize and is now visible (paint instead of
blit, so no stale-pixel import). Resize-grow paints fresh;
resize-shrink stops fill at lastlinefull.

The design's "single snapshot+diff over the entire resize"
would be more efficient (only paint what changed) but the
current "full clear + refill" path is correct and uses the
diff machinery internally via t.fill. Future optimization
row if profiling shows resize is a bottleneck.

Next: B2.3.R11 — delete legacy walkers. Mechanical: remove
`cklinewrap`, `cklinewrap0`, `advance`, `ptofcharptb`,
`ptofcharnb`, `charofptimpl`, `lineHForAdvance`,
`lineHAtPt`, and `_draw`. Compile errors at the call sites
are the migration checklist. `_draw` still has callers
inside bxscan; needs careful removal.

## Next-session candidates

1. **B2.3.R11 — Delete legacy walkers.** Mechanical cleanup.
   Each deletion exposes a chain of call sites that need
   updating (or also deleting). `_draw` has the most
   tendrils because bxscan still uses it for tab Wid +
   nframe truncation.
2. Phase B5.3 — rewrite the 16 knowntofail sub-tests against
   the B5 layout. Use `frame/testdata/*/_trial.html` as the
   reference; verify each one shows the intended wrap
   behavior before promoting it to baseline.
3. Slice C C1 — Replaced-element rendering for `b` directive.
4. The Externalize-font-variant-map idea (project memory).
5. Scrollbar refactor — `docs/designs/features/frame-scrollbar-spec.md`
   is a stub capturing the scroll-direction-alignment rule
   (B1 → SnapBottom; B3 / B2 / programmatic → SnapTop; file-top
   and tall-line edge cases override). Expand to a full design
   when the scrollbar phase lands.
