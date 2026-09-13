package storage

import "testing"

func TestPriceForProviderModelKnown(t *testing.T) {
	in, out, _, _, ok := PriceForProviderModel("opencode-go", "kimi-k2.6", 0)
	if !ok || in == 0 || out == 0 {
		t.Fatalf("kimi-k2.6 price got ok=%v in=%v out=%v", ok, in, out)
	}
}

func TestPriceForProviderModelSubstring(t *testing.T) {
	in, out, _, _, ok := PriceForProviderModel("opencode-go", "glm-5.1", 0)
	if !ok || in == 0 || out == 0 {
		t.Fatalf("glm-5.1 price got ok=%v in=%v", ok, in)
	}
}

func TestPriceForProviderModelUnknown(t *testing.T) {
	if _, _, _, _, ok := PriceForProviderModel("opencode-go", "no-such-model-xyz", 0); ok {
		t.Fatal("unknown model should not match")
	}
}

// The rates are per platform, and the two platforms genuinely disagree on the
// same model name. A single shared table would price one platform's traffic
// with the other's rates and produce a well-formed, wrong number, so this
// asserts the tables are actually separate rather than merely reachable.
func TestPriceTablesArePerPlatform(t *testing.T) {
	// Many models the two platforms share do cost the same (deepseek-v4-flash
	// is 0.15/0.60 on both), so the shared set cannot carry this assertion.
	// claude-opus-4-8 is billed at OpenCode Go's Claude tier and at
	// CommandCode's own published rate, and those genuinely differ.
	goIn, goOut, _, _, goOK := PriceForProviderModel("opencode-go", "claude-opus-4-8", 0)
	ccIn, ccOut, _, _, ccOK := PriceForProviderModel("commandcode", "claude-opus-4-8", 0)
	if !goOK || !ccOK {
		t.Fatalf("claude-opus-4-8 must be priced on both platforms: go=%v cc=%v", goOK, ccOK)
	}
	if goIn == ccIn && goOut == ccOut {
		t.Fatalf("both platforms priced claude-opus-4-8 identically (%v/%v); the tables are not separate", goIn, goOut)
	}
}

// A platform that publishes no prices must not borrow another platform's: the
// model name is not a key on its own.
func TestPriceIsNotResolvedAcrossPlatforms(t *testing.T) {
	if _, _, _, _, ok := PriceForProviderModel("opencode-zen", "deepseek-v4-flash", 0); ok {
		t.Fatal("opencode-zen publishes no rate table, so it must not resolve a price")
	}
	if _, _, _, _, ok := PriceForProviderModel("no-such-platform", "deepseek-v4-flash", 0); ok {
		t.Fatal("an unknown platform must not resolve a price")
	}
}
