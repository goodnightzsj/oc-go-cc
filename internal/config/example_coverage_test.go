package config

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// The example file is the operator's reference. It used to omit five top-level
// fields, including the whole cline_pass block - the platform production runs
// on - while the project claimed it documented every option.
func TestExampleConfigCoversEveryTopLevelField(t *testing.T) {
	raw, err := os.ReadFile("../../configs/config.example.json")
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	var example map[string]json.RawMessage
	if err := json.Unmarshal(raw, &example); err != nil {
		t.Fatalf("parse example: %v", err)
	}

	configType := reflect.TypeOf(Config{})
	for i := 0; i < configType.NumField(); i++ {
		field := configType.Field(i)
		if !field.IsExported() {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		if _, ok := example[name]; !ok {
			t.Errorf("config.example.json is missing %q (struct field %s)", name, field.Name)
		}
	}
}
