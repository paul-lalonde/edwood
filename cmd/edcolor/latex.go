package main

type latexToken struct {
	start, end int // byte offsets
	kind       int
}

// tokenizeLatex lexes src as LaTeX source and returns colored regions.
func tokenizeLatex(src string) []region {
	b2r := byteToRuneIndex(src)
	tokens := lexLatex(src)

	var regions []region
	for _, t := range tokens {
		runeStart := b2r[t.start]
		runeEnd := b2r[t.end]
		if runeEnd <= runeStart {
			continue
		}
		var color string
		var bold bool
		switch t.kind {
		case tokKeyword:
			color, bold = colorKeyword, true
		case tokComment:
			color = colorComment
		case tokString:
			color = colorString
		case tokBuiltin:
			color = colorBuiltin
		}
		regions = append(regions, region{runeStart, runeEnd, color, bold})
	}

	return regions
}

// lexLatex tokenizes LaTeX source and returns tokens for
// commands, comments, math mode, and environment names.
func lexLatex(src string) []latexToken {
	var tokens []latexToken
	i := 0
	n := len(src)

	for i < n {
		c := src[i]

		// Comment: % to end of line (but not \%).
		if c == '%' && (i == 0 || src[i-1] != '\\') {
			start := i
			for i < n && src[i] != '\n' {
				i++
			}
			tokens = append(tokens, latexToken{start, i, tokComment})
			continue
		}

		// Command: \ followed by letters or a single non-letter char.
		if c == '\\' && i+1 < n {
			next := src[i+1]

			// \letter... — command name.
			if isLetter(next) {
				start := i
				i += 2
				for i < n && isLetter(src[i]) {
					i++
				}
				cmd := src[start:i]

				tokens = append(tokens, latexToken{start, i, tokKeyword})

				// If command is \begin or \end, look for {envname}.
				if cmd == `\begin` || cmd == `\end` {
					// Skip optional whitespace.
					j := i
					for j < n && (src[j] == ' ' || src[j] == '\t') {
						j++
					}
					if j < n && src[j] == '{' {
						j++ // skip '{'
						envStart := j
						for j < n && src[j] != '}' && src[j] != '\n' {
							j++
						}
						if j < n && src[j] == '}' {
							if envStart < j {
								tokens = append(tokens, latexToken{envStart, j, tokBuiltin})
							}
							j++ // skip '}'
						}
						i = j
					}
				}
				continue
			}

			// \<non-letter> — single-char command (\\, \%, \$, \{, etc.).
			start := i
			i += 2
			tokens = append(tokens, latexToken{start, i, tokKeyword})
			continue
		}

		// Display math: $$...$$
		if c == '$' && i+1 < n && src[i+1] == '$' {
			start := i
			i += 2
			for i+1 < n {
				if src[i] == '\\' {
					i += 2 // skip escaped char
					continue
				}
				if src[i] == '$' && i+1 < n && src[i+1] == '$' {
					i += 2
					break
				}
				i++
			}
			// Handle unterminated display math at EOF.
			if i > n {
				i = n
			}
			tokens = append(tokens, latexToken{start, i, tokString})
			continue
		}

		// Inline math: $...$
		if c == '$' {
			start := i
			i++
			for i < n {
				if src[i] == '\\' && i+1 < n {
					i += 2 // skip escaped char
					continue
				}
				if src[i] == '$' {
					i++
					break
				}
				if src[i] == '\n' {
					// Allow math across lines, but stop at double-newline
					// to avoid runaway highlighting.
					if i+1 < n && src[i+1] == '\n' {
						break
					}
				}
				i++
			}
			tokens = append(tokens, latexToken{start, i, tokString})
			continue
		}

		// Everything else: skip.
		i++
	}

	return tokens
}

// isLetter reports whether c is an ASCII letter.
func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
