package compatir

type WarningCode string

const (
	WarningUnsupportedField WarningCode = "unsupported_field"
	WarningLossyConversion  WarningCode = "lossy_conversion"
	WarningIgnoredField     WarningCode = "ignored_field"
)

type ConversionWarning struct {
	Code    WarningCode `json:"code"`
	Field   string      `json:"field,omitempty"`
	Message string      `json:"message,omitempty"`
}
