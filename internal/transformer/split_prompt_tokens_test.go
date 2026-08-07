package transformer

import (
	"testing"

	"github.com/routatic/proxy/pkg/types"
)

func TestSplitPromptTokens(t *testing.T) {
	tests := []struct {
		name                             string
		usage                            types.UsageInfo
		wantIn, wantRead, wantCreate     int
	}{
		{
			name:  "deepseek partitioned: hit+miss == prompt",
			usage: types.UsageInfo{PromptTokens: 100000, PromptCacheHitTokens: 96000, PromptCacheMissTokens: 4000},
			// miss is the portion billed at the full input rate.
			wantIn: 4000, wantRead: 96000, wantCreate: 0,
		},
		{
			name:  "no cache fields reported: whole prompt is input",
			usage: types.UsageInfo{PromptTokens: 628385},
			wantIn: 628385, wantRead: 0, wantCreate: 0,
		},
		{
			name:  "fully cached prompt",
			usage: types.UsageInfo{PromptTokens: 50000, PromptCacheHitTokens: 50000},
			wantIn: 0, wantRead: 50000, wantCreate: 0,
		},
		{
			name:  "additive form: cache counts sit outside prompt_tokens",
			usage: types.UsageInfo{PromptTokens: 10000, PromptCacheHitTokens: 3000, PromptCacheMissTokens: 2000},
			wantIn: 5000, wantRead: 3000, wantCreate: 2000,
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
