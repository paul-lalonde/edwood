# `md2spans` v1 — Implementation Plan

Build the first external tool that consumes markdown and emits
spans-protocol output, **acme-integrated** (like `edcolor`) so v1
is immediately user-testable in edwood. Phase 2 of the
markdown-externalization plan
([../designs/features/markdown-externalization.md](../designs/features/markdown-externalization.md)).

**Base design**: [`cmd/md2spans/md2spans.design.md`](../../cmd/md2spans/md2spans.design.md).

**Branch**: `md2spans-v1`.

**Phase 2 outcome**: a working `cmd/md2spans/` binary that
attaches to a window via `$winid`, reads markdown, and writes
spans-protocol bytes to the window's `spans` file. Watches body
edits with debounce. Renders paragraphs, emphasis (italic /
bold / bold-italic), and links. Anything beyond falls through
unstyled. The tool is a sibling of `edcolor`, structured the
same way.

**Files created**:
- `cmd/md2spans/md2spans.design.md` — design (this row's input).
- `cmd/md2spans/main.go` — CLI dispatch, `$winid` handling,
  watch loop, acme calls.
- `cmd/md2spans/parser.go` — markdown → `[]StyleRun`.
- `cmd/md2spans/emit.go` — `[]StyleRun` → spans-protocol bytes.
- `cmd/md2spans/parser_test.go` — parser unit tests.
- `cmd/md2spans/emit_test.go` — emitter unit tests.
- `cmd/md2spans/main_test.go` — exec-based CLI tests.
- `cmd/md2spans/README.md` — usage + scope notes.

**Files NOT touched**: nothing in `rich/`, `markdown/`, `wind.go`,
`xfid.go`, or any in-tree edwood code. Phase 2 is purely a new
external tool.

---

## Phase 2.0: Plan + design

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [x] Design | `md2spans.design.md` drafted (acme-integrated, hand-rolled parser) | — | Decisions confirmed: option 1 (acme-integrated from day one) and option 3 (hand-roll for v1). |
| [x] Tests | n/a (planning row) | — | — |
| [x] Iterate | Write this plan + the design | — | This file. |
| [ ] Commit | Commit plan + design | — | `Add md2spans Phase 2 plan and design` |

## Phase 2.1: Acme-client skeleton

`cmd/md2spans/main.go` reads `$winid`, attaches to the window via
`9fans.net/go/acme`, reads the body, and writes a `c` (clear)
line to the spans file — no parsing yet. Verifies the toolchain:
package compiles, `go install` works, the spans file accepts our
write. `-h` and bad-args paths handled.

Cribs heavily from `cmd/edcolor/main.go` for the acme-client
boilerplate.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Confirm CLI surface (R1) and acme attach pattern | `md2spans.design.md` § R1, `cmd/edcolor/main.go` | — |
| [ ] Tests | `main_test.go`: `-h` exit 0; missing `$winid` exit 1; bad args exit 2 | `md2spans.design.md` § R9 | Exec-based; no acme dependency. |
| [ ] Iterate | Write `main.go`: arg parse, `$winid` attach, read body, write `c\n` to spans, exit (no watch loop yet) | `cmd/edcolor/main.go` for the acme idioms | — |
| [ ] Commit | — | — | `md2spans: acme-client skeleton with $winid attach` |

## Phase 2.2: Paragraph parsing (no spans emitted)

Walk lines, group into paragraphs (the body is the user's
markdown source verbatim — we don't transform it). v1 emits no
spans for plain text since the default style suffices; the
parser layer establishes the structure that emphasis (2.3) and
links (2.4) plug into. The clear (`c\n`) is still the only
output to the spans file at this row.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Confirm paragraph rules (R3); pin `Span` / `[]StyleRun` IR for the parser | `md2spans.design.md` § R3 | The parser returns `[]StyleRun` matching the in-tree `spanstore.go:StyleRun` shape (or our local equivalent). |
| [ ] Tests | `parser_test.go`: single paragraph; multi-paragraph; trailing whitespace; empty input. Each input → empty `[]StyleRun`. | `md2spans.design.md` § R3 | — |
| [ ] Iterate | Write `parser.go` with paragraph-grouping; main wires it but emits the same `c\n` (no spans yet) | — | — |
| [ ] Commit | — | — | `md2spans: paragraph parsing scaffold` |

## Phase 2.3: Emphasis (italic / bold / bold-italic)

In-paragraph emphasis: `*x*`, `_x_`, `**x**`, `__x__`,
`***x***`, `___x___`. Emit `s` lines for the styled inner text;
the marker characters remain in the body (R4 design note).
Greedy matcher; document CommonMark divergences in tests.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Pin the emphasis-matcher rules; document divergences from CommonMark spec | `md2spans.design.md` § R4 | Greedy: shortest-closing match by adjacency, not by CommonMark flanking. |
| [ ] Tests | `parser_test.go`: italic-only, bold-only, bold-italic, mixed in same paragraph, unclosed (literal pass-through), nested emphasis (e.g. `*a **b** c*`) | `md2spans.design.md` § R4 | — |
| [ ] Iterate | Add emphasis tokenizer to `parser.go`; write `emit.go` to format `[]StyleRun` as spans-protocol bytes (`c\n` + `s` lines) | — | Span offsets are byte-indexed (R7). |
| [ ] Commit | — | — | `md2spans: emphasis (italic, bold, bold-italic)` |

## Phase 2.4: Inline links

`[text](url)` → link-text styled with `Fg = #0000cc`, URL
dropped. Reference / autolinks are literal text.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Confirm link recognition (R5); decide URL-drop semantics | `md2spans.design.md` § R5 | — |
| [ ] Tests | Link in paragraph middle; link adjacent to emphasis; malformed `[text](no-paren` falls through as literal | — | — |
| [ ] Iterate | Add link tokenizer; emit colored span | — | — |
| [ ] Commit | — | — | `md2spans: inline links rendered as colored runs` |

## Phase 2.5: Watch loop + README

Add the body-edit watch loop to `main.go` so v1 re-renders on
each edit (debounced 200 ms, mirroring `edcolor`). Then the
README with usage examples, the v1 scope summary, and a
"what's not supported (yet)" pointer at Phase 3 round numbers.

| Stage | Description | Read | Notes |
|-------|-------------|------|-------|
| [ ] Design | Confirm watch-loop semantics (R8) match edcolor | `md2spans.design.md` § R8, `cmd/edcolor/main.go` watch logic | Reuse the debounce timer pattern. |
| [ ] Tests | Acme integration is manually-verified (deferred to smoke testing). Parser/emitter tests already cover correctness; the watch loop is timing/event plumbing best validated in a live window. | — | Document the manual-verification protocol in the commit message. |
| [ ] Iterate | Add the watch loop to `main.go`; write `README.md` | — | — |
| [ ] Commit | — | — | `md2spans: watch loop + README` |

---

## After Phase 2

`cmd/md2spans` builds and renders v1's markdown subset live in
edwood. Documents containing unsupported constructs render their
text passably (literal); they gain proper styling as Phase 3
rounds enrich the protocol and the tool.

**Phase 3 round 1** (font scale, headings) is the natural next
work: the protocol gains a `Scale` field on `StyleAttrs`, edwood
renders it, `md2spans` learns to emit it for `# heading` /
`## heading` lines.

---

## Risks for Phase 2

1. **Byte vs rune offset confusion** (R7 in design). Verify
   against `xfid.go:parseSpanMessage` at row 2.1 / 2.2.
2. **Markup runes visible in body** (design § "Architecture"
   final paragraph). v1 leaves `*`, `**`, `[`, `]`, `(`, `)`
   visible; internal preview hides them. Side-by-side
   comparison will look different. Document in README.
3. **Emphasis matcher divergences from CommonMark.** Acceptable
   for v1; tests pin known cases.
4. **Acme integration churn.** `9fans.net/go/acme` semantics
   for body-edit events may differ across edwood / acme
   versions; cribbing edcolor's exact pattern minimizes risk.

---

## Status

Plan + design drafted. Awaiting review for the renumbered
architecture decisions (acme-integrated, hand-rolled). Ready
to start row 2.1 once approved.
