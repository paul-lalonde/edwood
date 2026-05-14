# Layout Edge-Case Test File

A controlled markdown file that exercises every variable-line-height
edge case the frame must handle correctly. Each section names the
scenario it covers and what to look for.

---

## §1 Heading at top of file

The `#` heading immediately above. After md2spans the heading's line
gets `scale=2.0` so its LineH is taller than body. Body paragraph
follows immediately so the user can see the heading↔body baseline.

## §2 Heading immediately after heading

### Sub-heading (no body between)
### Another sub-heading (back to back)
Body text after two adjacent sub-headings. Check that there's no gap
or overlap between the two `###` lines.

## §3 Bold text spanning multiple visual lines

**Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco.**

Body line after the wrapped bold paragraph. Check that the bold span's
overlay (when Spans is on) outlines every visual line of the bold run.

## §4 Soft-wrap within a heading

### A heading that is long enough to soft-wrap when the window is narrow, exercising the cklinewrap path on a scaled line

Body paragraph follows.

## §5 Plain paragraph that wraps

Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore.

## §6 Fenced code block

```go
// Code block exercises family=code (monospace font selection).
// Multiple lines.
func example() {
    return 42
}
```

Body line after the code block.

## §7 Heading right before a code block

```
preformatted
text
```

Body resumes.

## §8 List with mixed bold + plain items

- Plain list item — short.
- **Bold prefix** then plain continuation that wraps across multiple visual lines so we exercise the wrap path with mixed inline styling.
- `inline code` then more text.
- Another plain item.

## §9 Nested list

1. First numbered item.
   1. Nested.
   2. Second nested.
2. Second numbered item.

## §10 Tab characters

	A line that begins with a tab.
	Another tabbed line.
Plain line.

## §11 Empty lines

The line above this paragraph is empty.

The line above this one is also empty.



Multiple blank lines above (3 of them).

## §12 Long word that exceeds line width

This paragraph contains a really-really-really-really-really-really-really-really-really-really-really-long-hyphenated-word that should overflow / character-split.

## §13 Heading at end of file (and the bottom edge)

The next heading is the last visible content in the file. Scroll
behaviour around the bottom edge of the frame is what we test here:
the heading should land on a complete visual line; if it can't fit
fully, the bottom-edge alignment rule (per frame-scrollbar-spec)
should choose between top-aligned and bottom-aligned consistently
with the scroll direction.

### Final heading

End of file.
