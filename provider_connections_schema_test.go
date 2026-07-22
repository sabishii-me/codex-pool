package main

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestProviderConnectionsV2SchemaMatchesCanonicalDTO(t *testing.T) {
	data, err := os.ReadFile("schemas/provider-connections-v2.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Definitions map[string]struct {
			Required   []string                   `json:"required"`
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	definition, ok := schema.Definitions["OperatorProviderConnectionV2"]
	if !ok {
		t.Fatal("schema lacks OperatorProviderConnectionV2 definition")
	}
	required := make(map[string]bool, len(definition.Required))
	for _, name := range definition.Required {
		required[name] = true
	}
	typeOf := reflect.TypeOf(OperatorProviderConnectionView{})
	seen := make(map[string]bool, typeOf.NumField())
	for index := 0; index < typeOf.NumField(); index++ {
		field := typeOf.Field(index)
		parts := strings.Split(field.Tag.Get("json"), ",")
		name := parts[0]
		if name == "" || name == "-" {
			continue
		}
		seen[name] = true
		if _, ok := definition.Properties[name]; !ok {
			t.Errorf("canonical DTO field %q is absent from schema", name)
		}
		optional := len(parts) > 1 && parts[1] == "omitempty"
		if required[name] == optional {
			t.Errorf("field %q required mismatch: schema=%v omitempty=%v", name, required[name], optional)
		}
	}
	for name := range definition.Properties {
		if !seen[name] {
			t.Errorf("schema field %q is absent from canonical DTO", name)
		}
	}
}
