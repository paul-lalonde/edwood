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

// parseSpanDefs parses span definition lines from a spans file write.
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

	// Clamp region to buffer length. The coloring tool may have read
	// a stale snapshot of the body; rather than rejecting the entire
	// write, truncate the trailing run so the spans fit. The tool
	// will re-color on the next edit event.
	totalLen := 0
	for _, r := range runs {
		totalLen += r.Len
	}
	if regionStart+totalLen > bufLen {
		excess := regionStart + totalLen - bufLen
		// Walk backwards, trimming runs until the excess is absorbed.
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

	return runs, regionStart, nil
}
