package gui

import (
	"strings"
	"testing"
)

// The listing carries rates, context windows and capability flags. Rebuilding
// each entry as {provider, model_id} discarded all of it, so every picker row
// rendered without a price even though the data had arrived.
func TestPickerKeepsTheListingFields(t *testing.T) {
	js := readEmbeddedAsset(t, "assets/app.js")
	if !strings.Contains(js, "{ ...m }") && !strings.Contains(js, "{...m}") {
		t.Error("the candidate merge does not spread the listing record; its fields are lost")
	}
	// A row reads these; if the merge dropped them the row is blank.
	for _, field := range []string{"model.input_per_m", "model.context_window", "model.vision"} {
		if !strings.Contains(js, field) {
			t.Errorf("pickerRow no longer reads %s", field)
		}
	}
}

// The page showed a "primary model" card carrying models.default. On this
// instance that entry never runs - requests are routed by the model the client
// names - so it displayed a value nobody could act on that read as a current
// selection. The card is gone; the chain is the part that decides anything.
func TestNoPrimaryCardIsShown(t *testing.T) {
	html := readEmbeddedAsset(t, "assets/index.html")
	for _, stale := range []string{"fallback-primary-card", "fallback-primary-note", `id="fallback-primary"`} {
		if strings.Contains(html, stale) {
			t.Errorf("the primary card is still present: %s", stale)
		}
	}
	js := readEmbeddedAsset(t, "assets/app.js")
	if strings.Contains(js, "fallback-primary") {
		t.Error("the renderer still looks for the removed card")
	}
}
