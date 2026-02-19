package main

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

// parseColor parses a color string: "-" for default (nil), or "#rrggbb" for an explicit color.
func parseColor(s string) (color.Color, error) {
	if s == "-" {
		return nil, nil
	}
	if len(s) != 7 || s[0] != '#' {
		return nil, fmt.Errorf("bad color value: %q", s)
	}
	r, err := strconv.ParseUint(s[1:3], 16, 8)
	if err != nil {
		return nil, fmt.Errorf("bad color value: %q", s)
	}
	g, err := strconv.ParseUint(s[3:5], 16, 8)
	if err != nil {
		return nil, fmt.Errorf("bad color value: %q", s)
	}
	b, err := strconv.ParseUint(s[5:7], 16, 8)
	if err != nil {
		return nil, fmt.Errorf("bad color value: %q", s)
	}
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 0xff}, nil
}

// parseSpanMessage parses a spans file write using the prefixed message format.
// Each line begins with a single-letter prefix: "c" (clear), "s" (span), "b" (box).
// Returns the parsed runs, region start offset, whether this is a clear command, and any error.
func parseSpanMessage(data string, bufLen int) (runs []StyleRun, regionStart int, isClear bool, err error) {
	lines := strings.Split(data, "\n")
	regionStart = -1
	expectedOffset := -1

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		prefix := fields[0]

		switch prefix {
		case "c":
			if len(runs) > 0 {
				return nil, 0, false, fmt.Errorf("clear must be the only command in a write")
			}
			return nil, 0, true, nil

		case "s":
			offset, length, run, parseErr := parseSpanLine(fields[1:])
			if parseErr != nil {
				return nil, 0, false, parseErr
			}

			// Silently discard spans that start past the buffer end.
			if offset >= bufLen {
				break
			}

			if regionStart == -1 {
				regionStart = offset
				expectedOffset = offset
			}
			if offset != expectedOffset {
				return nil, 0, false, fmt.Errorf("spans must be contiguous: expected offset %d, got %d", expectedOffset, offset)
			}
			expectedOffset = offset + length
			runs = append(runs, run)

		case "b":
			offset, length, run, parseErr := parseBoxLine(fields[1:])
			if parseErr != nil {
				return nil, 0, false, parseErr
			}

			// Silently discard spans that start past the buffer end.
			if offset >= bufLen {
				break
			}

			if regionStart == -1 {
				regionStart = offset
				expectedOffset = offset
			}
			if offset != expectedOffset {
				return nil, 0, false, fmt.Errorf("spans must be contiguous: expected offset %d, got %d", expectedOffset, offset)
			}
			expectedOffset = offset + length
			runs = append(runs, run)

		default:
			return nil, 0, false, fmt.Errorf("unknown span command: %q", prefix)
		}
	}

	if regionStart == -1 {
		regionStart = 0
	}

	// Clamp region to buffer length.
	runs = clampRunsToBuffer(runs, regionStart, bufLen)

	return runs, regionStart, false, nil
}

// parseSpanLine parses the fields after the "s" prefix.
// Format: offset length fg-color [bg-color] [flags...]
func parseSpanLine(fields []string) (offset, length int, run StyleRun, err error) {
	if len(fields) < 3 {
		return 0, 0, StyleRun{}, fmt.Errorf("bad span format: need at least offset length color")
	}

	offset, err = strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, StyleRun{}, fmt.Errorf("bad span offset: %q", fields[0])
	}

	length, err = strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, StyleRun{}, fmt.Errorf("bad span length: %q", fields[1])
	}

	if offset < 0 || length < 0 {
		return 0, 0, StyleRun{}, fmt.Errorf("negative span offset or length")
	}

	fg, err := parseColor(fields[2])
	if err != nil {
		return 0, 0, StyleRun{}, err
	}

	var bg color.Color
	flagStart := 3

	if len(fields) > 3 {
		f3 := fields[3]
		if f3 == "-" || strings.HasPrefix(f3, "#") {
			bg, err = parseColor(f3)
			if err != nil {
				return 0, 0, StyleRun{}, err
			}
			flagStart = 4
		}
	}

	var bold, italic, hidden bool
	for _, flag := range fields[flagStart:] {
		switch flag {
		case "bold":
			bold = true
		case "italic":
			italic = true
		case "hidden":
			hidden = true
		default:
			return 0, 0, StyleRun{}, fmt.Errorf("unknown span flag: %q", flag)
		}
	}

	run = StyleRun{
		Len: length,
		Style: StyleAttrs{
			Fg:     fg,
			Bg:     bg,
			Bold:   bold,
			Italic: italic,
			Hidden: hidden,
		},
	}
	return offset, length, run, nil
}

