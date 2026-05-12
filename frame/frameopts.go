package frame

import (
	"github.com/rjkroege/edwood/draw"
)

// optioncontext is context passed into each option function
// that aggregates knowledge about additional updates needed
// to do to the Frame object that should only be one once per
// call to Init.
type optioncontext struct {
	updatetick  bool // True if the tick needs to initialized
	maxtabchars int  // Number of '0' characters that should be the width of a tab.
}

// Option handling per https://commandcenter.blogspot.ca/2014/01/self-referential-functions-and-design.html
//
// Returns true if the option requires resetting the tick.
// TODO(rjk): It is possible to generalize this as needed with a more
// complex state object. One might imagine a set of updater functions?
type OptionClosure func(*frameimpl, *optioncontext)

// Option sets the options specified and returns true if
// we need to init the tick.
func (f *frameimpl) Option(opts ...OptionClosure) *optioncontext {
	ctx := &optioncontext{
		updatetick:  false,
		maxtabchars: -1,
	}

	for _, opt := range opts {
		opt(f, ctx)
	}
	return ctx
}

// OptColors sets the default colours.
func OptColors(cols [NumColours]draw.Image) OptionClosure {
	return func(f *frameimpl, ctx *optioncontext) {
		f.cols = cols
		// TODO(rjk): I think so. Make sure that this is required.
		ctx.updatetick = true
	}
}

// OptBackground sets the background screen image.
func OptBackground(b draw.Image) OptionClosure {
	return func(f *frameimpl, ctx *optioncontext) {
		f.background = b
		// TODO(rjk): This is safe but is it necessary? I think so.
		ctx.updatetick = true
	}
}

// OptFont sets the default font.
func OptFont(ft draw.Font) OptionClosure {
	return func(f *frameimpl, ctx *optioncontext) {
		f.font = ft
		ctx.updatetick = f.defaultfontheight != f.font.Height()
	}
}

// OptBoldFont sets the font used for runs whose Style.Kind has
// KindBold (and not KindItalic). nil falls back to the base font
// — Slice B's "no font height change" guarantee assumes the
// caller picks a variant of the same family / metrics.
func OptBoldFont(ft draw.Font) OptionClosure {
	return func(f *frameimpl, ctx *optioncontext) {
		f.fontBold = ft
	}
}

// OptItalicFont sets the font used for runs whose Style.Kind has
// KindItalic (and not KindBold). nil falls back to the base font.
func OptItalicFont(ft draw.Font) OptionClosure {
	return func(f *frameimpl, ctx *optioncontext) {
		f.fontItalic = ft
	}
}

// OptBoldItalicFont sets the font used for runs whose Style.Kind
// has both KindBold and KindItalic. nil falls back to fontBold,
// then fontItalic, then the base font.
func OptBoldItalicFont(ft draw.Font) OptionClosure {
	return func(f *frameimpl, ctx *optioncontext) {
		f.fontBoldItalic = ft
	}
}

// OptCodeFont sets the monospace ("code") variant used for runs
// whose Style.Kind has KindCodeFamily (md2spans's inline code and
// fenced-code-block body). Family is a stronger choice than
// weight or italic — code wins over bold/italic in fontFor — so
// the variant is a single slot (no bold-code or italic-code
// sub-variants in v1). nil falls back to the base font.
func OptCodeFont(ft draw.Font) OptionClosure {
	return func(f *frameimpl, ctx *optioncontext) {
		f.fontCode = ft
	}
}

// OptMaxTab sets the default tabwidth in `0` characters.
func OptMaxTab(maxtabchars int) OptionClosure {
	return func(f *frameimpl, ctx *optioncontext) {
		ctx.maxtabchars = maxtabchars
	}
}

// computemaxtab returns the new ftw value
func (ctx *optioncontext) computemaxtab(maxtab, ftw int) int {
	if ctx.maxtabchars < 0 {
		return maxtab
	}
	return ctx.maxtabchars * ftw
}
