package document

import (
	"bytes"
	"errors"
	"io"

	"go.yaml.in/yaml/v3"
)

func NewYAML() Handler {
	return FormattedCodeHandler{
		CodeHandler:  CodeHandler{Definition: Type{ID: "yaml", Label: "YAML", Extensions: []string{".yaml", ".yml"}, Language: "yaml", DefaultMode: "source"}},
		FormatSource: formatYAML,
	}
}

func formatYAML(source []byte) ([]byte, error) {
	var output bytes.Buffer
	decoder := yaml.NewDecoder(bytes.NewReader(source))
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	for {
		// Nodes preserve comments, order, scalar styles, and aliases. Do not load
		// into a generic map, resolve aliases, or ignore subsequent documents.
		var node yaml.Node
		err := decoder.Decode(&node)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if err := encoder.Encode(&node); err != nil {
			return nil, err
		}
	}
	// A comment-only stream has no decoded document; retain its original text.
	if output.Len() == 0 {
		return append([]byte{}, source...), nil
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
