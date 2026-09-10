package quota

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/routatic/proxy/internal/config"
)

const (
	BedrockBillingEndpoint = "https://ce.us-east-1.amazonaws.com"
	BedrockBillingTTL      = 24 * time.Hour
	bedrockBillingMetric   = "UnblendedCost"
	// Bound billable pagination; exceeding the budget fails without a partial total.
	maxBedrockBillingPages = 20
)

// BedrockBilling is official service cost, not a balance or the proxy's ledger.
// Services identifies the exact discovered billing names included in the query.
type BedrockBilling struct {
	LinkedAccountID string             `json:"linked_account_id"`
	StartDate       string             `json:"start_date"`
	EndDate         string             `json:"end_date"` // Exclusive, UTC.
	Metric          string             `json:"metric"`
	Services        []string           `json:"services"`
	Currency        string             `json:"currency,omitempty"`
	TotalCost       *float64           `json:"total_cost"`
	Estimated       bool               `json:"estimated"`
	Daily           []BedrockDailyCost `json:"daily"`
}

type BedrockDailyCost struct {
	Date      string  `json:"date"`
	Cost      float64 `json:"cost"`
	Estimated bool    `json:"estimated"`
}

// FetchBedrockBilling must only be called for an explicit, opted-in query.
// The standard AWS SDK identity is wholly separate from inference credentials.
func FetchBedrockBilling(ctx context.Context, billing config.AWSBillingConfig) (*BedrockBilling, error) {
	if !billing.Enabled {
		return nil, errors.New("AWS billing queries are disabled")
	}
	if err := billing.Validate(); err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: RequestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	opts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion("us-east-1"), awsconfig.WithHTTPClient(client)}
	if billing.Profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(billing.Profile))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		// SDK configuration errors can include credential file contents or paths.
		return nil, errors.New("could not load the AWS billing identity; check the service's AWS SDK profile configuration")
	}
	// Use the fixed official endpoint, not a Bedrock URL or an SDK endpoint override.
	ce := costexplorer.New(costexplorer.Options{
		Region: "us-east-1", BaseEndpoint: aws.String(BedrockBillingEndpoint),
		Credentials: cfg.Credentials, HTTPClient: client, RetryMaxAttempts: 1,
	})
	return fetchBedrockBilling(ctx, ce, billing.LinkedAccountID, time.Now())
}

