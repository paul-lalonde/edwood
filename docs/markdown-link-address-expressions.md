# Markdown Link Address Expressions in Edwood

## Overview

Edwood extends Acme's markdown preview with powerful link navigation using **address expressions**. This allows markdown links to use the same address syntax as Acme's `addr` file, enabling precise jumps to specific locations in a document.

Instead of just supporting external URLs and local file paths, links can now contain:

- **Address expressions**: Navigate to exact search results within the document
- **Literal text search**: Jump to any text in the document
- **URLs and file paths**: Plumbed to external tools as fallback

## Design Goals

1. **Consistency with Acme**: Use familiar Acme address syntax
2. **Power without complexity**: Simple syntax for common cases, full regex support available
3. **Graceful fallback**: Try search first, plumb URL if not found
4. **Architecture preservation**: No breaking changes to markdown or Acme's design

## Syntax

### Address Expressions

Address expressions use Acme syntax with a required prefix character:

| Prefix | Meaning | Syntax | Examples |
|--------|---------|--------|----------|
| `:` | Address expression | `:[address]` | `:/^# Index`, `:/func main`, `:/,/^---` |
| `/` | Forward regex search | `/[pattern]/` | `/test`, `/\bword\b`, `/^##` |
| `?` | Backward regex search | `?[pattern]?` | `?test`, `?\bword\b` |

### Plain Strings

Without a prefix character, links are treated as:

1. **Literal text search**: Search for the text in the document
2. **URL plumbing**: If not found, plumb the text to external tools

Examples:
- `[Slide 2](Slide 2: Introduction to Acme)` → Search for "Slide 2: Introduction to Acme"
- `[GitHub](https://github.com)` → Plumb to browser
- `[Link](nonexistent)` → Search for "nonexistent", then plumb if not found

## Address Expression Details

### Syntax Overview

Address expressions follow Acme's address syntax. Common patterns:

```
:/^# Index         # Line starting with "# Index"
:/func main        # Next occurrence of "func main"
:/^---             # Horizontal rule (line starting with "---")
:/,/^---           # Range from current position to next hrule
?pattern?          # Backward search for pattern
```

### Pattern Matching