// parseBoxLine parses the fields after the "b" prefix.
// Format: offset length width height [fg-color] [bg-color] [flags...] [payload...]
func parseBoxLine(fields []string) (offset, length int, run StyleRun, err error) {
	if len(fields) < 4 {
		return 0, 0, StyleRun{}, fmt.Errorf("bad box format: need at least offset length width height")
	}

	offset, err = strconv.Atoi(fields[0])
	if err != nil {
		return 0, 0, StyleRun{}, fmt.Errorf("bad box offset: %q", fields[0])
	}

	length, err = strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, StyleRun{}, fmt.Errorf("bad box length: %q", fields[1])
	}

	if offset < 0 || length < 0 {
		return 0, 0, StyleRun{}, fmt.Errorf("negative box offset or length")
	}

	width, err := strconv.Atoi(fields[2])
	if err != nil {
		return 0, 0, StyleRun{}, fmt.Errorf("bad box width: %q", fields[2])
	}

	height, err := strconv.Atoi(fields[3])
	if err != nil {
		return 0, 0, StyleRun{}, fmt.Errorf("bad box height: %q", fields[3])
	}

	if width < 0 || height < 0 {
		return 0, 0, StyleRun{}, fmt.Errorf("negative box width or height")
	}

	// Parse optional colors, flags, and payload from remaining fields.
	var fg, bg color.Color
	var bold, italic, hidden bool
	var payloadParts []string
	idx := 4
	inPayload := false

	for idx < len(fields) {
		f := fields[idx]
		if inPayload {
			payloadParts = append(payloadParts, f)
			idx++
			continue
		}

		// Try to parse as a color.
		if f == "-" || strings.HasPrefix(f, "#") {
			c, cerr := parseColor(f)
			if cerr != nil {
				return 0, 0, StyleRun{}, cerr
			}
			if fg == nil && bg == nil {
				// First color seen after width/height is fg.
				// But we need to check: if fg was already set to nil via "-",
				// we use a sentinel. Use a different approach:
				// First color slot goes to fg, second to bg.
				fg = c
				// Mark that we consumed a fg color, even if it was nil ("-").
				// We handle this by tracking whether we've seen colors.
				idx++
				// Check if next field is also a color.
				if idx < len(fields) {
					f2 := fields[idx]
					if f2 == "-" || strings.HasPrefix(f2, "#") {
						bg, err = parseColor(f2)
						if err != nil {
							return 0, 0, StyleRun{}, err
						}
						idx++
					}
				}
			}
			continue
		}

		// Try known flags.
		switch f {
		case "bold":
			bold = true
			idx++
		case "italic":
			italic = true
			idx++
		case "hidden":
			hidden = true
			idx++
		default:
			// Start of payload — collect this and all remaining tokens.
			inPayload = true
			payloadParts = append(payloadParts, f)
			idx++
		}
	}

	payload := strings.Join(payloadParts, " ")

	run = StyleRun{
		Len: length,
		Style: StyleAttrs{
			Fg:         fg,
			Bg:         bg,
			Bold:       bold,
			Italic:     italic,
			Hidden:     hidden,
			IsBox:      true,
			BoxWidth:   width,
			BoxHeight:  height,
			BoxPayload: payload,
		},
	}
	return offset, length, run, nil
}

