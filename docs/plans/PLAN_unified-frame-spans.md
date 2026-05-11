# Plan — Unified Frame + Spans

Working checklist for the design at
`docs/designs/features/unified-frame-spans.md`. Each numbered row
is one CODING-PROCESS pass on a specific deliverable. Treat each
row as the entire scope of one sitting: do not skip the test
stage, do not stage-jump on implementation, and do not skip the
commit.

The work lands in three vertical slices (design §12). Slice A is
the first shippable end-to-end vertical (coloring + `edcolor`).
Slice B adds typographic variation. Slice C adds replaced
elements, block context, and the remaining producers.

Row legend (per project CLAUDE.md):
- `[ ] Design`  — confirm the relevant slice of the design doc
- `[ ] Tests`   — write tests against the requirements
- `[ ] Iterate` — implement red → green → review
- `[ ] Commit`  — commit with the message specified in the row

---

## Phase 0 — Setup

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| 0.1 | [x] §12 Phase 0 | [x] `go test -race ./...` green at HEAD | [x] `regression.sh`, working log, this plan | [x] `230d818` + `dc5fae9` | Two commits: project instruction docs, then design+plan+log+runner. |

Exit criterion: `./regression.sh` green; working log and plan
present; `cleanroom` sits on `upstream/master` HEAD.

---

# Slice A — Coloring

Minimum end-to-end vertical: `Style` carries only `Fg` and `Bg`;
`edcolor` works.

## Phase A1 — Frame data types (color-only)

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| A1.1 | [x] §5.3 (initial color subset) | [x] `IsZero()` returns true on `Style{}` and false on any non-default field | [x] Add `frame.StyleRun`, `frame.Style{Fg, Bg}`, `frame.ReplacedKind` enum (declared, unused) | [x] `927a34a` — `frame: introduce StyleRun, Style (color subset), ReplacedKind` | Superseded by A1.2 (Stage-4 "wrong design"): `IsZero()` rolled into Kind-bitmask model. |
| A1.2 | [x] §5.3 reworked (Kind bitmask, IsPlain semantics) | [x] `IsPlain()` reflects Kind alone; KindColored distinct; any non-zero Kind is non-plain | [x] Add `Kind` bitmask (`KindPlain=0`, `KindColored=1<<0`); replace `IsZero` with `IsPlain`; update §5.3/§5.4/§6.1/§12 A1+B1/§17 in design doc | [x] (pending commit) — `frame: rework Style with Kind bitmask and IsPlain` | Producer sets `Kind` to declare which fields are meaningful; `Kind==KindPlain` == upstream defaults. |

## Phase A2 — Frame styled methods (color-only impl)

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| A2.1 | [x] §5.1, §5.4 `InsertWithStyle` | [x] nil-styles ≡ upstream `Insert`; all-IsPlain slice ≡ fast path; color applied to boxes; split at style boundary; mismatched Lens panic; return value matches Insert | [x] Add `Style` to `frbox`; `InsertWithStyle` on `Frame`/`SelectScrollUpdater` + proxy; `bxscan`/`insertbyteimpl` take optional `runeStyles []Style` (nil = plain, unified path — no duplicated styled twin); `drawtext` honors `box.Style`; `clean` only merges same-Style boxes | [x] (pending commit) — `frame: add InsertWithStyle (color-only)` | Selection rendering on styled text deferred (see Notes in working log). |
| A2.2 | [ ] §5.4 `SetStyleRange` (no line-height recompute) | [ ] Re-style updates storage and repaints; out-of-range panics | [ ] Implement color-only path | [ ] `frame: add SetStyleRange (color-only)` | Line-height recompute waits for Slice B. |
| A2.3 | [ ] §5.4 `SetOriginYOffset` / `GetOriginYOffset` (stubs) | [ ] `Get` returns 0; `Set` is a no-op | [ ] Add stubs | [ ] `frame: add SetOriginYOffset/Get stubs` | Real behavior in Slice C (C2). |

