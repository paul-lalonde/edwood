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
| ATX headings (`# … ######`) | ✓ | Phase 3 round 1 — emits `scale=N.N` (H1=2.0 down to H6=1.0). The `# ` markup remains visible in the body. H6 is body-size; the in-tree path also makes it bold for visual distinction (md2spans does not in v1). |
| Inline code (`` `text` ``) | ✓ | Phase 3 round 2 — emits `family=code` over backtick-delimited content. Single-backtick form only; double-backtick form deferred. The backticks remain visible in the body. |
| Horizontal rules (`---` / `***` / `___`) | ✓ | Phase 3 round 3 — emits `hrule` over the marker runes; the renderer keeps the markers visible and draws a horizontal line over the same row (matching the "markup remains visible" stance of every other v1 feature). Simple form only (3+ same-character markers, no internal spaces, no trailing content). v1 may render a setext-heading underline (`---` after a text line) as a rule rather than a heading; users who write `---` between paragraphs may see this. |
| Inline images (`![alt](url)`, `![alt](url "width=Npx")`) | ✓ | Phase 3 round 4 — emits `b OFF 0 0 0 - - placement=below image:URL [width=N]` anchored at the start of the syntax. The image renders BELOW the line containing the source; `![alt](url ...)` text stays visible above (consistent with markers-stay-visible). Width comes from the title attr `width=Npx` if present; otherwise the renderer probes the file via its async cache. md2spans does no file IO — relative URLs are resolved by the consumer against the window's body file path. Inline-replacing form (length>0) is not emitted; users who want it keep using the in-tree markdown path until Phase 4. |
| Fenced / indented code blocks | — | Phase 3 round 5 (regions) |
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