// parseSpanDefs is the legacy parser for unprefixed span definitions.
// Each line has the format: offset length fg-color [bg-color] [flags...]
// Returns the parsed runs, the region start offset, and any error.
func parseSpanDefs(data string, bufLen int) ([]StyleRun, int, error) {
	lines := strings.Split(data, "\n")
	runs := make([]StyleRun, 0, len(lines))
	regionStart := -1
	expectedOffset := -1

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			return nil, 0, fmt.Errorf("bad span format: need at least offset length color")
		}

		// Parse offset.
		offset, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, 0, fmt.Errorf("bad span offset: %q", fields[0])
		}

		// Parse length.
		length, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, 0, fmt.Errorf("bad span length: %q", fields[1])
		}

		// Validate non-negative.
		if offset < 0 || length < 0 {
			return nil, 0, fmt.Errorf("negative span offset or length")
		}

		// Silently discard spans that start past the buffer end.
		// This can happen when the coloring tool read a stale body snapshot.
		if offset >= bufLen {
			break
		}

		// Set region start from first span.
		if regionStart == -1 {
			regionStart = offset
			expectedOffset = offset
		}

		// Validate contiguity.
		if offset != expectedOffset {
			return nil, 0, fmt.Errorf("spans must be contiguous: expected offset %d, got %d", expectedOffset, offset)
		}
		expectedOffset = offset + length

		// Parse fg-color.
		fg, err := parseColor(fields[2])
		if err != nil {
			return nil, 0, err
		}

		// Parse optional bg-color and flags.
		var bg color.Color
		flagStart := 3

		if len(fields) > 3 {
			f3 := fields[3]
			if f3 == "-" || strings.HasPrefix(f3, "#") {
				bg, err = parseColor(f3)
				if err != nil {
					return nil, 0, err
				}
				flagStart = 4
			}
		}

		// Parse flags.
		var bold, italic, hidden bool
		for _, flag := range fields[flagStart:] {
			switch flag {
			case "bold":
				bold = true
			case "italic":
				italic = true
			case "hidden":
				hidden = true
			default:
				return nil, 0, fmt.Errorf("unknown span flag: %q", flag)
			}
		}

		runs = append(runs, StyleRun{
			Len: length,
			Style: StyleAttrs{
				Fg:     fg,
				Bg:     bg,
				Bold:   bold,
				Italic: italic,
				Hidden: hidden,
			},
		})
	}

	if regionStart == -1 {
		regionStart = 0
	}

	runs = clampRunsToBuffer(runs, regionStart, bufLen)

	return runs, regionStart, nil
}

// isPrefixedFormat returns true if data uses the new prefixed message format.
// It checks whether the first non-empty line starts with a recognized prefix
// ("c", "s", or "b") followed by either end-of-line or a space.
func isPrefixedFormat(data string) bool {
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// A line starting with "c", "s", or "b" followed by space or end-of-line.
		if len(line) == 1 {
			return line == "c" || line == "s" || line == "b"
		}
		if line[1] == ' ' || line[1] == '\t' {
			return line[0] == 'c' || line[0] == 's' || line[0] == 'b'
		}
		// Not prefixed format (first token is numeric or something else).
		return false
	}
	return false
}

// clampRunsToBuffer truncates trailing runs so the region fits within bufLen.
func clampRunsToBuffer(runs []StyleRun, regionStart, bufLen int) []StyleRun {
	totalLen := 0
	for _, r := range runs {
		totalLen += r.Len
	}
	if regionStart+totalLen > bufLen {
		excess := regionStart + totalLen - bufLen
		for i := len(runs) - 1; i >= 0 && excess > 0; i-- {
			if runs[i].Len <= excess {
				excess -= runs[i].Len
				runs[i].Len = 0
			} else {
				runs[i].Len -= excess
				excess = 0
			}
		}
		// Remove zero-length trailing runs.
		for len(runs) > 0 && runs[len(runs)-1].Len == 0 {
			runs = runs[:len(runs)-1]
		}
	}
	return runs
}
