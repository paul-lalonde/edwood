// Wire-format parser for the spans protocol. The protocol is
// authoritatively defined in the published spec
// (/Users/paul/dev/edwood/docs/designs/spans-protocol.md in the
// upstream tree). This implementation covers what Slices A and B
// support today:
//
//   c
//     Clears all spans for the window. Must be alone in its
//     write.
//
//   s <offset> <length> <fg> [<bg>] [<flag>...]
//     Defines a styled run. <fg> and <bg> are either `#rrggbb`
//     (lowercase hex, exactly 6 digits) or `-` (default).
//     Producers fill gaps in the styled region with default-
//     styled lines (`s <off> <len> -`).
//     Flag tokens (Slice B):
//       bold    → Directive.Kind |= frame.KindBold
//       italic  → Directive.Kind |= frame.KindItalic
//       hidden  → Directive.Kind |= frame.KindHidden
//     Other recognised flags (hrule, scale=N.N, family=NAME) are
//     accepted but currently produce no bits; rendering arrives
//     in Slice C / a follow-up. Unknown flag spellings are an
//     error per the published spec.
//
// The `b` (replaced-element) directive and `begin region` /
// `end region` forms are rejected — Slice C will land them.
// The legacy unprefixed format is not accepted; this is a
// producer-side cleanroom rewrite, so we require the prefixed
// protocol from the start.
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

	"github.com/rjkroege/edwood/frame"
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
	// OpNoOp marks a directive whose form is valid on the wire
	// but whose semantics are inert in the current slice (Phase
	// B4 silently accepts `b`, `begin region`, `end region` —
	// see §6.4 / §12 Phase B4). ParseAll keeps OpNoOps in the
	// returned slice so per-line diagnostics stay precise; the
	// applier ignores them.
	OpNoOp
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

	// Kind carries the protocol's flag tokens translated to
	// frame.Style.Kind bits — `bold` → KindBold, `italic` →
	// KindItalic, `hidden` → KindHidden, `hrule` → KindHRule,
	// `family=code` → KindCodeFamily, `scale=N.N` →
	// KindScale (B2.2 R4). Other recognised flags
	// (`family=NAME` for non-code values) are silently
	// accepted on the wire but don't yet set bits.
	Kind frame.Kind

	// Scale carries scale=N.N's parsed value when
	// Kind & frame.KindScale is set. The frame applier
	// installs it into frame.Style.Scale.
	Scale float32
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
		return parseBoxAsNoOp(fields[1:])
	case "begin":
		return parseBeginRegionAsNoOp(fields[1:])
	case "end":
		return parseEndRegionAsNoOp(fields[1:])
	default:
		return Directive{}, fmt.Errorf("spans: unknown directive op %q", fields[0])
	}
}

// parseBoxAsNoOp validates a `b` line's published-protocol shape
// (off, len, width, height, fg, bg, [flags...]) and emits an
// OpNoOp directive. Phase B4 silently accepts the line so md2spans
// output flows through; Slice C C1 will translate it into real
// replaced-element rendering.
func parseBoxAsNoOp(rest []string) (Directive, error) {
	if len(rest) < 6 {
		return Directive{}, fmt.Errorf("spans: `b` requires <off> <len> <width> <height> <fg> <bg> [<flag>...] (got %d fields)", len(rest))
	}
	if _, err := parseUint(rest[0]); err != nil {
		return Directive{}, fmt.Errorf("spans: `b` off: %w", err)
	}
	if _, err := parseUint(rest[1]); err != nil {
		return Directive{}, fmt.Errorf("spans: `b` len: %w", err)
	}
	if _, err := parseUint(rest[2]); err != nil {
		return Directive{}, fmt.Errorf("spans: `b` width: %w", err)
	}
	if _, err := parseUint(rest[3]); err != nil {
		return Directive{}, fmt.Errorf("spans: `b` height: %w", err)
	}
	if _, err := parseColorToken(rest[4]); err != nil {
		return Directive{}, fmt.Errorf("spans: `b` fg: %w", err)
	}
	if _, err := parseColorToken(rest[5]); err != nil {
		return Directive{}, fmt.Errorf("spans: `b` bg: %w", err)
	}
	// Trailing flag/payload tokens are not validated here; the
	// published spec allows producer-defined keys (placement=,
	// image:URL, etc.) and Slice C will tighten this up.
	return Directive{Op: OpNoOp}, nil
}