func fetchBedrockBilling(ctx context.Context, client *costexplorer.Client, account string, now time.Time) (*BedrockBilling, error) {
	end := now.UTC().Truncate(24 * time.Hour)
	start := end.AddDate(0, 0, -30)
	report := &BedrockBilling{
		LinkedAccountID: account, StartDate: start.Format(time.DateOnly), EndDate: end.Format(time.DateOnly),
		Metric: bedrockBillingMetric, Services: []string{}, Daily: []BedrockDailyCost{},
	}
	period := &types.DateInterval{Start: aws.String(report.StartDate), End: aws.String(report.EndDate)}
	accountFilter := types.Expression{Dimensions: &types.DimensionValues{Key: types.DimensionLinkedAccount, Values: []string{account}}}
	dimensions := &costexplorer.GetDimensionValuesInput{
		TimePeriod: period, Context: types.ContextCostAndUsage, Dimension: types.DimensionService,
		SearchString: aws.String("Bedrock"), Filter: &accountFilter,
	}
	seen := make(map[string]bool)
	for page := 0; ; page++ {
		if page == maxBedrockBillingPages {
			return nil, errors.New("AWS billing service lookup exceeded the page budget; no partial bill is shown")
		}
		out, err := client.GetDimensionValues(ctx, dimensions)
		if err != nil {
			return nil, bedrockBillingError("GetDimensionValues", err)
		}
		for _, item := range out.DimensionValues {
			name := aws.ToString(item.Value)
			if !strings.Contains(strings.ToLower(name), "bedrock") {
				return nil, errors.New("AWS billing service lookup returned an unrelated service")
			}
			if !slices.Contains(report.Services, name) {
				report.Services = append(report.Services, name)
			}
		}
		token := aws.ToString(out.NextPageToken)
		if token == "" {
			break
		}
		if seen[token] {
			return nil, errors.New("AWS billing service lookup repeated a pagination token")
		}
		seen[token], dimensions.NextPageToken = true, out.NextPageToken
	}
	if len(report.Services) == 0 {
		return report, nil
	}
	slices.Sort(report.Services)
	query := &costexplorer.GetCostAndUsageInput{
		TimePeriod: period, Granularity: types.GranularityDaily, Metrics: []string{bedrockBillingMetric},
		Filter: &types.Expression{And: []types.Expression{
			accountFilter,
			{Dimensions: &types.DimensionValues{Key: types.DimensionService, Values: report.Services}},
		}},
	}
	clear(seen)
	dates := make(map[string]bool)
	total := 0.0
	for page := 0; ; page++ {
		if page == maxBedrockBillingPages {
			return nil, errors.New("AWS billing cost lookup exceeded the page budget; no partial bill is shown")
		}
		out, err := client.GetCostAndUsage(ctx, query)
		if err != nil {
			return nil, bedrockBillingError("GetCostAndUsage", err)
		}
		for _, item := range out.ResultsByTime {
			if item.TimePeriod == nil {
				return nil, errors.New("AWS billing result has no time period")
			}
			date, err := time.Parse(time.DateOnly, aws.ToString(item.TimePeriod.Start))
			if err != nil || date.Before(start) || !date.Before(end) || aws.ToString(item.TimePeriod.End) != date.AddDate(0, 0, 1).Format(time.DateOnly) {
				return nil, errors.New("AWS billing result has an invalid daily period")
			}
			day := date.Format(time.DateOnly)
			if dates[day] {
				return nil, errors.New("AWS billing result repeated a daily total; no partial bill is shown")
			}
			metric, ok := item.Total[bedrockBillingMetric]
			currency := aws.ToString(metric.Unit)
			if !ok || metric.Amount == nil || len(currency) != 3 || strings.IndexFunc(currency, func(r rune) bool { return r < 'A' || r > 'Z' }) >= 0 {
				return nil, errors.New("AWS billing result is missing a cost or currency")
			}
			amount, err := strconv.ParseFloat(*metric.Amount, 64)
			if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) {
				return nil, errors.New("AWS billing result has an invalid cost amount")
			}
			if report.Currency != "" && currency != report.Currency {
				return nil, errors.New("AWS billing result mixes currencies; costs cannot be added")
			}
			report.Currency = currency
			report.Estimated = report.Estimated || item.Estimated
			report.Daily = append(report.Daily, BedrockDailyCost{Date: day, Cost: amount, Estimated: item.Estimated})
			dates[day] = true
			total += amount
		}
		token := aws.ToString(out.NextPageToken)
		if token == "" {
			break
		}
		if seen[token] {
			return nil, errors.New("AWS billing cost lookup repeated a pagination token; no partial bill is shown")
		}
		seen[token], query.NextPageToken = true, out.NextPageToken
	}
	if math.IsInf(total, 0) {
		return nil, errors.New("AWS billing total is out of range")
	}
	if len(report.Daily) > 0 {
		report.TotalCost = &total
	}
	slices.SortFunc(report.Daily, func(a, b BedrockDailyCost) int { return strings.Compare(a.Date, b.Date) })
	return report, nil
}

func bedrockBillingError(operation string, err error) error {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "AccessDenied", "AccessDeniedException":
			return fmt.Errorf("AWS %s: access denied; grant ce:GetDimensionValues and ce:GetCostAndUsage for the billing identity and account", operation)
		case "ExpiredToken", "ExpiredTokenException", "UnrecognizedClientException", "InvalidClientTokenId", "InvalidSignatureException":
			return fmt.Errorf("AWS %s: billing credentials rejected or expired; renew the service's AWS SDK identity", operation)
		case "DataUnavailableException":
			return fmt.Errorf("AWS %s: billing data is not available yet; check Cost Explorer enablement and data delay", operation)
		case "LimitExceededException", "ThrottlingException":
			return fmt.Errorf("AWS %s: rate limited; retry later", operation)
		}
	}
	var responseErr *smithyhttp.ResponseError
	if errors.As(err, &responseErr) {
		return fmt.Errorf("AWS %s: HTTP %d; check Cost Explorer access and availability", operation, responseErr.HTTPStatusCode())
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("AWS %s: query canceled or timed out", operation)
	}
	// Never return an SDK message: transports and services may echo signed headers.
	return fmt.Errorf("AWS %s: request failed; check the billing identity and network", operation)
}
