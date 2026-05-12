// Wire-format parser for the spans protocol. The protocol is
// authoritatively defined in the published spec
// (/Users/paul/dev/edwood/docs/designs/spans-protocol.md in the
// upstream tree). This implementation covers the Slice A subset:
//
//   c
//     Clears all spans for the window. Must be alone in its
//     write.
//
//   s <offset> <length> <fg> [<bg>]
//     Defines a styled run. <fg> and <bg> are either `#rrggbb`
//     (lowercase hex, exactly 6 digits) or `-` (default).
//     Producers fill gaps in the styled region with default-
//     styled lines (`s <off> <len> -`).
//
// Slice A explicitly rejects the protocol's flag tokens (bold,
// italic, scale=, family=, hrule), the `b` directive (replaced
// elements — Slice C), and the begin/end region forms (Slice B
// / C). The legacy unprefixed format is not accepted; Slice A
// is a producer-side cleanroom rewrite, so we require the
// prefixed protocol from the start.
//
// Within a write, the protocol's contiguity rule applies: each
// `s` directive's offset must equal the previous directive's
// offset + length. The first `s` sets the region's start; gaps
// must be filled by the producer.

package spans

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

// DirOp identifies the directive type.
type DirOp int

const (
	OpInvalid DirOp = iota
	// OpClearAll is the `c` directive — clears every styled
	// span for the window. Carries no offset/length and no
	// color fields.
	OpClearAll
	// OpSetStyle is the `s` directive — sets a styled run on a
	// contiguous range. Fg / Bg are nil when the directive's
	// corresponding token is `-` (default).
	OpSetStyle
)

// Directive is one parsed line of the spans-file protocol.
// Colors are color.Color (image/color) so the spans package
// stays out of the draw dependency; the caller resolves to
// draw.Image before mutating the Store.
type Directive struct {
	Op  DirOp
	Off int         // valid only for OpSetStyle
	Len int         // valid only for OpSetStyle
	Fg  color.Color // nil = default (the `-` token)
	Bg  color.Color // nil = default (the `-` token or omitted)
}

// ParseDirective parses one non-empty line. Returns an error
// for the empty string; use ParseAll for multi-line input that
// silently skips blanks.
func ParseDirective(line string) (Directive, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return Directive{}, fmt.Errorf("spans: empty directive")
	}
	switch fields[0] {
	case "c":
		return parseClear(fields[1:])
	case "s":
		return parseSet(fields[1:])
	case "b":
		return Directive{}, fmt.Errorf("spans: `b` directives not supported in Slice A (replaced elements live in Slice C)")
	case "begin", "end":
		return Directive{}, fmt.Errorf("spans: region directives not supported in Slice A")
	default:
		return Directive{}, fmt.Errorf("spans: unknown directive op %q", fields[0])
	}
}

// ParseAll parses a single Twrite payload — a possibly
// multi-line block. Blank lines are skipped. Returns the
// directives in input order along with any parse error
// (annotated with the offending line number).
//
// Protocol invariants enforced:
//
//   - `c` is exclusive: if the payload contains a `c`, it must
//     be the only directive.
//   - `s` contiguity: each `s` directive's offset must equal
//     the previous directive's offset + length.
func ParseAll(text string) ([]Directive, error) {
	var out []Directive
	for i, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		d, err := ParseDirective(trimmed)
		if err != nil {
			return out, fmt.Errorf("spans: line %d: %w", i+1, err)
		}
		out = append(out, d)
	}
	// Enforce write-level invariants now that we know what's
	// in the payload.
	for j, d := range out {
		if d.Op == OpClearAll && len(out) != 1 {
			return out, fmt.Errorf("spans: `c` must be alone in its write (line %d, but %d directives present)", j+1, len(out))
		}
	}
	for j := 1; j < len(out); j++ {
		if out[j].Op != OpSetStyle {
			continue
		}
		prev := out[j-1]
		if prev.Op != OpSetStyle {
			continue
		}
		if out[j].Off != prev.Off+prev.Len {
			return out, fmt.Errorf("spans: contiguity violated at line %d: offset %d, expected %d", j+1, out[j].Off, prev.Off+prev.Len)
		}
	}
	return out, nil
}

func parseClear(rest []string) (Directive, error) {
	if len(rest) != 0 {
		return Directive{}, fmt.Errorf("spans: `c` takes no arguments (got %d)", len(rest))
	}
	return Directive{Op: OpClearAll}, nil
}

func parseSet(rest []string) (Directive, error) {
	if len(rest) < 3 {
		return Directive{}, fmt.Errorf("spans: `s` requires <off> <len> <fg> [<bg>]")
	}
	off, err := parseUint(rest[0])
	if err != nil {
		return Directive{}, fmt.Errorf("spans: `s` off: %w", err)
	}
	n, err := parseUint(rest[1])
	if err != nil {
		return Directive{}, fmt.Errorf("spans: `s` len: %w", err)
	}
	d := Directive{Op: OpSetStyle, Off: off, Len: n}
	fg, err := parseColorToken(rest[2])
	if err != nil {
		return Directive{}, fmt.Errorf("spans: `s` fg: %w", err)
	}
	d.Fg = fg
	if len(rest) == 3 {
		return d, nil
	}
	// Discriminate the fourth field by appearance: a color-shaped
	// token (`#...` or `-`) is the optional bg; anything else is
	// a flag — Slice A rejects all flags.
	if !looksLikeColor(rest[3]) {
		return Directive{}, fmt.Errorf("spans: `s` flag %q not supported in Slice A", rest[3])
	}
	bg, err := parseColorToken(rest[3])
	if err != nil {
		return Directive{}, fmt.Errorf("spans: `s` bg: %w", err)
	}
	d.Bg = bg
	if len(rest) > 4 {
		return Directive{}, fmt.Errorf("spans: `s` flag %q not supported in Slice A", rest[4])
	}
	return d, nil
}

// looksLikeColor reports whether s parses as a protocol color
// token (`-` or `#rrggbb`). Used to discriminate bg from a
// trailing flag without parsing.
func looksLikeColor(s string) bool {
	return s == "-" || (len(s) == 7 && s[0] == '#')
}

// parseColorToken decodes a protocol color token: `-` returns
// nil (default), `#rrggbb` returns color.RGBA with A=0xff.
func parseColorToken(s string) (color.Color, error) {
	if s == "-" {
		return nil, nil
	}
	if len(s) != 7 || s[0] != '#' {
		return nil, fmt.Errorf("color must be `-` or #rrggbb, got %q", s)
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return nil, fmt.Errorf("non-hex color %q", s)
	}
	return color.RGBA{
		R: uint8(v >> 16),
		G: uint8(v >> 8),
		B: uint8(v),
		A: 0xff,
	}, nil
}

// parseUint accepts only non-negative decimal integers.
func parseUint(s string) (int, error) {
	if strings.HasPrefix(s, "-") {
		return 0, fmt.Errorf("negative %q", s)
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("malformed integer %q", s)
	}
	return n, nil
}
