package walkthrough

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// TestSchemaMatchesDraftFields keeps the published schema's properties and
// required fields aligned with the JSON fields Decode accepts.
func TestSchemaMatchesDraftFields(t *testing.T) {
	type object struct {
		Properties           map[string]json.RawMessage `json:"properties"`
		Required             []string                   `json:"required"`
		AdditionalProperties *bool                      `json:"additionalProperties"`
	}
	var schema struct {
		object
		Defs map[string]object `json:"$defs"`
	}
	if err := json.Unmarshal(Schema, &schema); err != nil {
		t.Fatal(err)
	}
	for name, pair := range map[string]struct {
		schema object
		value  any
	}{
		"draft":      {schema.object, Draft{}},
		"step":       {schema.Defs["step"], Step{}},
		"fileTarget": {schema.Defs["fileTarget"], FileTarget{}},
		"diffTarget": {schema.Defs["diffTarget"], DiffTarget{}},
		"compare":    {schema.Defs["compare"], Compare{}},
	} {
		var properties []string
		for property := range pair.schema.Properties {
			properties = append(properties, property)
		}
		var fields, required []string
		kind := reflect.TypeOf(pair.value)
		for i := range kind.NumField() {
			tag := kind.Field(i).Tag.Get("json")
			field, options, _ := strings.Cut(tag, ",")
			fields = append(fields, field)
			if !strings.Contains(options, "omitempty") {
				required = append(required, field)
			}
		}
		slices.Sort(properties)
		slices.Sort(fields)
		slices.Sort(required)
		schemaRequired := slices.Sorted(slices.Values(pair.schema.Required))
		if !slices.Equal(properties, fields) {
			t.Errorf("%s properties = %v, Go fields = %v", name, properties, fields)
		}
		if !slices.Equal(schemaRequired, required) {
			t.Errorf("%s required = %v, Go fields without omitempty = %v", name, schemaRequired, required)
		}
		if pair.schema.AdditionalProperties == nil || *pair.schema.AdditionalProperties {
			t.Errorf("%s allows additional properties; Decode rejects them", name)
		}
	}
}
