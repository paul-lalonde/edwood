# Frame scrollbar: behavior and invariants

**Status:** stub. Expand when the scrollbar refactor lands on
the plan (Slice C C2 / C5 / C7, or possibly its own phase).
The capture here is the **scroll-direction alignment** rule
that's easy to forget and was the central design insight of
the prior tree's scrollbar work
(`/Users/paul/dev/edwood/docs/designs/features/unified-scrollbar.md`).

---

## Scroll-direction alignment

When the visible viewport contains only a partial line at one
edge of the frame, the partial side depends on the action that
brought the user there. The user's eye expects:

- **Scroll up** (B1, reveal content above) → the **bottom** of
  the viewport is line-aligned; the **top** absorbs the partial.
  Rationale: the new content at the top has its baseline /
  bottom visible (so the user can read the line bottom-up as
  they continue scrolling), while the top of the line is
  clipped and will be filled in by the next scroll-up.
- **Scroll down** (B3, advance through document) → the **top**
  of the viewport is line-aligned; the **bottom** absorbs the
  partial. Rationale: new content arrives top-first; the
  ascender / top of the next line is visible, and the user
  reads down into it.
- **B2 / programmatic** (Look, search, addr expression, etc.)
  → **top** aligned. Predictable landing point.
- **Edge case: file top** (`origin == 0 && offset == 0`) →
  **top** aligned regardless of last scroll direction. The
  user can otherwise never see the first line fully aligned
  at the viewport top.
- **Edge case: tall line larger than viewport** (e.g. an
  image taller than the frame, Slice C C2) → **pixel** scroll
  (no line-boundary alignment). The user scrolls *within* the
  tall line; snapping to a line boundary would defeat the
  feature.

This is the legacy `snapBottomLine` flag from the prior tree
generalized into a per-scroll-action property. Each scroll
handler sets the snap before calling `SetOrigin` /
`SetOriginYOffset`; edge cases override.

### Proposed API (preserved verbatim from prior tree)

```go
type ScrollSnap int

const (
    // SnapTop aligns the first visible line's top to the viewport
    // top. Default for B3, B2, programmatic, and file-top.
    SnapTop ScrollSnap = iota

    // SnapBottom aligns the last visible line's bottom to the
    // viewport bottom; the first line absorbs partial-line clipping.
    // Used after B1 (scroll up) so revealed content has its
    // baseline visible.
    SnapBottom

    // SnapPixel honors originYOffset literally with no
    // line-boundary alignment. Used when the origin line is taller
    // than the viewport (replaced elements).
    SnapPixel
)

// SetScrollSnap configures snap behavior for the subsequent
// SetOrigin / SetOriginYOffset call. Edge cases (file top, tall
// origin line) override.
func (Frame) SetScrollSnap(s ScrollSnap)
```

### Failure modes the rule prevents

- **"Can't scroll back to the top of the first line."** If
  `SnapBottom` is sticky (legacy bug), the first line is
  permanently clipped at `frameHeight % lineHeight` pixels.
  `SnapTop` override at file-top fixes this.
- **"Tall image jumps when I scroll within it."** Line-level
  snap forces tall content to align at line boundaries; pixel
  scroll inside a tall element doesn't work. `SnapPixel`
  override fixes this.
- **"Last visible word is always clipped half-way down."**
  Steady-state mid-document with `SnapBottom` default: the
  first line is partially clipped instead of the last. Switch
  to `SnapTop` default — the *first* line of a freshly-
  rendered viewport reads cleanly, and the trailing
  partial-line clip alternates with each B1 (which switches to
  `SnapBottom`).

---

## Open work (not yet specced)

This doc only captures the alignment rule above. Real
scrollbar work — geometry math, drag semantics, click→line
mapping, latch debounce, hold-to-scroll, B2 jump-to-fraction —
deserves a full design pass. Reference points:

- Prior tree's scrollbar design:
  `/Users/paul/dev/edwood/docs/designs/features/unified-scrollbar.md`
  Detailed acme-canonical drag semantics, the
  `richScrollModel` API, and the rationale for replacing the
  `previewVScrollLatch` (a re-derived-in-pixel-space
  duplicate of upstream's acme drag loop).
- Prior tree's plan: `PLAN_unified-scrollbar.md`.
- Edwood-cleanroom's current scrollbar code: lives in upstream
  `frame/select.go` (drag-scroll) and the Text-side `ScrDraw`
  helpers.

When this work starts, this stub becomes the seed of a fully-
specced design doc analogous to `frame-rendering-spec.md`:
goals, data model, per-operation contracts, algorithms, edge
cases, tests, implementation order, open questions.