## Phase A3 — Spans package

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| A3.1 | [ ] §6.1, §6.3 (`Store`, `GetStyleRuns` shape) | [ ] Empty store, single region, multi-region, full coverage of `[p0,p1)`, Len-sum invariant | [ ] In-memory store with sorted regions, binary search | [ ] `spans: introduce Store with GetStyleRuns` | |
| A3.2 | [ ] §6.2 `Inserted` / `Deleted` observer rules | [ ] Trailing-edge extension, leading-edge no-extension, deletion clipping / merging / erasing, post-delete shift | [ ] Implement observer attach + index maintenance | [ ] `spans: maintain index across buffer mutations` | The trailing-edge rule (§6.2 rationale) is the subtle bit. |
| A3.3 | [ ] §6.1 `Observe` API | [ ] Callback fires on `SetRegion` / `ClearRegion`; not fired by buffer-driven shifts | [ ] Add observer slice + dispatch | [ ] `spans: add style-change Observe callback` | Buffer-driven shifts are bookkeeping only. |

## Phase A4 — Text wiring (no producers)

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| A4.1 | [ ] §7.1, §7.2, §8.1 | [ ] Window construction registers spans observer before Text observer; assertion catches reversed order | [ ] Add `Text.spans` field; thread store through Window construction | [ ] `text: thread spans store through Window construction` | Tags get `nil` (§8.4). |
| A4.2 | [ ] §7.3 `Inserted`, §7.4 `Deleted` | [ ] Insert with `spans == nil` matches upstream; with empty store also matches upstream (fast path); with non-empty store, styles propagate to frame | [ ] Modify `Text.Inserted` to call `InsertWithStyle` when applicable | [ ] `text: style-aware Inserted` | `Deleted` requires no change. |
| A4.3 | [ ] §7.5 `fill` / `setorigin` | [ ] Visible runes carry their styles after scroll; y-offset wiring present but no-op (stub still in place) | [ ] Wire `GetStyleRuns` into fill; thread y-offset call through `setorigin` | [ ] `text: style-aware fill and setorigin` | |
| A4.4 | [ ] §7.6 `attachSpans` | [ ] Observer clips to visible range; `SetStyleRange` called with frame-relative args | [ ] Implement helper; register on store | [ ] `text: attachSpans helper` | |

## Phase A5 — 9P spans file (color-only directives)

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| A5.1 | [ ] §6.4 directive format (`s`, `c`, `fg=`, `bg=` only) | [ ] Parser/serializer round-trip; malformed input rejected | [ ] Implement parser + serializer in `spans` package | [ ] `spans: directive parser/serializer (color-only)` | `b` directives and font keys parse-reject in Slice A. |
| A5.2 | [ ] §8.3 `QWspans` qid | [ ] xfid open/read/write/close; multiple concurrent readers; "last writer wins" | [ ] Add qid in `xfid.go`; hook to per-window store | [ ] `xfid: add QWspans qid` | |
| A5.3 | [ ] Integration | [ ] Hand-written test producer writes directives over 9P; spans changes propagate to Text and onto the frame | [ ] Wire end-to-end | [ ] `text: integrate spans file with producer flow` | |

## Phase A6 — 'S' event

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| A6.1 | [ ] §9.3 emission conditions | [ ] Emitted when body + spans-attached + listener + selection-changed; suppressed when *any* condition fails | [ ] Wire emit in `Text.SetSelect` | [ ] `text: emit S event on selection change` | New event-file char; minimal vocabulary addition. |

## Phase A7 — `edcolor` clean-room rewrite

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| A7.1 | [ ] §11 `edcolor` contract | [ ] Golden-output tests on representative source files per language | [ ] Clean-room re-impl as 9P client; per-language colorizers | [ ] `cmd/edcolor: clean-room rewrite` | |
| A7.2 | [ ] §9.3 'S'-driven highlight | [ ] On `S` event, all matches of selected token are highlighted; on selection clear, highlights drop | [ ] Watch event stream; emit/clear color spans | [ ] `cmd/edcolor: selection-driven highlights` | |

