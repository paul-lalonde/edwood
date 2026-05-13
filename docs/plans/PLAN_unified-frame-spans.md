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
| A2.2 | [x] §5.4 `SetStyleRange` (no line-height recompute) | [x] Re-style updates storage and repaints; partial range; mid-box split; out-of-range/Len-mismatch panic; empty range no-op; selection bounds unchanged | [x] Add `SetStyleRange` on `Frame`; box walk with mid-box `splitbox` at style boundaries; `repaintBoxRange` helper (always clears each box's bg); `clean` merges adjacent same-Style boxes after | [x] (pending commit) — `frame: add SetStyleRange (color-only)` | Line-height recompute waits for Slice B. Selection-overlap repaint deferred (consistent with A2.1). |
| A2.3 | [x] §5.4 `SetOriginYOffset` / `GetOriginYOffset` (stubs) | [x] `Get` returns 0 on fresh frame; `Set(7)` leaves `Get()` at 0 | [x] Add stubs on Frame and `frameimpl`; MockFrame entries | [x] (pending commit) — `frame: add SetOriginYOffset/Get stubs` | Real behavior in Slice C (C2). |

## Phase A3 — Spans package

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| A3.1 | [x] §6.1, §6.3 (`Store`, `GetStyleRuns` shape); switched internal layout from sparse to dense full-coverage | [x] Empty defaults, plain coverage still Empty, SetRegion makes non-empty, ClearRegion + SetRegion(plain) restore Empty, single/multi region, full-coverage invariant table-test, overlap, partial clear splits, Snapshot sorted | [x] `spans/` package; sorted `[]Region` covering `[0, totalLen)`; `Empty`/`GetStyleRuns`/`SetRegion`/`ClearRegion`/`Snapshot`; `newStoreWithLen` test helper | [x] (pending commit) — `spans: introduce Store with GetStyleRuns` | Dense full-coverage chosen over sparse for simpler GetStyleRuns synthesis and uniform observer rule (per design discussion). |
| A3.2 | [x] §6.2 `Inserted` / `Deleted` observer rules under dense layout | [x] Inserted: empty store, end-of-buffer, mid-region, leading-edge with prev, leading-edge with plain region 0, leading-edge with non-plain region 0. Deleted: contained, straddles-left, after-shifts, before-shifts, wraps. Integration via real ObservableEditableBuffer. | [x] Add `Inserted` / `Deleted` on `*store`; `NewStore` calls `buf.AddObserver`; dense full-coverage invariant maintained via shift + coalesce | [x] (pending commit) — `spans: maintain index across buffer mutations` | First implementation had a bug in the "plain region 0 leading-edge extension" case — extended region 0 but failed to shift subsequent regions, breaking the dense invariant. Caught by integration test, fixed. |
| A3.3 | [x] §6.1 `Observe` API | [x] Fires on SetRegion / ClearRegion with (p0, p1); not fired by Inserted / Deleted; supports multiple observers; empty range is no-op (no fire) | [x] Add `observers []func(p0, p1 int)` to store; `Observe(fn)`; `notify` called at end of SetRegion (ClearRegion delegates through) | [x] (pending commit) — `spans: add style-change Observe callback` | Buffer-driven shifts are bookkeeping only — Inserted / Deleted do not fire. |

## Phase A4 — Text wiring (no producers)

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| A4.1 | [x] §7.1, §7.2, §8.1; also discovered the `file.observers` map gave non-deterministic firing order (fixed in prep commit `9c5262f`) and that `HasMultipleObservers` needed an aux-observer carve-out so spans doesn't false-positive as a clone | [x] Body has non-nil spans; tag has nil; spans registered before Text on the body buffer's observer chain | [x] Add `Text.spans` field + `attachSpans` helper; in `initHeadless` build `spans.NewStore(f)` and attach BEFORE `f.AddObserver(&w.body)`; mark `spans.Store` as `file.AuxiliaryObserver` so `HasMultipleObservers` excludes it | [x] (pending commit) — `text: thread spans store through Window construction` | Tags get `nil` (§8.4). |
| A4.2 | [x] §7.3 `Inserted`, §7.4 `Deleted` | [x] nil spans → InsertByte; empty spans → InsertByte; non-empty spans → InsertWithStyle with GetStyleRuns(q0, q0+nr); styles propagate correctly; plain-range insert still uses InsertWithStyle (frame fast-paths internally) | [x] In Text.Inserted, branch on `t.spans != nil && !t.spans.Empty()`; convert bytes→runes and call `t.fr.InsertWithStyle` with `GetStyleRuns(q0, q0+nr)`; otherwise call `t.fr.InsertByte` (unchanged) | [x] (pending commit) — `text: style-aware Inserted` | `Deleted` requires no change. |
| A4.3 | [x] §7.5 `fill` / `setorigin` | [x] fill: nil/empty spans → fr.Insert; styled spans → fr.InsertWithStyle with GetStyleRuns. setorigin: same for its internal scroll-forward Insert; SetOriginYOffset called once per setorigin invocation | [x] In `fill`, branch on `t.spans != nil && !t.spans.Empty()` before the frame insert. In `setorigin`, same branch for the scroll-forward Insert path; add `t.fr.SetOriginYOffset(0)` after fill | [x] (pending commit) — `text: style-aware fill and setorigin` | SetOriginYOffset is the A2.3 stub (no-op returning 0); Slice C C2 wires the real `computeTallElementYOffset`. |
| A4.4 | [x] §7.6 `attachSpans` | [x] In-window SetRegion → SetStyleRange with frame-relative args; out-of-window → skipped; partial overlap → clipped; non-zero t.org → offsets converted | [x] Extend `attachSpans` to register an `Observe` callback that clips `[p0,p1)` to `[t.org, t.org+Nchars)`, queries `GetStyleRuns`, and calls `t.fr.SetStyleRange(p0-t.org, p1-t.org, runs)` | [x] (pending commit) — `text: attachSpans helper` | |

## Phase A5 — 9P spans file (color-only directives)

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| A5.1 | [x] §6.4 wire format = published spec (positional, not key=value); Slice A subset (no flags, no `b`, no regions); contiguity + `c`-exclusivity enforced | [x] c-no-args, s positional with - default, fg/bg discriminated by appearance, malformed colors / integers rejected, flags rejected, b rejected, regions rejected, ParseAll enforces contiguity + c-exclusivity | [x] `spans/parse.go` rewritten to match published protocol; first pass used invented key=value format that was incompatible with the prior `edcolor` and the published paper | [x] (pending commit) — `spans: align parser with published protocol` | Published spec is authoritative; we can't change it. Prior commits 6e4e14e + 4807fe5 + e95199e are superseded by this rework. |
| A5.2 | [x] §8.3 `QWspans` qid | [x] write applies single/multi directives; set / clear paths; color resolution observed in store; bad directive → error; nil spans → error; bg-only path | [x] `QWspans` in dat.go enum + fsys.go dirtab; xfid read = empty stub; xfid write → `xfidspanswrite` → `writeSpansToStore` (testable helper); `allocColorImage` resolves color.Color → draw.Image | [x] (pending commit) — `xfid: add QWspans qid` | Read is a stub (serialization deferred per A5.1 note); open/close nopen tracking deferred until A6 needs it. |
| A5.3 | [x] Integration | [x] writeSpansToStore → SetRegion → A4.4 Observe → frame.SetStyleRange with correct frame-relative offsets and KindColored runs | [x] Single integration test that exercises the full chain via the recording mock frame | [x] (pending commit) — `text: integrate spans file with producer flow` | xfid I/O path itself isn't end-to-end tested (would need an Xfid scaffold); we test the apply path which is the hard part. Smoke test via prior `edcolor` is a manual follow-up. |

## Phase A6 — 'S' event

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| A6.1 | [x] §9.3 emission conditions | [x] Fires on body selection change with spans attached and listener open; suppressed on tag, nil spans, no listener, unchanged selection; fires again on subsequent change | [x] `Text.SetSelect` saves old (q0, q1), then after the update calls `t.w.Eventf("S%d %d 0 0 \n", q0, q1)` if `t.what == Body && t.spans != nil && selection changed`. The listener gate (`nopen[QWevent] > 0`) is enforced by Eventf itself | [x] (pending commit) — `text: emit S event on selection change` | Format matches the published event-file vocabulary (single-char prefix + four space-separated fields). |

## Phase A7 — `edcolor` clean-room rewrite — **SKIPPED**

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| A7.1 | [skip] §11 `edcolor` contract | n/a | n/a | n/a | Skipped — existing external `edcolor` (from the upstream tree at `/Users/paul/dev/edwood/cmd/edcolor`) is reused. Slice A's wire-format compliance (the protocol-rework commit and silent-flag-accept commit) makes the external tool work against the cleanroom edwood end-to-end. |
| A7.2 | [skip] §9.3 'S'-driven highlight | n/a | n/a | n/a | Same: the external `edcolor` already implements the `S`-driven highlight pattern; A6's emission in `Text.SetSelect` is the cleanroom side of that contract. |

**Slice A exit criterion (as actually shipped).** The cleanroom
`edwood` binary runs the external `edcolor` (and `md2spans`,
once Slice B / C land) without modification. Plain text and tag
bars are byte-identical to upstream. `./regression.sh` green.
The S event fires correctly; the spans store updates on
producer writes; the frame repaints styled runs. A7 (a
cleanroom rewrite of `edcolor`) is deferred indefinitely — the
external tool serves our needs.

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
| B2.1 | [x] §5.4 InsertWithStyle font handling | [x] Bold/italic render correctly; mixed-flag run on one line | [x] Extend frame render path to honor font flags | [x] (landed in Slice B B1 + B4) | Still constant-height. |
| B2.2 | (restart — see B2.2.Rx rows below) | | | | The substantive piece of Slice B. First attempt at this row used walk-time `f.curLineH` state; was abandoned and reverted (tag `b22-attempt-1`) when the walk-coherence assumption broke. Restart uses per-box `Y`/`LineH`/`LineA` fields with a single forward layout pass and walks-as-readers. Design lives at `docs/designs/features/frame-rendering-spec.md`. |
| B2.3 | [ ] §13.3 perf | [ ] Plain-text Insert throughput within 5% of upstream after this change | [ ] Profile + optimize hot path | [ ] (only if regression observed) | Confirm the new code didn't slow the fast path. |

### Phase B2.2 (restart) — per-box-Y architecture

| # | Design | Tests | Iterate | Commit |
|---|---|---|---|---|
| B2.2.R1 | [x] frame-rendering-spec §2.1 (box fields) | [x] `frbox.Y`, `frbox.LineH`, `frbox.LineA` exist; init populates LineH = LineA = defaultfontheight; existing tests still green with no behavior change | [x] Add `Y`, `LineH`, `LineA` int fields to `frbox`; initialize in `bxscan` and `addifnonempty` via setBoxLineDefaults; no consumer reads them yet | [x] `frame: per-box Y/LineH/LineA fields (no behavior change)` (642ef2f) |
| B2.2.R2 | [x] frame-rendering-spec §5 (layout pass) | [x] After Insert / Delete / SetStyleRange, every box's `Y` equals the visual-line top Y; `LineH` / `LineA` equal `defaultfontheight` (constant pre-R4); multi-line content has monotonic `Y` | [x] Added `(*frameimpl).relayoutFrom(nb0)` in relayout.go (two-phase per-line pass); wired into insertbyteimpl, deleteimpl, SetStyleRange | [x] `frame: single-pass layout populates per-box geometry` (266a454) |
| B2.2.R3 | [x] frame-rendering-spec §5.1 + §6 (walks as readers; partial — see narrowed scope below) | [x] Public Ptofchar / Charofpt / SetStyleRange-driven repaintBoxRange read box.X / box.Y / box.LineH; sentinel-poke tests confirm the seam is load-bearing | [x] Added ptOfCharReader and charOfPtReader; switched public Ptofchar / Charofpt to use them; repaintBoxRange passes (b.X, b.Y). Legacy accumulator kept on the mutation-internal paths (drawtext on nframe, ptofcharptb mid-deleteimpl, _draw) because those run on un-relayouted box state | [x] `frame: external Ptofchar / Charofpt / repaintBoxRange read box geometry` (2f4fa52) |
| B2.2.R4 | [x] frame-rendering-spec §2.2 + §5.2 | [x] Style.Scale + KindScale exist; spans parser sets both; OptScaleFonts installs map; fontFor dispatches scale > family > weight > base; updateLineMaxes uses fontFor heights; plain fast path unchanged | [x] Scale+KindScale added; OptScaleFonts; fontFor extended; updateLineMaxes method form; bxscan propagates fontByScale to nframe so child-Wid agrees with parent validate | [x] `frame: honor scale=N.N via per-style scale-font map` (07380ba) |
| B2.2.R4.1 | [x] frame-rendering-spec §6.3 (bg uses LineH); per-mutation paint-clear policy | [x] Text loads md2spans scaled fonts via fontsrv path surgery; SetStyleRange mutates→cleans→relayouts→paints (order fixed); contentBottomY snapshot detects vertical shifts and triggers full clear+repaint, otherwise narrow; paintBox bg uses box.LineH and bails when fully off-screen; relayoutFrom no longer truncates at rect.Max.Y so shift detection sees true extent; Frame.LineHAt + Text.paintSpansOverlay sized correctly per-segment | [x] scaledFontPathFor + tryLoadScaledFont in acme.go; loadScaleFonts builds the OptScaleFonts map from md2spans's heading-scale set; Text.Init threads it through frame.NewFrame; SetStyleRange reordered with content-shift detection; paintBox + relayoutFrom adjusted; LineHAt on Frame interface | [x] `text: wire scaled fonts; SetStyleRange clears on vertical shift` (c7789c6) |
| B2.2.R5 | [x] frame-rendering-spec §3.3 + §6.3 | [x] paintBox paints at pt.Y + (LineA - fontAscent(box)); single-font lines unchanged; mixed-ascent lines share a baseline | [x] draw.Font gained Ascent(); updateLineMaxes uses Ascent; setBoxLineDefaults seeds LineA = base.Ascent; bxscan child runs relayoutFrom so paintBox sees fresh LineA | [x] `frame: baseline-align glyph paint within line` (33e71e4) |
| B2.2.R6 | [ ] frame-rendering-spec §4.8 (tick) | [ ] `Tick(pt, ticked)` reads `box.LineH` for the containing box's line height; the tick rendered on a heading line is the heading's full height; tick on a plain line is `defaultfontheight` | [ ] In `tick`, look up the box containing `pt` (via `Charofpt`+findbox or direct walk), read its `LineH`; fall back to `defaultfontheight` if no box at pt | [ ] `frame: tick honors per-line height` |
| B2.2.R7 | [ ] frame-rendering-spec §4.2 + scrollbar-spec | [ ] `setorigin`'s blit math uses sum of `box.LineH` over the deleted range (not `n * defaultfontheight`); `SetOriginYOffset(yPx)` clips the top line by yPx; scroll across a mixed-height region (heading + body) aligns the new top line to `rect.Min.Y`; smoke: scroll a markdown file with headings, no visual misalignment | [ ] In Frame `Delete`'s blit-coordinate math, use a per-line height accumulator; honor `SetOriginYOffset` in `_draw` (currently a stub) | [ ] `frame: scroll math uses per-line heights` |

**Phase B2.2 exit criterion.** Headings render at their
scaled size; a heading and adjacent body share a common
baseline; tick and scroll behave correctly across mixed-
height lines; the Spans overlay outlines headings (because
`scale=` now sets `KindScale`, making the region non-plain).

## Phase B3 — (optional) heading-only `md2spans` — **SKIPPED**

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| B3.1 | [skip] §11 `md2spans` (headings + emphasis only) | n/a | n/a | n/a | Skipped — the external `md2spans` from `/Users/paul/dev/edwood/cmd/md2spans` is reused once Phase B4 lands the parser surface and the rendering for `hrule` / `family=code`. |

## Phase B4 — md2spans compatibility (parser surface + small render wins)

The remaining work to make external `md2spans` produce useful output
end-to-end. See design §12 Phase B4 for R-B4.1..R-B4.11.

| # | Design | Tests | Iterate | Commit | Notes |
|---|---|---|---|---|---|
| B4.1 | [ ] §6.4 + R-B4.1, R-B4.2, R-B4.3, R-B4.4 | [ ] `b OFF LEN ...` → `OpNoOp`; `begin region X k=v` → `OpNoOp`; `end region` → `OpNoOp`; malformed `b` (short field count) still errors; contiguity finds previous `OpSetStyle` across `OpNoOp`s; `family=code` sets `Kind & KindCodeFamily`; `hrule` sets `Kind & KindHRule`; existing `bold`/`italic`/`hidden` translation unchanged | [ ] Add `OpNoOp` to `spans.Op`; rewrite `ParseDirective` switch on `b`/`begin`/`end` to construct `OpNoOp`; teach `ParseAll` contiguity to skip `OpNoOp`; teach `parseSet` flag loop to set new Kind bits | [ ] `spans: silently accept b/region directives; translate hrule and family=code` | All parser changes. |
| B4.1.5 | [ ] R-B4.12 (paintBox); R-B4.13 (boxWid) | [ ] `paintBox` is called once per styled paint of each content box from both `drawtext` and `repaintBoxRange`; visual baseline (existing tests) byte-identical pre/post; `boxWid(b)` returns the same value as the prior 8 inlined sites for every test fixture; `validateboxmodel` panics when a content box's `Wid` doesn't match `boxWid(b)`; the §13.2 frame test suite continues green | [ ] Extract `(*frameimpl).paintBox(b, pt, text, back)` consolidating font lookup + color resolution + bg + glyph + KindHidden suppression; reduce `drawtext` and `repaintBoxRange` to walk-and-call loops; extract `(*frameimpl).boxWid(b *frbox) int`; replace inline `fontFor(b.Style).BytesWidth(b.Ptr)` at insert.go:23, box.go:87/100/199, util.go:34, ptofchar.go:23/100, draw.go:238 with `boxWid` calls; extend `validateboxmodel`'s width-equality check to use `boxWid` | [ ] `frame: extract paintBox and boxWid helpers (refactor)` | Pure refactor; no behavior change. Lands before B4.2 so the new bits attach to one site. |
| B4.2 | [ ] §5.3 + R-B4.5; §5.1 + R-B4.6; §5.4 + R-B4.7, R-B4.8, R-B4.9 | [ ] `KindHRule` and `KindCodeFamily` bits exist at the §5.3 positions; `OptCodeFont` installs a fontCode slot; `fontFor` returns it for `KindCodeFamily` and falls back to base when not configured; `KindCodeFamily` combined with `KindBold` still picks fontCode (no bold-code variant in v1); a box with `KindHRule` produces a `Draw(rect, fg, ...)` op of 1-pixel height across the box's rect at the row's vertical center; the glyph paint still happens; `repaintBoxRange` paints the rule on re-style too | [ ] Add `KindHRule`/`KindCodeFamily` to `frame/style.go`; add `fontCode` field + `OptCodeFont`; extend `fontFor` to consult code first; in `paintBox` (post-B4.1.5), after the glyph paint, draw the hrule line if the bit is set | [ ] `frame: render hrule and family=code` | Small now — single edit in `paintBox` for hrule plus the font-lookup extension. |
| B4.3 | [ ] §7 + R-B4.10 | [ ] `tryLoadFontVariant` returns the GoMono variant for a GoRegular base; integration test (or smoke description in working log) showing the frame Init receives the code font when the base font matches a known family | [ ] Extend `acme.tryLoadFontVariant` and `acme.go` font-loading site to probe + thread `OptCodeFont` through `frame.Init` | [ ] `text: thread code-family variant font through Init` | Text/acme wiring. |

**Phase B4 exit criterion.** External `md2spans` runs against the
cleanroom edwood end-to-end; bold/italic/links/hrule/family=code
all render. Slice A and Slice B producers unaffected.
`./regression.sh` green.

## Phase B5 — Word-boundary line wrapping

Soft-wrap inside paragraphs currently breaks mid-word because
bxscan emits one box per style run. Split content boxes at
U+0020 SPACE boundaries; `cklinewrap`'s existing wrap test
naturally produces word-boundary breaks. Design §12 Phase B5
for R-B5.1..R-B5.7.

| # | Design | Tests | Iterate | Commit |
|---|---|---|---|---|
| B5.1 | [x] §12 + R-B5.1, R-B5.2 | [x] bxscan on plain "one two three" emits 3 word boxes + 2 space boxes; clean does NOT merge a word and an adjacent space; clean DOES merge two adjacent space-only boxes | [x] bxscan's content branch flushes wipbox at every space ↔ non-space transition; helper `isSpaceOnlyBox(b)` for clean's merge predicate | [x] `frame: split content at spaces in bxscan` (cce20b4) |
| B5.2 | [x] §12 + R-B5.3, R-B5.4 | [x] A line whose rightmost word doesn't fit wraps just before that word, not mid-word; a single line-width-exceeding word wraps to its own line and overflows | [x] cklinewrap0 wraps whole content box when it doesn't fit; canfit+splitbox handles long-word fallback on the fresh line | [x] `frame: word-boundary line wrap in cklinewrap0 (B5.2)` (042fdc0) |
| B5.3 | [ ] §12 + R-B5.5, R-B5.6, R-B5.7 | [ ] SetStyleRange across a previously-split space-boundary correctly styles both halves; selection covers all visible runes; ptofcharptb / charofptimpl round-trip correctly; rewrite the 16 knowntofail TestInsert/TestInsertAligned/TestDelete sub-tests against B5 layout (per design+trial HTML review) | [ ] No production code change expected; scan for callsites assuming "one box per line" via grep | [ ] `frame: regression for word-split layout walks` |

**Phase B5 exit criterion.** Markdown paragraphs wrap at word
boundaries; the bold "**Before writing any code...**" line
wraps before the first word that doesn't fit, not mid-word.

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
