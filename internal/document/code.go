package document

// CodeHandler is reusable for explicitly registered source languages and image
// families: source and no HTML preview.
type CodeHandler struct {
	Definition Type
}

func (h CodeHandler) Type() Type                              { return h.Definition }
func (h CodeHandler) Preview([]byte, Context) (string, error) { return "", nil }

// FormattedCodeHandler adds an on-demand, read-only formatted view to
// source. FormatSource receives a copy of the file bytes.
type FormattedCodeHandler struct {
	CodeHandler
	FormatSource func([]byte) ([]byte, error)
}

func (h FormattedCodeHandler) Format(source []byte) ([]byte, error) {
	return h.FormatSource(append([]byte{}, source...))
}
