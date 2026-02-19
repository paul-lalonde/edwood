package main

import "strings"

// ansiState represents the parser's current state.
type ansiState int

const (
	stateGround   ansiState = iota // Normal text — printable runes emitted
	stateEsc                       // Received ESC (0x1B), awaiting next byte
	stateCSI                       // CSI introducer received (ESC [)
	stateCSIParam                  // Collecting CSI numeric parameters
	stateOSC                       // OSC introducer received (ESC ])
	stateOSCString                 // Collecting OSC payload string
	stateIgnore                    // Consuming unsupported sequence bytes
)

// ansiParser strips ANSI escape sequences from PTY output and tracks
// cumulative SGR styling state. It persists across Process() calls to
// handle split sequences.
type ansiParser struct {
	state    ansiState
	params   []int  // accumulated CSI parameters
	curParam int    // current parameter being built
	hasParam bool   // true if at least one digit seen for curParam
	private  byte   // private marker ('?', '>', '!') or 0
	oscNum   int    // OSC command number
	oscBuf   []rune // OSC string payload accumulator
	sgr      sgrState
	prevOSC  bool // true if transitioning to stateEsc from an OSC state

	// titleFunc is called when an OSC title sequence is complete.
	// Nil-safe: if nil, OSC titles are silently consumed.
	titleFunc func(title string)
}

// NewAnsiParser creates a parser in the ground state.
func NewAnsiParser(titleFunc func(string)) *ansiParser {
	return &ansiParser{
		state:     stateGround,
		params:    make([]int, 0, 16),
		curParam:  0,
		titleFunc: titleFunc,
	}
}

// Process scans input runes, strips escape sequences, and returns clean
// text plus styled runs. O(n) single pass.
func (p *ansiParser) Process(input []rune) (clean []rune, runs []styledRun) {
	clean = make([]rune, 0, len(input))
	var currentRun styledRun
	currentRun.style = p.sgr // inherit style from previous call

	for _, r := range input {
		switch p.state {

		case stateGround:
			if r == 0x1B {
				p.state = stateEsc
			} else {
				// Printable or C0 control — emit to output.
				if p.sgr != currentRun.style {
					if len(currentRun.text) > 0 {
						runs = append(runs, currentRun)
					}
					currentRun = styledRun{style: p.sgr}
				}
				currentRun.text = append(currentRun.text, r)
				clean = append(clean, r)
			}

		case stateEsc:
			if p.prevOSC && r == '\\' {
				p.dispatchOSC()
				p.prevOSC = false
				p.state = stateGround
				continue
			}
			p.prevOSC = false
			switch r {
			case '[':
				p.resetCSI()
				p.state = stateCSI
			case ']':
				p.oscNum = 0
				p.oscBuf = p.oscBuf[:0]
				p.state = stateOSC
			case '(', ')', '*', '+':
				p.state = stateIgnore
			case '=', '>':
				p.state = stateGround
			case 0x1B:
				p.state = stateEsc // stay
			default:
				p.state = stateGround
			}

		case stateCSI:
			switch {
			case r >= '0' && r <= '9':
				p.curParam = int(r - '0')
				p.hasParam = true
				p.state = stateCSIParam
			case r == ';':
				p.params = append(p.params, 0)
				p.state = stateCSIParam
			case r == '?' || r == '>' || r == '!':
				p.private = byte(r)
				p.state = stateCSIParam
			case r == 'm':
				p.pushParam()
				if p.private == 0 {
					p.dispatchSGR()
				}
				p.state = stateGround
			case r >= 0x40 && r <= 0x7E:
				p.pushParam()
				// non-SGR CSI: silently consumed
				p.state = stateGround
			case r == 0x1B:
				p.state = stateEsc
			}

		case stateCSIParam:
			switch {
			case r >= '0' && r <= '9':
				p.curParam = p.curParam*10 + int(r-'0')
				p.hasParam = true
			case r == ';':
				p.params = append(p.params, p.curParam)
				p.curParam = 0
				p.hasParam = false
			case r == 'm':
				p.pushParam()
				if p.private == 0 {
					p.dispatchSGR()
				}
				p.state = stateGround
			case r >= 0x40 && r <= 0x7E:
				p.pushParam()
				p.state = stateGround
			case r == 0x1B:
				p.state = stateEsc
			}

		case stateOSC:
			switch {
			case r >= '0' && r <= '9':
				p.oscNum = p.oscNum*10 + int(r-'0')
			case r == ';':
				p.state = stateOSCString
			case r == 0x07: // BEL
				p.dispatchOSC()
				p.state = stateGround
			case r == 0x1B:
				p.prevOSC = true
				p.state = stateEsc
			default:
				p.state = stateGround // malformed
			}

		case stateOSCString:
			switch {
			case r == 0x07: // BEL
				p.dispatchOSC()
				p.state = stateGround
			case r == 0x1B:
				p.prevOSC = true
				p.state = stateEsc
			default:
				p.oscBuf = append(p.oscBuf, r)
			}

		case stateIgnore:
			// Consume one byte and return to ground.
			p.state = stateGround
		}
	}

	// Flush remaining run.
	if len(currentRun.text) > 0 {
		runs = append(runs, currentRun)
	}

	return clean, runs
}

