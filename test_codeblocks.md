# Testing Code Blocks

This is a test file for verifying code block rendering.

## Inline Code

Here is some `inline code` in a sentence.

Multiple `inline` code `spans` in one line.

## Quoted text

>Quoted
>>Double Quoted
>> `inline code`


## Local JPEG Image

Here is a robot: ![Robot](robot.jpg)

# Tables

Simple 2-column table:

| Name    | Score |
|---------|-------|
| Alice   | 95    |
| Bob     | 87    |
| Charlie | 100   |

Mixed alignment (un-padded source — alignment SHOULD be visible):

| L | C | R |
|:---|:---:|---:|
| a | b | c |
| longer text | longer too | longer right |

Mixed alignment (pre-padded source — column widths already filled, alignment markers have nothing to redistribute):

| Left   | Center | Right |
|:-------|:------:|------:|
| a      | b      | c     |
| longer | text   | here  |

Empty cells:

|  | A | B |
|---|---|---|
|  |   |   |
| 1 | 2 | 3 |

Table inside a blockquote:

> | H1 | H2 |
> |----|----|
> | x  | y  |

Pipe-line that is NOT a table (no separator follows):

| this | should be plain text |
| because | the next line isn't a separator

Single pipe line at end:

| solo

## Fenced Code Blocks

Here is a Go code block:

```go
func main() {
    fmt.Println("Hello, World!")
}
```

And here is a plain code block:

```
just some code
with multiple lines
    and indentation
```

## Lists
- item
- item
>- item
>- item
>- item
>Not an item

- top-level
  - one level deep
    - two levels deep
  - back at one level
- back at top
  - mixed:
  1. ordered nested
  2. another
- inside blockquote:
> - quoted-list
>   - quoted-nested
> - quoted-sibling

- first item
  continues onto line two
  and line three
- second item with no continuation
- third item
  with continuation
  and another continuation

1. ordered with continuation
   needs three-space indent (content col = 3)
2. another

- back to top
  - nested item
    with its own continuation
- sibling

> blockquote with a list inside
> - item with continuation
>   continues here
> - second item

This line is a fresh paragraph (lazy continuation NOT supported,
so the leading text starting with `This` doesn't fold into anything).

## Mixed Content

Some text before the code block.

```
code inside
```

Some text after the code block.

## Horizontal Rules

Testing horizontal rules with different syntaxes:

Three hyphens:

---

Three asterisks:

***

Three underscores:

___

With spaces between:

- - -

* * *

_ _ _

More than three:

-----

*****

_____

## End

That's all for code block and horizontal rule testing.
