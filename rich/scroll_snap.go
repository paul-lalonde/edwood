package rich

// ScrollSnap selects how layoutFromOrigin aligns visible lines to
// the viewport edges. See docs/designs/features/unified-scrollbar.md
// § "Scroll snap policy (rich mode)" for the policy and rationale.
type ScrollSnap int

const (
	// SnapTop aligns the first visible line's top to the viewport
	// top. This is the default and matches a freshly-loaded
	// document. Mid-document the last line may be partially clipped
	// at the bottom; that's the accepted trade-off.
	SnapTop ScrollSnap = iota

	// SnapBottom aligns the last visible line's bottom to the
	// viewport bottom; the first visible line absorbs the
	// partial-line clipping. Set by B1 click handlers (revealing
	// earlier content) so the bottom edge of the new viewport
	// remains a clean line boundary.
	SnapBottom

	// SnapPixel honors originYOffset literally with no
	// line-boundary alignment. Forced when the origin line is
	// taller than the viewport (e.g. a large image), where
	// line-level snapping would prevent the user from scrolling
	// within the line pixel-by-pixel.
	SnapPixel
)