**Slice A exit criterion.** `edcolor` syntax-colors a Go file in
an acme window; highlights-on-selection track the cursor. Plain
text and tag bars byte-identical to upstream. `./regression.sh`
green.

---

# Slice B — Typographic variation

Builds on Slice A. `Style` grows the emphasis/font fields; the
frame learns variable line heights.

## Phase B1 — Font fields on Style

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| B1.1 | [ ] §5.3 (font subset) | [ ] `IsZero()` accounts for new fields; round-trip | [ ] Extend `frame.Style` with `Bold`, `Italic`, `Underline`, `FontIdx` | [ ] `frame: extend Style with font flags` | |
| B1.2 | [ ] §6.4 directive format (font keys) | [ ] Parser accepts `bold=`, `italic=`, `underline=`, `font=`; round-trip | [ ] Extend directive parser/serializer | [ ] `spans: parse font directives` | |

## Phase B2 — Frame variable-height line breaking

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| B2.1 | [ ] §5.4 InsertWithStyle font handling | [ ] Bold/italic/underline render correctly; `FontIdx` switches font; mixed-flag run on one line | [ ] Extend frame render path to honor font flags | [ ] `frame: render font flags` | Still constant-height if `FontIdx` is fixed. |
| B2.2 | [ ] §5.4 SetStyleRange line-height recompute | [ ] Range flip changes line height; reflow correct; scroll math holds | [ ] Per-line height tracking; recompute on font deltas | [ ] `frame: variable line-height in SetStyleRange` | The substantive piece of Slice B. |
| B2.3 | [ ] §13.3 perf | [ ] Plain-text Insert throughput within 5% of upstream after this change | [ ] Profile + optimize hot path | [ ] (only if regression observed) | Confirm the new code didn't slow the fast path. |

## Phase B3 — (optional) heading-only `md2spans`

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| B3.1 | [ ] §11 `md2spans` (headings + emphasis only) | [ ] Golden tests on minimal markdown | [ ] Subset implementation | [ ] `cmd/md2spans: heading + emphasis (slice B)` | Skip and roll into Slice C if cleaner. |

**Slice B exit criterion.** Body text carries mixed bold, italic,
underline, and font sizes; line heights adapt. Slice A producers
(`edcolor`) still work. `./regression.sh` green.

---

# Slice C — Replaced elements and block context

Builds on Slices A and B. Adds the remaining §5.3 fields and the
layout machinery they require.

## Phase C1 — Replaced rendering

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| C1.1 | [ ] §5.3 Replaced fields | [ ] `IsZero()` accounts for new fields | [ ] Extend `frame.Style` with `Replaced`, `ReplacedWidth`, `ReplacedHeight`, `ReplacedKind`, `ReplacedRef` | [ ] `frame: extend Style with replaced-element fields` | |
| C1.2 | [ ] §5.4 replaced-rune rendering | [ ] Width/height honored; line height bumped; single-rune line break; click-to-charofpt inside element | [ ] Render path for `Replaced=true`; unbreakable single-character layout | [ ] `frame: render Replaced runes` | |
| C1.3 | [ ] §6.4 `b` directive | [ ] Parser accepts `b <off> <len> <kind> <w> <h> <ref>`; round-trip | [ ] Extend directive parser | [ ] `spans: parse b (replaced-element) directives` | |

## Phase C2 — Tall-element y-offset

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| C2.1 | [ ] §5.4 `SetOriginYOffset` real behavior | [ ] Non-zero yPx clips top of tall element; clamped to 0 for non-tall; reset to 0 on `Delete(0, *)` | [ ] Replace A2.3 stubs | [ ] `frame: SetOriginYOffset clips tall elements` | |
| C2.2 | [ ] §7.5 `computeTallElementYOffset`, `tallY` state | [ ] `setorigin` emits correct y-offset for tall-element scrolls | [ ] Add helper + state to `Text` | [ ] `text: tall-element y-offset state` | |

## Phase C3 — Image cache

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| C3.1 | [ ] §15 item 4 cache scope | [ ] LRU eviction, cache hit/miss, decode correctness | [ ] LRU cache; consult from Replaced render path; inject via Init option | [ ] `frame: image cache for replaced elements` | Default to global scope unless profiling argues otherwise. |

