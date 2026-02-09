# Edwood Presentation Slides

---

[Slide+](:/^# Slide 2: Introduction to Acme) | [Index](:/^# Index)

# Slide 1: Title

## Rich Text, Markdown Preview, and External Syntax Coloring in Edwood

**Paul Lalonde**

*International Workshop on Plan 9*

- A Go reimplementation of Rob Pike's acme editor
- Adding rich text rendering without breaking architecture
- Preserving acme's philosophy: mechanism vs. policy

---
---

[Slide-](:/^# Slide 1: Title) | [Slide+](:/^# Slide 3: Enter Edwood) | [Index](:/^# Index)

# Slide 2: Introduction to Acme

## Acme: The Text Editor from Plan 9

### Two Core Ideas
1. **Text as universal interface** — everything communicates through text
2. **Editor as tool host** — provides mechanism, not policy

### The 9P Filesystem Interface
Every window exposes synthetic files:
- `body` — the file content
- `tag` — the window tag/menu
- `ctl` — control commands
- `addr`, `data`, `event` — cursor and event stream

External programs interact by reading/writing these files.

---
---

[Slide-](:/^# Slide 2: Introduction to Acme) | [Slide+](:/^# Slide 4: The Problem) | [Index](:/^# Index)

# Slide 3: Enter Edwood

## Edwood: Acme in Go

### What is Edwood?
- A complete Go port of acme from Plan 9
- Functional drop-in replacement
- Preserves 9P filesystem interface
- Retains mouse-chord commands and three-button interaction

### Why it Matters
Maintains acme's elegant design while enabling modern features.

### Historical Context
"I've tried to implement rich text in acme at least 3 times in 20 years—now it's possible."

---
---

[Slide-](:/^# Slide 3: Enter Edwood) | [Slide+](:/^# Slide 5: The Solution Overview) | [Index](:/^# Index)

# Slide 4: The Problem

## What's Missing in Acme?

### Design Choice: Single-Height, Single-Color
Acme deliberately renders in one font with no color, bold, or italic:
- Keeps the editor simple
- Avoids complexity of rich text layout
- **But:** makes syntax coloring structurally impossible

### User Demands
1. **Syntax coloring** — "the most widely requested feature"
2. **Markdown preview** — must shell out to external renderer
3. **Rich text** — not accessible

---
---

[Slide-](:/^# Slide 4: The Problem) | [Slide+](:/^# Slide 6: Rich Text Rendering Engine) | [Index](:/^# Index)

# Slide 5: The Solution Overview

## Three Layered Capabilities

### 1. Rich Text Rendering Engine
- `rich.Frame` supports proportional fonts, colors, bold, italic
- Inline images, heading scaling, code blocks, tables
- Parallel to existing plain text renderer

### 2. Span Filesystem
- New `spans` file exposed through 9P per window
- External tools write colored regions
- Follows acme's "mechanism, not policy" philosophy

### 3. Markdown Preview
- Parse `.md` files and render inline
- Source-position mapping for editing
- Toggle between preview and source

---
---

[Slide-](:/^# Slide 5: The Solution Overview) | [Slide+](:/^# Slide 7: Three Rendering States) | [Index](:/^# Index)

# Slide 6: Rich Text Rendering Engine

## The rich.Frame Component

### Data Structures
```go
type Style struct {
    Fg, Bg color.Color
    Bold, Italic, Code, Link bool
    Scale float64
    Image bool
}
type Span struct {
    Text string
    Style Style
}
type Content []Span
```

### Capabilities
- Converts styled content to layout boxes
- Wraps content into lines with appropriate fonts
- Provides coordinate mapping for mouse interaction
- Supports selection, scrolling, partial-line offsets

---
---

[Slide-](:/^# Slide 6: Rich Text Rendering Engine) | [Slide+](:/^# Slide 8: The Span Filesystem) | [Index](:/^# Index)

# Slide 7: Three Rendering States

## Windows Have Three Rendering Modes

| State | Renderer | Trigger |
|-------|----------|---------|
| **Plain** | `frame.Frame` | Default; `clear` command; Plain toggle |
| **Styled** | `rich.Frame` | First span write via `spans` file |
| **Preview** | `rich.Frame` | `Markdown` command on `.md` file |

### Design Philosophy
- `rich.Frame` doesn't replace `frame.Frame`
- Every window is in exactly one state
- User can toggle between modes

---
---

[Slide-](:/^# Slide 7: Three Rendering States) | [Slide+](:/^# Slide 9: Span File Format) | [Index](:/^# Index)

# Slide 8: The Span Filesystem

## The Central Contribution: The spans File

### Path
```
/mnt/acme/<id>/spans
```

### Philosophy
- **Write-only** (`0200` permissions)
- External tools define spans
- Editor renders them
- Preserves acme's "tool host" architecture

### Example: Blue Keywords
```
0 4 #0000ff
4 1 -
5 4 #000000
9 1 -
10 1 #000000
11 1 -
```

"func main() {" with keywords in blue

---
---

[Slide-](:/^# Slide 8: The Span Filesystem) | [Slide+](:/^# Slide 10: Stale Data & Internal Storage) | [Index](:/^# Index)

# Slide 9: Span File Format

## Writing Spans: The Format

### Per-Line Format
```
<offset> <length> <fg-color> [<bg-color>] [<flags>...]
```

| Field | Format | Required |
|-------|--------|----------|
| `offset` | Decimal rune offset in body buffer | Yes |
| `length` | Decimal rune count | Yes |
| `fg-color` | `#rrggbb` hex or `-` for default | Yes |
| `bg-color` | `#rrggbb` hex or `-` for default | No |
| `flags` | `bold`, `italic`, `hidden` (space-separated) | No |

### Key Rules
- Spans must be **contiguous and non-overlapping**
- Write replaces all styles in covered range
- `clear` keyword removes all spans and reverts to plain text

---
---

[Slide-](:/^# Slide 9: Span File Format) | [Slide+](:/^# Slide 11: Markdown Preview) | [Index](:/^# Index)

# Slide 10: Stale Data & Internal Storage

## Robustness: Handling Edit Race Conditions

### Stale Data Tolerance
If user edits buffer between read and write:
- Spans past buffer end are **silently discarded**
- Last span within range is **truncated to fit**
- Tool re-colors on next edit event

### Internal Representation: Gap Buffer
```go
type StyleRun struct {
    Len int
    Style StyleAttrs
}
```

**Why gap buffer?**
- O(1) amortized cost for insertions at cursor
- Positions derived from cumulative lengths
- Implicit shifting when user types

---
---

[Slide-](:/^# Slide 10: Stale Data & Internal Storage) | [Slide+](:/^# Slide 12: The S Selection Event) | [Index](:/^# Index)

# Slide 11: Markdown Preview

## Live Markdown Rendering

### Supported Elements
- Headings (H1-H6 with scaling)
- Bold, italic, inline code
- Fenced code blocks with language tags
- Links, images, lists (ordered & unordered)
- Tables, blockquotes, horizontal rules
- Double hrules (---\n---) as page breaks
- CommonMark subset parser


### Key Feature: Source Position Mapping
When user clicks in rendered preview:
- Maps click back to byte offset in markdown source
- Enables inline editing
- `LinkMap` tracks link positions for B3 (right-click look)

### Automatic Activation
- Activates automatically on opening `.md` files
- `Markdown` tag command toggles preview ↔ source
- Incremental updates avoid full re-render

---
---

[Slide-](:/^# Slide 11: Markdown Preview) | [Slide+](:/^# Slide 13: The edcolor Tool) | [Index](:/^# Index)

# Slide 12: The S Selection Event

## New Event: Selection Changes

### Acme's Event File (Before)
- `I`/`i` — text insertion
- `D`/`d` — deletion
- `X`/`x` — execution
- `L`/`l` — look (search)

### New: S Event Format
```
S<q0> <q1> 0 0
```

| Field | Meaning |
|-------|---------|
| `S` | Body selection changed (lowercase `s` for tag) |
| `q0` | Rune offset of selection start |
| `q1` | Rune offset of selection end |

### Use Case: Occurrence Highlighting
- Tool reads selection change events
- 100ms debounce to avoid flashing
- Highlights all matching text in document

---
---

[Slide-](:/^# Slide 12: The S Selection Event) | [Slide+](:/^# Slide 14: edcolor Color Scheme) | [Index](:/^# Index)

# Slide 13: The edcolor Tool

## Reference Syntax Coloring Implementation

### Workflow
1. Identify target window (read `$winid`)
2. Determine filename and select lexer from tag
3. Open event file, re-enable tag menu
4. Mount 9P filesystem for spans writing
5. Read body, tokenize, write colored spans
6. Enter event loop:
   - Re-color on edits (300ms debounce)
   - Highlight matches on selection (100ms debounce)

### Supported Languages
- **Go** — using standard library `go/scanner`
- **Python** — custom tokenizer
- **Rust** — custom tokenizer

---
---

[Slide-](:/^# Slide 13: The edcolor Tool) | [Slide+](:/^# Slide 15: Related Work & Architecture Comparison) | [Index](:/^# Index)

# Slide 14: edcolor Color Scheme

## Minimal, Hardcoded Color Palette

| Element | Color |
|---------|-------|
| Keyword | `#0000cc` (blue) |
| String | `#008000` (green) |
| Comment | `#808080` (gray) |
| Number | `#cc6600` (orange) |
| Builtin | `#008080` (teal) |
| Match highlight | `#f0f4ff` (light blue background) |

### Matching Occurrences
- When user selects 2+ characters
- edcolor finds all other occurrences
- Merges light blue background into syntax colors
- Preserves foreground colors

### Implementation Detail
Chunked writes stay within 9P message limits (4000 bytes per chunk)

---
---

[Slide-](:/^# Slide 14: edcolor Color Scheme) | [Slide+](:/^# Slide 16: Conclusion & Future Work) | [Index](:/^# Index)

# Slide 15: Related Work & Architecture Comparison

## Where Does edwood Fit?

### Syntax Coloring Architectures

| Approach | Example | Pros | Cons |
|----------|---------|------|------|
| Built-in lexers | Vim | Fast | Couples language to editor |
| Grammar specs | VS Code, Sublime | Portable grammars | Regex can't handle nesting |
| Incremental parsing | Tree-sitter | Accurate structure | Requires grammar + library per language |
| Language servers | LSP | Most accurate | Latency, complexity |
| **Span filesystem** | **edwood** | **Tool agnostic** | **Concrete colors, not semantic** |

### edwood's Position
- **Protocol-level** text-based wire format
- Tool can use any tokenization strategy
- Follows Plan 9 philosophy: small composable tools
- Trade-off: theming requires changing tool

---
---

[Slide-](:/^# Slide 15: Related Work & Architecture Comparison) | [Index](:/^# Index)

# Slide 16: Conclusion & Future Work

## Bringing Rich Text to Acme

### What We Achieved
- Rich text rendering without compromising architecture
- External syntax coloring through filesystem interface
- Markdown preview with source mapping
- Selection events for tool feedback

### The Philosophy Preserved
Just as external programs read `event` and write `ctl`, they now write `spans` to control presentation.

### Future Directions
1. Move Markdown rendering to external tool
2. Expose canvas/framebuffer for helper tools
3. Add scope-name field for editor-based theming
4. Extend to additional languages

### Implementation Notes
- 53,000 lines of tests, 17,000 lines of implementation
- "The tests capture interaction decisions; the code has little intrinsic value."
- Available at author's edwood fork

---
---

[Slide+](:/^# Slide 1: Title)

# Index

**Slide Navigation**

1. [Slide 1: Title](:/^# Slide 1: Title)
2. [Slide 2: Introduction to Acme](:/^# Slide 2: Introduction to Acme)
3. [Slide 3: Enter Edwood](:/^# Slide 3: Enter Edwood)
4. [Slide 4: The Problem](:/^# Slide 4: The Problem)
5. [Slide 5: The Solution Overview](:/^# Slide 5: The Solution Overview)
6. [Slide 6: Rich Text Rendering Engine](:/^# Slide 6: Rich Text Rendering Engine)
7. [Slide 7: Three Rendering States](:/^# Slide 7: Three Rendering States)
8. [Slide 8: The Span Filesystem](:/^# Slide 8: The Span Filesystem)
9. [Slide 9: Span File Format](:/^# Slide 9: Span File Format)
10. [Slide 10: Stale Data & Internal Storage](:/^# Slide 10: Stale Data & Internal Storage)
11. [Slide 11: Markdown Preview](:/^# Slide 11: Markdown Preview)
12. [Slide 12: The S Selection Event](:/^# Slide 12: The S Selection Event)
13. [Slide 13: The edcolor Tool](:/^# Slide 13: The edcolor Tool)
14. [Slide 14: edcolor Color Scheme](:/^# Slide 14: edcolor Color Scheme)
15. [Slide 15: Related Work & Architecture Comparison](:/^# Slide 15: Related Work & Architecture Comparison)
16. [Slide 16: Conclusion & Future Work](:/^# Slide 16: Conclusion & Future Work)