// parseBeginRegionAsNoOp validates that the second token is
// "region" and emits an OpNoOp. Block-context layout lives in
// Slice C C4.
func parseBeginRegionAsNoOp(rest []string) (Directive, error) {
	if len(rest) < 2 || rest[0] != "region" {
		return Directive{}, fmt.Errorf("spans: `begin` only valid as `begin region <kind> ...`")
	}
	return Directive{Op: OpNoOp}, nil
}

// parseEndRegionAsNoOp accepts `end region` (with optional
// trailing tokens) and emits an OpNoOp. Anything else is an
// unknown-op style error.
func parseEndRegionAsNoOp(rest []string) (Directive, error) {
	if len(rest) < 1 || rest[0] != "region" {
		return Directive{}, fmt.Errorf("spans: `end` only valid as `end region [...]`")
	}
	return Directive{Op: OpNoOp}, nil
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
	// `s` contiguity is enforced *across* OpNoOp directives —
	// md2spans interleaves `begin region` markers between style
	// spans, but the spans themselves must still tile the buffer
	// without gaps. Find the most recent OpSetStyle predecessor.
	var prevSet *Directive
	for j := range out {
		d := &out[j]
		if d.Op != OpSetStyle {
			continue
		}
		if prevSet != nil && d.Off != prevSet.Off+prevSet.Len {
			return out, fmt.Errorf("spans: contiguity violated at line %d: offset %d, expected %d", j+1, d.Off, prevSet.Off+prevSet.Len)
		}
		prevSet = d
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
		return Directive{}, fmt.Errorf("spans: `s` requires <off> <len> <fg> [<bg>] [<flag>...]")
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

	i := 3
	// Optional bg, discriminated by appearance: a color-shaped
	// 4th token (`#rrggbb` or `-`) is bg; anything else is the
	// start of the flag block.
	if i < len(rest) && looksLikeColor(rest[i]) {
		bg, err := parseColorToken(rest[i])
		if err != nil {
			return Directive{}, fmt.Errorf("spans: `s` bg: %w", err)
		}
		d.Bg = bg
		i++
	}

	// Remaining tokens are flags. Slice B translates the
	// no-line-height-change subset (bold, italic, hidden) into
	// Directive.Kind bits the applier maps to frame.Style.Kind.
	// The remaining recognised flags (hrule, scale=N.N,
	// family=NAME) are still silently accepted but don't set
	// bits — Slice C and follow-ups land them. Unknown flag
	// spellings remain errors per the published spec.
	for ; i < len(rest); i++ {
		switch {
		case rest[i] == "bold":
			d.Kind |= frame.KindBold
		case rest[i] == "italic":
			d.Kind |= frame.KindItalic
		case rest[i] == "hidden":
			d.Kind |= frame.KindHidden
		case rest[i] == "hrule":
			d.Kind |= frame.KindHRule
		case strings.HasPrefix(rest[i], "scale="):
			// B2.2 R4: parse the value and surface both the
			// bit and the float so the frame can pick the
			// matching scaled font.
			v, err := strconv.ParseFloat(rest[i][len("scale="):], 32)
			if err != nil {
				return Directive{}, fmt.Errorf("spans: scale= bad value: %w", err)
			}
			d.Kind |= frame.KindScale
			d.Scale = float32(v)
		case rest[i] == "family=code":
			d.Kind |= frame.KindCodeFamily
		case strings.HasPrefix(rest[i], "family="):
			// silent accept for non-`code` families; no defined
			// rendering for them yet
		default:
			return Directive{}, fmt.Errorf("spans: `s` unknown flag %q", rest[i])
		}
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
