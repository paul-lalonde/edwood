package main

import (
	"fmt"
	"strings"
)

// buildSpanWrite converts styled runs into span protocol messages.
// Returns empty string if all runs use default styling.
func buildSpanWrite(baseOffset int, runs []styledRun) string {
	// Default optimization: if every run is default, skip span generation.
	allDefault := true
	for _, r := range runs {
		if !isDefaultStyle(r.style) {
			allDefault = false
			break
		}
	}
	if allDefault {
		return ""
	}

	var b strings.Builder
	offset := baseOffset
	for _, r := range runs {
		length := len(r.text)
		if length == 0 {
			continue
		}
		fg, bg := resolveColors(r.style)
		fgHex := colorToHex(fg)
		bgHex := colorToHex(bg)
		flags := buildFlags(r.style)

		fmt.Fprintf(&b, "s %d %d %s", offset, length, fgHex)

		if bgHex != "-" || len(flags) > 0 {
			fmt.Fprintf(&b, " %s", bgHex)
		}
		for _, flag := range flags {
			fmt.Fprintf(&b, " %s", flag)
		}
		b.WriteByte('\n')
		offset += length
	}
	return b.String()
}

// buildFlags returns span flag tokens for the given style.
func buildFlags(s sgrState) []string {
	var flags []string
	if s.bold {
		flags = append(flags, "bold")
	}
	if s.italic {
		flags = append(flags, "italic")
	}
	if s.hidden {
		flags = append(flags, "hidden")
	}
	return flags
}