// resetCSI clears CSI parameter state for a new sequence.
func (p *ansiParser) resetCSI() {
	p.params = p.params[:0]
	p.curParam = 0
	p.hasParam = false
	p.private = 0
}

// pushParam appends curParam to params and resets for the next parameter.
func (p *ansiParser) pushParam() {
	p.params = append(p.params, p.curParam)
	p.curParam = 0
	p.hasParam = false
}

// dispatchSGR processes accumulated CSI parameters as SGR codes,
// updating p.sgr.
func (p *ansiParser) dispatchSGR() {
	params := p.params
	if len(params) == 0 {
		params = []int{0} // ESC[m = reset
	}

	for i := 0; i < len(params); i++ {
		code := params[i]
		switch {
		case code == 0:
			p.sgr.reset()

		// Attribute on
		case code == 1:
			p.sgr.bold = true
		case code == 2:
			p.sgr.dim = true
		case code == 3:
			p.sgr.italic = true
		case code == 4:
			p.sgr.underline = true
		case code == 5 || code == 6:
			p.sgr.blink = true
		case code == 7:
			p.sgr.inverse = true
		case code == 8:
			p.sgr.hidden = true
		case code == 9:
			p.sgr.strike = true

		// Attribute off
		case code == 21:
			p.sgr.bold = false
		case code == 22:
			p.sgr.bold = false
			p.sgr.dim = false
		case code == 23:
			p.sgr.italic = false
		case code == 24:
			p.sgr.underline = false
		case code == 25:
			p.sgr.blink = false
		case code == 27:
			p.sgr.inverse = false
		case code == 28:
			p.sgr.hidden = false
		case code == 29:
			p.sgr.strike = false

		// Standard foreground colors (30-37)
		case code >= 30 && code <= 37:
			idx := code - 30
			c := ansiPalette[idx]
			p.sgr.fg = ansiColor{set: true, r: c[0], g: c[1], b: c[2]}

		// Extended foreground (38;5;N or 38;2;R;G;B)
		case code == 38:
			i = p.parseExtendedColor(params, i, &p.sgr.fg)

		// Default foreground
		case code == 39:
			p.sgr.fg = ansiColor{}

		// Standard background colors (40-47)
		case code >= 40 && code <= 47:
			idx := code - 40
			c := ansiPalette[idx]
			p.sgr.bg = ansiColor{set: true, r: c[0], g: c[1], b: c[2]}

		// Extended background (48;5;N or 48;2;R;G;B)
		case code == 48:
			i = p.parseExtendedColor(params, i, &p.sgr.bg)

		// Default background
		case code == 49:
			p.sgr.bg = ansiColor{}

		// Bright foreground colors (90-97)
		case code >= 90 && code <= 97:
			idx := code - 90 + 8
			c := ansiPalette[idx]
			p.sgr.fg = ansiColor{set: true, r: c[0], g: c[1], b: c[2]}

		// Bright background colors (100-107)
		case code >= 100 && code <= 107:
			idx := code - 100 + 8
			c := ansiPalette[idx]
			p.sgr.bg = ansiColor{set: true, r: c[0], g: c[1], b: c[2]}

		// Unknown codes: silently ignored
		}
	}
}

// parseExtendedColor handles 38;5;N (256-color) and 38;2;R;G;B (truecolor).
// i is the index of the 38 or 48 parameter. Returns the new index (advanced
// past consumed sub-parameters).
func (p *ansiParser) parseExtendedColor(params []int, i int, target *ansiColor) int {
	if i+1 >= len(params) {
		return i // malformed, ignore
	}
	switch params[i+1] {
	case 5: // 256-color: 38;5;N
		if i+2 >= len(params) {
			return i + 1 // malformed
		}
		idx := params[i+2]
		if idx >= 0 && idx <= 255 {
			c := ansiPalette[idx]
			*target = ansiColor{set: true, r: c[0], g: c[1], b: c[2]}
		}
		return i + 2

	case 2: // truecolor: 38;2;R;G;B
		if i+4 >= len(params) {
			return i + 1 // malformed
		}
		r := clampByte(params[i+2])
		g := clampByte(params[i+3])
		b := clampByte(params[i+4])
		*target = ansiColor{set: true, r: r, g: g, b: b}
		return i + 4

	default:
		return i + 1 // unknown sub-type, skip
	}
}

func clampByte(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// formatWindowTitle produces an acme window name from an OSC title.
// If the title already contains "-", it is used as-is.
// Otherwise, "/-" and sysname are appended.
func formatWindowTitle(title, sysname string) string {
	if strings.Contains(title, "-") {
		return title
	}
	return title + "/-" + sysname
}

// dispatchOSC handles completed OSC sequences. OSC 0/1/2 invoke the
// titleFunc callback; all other OSC numbers are silently consumed.
func (p *ansiParser) dispatchOSC() {
	switch p.oscNum {
	case 0, 1, 2:
		title := string(p.oscBuf)
		if p.titleFunc != nil {
			p.titleFunc(title)
		}
	}
}
