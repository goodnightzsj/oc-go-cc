package transformer

import (
	"testing"

	"github.com/routatic/proxy/pkg/types"
)

func TestSplitPromptTokens(t *testing.T) {
	tests := []struct {
		name                         string
		usage                        types.UsageInfo
		wantIn, wantRead, wantCreate int
	}{
		{
			name:  "deepseek partitioned: hit+miss == prompt",
			usage: types.UsageInfo{PromptTokens: 100000, PromptCacheHitTokens: 96000, PromptCacheMissTokens: 4000},
			// miss is the portion billed at the full input rate.
			wantIn: 4000, wantRead: 96000, wantCreate: 0,
		},
		{
			name:   "no cache fields reported: whole prompt is input",
			usage:  types.UsageInfo{PromptTokens: 628385},
			wantIn: 628385, wantRead: 0, wantCreate: 0,
		},
		{
			name:   "fully cached prompt",
			usage:  types.UsageInfo{PromptTokens: 50000, PromptCacheHitTokens: 50000},
			wantIn: 0, wantRead: 50000, wantCreate: 0,
		},
		{
			name:   "additive form: cache counts sit outside prompt_tokens",
			usage:  types.UsageInfo{PromptTokens: 10000, PromptCacheHitTokens: 3000, PromptCacheMissTokens: 2000},
			wantIn: 5000, wantRead: 3000, wantCreate: 2000,
		},
		{
			// Real capture from OpenCode Go oa-compat gateway:
			// usage: {"prompt_tokens":427549,"prompt_tokens_details":{"cached_tokens":426240}}
			name:   "opencode oa-compat: cached_tokens in prompt_tokens_details",
			usage:  types.UsageInfo{PromptTokens: 427549, PromptTokensDetails: &types.PromptTokensDetails{CachedTokens: 426240}},
			wantIn: 1309, wantRead: 426240, wantCreate: 0,
		},
		{
			name:   "cached_tokens with empty details object is treated as no cache",
			usage:  types.UsageInfo{PromptTokens: 84, PromptTokensDetails: &types.PromptTokensDetails{}},
			wantIn: 84, wantRead: 0, wantCreate: 0,
		},
		{
			// Real capture (2026-09-02): oa-compat non-streaming response
			// reports prompt_cache_miss_tokens as the FULL prompt (261241),
			// not cache-stripped, beside the hit count (261120); the fresh
			// input is prompt - hit = 121. Billing the miss verbatim would
			// price the cached prefix twice.
			name:   "opencode oa-compat non-streaming: miss is un-stripped full prompt",
			usage:  types.UsageInfo{PromptTokens: 261241, PromptCacheHitTokens: 261120, PromptCacheMissTokens: 261241},
			wantIn: 121, wantRead: 261120, wantCreate: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, read, create := splitPromptTokens(&tt.usage)
			if in != tt.wantIn || read != tt.wantRead || create != tt.wantCreate {
				t.Errorf("splitPromptTokens() = (in=%d, read=%d, create=%d), want (in=%d, read=%d, create=%d)",
					in, read, create, tt.wantIn, tt.wantRead, tt.wantCreate)
			}
		})
	}
}
