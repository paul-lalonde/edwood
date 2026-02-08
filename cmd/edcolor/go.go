package main

import (
	"go/scanner"
	"go/token"
)

// Predeclared Go identifiers that get special coloring.
var goBuiltins = map[string]bool{
	// Types
	"bool": true, "byte": true, "complex64": true, "complex128": true,
	"error": true, "float32": true, "float64": true, "int": true,
	"int8": true, "int16": true, "int32": true, "int64": true,
	"rune": true, "string": true, "uint": true, "uint8": true,
	"uint16": true, "uint32": true, "uint64": true, "uintptr": true,
	"any": true, "comparable": true,
	// Functions
	"append": true, "cap": true, "clear": true, "close": true,
	"complex": true, "copy": true, "delete": true, "imag": true,
	"len": true, "make": true, "max": true, "min": true,
	"new": true, "panic": true, "print": true, "println": true,
	"real": true, "recover": true,
	// Constants
	"true": true, "false": true, "nil": true, "iota": true,
}

// tokenizeGo lexes src as Go source and returns colored regions.
func tokenizeGo(src string) []region {
	b2r := byteToRuneIndex(src)

	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))

	var s scanner.Scanner
	// Suppress error printing; color what we can.
	s.Init(file, []byte(src), func(token.Position, string) {}, scanner.ScanComments)

	var regions []region

	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}

		// Skip auto-inserted semicolons; they have no source text.
		if tok == token.SEMICOLON && lit == "\n" {
			continue
		}

		byteOff := int(pos) - file.Base()
		byteEnd := byteOff
		if lit != "" {
			byteEnd += len(lit)
		} else {
			byteEnd += len(tok.String())
		}

		color, bold := goTokenStyle(tok, lit)
		if color == "" {
			continue
		}

		runeStart := b2r[byteOff]
		runeEnd := b2r[byteEnd]
		if runeEnd > runeStart {
			regions = append(regions, region{runeStart, runeEnd, color, bold})
		}
	}

	return regions
}

// goTokenStyle returns the color and bold flag for a Go token.
// An empty color means use default (skip coloring this token).
func goTokenStyle(tok token.Token, lit string) (color string, bold bool) {
	switch {
	case tok.IsKeyword():
		return colorKeyword, true
	case tok == token.COMMENT:
		return colorComment, false
	case tok == token.STRING, tok == token.CHAR:
		return colorString, false
	case tok == token.INT, tok == token.FLOAT, tok == token.IMAG:
		return colorNumber, false
	case tok == token.IDENT && goBuiltins[lit]:
		return colorBuiltin, false
	default:
		return "", false
	}
}
