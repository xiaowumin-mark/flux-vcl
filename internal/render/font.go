package render

// FontController is the optional native capability for applying an effective
// FontSpec to a control. Renderer intentionally remains source-compatible with
// pre-CD2 backends; layout still uses StyledTextMeasurer or TextExtent when
// this capability is absent.
type FontController interface {
	SetFont(Handle, FontSpec)
}
