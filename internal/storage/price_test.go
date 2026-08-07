package storage

import "testing"

func TestPriceForModelKnown(t *testing.T) {
	in, out, _, _, ok := PriceForModel("kimi-k2.6")
	if !ok || in == 0 || out == 0 {
		t.Fatalf("kimi-k2.6 price got ok=%v in=%v out=%v", ok, in, out)
	}
}
func TestPriceForModelSubstring(t *testing.T) {
	in, out, _, _, ok := PriceForModel("glm-5.1")
	if !ok || in == 0 || out == 0 {
		t.Fatalf("glm-5.1 price got ok=%v in=%v", ok, in)
	}
}
func TestPriceForModelUnknown(t *testing.T) {
	if _, _, _, _, ok := PriceForModel("no-such-model-xyz"); ok {
		t.Fatal("unknown model should not match")
	}
}
