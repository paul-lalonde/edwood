# Rich Text, Markdown Preview, and External Syntax Coloring in Edwood

*Paul Lalonde*

![Edwood showing markdown preview (left) and syntax-colored Go source (right).](edwood.png)

## Abstract

Edwood is a Go reimplementation of Rob Pike's acme editor from Plan 9.
This paper describes three recent extensions that add rich text
rendering, live markdown preview, and external syntax coloring to
edwood while preserving acme's central architectural principle: the
editor provides mechanism; external tools provide policy. A new
per-window `spans` file in the 9P filesystem allows external programs
to set foreground color, background color, bold, and italic attributes
on arbitrary ranges of body text. The editor renders styled text
through a new rich text engine that supports proportional fonts,
inline images, and heading scaling. A selection-change event (`S`)
enables tools to react to cursor movement and text selection,
supporting features like matching-occurrence highlighting. We describe
the design and specification of these extensions, present `edcolor`, a
reference syntax coloring tool for Go, Rust and Python, and survey the
current landscape of editor syntax coloring and markdown rendering.

## 1. Introduction: Acme and Edwood

Acme [1] is a text editor and programming environment from Plan 9 [2]
built on two ideas: that text is the universal interface, and that the
editor should be a host for external tools rather than an extensible
application in its own right. Every window in acme exposes a set of
synthetic files through the 9P protocol [3]---`body`, `tag`, `ctl`,
`addr`, `data`, `event`---that external programs read and write to
inspect and modify editor state. This design means that a spell
checker, a debugger, and a mail reader are not editor plugins but
ordinary programs that happen to communicate through the filesystem.

Edwood [4] is a Go port of acme, derived but now very divergent
from the Project Serenity transliteration of the original C source from plan9port. It
preserves the 9P filesystem interface, the mouse-chord command language,
and the three-button interaction model. It is a
functional drop-in replacement for plan9port acme.

Acme has always rendered text in a single height font with
no color, no bold, no italic. This is a deliberate design choice---it
keeps the editor simple and avoids the complexity of rich text
layout---but it means that syntax coloring, one of the most widely
requested features in any programmer's editor, is structurally
impossible. Markdown documents, meanwhile, can only be previewed by
shelling out to an external renderer.

The work described here adds three capabilities to edwood:

1. A **rich text rendering engine** (`rich.Frame`) that supports
   proportional fonts, per-span colors, bold, italic, inline images,
   heading scaling, code blocks, and tables.

2. A **span filesystem** that exposes a per-window `spans` file
   through 9P, allowing external tools to color arbitrary regions of
   body text.

3. A **markdown preview mode** that parses `.md` files and renders
   them inline in the editor window, with source-position mapping for
   editing.

These extensions are layered: the rich text engine is used by both the
markdown previewer and the span renderer, and the span filesystem
follows the same external-tool philosophy as acme's existing
`event` and `ctl` files.

## 2. Rich Text Rendering

Edwood's original rendering engine, `frame.Frame`, draws
single-height, variable or monospaced text into a rectangle with
variable width character cells of the same height. It supports selection
highlighting and scrolling but has no concept of font variation or
color.

The new `rich.Frame` is a parallel rendering engine that accepts
styled content as input. A `rich.Content` value is a sequence of
`rich.Span` values, each carrying a text string and a `rich.Style`:

```go
type Style struct {
    Fg, Bg     color.Color
    Bold, Italic, Code, Link bool
    Scale      float64
    Image      bool
    ImageURL   string
}

type Span struct {
    Text  string
    Style Style
}

type Content []Span
```

`rich.Frame` converts content into layout boxes, wraps them into
lines, and draws them with the appropriate font variant at each span
boundary. It provides the coordinate-mapping functions `Ptofchar` and
`Charofpt` needed for mouse interaction, and supports selection
drawing, scrolling from an origin line, and partial-line pixel offsets
for smooth scrolling past tall elements like images.

The `RichText` wrapper component adds a scrollbar, font management
(bold, italic, bold-italic, code, and scaled heading fonts), and an
image cache for asynchronous image loading. It is used by both
markdown preview mode and span-styled rendering.

`rich.Frame` does not replace `frame.Frame`. A window in edwood is in
exactly one of three rendering states:

| State    | Renderer       | Trigger                     |
|----------|----------------|-----------------------------|
| Plain    | `frame.Frame`  | Default; `clear`; `Plain` toggle |
| Styled   | `rich.Frame`   | First span write via `spans` file |
| Preview  | `rich.Frame`   | `Markdeep` command on `.md` file |

## 3. The Span Filesystem

