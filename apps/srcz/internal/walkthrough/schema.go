package walkthrough

import _ "embed"

// Schema is the JSON Schema of a Draft, served to agents.
//
//go:embed schema.json
var Schema []byte
