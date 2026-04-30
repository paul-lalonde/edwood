# `cmd/md2spans` — minimal external markdown → spans tool — Design

## Purpose

Phase 2 of the markdown-externalization plan
([../../docs/designs/features/markdown-externalization.md](../../docs/designs/features/markdown-externalization.md)).
Build the first external tool that consumes markdown and emits
spans-protocol output, establishing the externalized rendering
pipeline that future Phase 3 rounds will enrich.

`md2spans` v1 is deliberately minimal. The current spans protocol
can only express per-rune Fg/Bg/Bold/Italic plus replaced-box
mechanisms. v1 handles only what fits naturally in that protocol:
paragraph text, emphasis (italic / bold / bold-italic), and link
text rendered as colored runs (URL dropped).

Headings, lists, blockquotes, code blocks, tables, horizontal
rules, and images are deferred to Phase 3 rounds — each one waits
for the protocol primitive that expresses it. v1 does not error
on those constructs; it falls through to plain text so any
markdown document renders *something* through the tool.

## Why a hand-rolled parser

Three options for v1's parser were considered (per the
markdown-externalization design doc, "open questions"):

1. **Use the in-tree `markdown/` package.** Rejected — couples
   `md2spans` to the package we're trying to externalize, makes
   the tool internal-by-dependency rather than external.
2. **Use a third-party CommonMark library** (e.g. goldmark).
   Rejected for v1 — heavy for v1's tiny scope (paragraphs +
   emphasis + links). Revisit when Phase 3 rounds need
   features beyond what's small to hand-roll.
3. **Hand-roll a minimal parser.** **Chosen.** v1 needs ~150 LOC
   of parser; self-contained; demonstrates that md2spans can be
   independent of the codebase it's meant to displace.

If Phase 3 rounds (especially the table round) push parser
complexity past a reasonable threshold, we revisit and bring in
a CommonMark library.

## Architecture: acme-integrated, like `edcolor`

v1 follows `edcolor`'s shape (`cmd/edcolor/main.go`):

- Reads the `$winid` environment variable (set by edwood when a
  B2 command spawns a subprocess).
- Connects to the window via `9fans.net/go/acme`.
- Reads the window body (the markdown source).
- Parses → spans-protocol output.
- Writes the spans-protocol bytes to the window's `spans` file.
- Watches for body-edit events; re-parses and re-writes spans
  with a small debounce (mirrors `edcolor`'s edit-watch loop).
- Exits when the window is closed.

This **differs from a pure-filter shape** (which would just
emit to stdout). The acme-integrated shape is chosen so that v1
is immediately user-testable in edwood — open a markdown file,
invoke `md2spans` from the tag, observe the styled output, and
compare side-by-side with edwood's internal markdown path.

The pure parser logic (markdown bytes → `[]StyleRun`) is kept
**separable** from the acme-client driver so v1's tests can
exercise the parser without 9P plumbing. The acme client is a
thin wrapper around the parser.

The body of the window is the markdown source (what the user
typed). v1 does NOT replace the body with rendered output —
that would lose the source the user is editing. Spans style the
existing body in place. (This means literal `*` / `**` / `_` /
`__` / `[text](url)` markers remain visible in the body. They
get colored / italicized, but the markup characters themselves
are still there. v1 accepts this as a known limitation; future
rounds may decide to hide markup runes via the `Hidden` flag
that the protocol already supports.)

## Requirements

R1. **Invocation.**
    - `md2spans` (no args) — read `$winid`, attach to that
      window, watch the body, emit spans on each edit.
    - `md2spans -h` / `--help` — print brief usage and exit 0.
    - `md2spans -once` — attach via `$winid`, render once,
      exit. Useful for testing and for non-watch use.
    - Any other invocation: print usage to stderr, exit 2.
    - Missing `$winid`: print error to stderr, exit 1.

R2. **Spans-protocol output.** v1 emits zero or more `s` lines
    written to the window's `spans` file in a single write,
    matching `spanparse.go:parseSpanLine`'s expected format:

    ```
    s <offset> <length> <fg> [bg] [flags...]
    ```

    Where:
    - `<offset>`, `<length>` are byte offsets and byte lengths
      into the window body's UTF-8 representation. (R7 below
      confirms this against the in-tree parser.)
    - `<fg>`, `<bg>` are lowercase 6-digit hex `#RRGGBB`, or
      `-` for "default".
    - `<flags>`: `bold`, `italic`. v1 only emits these two.
      Bold-italic is `bold italic`.

    A `c` line on its own (alone in the write) clears all spans
    for the window. v1 sends a `c` followed by zero or more
    `s` lines on each render to fully replace prior styling.

R3. **Plain paragraph text.** Consecutive non-blank lines form a
    paragraph; paragraphs are separated by one or more blank
    lines. v1 produces NO spans for plain text — it inherits the
    window's default fg/bg. Paragraph structure does not need
    span output; the body is unchanged.