The span filesystem is the central contribution of this work. It
follows Acme's philosophy that the editor provides rendering mechanism
and external tools provide coloring policy. Rather than building a
syntax highlighting engine into the editor, we expose a file that any
program can write to.

### 3.1 The `spans` File

Each edwood window now exposes a `spans` file in its 9P directory
alongside the existing `body`, `tag`, `ctl`, `addr`, `data`, and
`event` files [7]:

```
/mnt/acme/<id>/spans
```

The file is write-only (`0200` permissions). External tools write span
definitions; the editor parses them and renders styled text.

### 3.2 Write Format

Each write to the `spans` file is a **region update**: a set of
newline-separated span definitions that replace all existing style
information within the covered region.

```
<offset> <length> <fg-color> [<bg-color>] [<flags>...]
```

| Field      | Format                              | Required |
|------------|-------------------------------------|----------|
| `offset`   | Decimal rune offset in body buffer  | Yes      |
| `length`   | Decimal rune count                  | Yes      |
| `fg-color` | `#rrggbb` hex or `-` for default    | Yes      |
| `bg-color` | `#rrggbb` hex or `-` for default    | No       |
| `flags`    | Space-separated: `bold`, `italic`, `hidden` | No |

Spans within a single write must be **contiguous and
non-overlapping**: the first span starts at offset *S*, and each
subsequent span starts where the previous one ended. The write
replaces all existing style information in the range [*S*,
*S*+*total\_length*). Gaps between the write region and existing spans
retain their existing style.

Example---coloring `func main() {` with keywords blue:

```
0 4 #0000ff
4 1 -
5 4 #000000
9 1 -
10 1 #000000
11 1 -
```

A write consisting of the single keyword `clear` removes all spans
and reverts the window to plain text rendering.

### 3.3 Stale Data Tolerance

A coloring tool reads the body, tokenizes it, and writes spans. If
the user edits the buffer between the read and the write, the spans
may reference positions past the end of the buffer. Rather than
rejecting the entire write, the editor **clamps** trailing spans:
spans starting past the buffer end are silently discarded, and the
last span within range is truncated to fit. The tool will re-color on
the next edit event.

### 3.4 Internal Representation

Spans are stored internally as a gap buffer [13] of `StyleRun` values:

```go
type StyleRun struct {
    Len   int
    Style StyleAttrs
}

type SpanStore struct {
    runs     []StyleRun
    gap0, gap1 int
    totalLen   int
}
```

The gap buffer provides O(1) amortized cost for insertions and
deletions at the cursor position. When the user types, the containing
run is extended and downstream runs shift implicitly (positions are
derived from cumulative lengths). When text is deleted, overlapping
runs are shrunk or removed. This keeps styles approximately correct
between external tool updates.

For region updates from the `spans` file, the gap is moved to the
target region and the covered runs are replaced with new runs parsed
from the write. The cost is O(*k*) for the gap move plus O(*m*) for
*m* new runs.

### 3.5 Rendering

When spans are present, the window builds `rich.Content` by iterating
the span store and reading the corresponding text from the body
buffer:

```go
func (w *Window) buildStyledContent() rich.Content {
    var content []rich.Span
    offset := 0
    w.spanStore.ForEachRun(func(run StyleRun) {
        text := w.body.file.ReadRuneSlice(
            offset, offset+run.Len)
        content = append(content, rich.Span{
            Text:  string(text),
            Style: styleAttrsToRichStyle(run.Style),
        })
        offset += run.Len
    })
    return rich.Content(content)
}
```

### 3.6 Auto-Switch and Plain Toggle

The first write to a window's `spans` file automatically transitions
the window from plain text rendering to styled rendering. The `Plain`
command (available as both a tag command and a `ctl` keyword) toggles
between styled and plain rendering without discarding the span data.

## 4. Markdown Preview

The `markdown` package parses a subset of CommonMark [5] and produces
`rich.Content` for rendering. It supports headings (H1--H6 with scale
factors 2.0 down to 0.875), bold, italic, inline code, fenced code
blocks with language tags, links, images, ordered and unordered lists,
tables, blockquotes, and horizontal rules. Like acme, the Markdown
preview is opinionated. It forms font names by hard-coding the naming
pattern of the Go fonts and provides no styling options. In the event
the naming doesn't resolve, fallbacks are used to maintain a usable
view.

A key challenge is source-position mapping. When the user clicks in
the rendered preview, the editor must map the click position back to a
byte offset in the markdown source for editing. The `SourceMap` type
maintains a bidirectional mapping between rendered rune positions and
source byte positions, with entries classified by kind (`PlainText`,
`SymmetricMarker`, `Prefix`, `TableCell`, `Synthetic`) to handle the
different gap behaviors at element boundaries.

