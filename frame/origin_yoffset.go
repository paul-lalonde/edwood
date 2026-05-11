package frame

// Phase A2.3 stubs for the tall-element y-offset surface. Slice A
// has no replaced-element machinery and no variable line heights,
// so these are no-ops: SetOriginYOffset accepts any argument
// without effect, and GetOriginYOffset always returns 0. Real
// behavior — clipping the top of the first visible line and
// tracking sub-element scroll — arrives in Slice C row C2.

func (f *frameimpl) SetOriginYOffset(yPx int) {
	// Intentional no-op for Slice A. See origin_yoffset.go header.
	_ = yPx
}

func (f *frameimpl) GetOriginYOffset() int {
	return 0
}
