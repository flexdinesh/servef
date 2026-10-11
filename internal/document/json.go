package document

import (
	"bytes"
	"encoding/json"
)

func NewJSON() Handler {
	return CodeHandler{
		Definition: Type{ID: "json", Label: "JSON", Extensions: []string{".json"}, Language: "json", PreviewKind: "code", PreviewLabel: "Formatted", DefaultMode: "preview"},
		Format:     formatJSON,
	}
}

func formatJSON(source []byte) ([]byte, error) {
	var output bytes.Buffer
	// Indent retains key order, duplicate keys, and numeric spelling/precision.
	if err := json.Indent(&output, source, "", "  "); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
