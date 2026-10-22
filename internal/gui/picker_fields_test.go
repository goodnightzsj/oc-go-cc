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

// The primary comes from the scenario config and is not editable here. Showing
// it as a bare value implied this page had selected it.
func TestPrimaryIsLabelledReadOnly(t *testing.T) {
	html := readEmbeddedAsset(t, "assets/index.html")
	if !strings.Contains(html, `id="fallback-primary-note"`) {
		t.Error("the primary has no explanatory note")
	}
	if !strings.Contains(html, `data-i18n="fallback.readOnly"`) {
		t.Error("the primary is not marked read-only")
	}
	js := readEmbeddedAsset(t, "assets/app.js")
	for _, key := range []string{"fallback.readOnly", "fallback.primaryReadOnly", "fallback.primaryNote"} {
		if n := strings.Count(js, "'"+key+"':"); n < 2 {
			t.Errorf("%q needs an English and a Chinese entry, found %d", key, n)
		}
	}
}
