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

## Next-session candidates

1. Row A2.2 — `SetStyleRange` (color-only; no line-height
   recompute, since Slice A line height is uniform). Re-style
   boxes already in the frame; repaint affected region.
2. Row A2.3 — `SetOriginYOffset` / `GetOriginYOffset` stubs (real
   behavior in Slice C2).
3. Selection rendering on styled text fix-up (see A2.1 deferral
   note above). Probably its own commit between A2 and A3.
4. Slice A's exit point is `edcolor` working end-to-end. Keep
   Slice A's `Style` minimal (`Kind`, `Fg`, `Bg`); resist pulling
   in font or replaced-element fields until B / C.
