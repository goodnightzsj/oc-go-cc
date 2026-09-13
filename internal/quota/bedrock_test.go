package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/routatic/proxy/internal/config"
)

func bedrockBillingTestClient(t *testing.T, handle func(http.ResponseWriter, *http.Request, map[string]any)) *costexplorer.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=SYNTHETIC_BILLING_ACCESS/") || r.Header.Get("X-Amz-Security-Token") != "synthetic-session" {
			t.Error("billing request must use SDK SigV4 and its separate session identity")
		}
		var input map[string]any
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		handle(w, r, input)
	}))
	t.Cleanup(server.Close)
	return costexplorer.New(costexplorer.Options{
		Region: "us-east-1", BaseEndpoint: aws.String(server.URL), HTTPClient: server.Client(), RetryMaxAttempts: 1,
		Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: "SYNTHETIC_BILLING_ACCESS", SecretAccessKey: "synthetic-billing-secret", SessionToken: "synthetic-session"}, nil
		}),
	})
}

func TestBedrockBillingScopePaginationAndCurrency(t *testing.T) {
	dimensions, costs := 0, 0
	client := bedrockBillingTestClient(t, func(w http.ResponseWriter, r *http.Request, input map[string]any) {
		period := input["TimePeriod"].(map[string]any)
		if period["Start"] != "2026-08-11" || period["End"] != "2026-09-10" {
			t.Errorf("expected 30 complete UTC days, got %v", period)
		}
		filter, _ := json.Marshal(input["Filter"])
		if !strings.Contains(string(filter), `"LINKED_ACCOUNT"`) || !strings.Contains(string(filter), `"123456789012"`) {
			t.Error("billing account scope missing")
		}
		switch r.Header.Get("X-Amz-Target") {
		case "AWSInsightsIndexService.GetDimensionValues":
			dimensions++
			if input["Dimension"] != "SERVICE" || input["SearchString"] != "Bedrock" || input["Context"] != "COST_AND_USAGE" {
				t.Error("service discovery contract changed")
			}
			if dimensions == 1 {
				_, _ = fmt.Fprint(w, `{"DimensionValues":[{"Value":"Amazon Bedrock"}],"NextPageToken":"services-next"}`)
			} else {
				if input["NextPageToken"] != "services-next" {
					t.Error("service pagination token missing")
				}
				_, _ = fmt.Fprint(w, `{"DimensionValues":[{"Value":"Amazon Bedrock Mantle"}]}`)
			}
		case "AWSInsightsIndexService.GetCostAndUsage":
			costs++
			if input["Granularity"] != "DAILY" || fmt.Sprint(input["Metrics"]) != "[UnblendedCost]" || !strings.Contains(string(filter), `"Amazon Bedrock Mantle"`) || !strings.Contains(string(filter), `"Amazon Bedrock"`) {
				t.Error("cost query lost discovered services, account, metric or granularity")
			}
			if costs == 1 {
				_, _ = fmt.Fprint(w, `{"ResultsByTime":[{"TimePeriod":{"Start":"2026-09-09","End":"2026-09-10"},"Estimated":true,"Total":{"UnblendedCost":{"Amount":"1.25","Unit":"EUR"}}}],"NextPageToken":"cost-next"}`)
			} else {
				if input["NextPageToken"] != "cost-next" {
					t.Error("cost pagination token missing")
				}
				_, _ = fmt.Fprint(w, `{"ResultsByTime":[{"TimePeriod":{"Start":"2026-09-08","End":"2026-09-09"},"Estimated":false,"Total":{"UnblendedCost":{"Amount":"-2.5","Unit":"EUR"}}}]}`)
			}
		default:
			t.Error("unexpected AWS operation")
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	now := time.Date(2026, 9, 11, 1, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	got, err := fetchBedrockBilling(context.Background(), client, "123456789012", now)
	if err != nil {
		t.Fatal(err)
	}
	if dimensions != 2 || costs != 2 || got.Currency != "EUR" || got.TotalCost == nil || *got.TotalCost != -1.25 || !got.Estimated || len(got.Daily) != 2 || got.Daily[0].Date != "2026-09-08" {
		t.Fatalf("incomplete or incorrectly normalized billing report: %+v", got)
	}
}

func TestBedrockBillingRejectsInvalidAndPartialData(t *testing.T) {
	const row = `{"TimePeriod":{"Start":"2026-09-09","End":"2026-09-10"},"Total":{"UnblendedCost":{"Amount":"1.25","Unit":"USD"}}}`
	for _, tc := range []struct{ name, first, second string }{
		{"missing amount", `{"ResultsByTime":[` + strings.ReplaceAll(row, `"1.25"`, `null`) + `]}`, ""},
		{"nan", `{"ResultsByTime":[` + strings.ReplaceAll(row, `"1.25"`, `"NaN"`) + `]}`, ""},
		{"infinity", `{"ResultsByTime":[` + strings.ReplaceAll(row, `"1.25"`, `"Inf"`) + `]}`, ""},
		{"missing currency", `{"ResultsByTime":[` + strings.ReplaceAll(row, `"USD"`, `null`) + `]}`, ""},
		{"bad date", `{"ResultsByTime":[` + strings.Replace(row, `2026-09-09`, `2026-07-09`, 1) + `]}`, ""},
		{"missing period", `{"ResultsByTime":[{"Total":{"UnblendedCost":{"Amount":"0","Unit":"USD"}}}]}`, ""},
		{"repeated day", `{"ResultsByTime":[` + row + `],"NextPageToken":"next"}`, `{"ResultsByTime":[` + row + `]}`},
		{"mixed currency", `{"ResultsByTime":[` + row + `],"NextPageToken":"next"}`, `{"ResultsByTime":[{"TimePeriod":{"Start":"2026-09-08","End":"2026-09-09"},"Total":{"UnblendedCost":{"Amount":"1","Unit":"EUR"}}}]}`},
		{"repeated token", `{"ResultsByTime":[],"NextPageToken":"next"}`, `{"ResultsByTime":[],"NextPageToken":"next"}`},
		{"later page denied", `{"ResultsByTime":[` + row + `],"NextPageToken":"next"}`, "denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := bedrockBillingTestClient(t, func(w http.ResponseWriter, r *http.Request, input map[string]any) {
				if strings.HasSuffix(r.Header.Get("X-Amz-Target"), ".GetDimensionValues") {
					_, _ = fmt.Fprint(w, `{"DimensionValues":[{"Value":"Amazon Bedrock"}]}`)
					return
				}
				calls++
				body := tc.first
				if calls > 1 {
					body = tc.second
				}
				if body == "denied" {
					w.WriteHeader(http.StatusForbidden)
					_, _ = fmt.Fprint(w, `{"__type":"AccessDeniedException","message":"synthetic-billing-secret synthetic-session"}`)
					return
				}
				_, _ = fmt.Fprint(w, body)
			})
			got, err := fetchBedrockBilling(context.Background(), client, "123456789012", time.Date(2026, 9, 10, 1, 0, 0, 0, time.UTC))
			if err == nil || got != nil {
				t.Fatal("invalid or partial bill was accepted")
			}
			if strings.Contains(err.Error(), "synthetic-") {
				t.Fatal("upstream error exposed a synthetic secret")
			}
		})
	}
}

func TestBedrockBillingEmptyZeroAndOptIn(t *testing.T) {
	for _, services := range []bool{false, true} {
		t.Run(fmt.Sprint(services), func(t *testing.T) {
			costCalls := 0
			client := bedrockBillingTestClient(t, func(w http.ResponseWriter, r *http.Request, _ map[string]any) {
				if strings.HasSuffix(r.Header.Get("X-Amz-Target"), ".GetDimensionValues") {
					if services {
						_, _ = fmt.Fprint(w, `{"DimensionValues":[{"Value":"Amazon Bedrock"}]}`)
					} else {
						_, _ = fmt.Fprint(w, `{"DimensionValues":[]}`)
					}
					return
				}
				costCalls++
				_, _ = fmt.Fprint(w, `{"ResultsByTime":[{"TimePeriod":{"Start":"2026-09-09","End":"2026-09-10"},"Estimated":false,"Total":{"UnblendedCost":{"Amount":"0","Unit":"USD"}}}]}`)
			})
			got, err := fetchBedrockBilling(context.Background(), client, "123456789012", time.Date(2026, 9, 10, 1, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatal(err)
			}
			if services && (costCalls != 1 || got.TotalCost == nil || *got.TotalCost != 0) {
				t.Fatal("explicit zero is not preserved")
			}
			if !services && (costCalls != 0 || got.TotalCost != nil || got.Daily == nil || got.Services == nil) {
				t.Fatal("no matching services must not become a zero balance or trigger a cost call")
			}
		})
	}
	for _, cfg := range []config.AWSBillingConfig{{}, {Enabled: true}} {
		if got, err := FetchBedrockBilling(context.Background(), cfg); err == nil || got != nil {
			t.Fatal("disabled or unscoped query was accepted")
		}
	}
}

type billingTransportFunc func(*http.Request) (*http.Response, error)

func (f billingTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestBedrockBillingSDKIdentityAndEndpoint(t *testing.T) {
	for _, profile := range []string{"", "billing-readonly"} {
		t.Run("profile="+profile, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOME", dir)
			t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "config"))
			t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "credentials"))
			t.Setenv("AWS_PROFILE", "")
			t.Setenv("AWS_DEFAULT_PROFILE", "")
			t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
			t.Setenv("AWS_ACCESS_KEY_ID", "SYNTHETIC_ENV")
			t.Setenv("AWS_SECRET_ACCESS_KEY", "synthetic-env-secret")
			t.Setenv("AWS_SESSION_TOKEN", "synthetic-env-session")
			t.Setenv("AWS_ENDPOINT_URL", "https://unrelated.invalid")
			t.Setenv("AWS_ENDPOINT_URL_COST_EXPLORER", "https://unrelated.invalid/costs")
			if err := os.WriteFile(filepath.Join(dir, "config"), []byte("[default]\nregion = eu-west-1\n[profile billing-readonly]\nregion = eu-west-1\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "credentials"), []byte("[billing-readonly]\naws_access_key_id = SYNTHETIC_PROFILE\naws_secret_access_key = synthetic-profile-secret\naws_session_token = synthetic-profile-session\n"), 0600); err != nil {
				t.Fatal(err)
			}
			identity, session := "SYNTHETIC_ENV", "synthetic-env-session"
			if profile != "" {
				identity, session = "SYNTHETIC_PROFILE", "synthetic-profile-session"
			}
			calls := 0
			original := http.DefaultTransport
			http.DefaultTransport = billingTransportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != BedrockBillingEndpoint+"/" || !strings.Contains(r.Header.Get("Authorization"), "Credential="+identity+"/") || !strings.Contains(r.Header.Get("Authorization"), "/us-east-1/ce/aws4_request") || r.Header.Get("X-Amz-Security-Token") != session {
					t.Error("SDK query lost the explicit identity, signing region or fixed official endpoint")
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/x-amz-json-1.1"}}, Body: io.NopCloser(strings.NewReader(`{"DimensionValues":[]}`)), Request: r}, nil
			})
			t.Cleanup(func() { http.DefaultTransport = original })
			got, err := FetchBedrockBilling(context.Background(), config.AWSBillingConfig{Enabled: true, Profile: profile, LinkedAccountID: "123456789012"})
			if err != nil || got == nil || calls != 1 {
				t.Fatalf("isolated SDK query failed: %v, calls=%d", err, calls)
			}
		})
	}
}