## Phase C4 — Block context

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| C4.1 | [ ] §5.3 block-context fields | [ ] `IsZero()` accounts for new fields | [ ] Extend `frame.Style` with `BlockquoteDepth`, `InCodeBlock`, `InTable` | [ ] `frame: extend Style with block context` | |
| C4.2 | [ ] §5.4 line-breaker indent | [ ] Blockquote nesting indents; code block continues across lines; table layout | [ ] Line breaker honors block-context indent | [ ] `frame: block-context indent in line breaker` | |

## Phase C5 — Horizontal scroll for wide replaced elements

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| C5.1 | [ ] §5.3 `HOffset` | [ ] `IsZero()` accounts | [ ] Extend `frame.Style` with `HOffset` | [ ] `frame: add HOffset to Style` | |
| C5.2 | [ ] §10.2 routing | [ ] `HScrollAt` hit-tests correctly; wheel over wide element updates `HOffset` (clamped); no vertical scroll | [ ] Add `Frame.HScrollAt`; route wheel in Text | [ ] `text: route wheel to wide replaced elements` | |
| C5.3 | [ ] §10.2 optional widget | [ ] Click/drag on widget updates `HOffset` | [ ] Render thin scrollbar at element bottom | [ ] `frame: per-element horizontal scrollbar widget` | Optional; ship if low cost. |

## Phase C6 — Producer rewrites (`md2spans`, `dirthumb`)

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| C6.1 | [ ] §11 `md2spans` (full) | [ ] Golden-output tests on sample markdown (qualify under §13.1) | [ ] Clean-room re-impl as 9P client of spans file | [ ] `cmd/md2spans: clean-room rewrite (full)` | Supersedes B3 if it landed. |
| C6.2 | [ ] §11 `dirthumb` | [ ] Directory listing → thumbnail directives | [ ] Clean-room re-impl | [ ] `cmd/dirthumb: clean-room rewrite` | |

## Phase C7 — Polish

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| C7.1 | [ ] §12 Slice C drag-scroll past edge | [ ] Drag past edge scrolls plain and styled identically | [ ] Move logic from per-mode path into Text | [ ] `text: unify drag-scroll past edge` | |
| C7.2 | [ ] §9.2 sub-element drag for very tall images | [ ] Reconsider only if real workflows demand | [ ] Deferred until called for | [ ] (no commit by default) | Explicitly *not* in v1 per §9.2. |
| C7.3 | [ ] §13.3 performance baselines | [ ] Plain-text within 5% of upstream; styled within 25% of plain; `GetStyleRuns` p99 < 50 µs on 10 K-region store | [ ] Profile, optimize, record numbers in working log | [ ] (only if work required) | |

**Slice C exit criterion.** Markdown bodies render with the full
§5.3 `Style` surface. `md2spans` and `dirthumb` ship with golden
tests. Slice A and B producers still work. `./regression.sh`
green; §13.3 baselines met.

---

## Cross-slice invariants

Every commit on this branch must keep these green:

1. `./regression.sh` (mirrors CI).
2. Plain-text behavior identical to upstream — measured by
   upstream's own test suite continuing to pass without
   modification.
3. Observer order: `spans.Store` registers on the buffer *before*
   any `Text` (§4 numbered diagram, §8.1).
4. No mode flags on `Window` (§2 non-goal, §8.2). Body styling
   presence is a property of `t.spans != nil` and
   `!t.spans.Empty()`.
5. No parallel mouse-input loop (§2, §9). All body mouse input
   goes through `Text.Select`.

## Bug classification (Stage 4) reminder

When a test fails on this branch, classify before fixing:

- **Implementation accident** — code does not match the design;
  fix the code.
- **Undefined behavior** — design is silent on this case; pause,
  decide, update the design doc, then fix the code.
- **Wrong design** — design says X but reality demands Y; pause,
  discuss with the user, update the design doc, then fix the
  code.

The fix starts at the earliest affected stage, not at the code.
