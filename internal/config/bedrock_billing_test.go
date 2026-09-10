package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBedrockBillingConfiguration(t *testing.T) {
	for _, name := range []string{"ROUTATIC_PROXY_AWS_BILLING_ENABLED", "ROUTATIC_PROXY_AWS_BILLING_PROFILE", "ROUTATIC_PROXY_AWS_BILLING_LINKED_ACCOUNT_ID"} {
		t.Setenv(name, "")
	}
	for _, tc := range []struct {
		name, billing string
		valid         bool
	}{
		{"disabled by default", `{}`, true},
		{"explicit scope", `{"enabled":true,"profile":"billing-readonly","linked_account_id":"123456789012"}`, true},
		{"unscoped enabled", `{"enabled":true}`, false},
		{"bad account", `{"enabled":true,"linked_account_id":"12345678901x"}`, false},
		{"short account", `{"linked_account_id":"123"}`, false},
		{"unresolved profile", `{"profile":"${UNSET_BEDROCK_BILLING_TEST_PROFILE}"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadJSON([]byte(`{"api_key":"synthetic-global","aws_bedrock":{"billing":` + tc.billing + `}}`))
			if tc.valid && err != nil {
				t.Fatal(err)
			}
			if !tc.valid && (err == nil || !strings.Contains(err.Error(), "aws_bedrock.billing")) {
				t.Fatalf("invalid billing configuration accepted or wrong error: %v", err)
			}
		})
	}

	t.Setenv("ROUTATIC_PROXY_AWS_BILLING_ENABLED", "true")
	t.Setenv("ROUTATIC_PROXY_AWS_BILLING_PROFILE", "billing-env")
	t.Setenv("ROUTATIC_PROXY_AWS_BILLING_LINKED_ACCOUNT_ID", "234567890123")
	cfg, err := LoadJSON([]byte(`{"api_key":"synthetic-global","aws_bedrock":{"api_key":"synthetic-inference","billing":{"enabled":false,"profile":"billing-file","linked_account_id":"123456789012"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(cfg.AWSBedrock)
	if err != nil {
		t.Fatal(err)
	}
	var fields struct {
		Billing struct {
			Enabled         bool   `json:"enabled"`
			Profile         string `json:"profile"`
			LinkedAccountID string `json:"linked_account_id"`
		} `json:"billing"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if !fields.Billing.Enabled || fields.Billing.Profile != "billing-env" || fields.Billing.LinkedAccountID != "234567890123" {
		t.Fatal("AWS billing environment overrides were not loaded")
	}
	keys := cfg.ProviderAPIKeys("aws-bedrock")
	if len(keys) != 1 || keys[0] != "synthetic-inference" {
		t.Fatal("billing configuration changed inference credentials")
	}
	t.Setenv("ROUTATIC_PROXY_AWS_BILLING_ENABLED", "not-a-bool")
	if _, err := LoadJSON([]byte(`{"api_key":"synthetic-global"}`)); err == nil {
		t.Fatal("invalid billing opt-in must not silently enable or disable queries")
	}
}
