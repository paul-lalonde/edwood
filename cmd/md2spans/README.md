# md2spans

External markdown → spans renderer for edwood.

Sibling of `cmd/edcolor`. Invoked from edwood as a B2 command in
the window tag (with `$winid` set in the environment); md2spans
attaches to the window via 9P, reads the markdown body, and
writes spans-protocol output to the window's `spans` file.
Watches body edits with debounce so styling tracks the user's
typing.

## Usage

```
md2spans              attach via $winid; render once and watch for edits
md2spans -once        attach via $winid; render once and exit
md2spans -h           print help and exit
```

In edwood: open a markdown file, then add `md2spans` to the
window tag. Click the word with B2. Edits to the body
re-render styled spans (debounced 200 ms).

## v1 scope

| Feature | v1 | Status |
|---|---|---|
| Plain paragraph text | ✓ | Default styling, no spans |
| Italic `*x*` / `_x_` | ✓ | |
| Bold `**x**` / `__x__` | ✓ | |
| Bold-italic `***x***` / `___x___` | ✓ | |
| Inline link `[text](url)` | ✓ | URL dropped, text colored `#0000cc` |
| Headings (`# title`) | — | Phase 3 round 1 (font scale) |
| Inline / fenced code | — | Phase 3 round 2 (font family) |
| Horizontal rules | — | Phase 3 round 3 |
| Images | — | Phase 3 round 4 |
| Block code with bg | — | Phase 3 round 5 (region) |
| Blockquote | — | Phase 3 round 6 (region) |
| Lists | — | Phase 3 round 7 (region) |
| Tables | — | Phase 3 round 8 (region) |
| Reference / autolinks | — | Future |
| Source map (Look) | — | Future |

Anything in the table marked `—` renders as **literal text** in
v1 — markup characters remain visible, no styling applied. v1 is
honest about its limited scope; subsequent rounds add real
support as the spans protocol grows.

## Known v1 differences from CommonMark

These are deliberate divergences in v1, documented so you don't
file them as bugs.

- **Greedy emphasis matcher.** Emphasis runs pair by EQUAL count
  and EQUAL character. `*a *b* c*` produces an italic run on
  `b` (not `a *b` or `a *b* c`). CommonMark's flanking-rune
  rules are not applied; `5*x*` is treated as emphasis.
- **Markup runes remain visible.** `*` / `**` / `_` / `__` /
  `[` / `]` / `(` / `)` are not hidden — they get styled along
  with the surrounding text but are still part of the body.
  Side-by-side comparison with the in-tree preview will look
  different (the in-tree path hides them). Future work may use
  the `Hidden` protocol flag to hide markup.
- **Emphasis inside link text not honored.** `[**bold**](u)`
  styles only the link color, not bold.
- **No reference / autolink support.** `<http://example.com>`
  and `[text][ref]` are literal text in v1.

## Architecture

```
┌─────────────────────┐
│ edwood window body  │ ← markdown source (the user's input)
└──────────┬──────────┘
           │ ReadAll("body")
           ▼
┌─────────────────────┐
│ Parse(src) []Span   │ ← cmd/md2spans/parser.go
└──────────┬──────────┘
           │ FormatSpans
           ▼
┌─────────────────────┐
│ spans-protocol bytes│ ← cmd/md2spans/emit.go
└──────────┬──────────┘
           │ writeSpans (9P)
           ▼
┌─────────────────────┐
│ window's spans file │ → edwood styles the body in place
└─────────────────────┘
```

Edits trigger the watch loop (`cmd/md2spans/main.go`), which
re-runs the pipeline.

## Files

- `main.go` — CLI dispatch, `$winid` handling, acme attach,
  watch loop, 9P write.
- `parser.go` — markdown → `[]Span` (rune-indexed).
- `emit.go` — `[]Span` → spans-protocol bytes.
- `*_test.go` — unit tests for the pure layers (parser, emit,
  CLI). Acme integration is verified by manual smoke testing in
  edwood.
- `md2spans.design.md` — design doc with full requirements (R1-R9).

## Future

Each Phase 3 round of the markdown-externalization plan adds
one feature both to the spans protocol and to md2spans. See
[`docs/designs/features/markdown-externalization.md`](../../docs/designs/features/markdown-externalization.md)
for the round map.

When `md2spans` covers everything the in-tree markdown path does,
edwood will switch its preview default to `md2spans` and delete
the in-tree `markdown/` package and `rich/mdrender` wrapper
(Phase 4).
