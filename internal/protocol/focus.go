package protocol

// Focus selects the internal prompt framing preset.
type Focus string

const (
	FocusCode   Focus = "code"
	FocusReview Focus = "review"
	FocusDesign Focus = "design"
)

// NormalizeFocus maps unknown or empty values back to the default focus.
func NormalizeFocus(focus Focus) Focus {
	switch focus {
	case FocusReview, FocusDesign:
		return focus
	default:
		return FocusCode
	}
}

// String returns the canonical string form for the focus preset.
func (f Focus) String() string {
	return string(NormalizeFocus(f))
}