Patterns are regular expressions (Go's `regexp` package):

- `^` = Start of line
- `$` = End of line
- `\b` = Word boundary
- `.` = Any character
- `*`, `+`, `?` = Quantifiers
- Character classes: `[a-z]`, `[^abc]`

### Search Direction and Wrapping

- **Forward search** (`:` or `/`): Searches from current cursor position forward, wraps to beginning if not found
- **Backward search** (`?`): Searches from current cursor position backward, wraps to end if not found

## Implementation

### Architecture

The markdown link handler (`wind.go:HandlePreviewMouse`) follows this logic:

```
1. Is URL an address expression? (prefix: `:`, `/`, `?`)
   ├─ YES: Parse with address() function
   │   ├─ Evaluate expression against source buffer
   │   ├─ Map result to rendered (preview) positions
   │   ├─ Select text, scroll, warp cursor, return
   │   └─ If eval fails, fall through to fallback
   │
   └─ NO: Is it a plain string?
       ├─ Advance past link markdown to avoid matching link itself
       ├─ Search source buffer for literal text
       ├─ Map result to rendered positions
       ├─ Select text, scroll, warp cursor, return
       └─ If search fails, fall through to fallback
           └─ Plumb URL to external tools (browser, plumber, etc.)
```

### Key Functions

- **`address()`** (`addr.go`): Evaluates Acme address expressions
  - Takes expression string, source buffer, search range
  - Returns `Range{q0, q1}` if successful

- **`search()`** (`look.go`): Regex search in buffer
  - Starts from `ct.q1` (current cursor)
  - Wraps around to buffer start if not found
  - Updates `ct.q0` and `ct.q1` with match range

- **`previewSourceMap.ToRendered()`**: Maps source buffer positions to rendered preview
  - Handles markdown-to-display coordinate transformation
  - Accounts for syntax highlighting, images, formatting

- **`scrollPreviewToMatch()`**: Scrolls window to show match
  - Positions match at top of window
  - Snaps to slide boundaries for presentation slides
  - Handles variable-height content

- **`SetSelection()` and `MoveTo()`**: Display updates
  - Updates selection highlight in rendered view
  - Warps cursor to match location

### Source-to-Rendered Position Mapping

The markdown preview maintains a **source position map** (`previewSourceMap`):

- Maps byte offsets in markdown source to rune positions in rendered output
- Enables clicking in preview to edit source
- Bidirectional: source→rendered and rendered→source

When a link search succeeds:
1. Find match in **source buffer** (plain markdown)
2. Map source position to **rendered position** using `ToRendered()`
3. Update selection, scroll, and cursor in **rendered view**

This ensures:
- Searches happen on source (without formatting noise)
- Results appear correctly positioned in preview
- Editing source continues to work

### Literal String Search Process

When a link contains plain text (not an address expression):

```go
1. Advance w.body.q1 past the link URL
   - Prevents matching the URL within its own link markdown
   - Example: [Link](text) advances past "text)"

2. Call search(&w.body, []rune(url))
   - Searches source buffer starting from new position
   - Uses regex matching (QuoteMeta escapes special chars)
   - Wraps around if not found

3. If match found:
   - Map source range to rendered positions
   - Update selection, scroll, cursor
   - Return success

4. If match not found:
   - Fall through to plumbing
   - Send URL to external tools
```

## Usage Examples

### Example 1: Presentation Slides

In a presentation markdown file with slide headings:

```markdown
# Slide 1: Title
...

# Slide 2: Introduction
...

# Slide 3: Conclusion
...

[Go to Slide 2](:/^# Slide 2)
```

B3-clicking `[Go to Slide 2]` jumps to the "# Slide 2" heading.

### Example 2: Table of Contents

Create an index with both address expressions and literal text:

```markdown
# Table of Contents

1. [Introduction](:/^# Introduction)
2. [Methods](:/^# Methods)
3. [Results](:/^# Results)
4. [Related Work](#related-work)

# Introduction
...

# Methods
...

# Results
...

# Related Work
...
```

### Example 3: Backward Search

Search backward to find previous occurrence:

```markdown
# First Appearance of Term

Text about something...

# Second Appearance of Term

[Back to first?(\bTerm\b)?][Find previous use]

# Third Appearance of Term
```

### Example 4: Complex Ranges

Match ranges using Acme address syntax:

```markdown
[Show function](:/func main/,/^}/)
```

Jumps to and selects the entire `main()` function.

### Example 5: Literal Text with Spaces

Search for exact text in document:

```markdown
[Jump to Section](Important Note: Read This First)
```

Searches for "Important Note: Read This First" literally in the source.

### Example 6: Fallback to URL

If search fails, plumbs URL:

```markdown
[Try local first, fallback to web](https://example.com)
```

If "https://example.com" doesn't exist in document, plumbs URL to browser.

## Rendering Behavior

### Cursor Positioning

When a link is B3-clicked:

1. **Search executes** at current cursor position
2. **Match found**: Cursor warps to match location
   - Y position: Line containing match
   - X position: Start of match + 4 pixels (matching Acme's look3 behavior)
3. **Selection highlighted**: Match range shown with selection color

### Scrolling

The preview window scrolls to show the match:

1. **Check if already visible**: Don't scroll if match is on-screen
2. **Find target line**: Locate line containing match
3. **Position at top**: Scroll so target line appears at window top
4. **Slide snapping**: For presentations, snaps to slide boundary (start of `---\n---` block)

### Slide Boundaries

Presentation slides use double horizontal rules as boundaries:

```markdown
---
---

# Slide Title
```

When scrolling to a target within a slide, the window positions the slide start (upper `---`) at the top, keeping the entire slide visible.

## Testing

### Manual Testing Procedure

#### Setup

1. Open Edwood in markdown preview mode
2. Open `/Users/flux/dev/edwood/docs/slides.md` (presentation with slides)
3. View tag menu to confirm markdown preview is active

#### Test Address Expression Forward Search

1. **Test case**: `[Index](:/^# Index)` in header
2. **Expected behavior**:
   - B3-click the link
   - Cursor jumps to the line starting with "# Index"
   - Window scrolls to show that line at top
   - Selection shows the matched line
3. **Verify**: Heading is visible, window positioned correctly

#### Test Address Expression Backward Search

1. **Test case**: `[Back](?\bSlide\b)` (if exists in slides.md)
2. **Expected behavior**:
   - B3-click the link
   - Searches backward from current position
   - Finds previous line containing word "Slide"
   - Cursor warps, selection shows match
3. **Verify**: Found correct previous occurrence, not current

#### Test Literal Text Search

1. **Test case**: `[Slide+](Slide 2: Introduction to Acme)` at top of slides.md
2. **Expected behavior**:
   - B3-click the link
   - Searches for "Slide 2: Introduction to Acme" in document
   - Finds the actual slide heading
   - Cursor jumps there, selection visible
3. **Verify**: Didn't match link markdown, found actual heading

#### Test Cursor Positioning

1. B3-click any link with successful search
2. Observe cursor position:
   - Should be on the matched line
   - X-offset should align with match start
3. **Verify**: Cursor in correct position

#### Test Scrolling and Slide Snapping

1. Jump to a slide in middle of document: `[Mid](:/^# Slide.*)`
2. **Expected behavior**:
   - Window scrolls to show slide start (upper `---`) at top
   - Matched heading visible below it
   - Slide content visible
3. **Verify**: Entire slide visible from top

#### Test Plumbing Fallback

1. **Test case**: `[GitHub](https://github.com)` (text not in document)
2. **Expected behavior**:
   - B3-click the link
   - No match found for "https://github.com"
   - Falls back to plumbing
   - Browser opens (or plumber error if not running)
3. **Verify**: Fallback works when text not found

### Automated Testing

Run existing tests:

```bash
cd /Users/flux/dev/edwood
go test ./wind -v
```

Key test coverage:
- Address expression parsing
- Literal text search
- Position mapping (source ↔ rendered)
- Selection updates
- Scrolling behavior

## Troubleshooting

### Link not found

**Symptom**: B3-clicking link does nothing

1. Check console for warnings: `Markdown B3: ...`
2. Verify text exists in document (with exact spelling/spacing)
3. For address expressions, test regex pattern manually
4. Check if plumber is running: `ps aux | grep plumber`

### Cursor warp looks wrong

**Symptom**: Cursor doesn't appear at match location

1. Verify `display` is initialized: Check if window is visible
2. Check if match is on-screen: Scrolling may reposition
3. Verify `previewSourceMap` is populated: Check markdown preview is active

### Selection not visible

**Symptom**: B3-click works but selection not highlighted

1. Verify rich text frame is active (not plain text)
2. Check selection range: `body.q0` and `body.q1` should differ
3. Verify `SetSelection()` call is executed

## Future Enhancements

1. **Case-insensitive search**: Add flag for case-insensitive matching
2. **Named sections**: Predefined anchor names like `#section-name`
3. **Regex flags**: Support Go regex flags in address syntax
4. **Multiple matches**: UI to jump through multiple matches
5. **External tool integration**: Export links to other tools via address expressions

## References

- **Acme address syntax**: See `addr.go` in Edwood
- **Go regexp package**: https://golang.org/pkg/regexp/
- **Markdown spec**: CommonMark 0.30