The `LinkMap` tracks link positions in rendered content, enabling B3
(right-click look) to plumb URLs to the system.

For `.md` files, preview mode activates automatically on open. The
`Markdeep` tag command toggles between preview and plain text editing.
Incremental preview updates avoid full re-renders on each keystroke.

## 5. The `S` Select Event

Acme's event file reports text insertions (`I`/`i`), deletions
(`D`/`d`), executions (`X`/`x`), and looks (`L`/`l`). We add a new
event type for selection changes:

```
S<q0> <q1> 0 0
```

| Field | Meaning |
|-------|---------|
| `S`   | Body selection changed (lowercase `s` for tag) |
| `q0`  | Rune offset of selection start |
| `q1`  | Rune offset of selection end |

The event is emitted whenever `SetSelect` is called with values
different from the current selection. It follows the existing event
format: origin character, type character, two addresses, flag, count,
and optional text.

The `S` event enables tools to react to cursor movement and text
selection. The `edcolor` tool uses it to highlight all occurrences of
the currently selected text, with a 100ms debounce to avoid flashing
during sweep drags.

## 6. The `edcolor` Tool

`edcolor` is the reference implementation of an external syntax
coloring tool. It demonstrates the intended workflow for the span
filesystem:

1. Read `$winid` to identify the target window.
2. Read the window tag to determine the filename and select a lexer.
3. Open the event file and re-enable the tag menu (`win.Ctl("menu")`).
4. Mount the 9P filesystem for spans writing.
5. Read the body, tokenize, write colored spans.
6. Enter an event loop, re-coloring on edits (300ms debounce) and
   highlighting matching occurrences on selection (100ms debounce).

`edcolor` ships with lexers for Go (using the standard library's
`go/scanner`), and Python and Rust (using custom tokenizers). The
color scheme is minimal and hardcoded:

| Element  | Color     |
|----------|-----------|
| Keyword  | `#0000cc` (blue) |
| String   | `#008000` (green) |
| Comment  | `#808080` (gray) |
| Number   | `#cc6600` (orange) |
| Builtin  | `#008080` (teal) |
| Match highlight | `#f0f4ff` background |

When the user selects two or more characters, `edcolor` finds all
other occurrences in the body and merges a light blue background into
the syntax-colored spans for those ranges, preserving foreground
colors.

The tool is invoked automatically via a file-hook mechanism: when a
window opens a `.go` or `.py` file, edwood looks up the extension in a
`fileHooks` map and runs the corresponding tool. The tool stays
resident for the lifetime of the window, exiting when the event
channel closes.

Span writes are chunked to stay within 9P message size limits (4000
bytes per chunk), with each chunk forming a valid region update.

## 7. Discussion and Related Work

### Syntax Coloring in Editors

Syntax coloring in text editors has taken several architectural forms.

**Built-in lexers.** Many editors embed language-specific tokenizers
directly. Vim's syntax highlighting uses a declarative regex-based
grammar tied to the rendering engine. This approach is fast but
couples language support to the editor.

**Grammar specifications.** TextMate [8] introduced a portable JSON
grammar format based on Oniguruma regular expressions and scope
naming. This format was adopted by Sublime Text, Atom, and Visual
Studio Code, creating a large corpus of reusable grammars. However,
regex-based tokenization cannot parse nested structures and frequently
produces incorrect highlighting.

**Incremental parsing.** Tree-sitter [9] builds a concrete syntax
tree from a formal grammar and updates it incrementally as text
changes. It provides accurate structural highlighting but requires a
grammar for each language and a native parsing library.

**Language servers.** The Language Server Protocol [10] defines
semantic token requests that let a language-aware server provide
highlighting information to the editor. This gives the most accurate
results but introduces latency and complexity.

The span filesystem occupies a different point in this design space.
It is **protocol-level**: the editor defines a simple text-based wire
format for colored regions and renders whatever it receives. The
coloring tool is a separate process that can use any tokenization
strategy---`go/scanner`, tree-sitter, a language server, or a shell
script piping through `sed`. This follows the Plan 9 philosophy of
small, composable tools communicating through the filesystem.

The tradeoff is that the protocol carries concrete colors rather than
semantic token types, so theming requires changing the tool rather
than the editor. A future extension could add a scope-name field to
span definitions, with the editor mapping scopes to colors via a
theme file.

### Markdown Rendering

Markdown [6] rendering in editors ranges from side-by-side preview
panes (VS Code, many Vim plugins) to inline WYSIWYG rendering
(Typora, Obsidian). Edwood's approach---rendering styled content
in-place using a source map for position translation---is closer to
the inline model but retains the source text as the authoritative
representation. The `Markdeep` toggle lets the user switch freely
between the rendered view and the raw source.

