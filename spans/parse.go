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
	OpSetStyle
	OpClearStyle
)

// Directive is a parsed spans-file directive in raw form. Colors
// are returned as color.Color so the spans package can stay
// independent of the draw package; the caller resolves them to
// draw.Image via display.AllocImage before invoking Store.
// SetRegion. See design §6.4.
type Directive struct {
	Op  DirOp
	Off int
	Len int
	Fg  color.Color // nil if not specified on the directive
	Bg  color.Color // nil if not specified on the directive
}

// ParseDirective parses one line of the spans-file format. Slice
// A recognizes:
//
//	s <off> <len> [fg=#RRGGBB] [bg=#RRGGBB]   set styling
//	c <off> <len>                              clear styling
//
// Other ops (`b` for replaced elements), unknown keys (bold,
// italic, underline, font, …), malformed integers, and malformed
// colors all return an error. Empty / whitespace-only lines
// return an error too; use ParseAll for multi-line input that
// silently skips blank lines.
func ParseDirective(line string) (Directive, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return Directive{}, fmt.Errorf("spans: empty directive")
	}
	switch fields[0] {
	case "s":
		return parseSet(fields[1:])
	case "c":
		return parseClear(fields[1:])
	case "b":
		return Directive{}, fmt.Errorf("spans: `b` (replaced-element) directives not supported in Slice A")
	default:
		return Directive{}, fmt.Errorf("spans: unknown directive op %q", fields[0])
	}
}

// ParseAll parses a multi-line block (e.g. a single 9P write
// payload) into a slice of directives. Blank lines are skipped;
// the first parse error stops parsing and is returned.
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
	return out, nil
}

func parseSet(fields []string) (Directive, error) {
	if len(fields) < 2 {
		return Directive{}, fmt.Errorf("spans: `s` requires <off> <len> [keys]")
	}
	off, err := parseUint(fields[0])
	if err != nil {
		return Directive{}, fmt.Errorf("spans: `s` off: %w", err)
	}
	n, err := parseUint(fields[1])
	if err != nil {
		return Directive{}, fmt.Errorf("spans: `s` len: %w", err)
	}
	d := Directive{Op: OpSetStyle, Off: off, Len: n}
	for _, kv := range fields[2:] {
		key, val, ok := strings.Cut(kv, "=")
		if !ok {
			return Directive{}, fmt.Errorf("spans: `s` malformed key=value: %q", kv)
		}
		switch key {
		case "fg":
			c, err := parseHexColor(val)
			if err != nil {
				return Directive{}, fmt.Errorf("spans: fg: %w", err)
			}
			d.Fg = c
		case "bg":
			c, err := parseHexColor(val)
			if err != nil {
				return Directive{}, fmt.Errorf("spans: bg: %w", err)
			}
			d.Bg = c
		default:
			return Directive{}, fmt.Errorf("spans: unknown key %q (Slice A: fg, bg only)", key)
		}
	}
	return d, nil
}

func parseClear(fields []string) (Directive, error) {
	if len(fields) != 2 {
		return Directive{}, fmt.Errorf("spans: `c` requires exactly <off> <len>")
	}
	off, err := parseUint(fields[0])
	if err != nil {
		return Directive{}, fmt.Errorf("spans: `c` off: %w", err)
	}
	n, err := parseUint(fields[1])
	if err != nil {
		return Directive{}, fmt.Errorf("spans: `c` len: %w", err)
	}
	return Directive{Op: OpClearStyle, Off: off, Len: n}, nil
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

// parseHexColor decodes "#RRGGBB" into color.RGBA. Alpha is 0xff.
// Other formats (named colors, #RGB shorthand, #RRGGBBAA) are
// rejected in Slice A.
func parseHexColor(s string) (color.Color, error) {
	if len(s) != 7 || s[0] != '#' {
		return nil, fmt.Errorf("color must be #RRGGBB, got %q", s)
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
