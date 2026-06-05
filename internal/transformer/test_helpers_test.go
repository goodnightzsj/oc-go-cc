package transformer

import (
	"testing"

	"oc-go-cc/pkg/types"
)

func usageInfoPtr(usage types.UsageInfo) *types.UsageInfo {
	return &usage
}

func requireUsage(t *testing.T, resp *types.MessageResponse) *types.Usage {
	t.Helper()
	if resp.Usage == nil {
		t.Fatal("Usage = nil, want reported usage")
	}
	return resp.Usage
}