R4. **Emphasis.** v1 supports CommonMark-style emphasis:
    - `*text*` and `_text_` → italic span, fg `-`, flag
      `italic`. Offset and length cover the *text* between
      the markers (not the markers themselves).
    - `**text**` and `__text__` → bold span.
    - `***text***` and `___text___` → bold-italic span.

    The marker characters (`*`, `_`) remain in the body. The
    span styles only the inner text. A future "hide markup"
    pass could collapse them via the `Hidden` flag.

    Emphasis is intra-paragraph; it does not span paragraph
    breaks. Mismatched / unclosed emphasis falls through as
    literal text (no span emitted).

    **v1's matcher is greedy and non-CommonMark-compliant.**
    It pairs delimiters by adjacency, not by CommonMark's
    flanking rules. Tests document the divergence.

R5. **Links.** v1 supports CommonMark inline links of the form
    `[text](url)`. The link text is emitted as a styled run
    with `Fg = #0000cc` (link blue), no flags. The URL is
    dropped — no protocol primitive yet for "this run's URL is
    X". The `[` and `]` markers and the `(url)` portion remain
    in the body (same reasoning as emphasis markers).

    Reference links and autolinks are not supported in v1
    (treated as literal text).

R6. **Pass-through fallback.** Any markdown construct not
    recognized in v1 (headings starting with `#`, list items
    starting with `- ` / `1. `, blockquotes starting with
    `> `, code blocks fenced or indented, horizontal rules,
    tables, images) emits NO span. The text remains in the
    body unchanged with default styling.

    This means a markdown file with a heading "## Goal"
    renders as the literal text `## Goal`, unstyled. v1 is
    honest about its limited scope; Phase 3 rounds add real
    heading support.

R7. **Byte vs. rune offsets.** Spans emitted use byte offsets
    into the body's UTF-8 byte stream. This is what
    `xfid.go:parseSpanMessage` and the gap-buffered
    `SpanStore` consume. Verify at iterate time by reading
    `spanparse.go:parseSpanLine` and the call chain that
    interprets `offset` / `length` against the window body.

R8. **Watch loop.** v1 watches the window's `event` file for
    body edits (acme `Mu` / similar). On each edit, after a
    short debounce (200 ms — match `edcolor`), re-read body,
    re-parse, re-emit spans. Exits when the window closes
    (the event channel returns a close).

R9. **Tests.** v1 splits cleanly into a parser layer and an
    acme-client layer. Tests cover:
    - Parser: input bytes + expected `[]StyleRun` (offset,
      length, style). Plain paragraph; one italic; one bold;
      bold-italic; link; emphasis adjacent to link;
      mismatched emphasis (literal pass-through);
      pass-through heading.
    - Emitter: `[]StyleRun` + body length → expected
      spans-protocol byte string. Includes the leading `c`
      to clear prior styling.
    - CLI: `-h` exits 0, bad args exit 2, missing `$winid`
      exits 1. (Exec-based test.)
    - Acme integration: deferred to manual smoke testing in
      edwood. The watch loop and 9P layer are exercised by
      running `md2spans` against a real edwood window during
      development.

## Layout

```
cmd/md2spans/
├── md2spans.design.md   ← this file
├── main.go              ← CLI dispatch, $winid handling, watch loop, acme calls
├── parser.go            ← markdown → []StyleRun
├── parser_test.go       ← unit tests for parser
├── emit.go              ← []StyleRun → spans-protocol bytes
├── emit_test.go         ← unit tests for emitter
├── main_test.go         ← exec-based CLI tests
└── README.md            ← usage + scope notes (Phase 2.5)
```

## Non-goals

- Headings, lists, code blocks, blockquotes, tables, horizontal
  rules, images — Phase 3 rounds.
- Source map (markdown source ↔ rendered offsets). v1's spans
  are emitted directly against body offsets; no separate map
  needed for v1's coverage. Source map becomes relevant when
  the body and rendered representations diverge (e.g. when
  markup runes are hidden).
- File-arg invocation (`md2spans <file>`). Could be added as
  a debug mode but doesn't fit the acme-integrated shape;
  defer.
- Color customization — `#0000cc` for links is hard-coded.
- Streaming output / partial re-render. Each watch tick does a
  full body re-parse and full spans replacement.

## Risks

1. **Byte vs. rune offset confusion** (R7). If the spans
   protocol elsewhere assumes runes, byte offsets mis-position
   styled runs on multi-byte content. Verify before committing.
2. **Hand-rolled parser bugs.** Greedy emphasis matching
   diverges from CommonMark. Document divergences in tests.
3. **Watch loop / acme event semantics.** edcolor's loop is
   200 ms debounced; emulate exactly until we have reason to
   diverge. The `9fans.net/go/acme` package abstracts most of
   the 9P plumbing.
4. **Markup-rune visibility.** v1 leaves `*`, `**`, `[`, `]`,
   `(`, `)`, etc. visible in the body. Side-by-side comparison
   with the internal preview will look different — internal
   preview hides these. v1 accepts this as a known difference
   so the user can compare structural rendering of styled runs.

## Status

Design — drafted. Awaiting review.