The CommonMark specification [5] provides a rigorous definition of
Markdown syntax. Edwood's parser implements a practical subset,
omitting features like reference links and nested blockquotes that
rarely appear in the kind of short documents edited in acme-style
workflows.

### Empirical Evidence

Research on syntax highlighting's effectiveness is mixed. Beelders and
du Plessis [11] found that syntax highlighting reduced visual search
time and fixation counts when reading source code. Hannebauer et al.
[12] found weaker effects for programming novices, suggesting that
the benefit depends on expertise level. Our work does not take a
position on the pedagogical value of syntax coloring; we simply
observe that it is widely demanded and provide a mechanism for
delivering it that is consistent with Acme's architecture.

### Implementation

This implementation was realized using Anthropic's Claude coding agent.
The process was driven spec-first (specifications can be found in
`docs/...`) using a test-driven model. The implementation has been
discarded twice as the specifications were refined. It may yet be
discarded a third time as debugging continues---the implementation
code assets themselves have little intrinsic value. The tests, however,
capture many of the interaction decisions and fixes that must be
respected in a new implementation. They are not particularly tight or
DRY, but do encode the required behavior. This corresponds to
approximately 53,000 lines of new test code to 17,000 lines of new
implementation.

The work represents approximately four days of my time, invested over
two weeks, interspersed across other tasks. Having tried to implement
rich text in both acme and edwood at least 3 times in the last 20
years, I'm glad to see an implementation method that has made it
possible.

## 8. Conclusion and Future Work

The extensions described here add rich text rendering, markdown
preview, and external syntax coloring to edwood without compromising
the editor's role as a tool host. The `spans` file is a natural
extension of acme's filesystem interface: just as external programs
read `event` and write `ctl` to interact with editor state, they now
write `spans` to control visual presentation. The `S` selection event
closes the feedback loop, letting tools react to what the user is
looking at.

In the future I would like to move Markdown rendering to an external
tool as well. This is challenging because of the external content:
images will require a path into the frame buffer, perhaps in the form
of an externally exposed box for the helper tool to fill, or an
exposed canvas to paint.

The implementation is available at the author's fork of the edwood
repository [4]. The `edcolor` tool, with lexers for Go, Rust and
Python, serves as both a useful tool and a template for building
additional language support.

## References

[1] Rob Pike. "Acme: A User Interface for Programmers."
In *Proceedings of the Winter 1994 USENIX Conference*, pp. 223--234.
San Francisco, CA, January 1994. USENIX Association.

[2] Rob Pike, Dave Presotto, Sean Dorward, Bob Flandrena, Ken
Thompson, Howard Trickey, and Phil Winterbottom. "Plan 9 from Bell
Labs." *Computing Systems*, 8(3):221--254, Summer 1995.

[3] "Introduction to the Plan 9 File Protocol, 9P." *Plan 9
Programmer's Manual*, Section 5 (intro(5)), Fourth Edition, 2002.
https://man.cat-v.org/plan_9/5/intro

[4] Paul Lalonde. "Edwood: Go version of Plan 9 Acme Editor."
https://github.com/paul-lalonde/edwood

[5] John MacFarlane. "CommonMark Spec, Version 0.31.2." January 2024.
https://spec.commonmark.org/0.31.2/

[6] John Gruber. "Markdown." Daring Fireball, December 2004.
https://daringfireball.net/projects/markdown/

[7] "acme(4)---Acme Filesystem Interface." *Plan 9 Programmer's
Manual*, Section 4, Fourth Edition, 2002.
https://man.cat-v.org/plan_9/4/acme

[8] Allan Odgaard. "Language Grammars." *TextMate Manual*.
https://macromates.com/manual/en/language_grammars

[9] Max Brunsfeld. "Tree-sitter: An Incremental Parsing System for
Programming Tools." https://tree-sitter.github.io/tree-sitter/

[10] Microsoft. "Language Server Protocol Specification, Version 3.17." 2022.
https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/

[11] Tanya R. Beelders and Jean-Pierre L. du Plessis. "Syntax
Highlighting as an Influencing Factor When Reading and Comprehending
Source Code." *Journal of Eye Movement Research*, 9(1), 2016.
DOI: 10.16910/jemr.9.1.1

[12] Christoph Hannebauer, Marc Hesenius, and Volker Gruhn. "Does
Syntax Highlighting Help Programming Novices?" *Empirical Software
Engineering*, 23:2795--2828, 2018.
DOI: 10.1007/s10664-017-9579-0

[13] Wilfred J. Hansen. "Data Structures in a Bit-Mapped Text
Editor." *Byte*, pp. 183--190, January 1987.
