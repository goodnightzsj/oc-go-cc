package config

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestMigrationPreservesEnvironmentKeyPool(t *testing.T) {
	for _, block := range []string{"", "opencode_go", "opencode_zen", "aws_bedrock", "openrouter", "commandcode", "cline_pass"} {
		for _, overrides := range [][2]string{{"synthetic-env", ""}, {"", "synthetic-env"}, {"synthetic-env", ","}, {"", ","}} {
			t.Run(block+strings.Join(overrides[:], "/"), func(t *testing.T) {
				prefix := "ROUTATIC_PROXY"
				if block != "" {
					prefix += "_" + strings.ToUpper(block)
				}
				t.Setenv(prefix+"_API_KEY", overrides[0])
				t.Setenv(prefix+"_API_KEYS", overrides[1])
				t.Setenv("MIGRATION_TEST_KEY", "synthetic-file")
				fields := map[string]any{"api_key": "${MIGRATION_TEST_KEY}", "api_keys": []string{"synthetic-array"}}
				var root any = fields
				if block != "" {
					root = map[string]any{block: fields}
				}
				data, _ := json.Marshal(root)
				before, err := LoadJSON(data)
				if err != nil {
					t.Fatal(err)
				}
				migrated, changed, err := MigrateKeyPools(data)
				if err != nil {
					t.Fatal(err)
				}
				after, err := LoadJSON(migrated)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(before.KeyPools()[block], after.KeyPools()[block]) {
					t.Fatal("migration changed the effective key pool or origins")
				}
				if changed || string(migrated) != string(data) {
					t.Fatal("environment-overridden field layout must remain unchanged")
				}
			})
		}
	}
}

func TestMigrationPreservesLegacyEnvironmentKey(t *testing.T) {
	t.Setenv("ROUTATIC_PROXY_API_KEY", "")
	t.Setenv("OC_GO_CC_API_KEY", "synthetic-legacy")
	data := []byte(`{"api_key":"synthetic-file"}`)
	migrated, changed, err := MigrateKeyPools(data)
	if err != nil {
		t.Fatal(err)
	}
	if changed || string(migrated) != string(data) {
		t.Fatal("legacy override changed field layout")
	}
	cfg, err := LoadJSON(migrated)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.KeyPools()[""].Keys, []string{"synthetic-legacy"}) {
		t.Fatal("legacy override revived file key")
	}
}
