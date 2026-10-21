package gui

import (
	"strings"
	"testing"
)

// The upstream channel is a ClinePass-only fact: its gateway names a channel per
// response, and no other platform does. So the badge and the detail row are
// driven by the presence of the value, not by a platform check - a row with no
// channel renders nothing, which is what keeps the other platforms from showing
// an empty marker that would read as "channel withheld".
//
// The renderer and the stylesheet are checked together: a class with no rule
// does nothing, and a rule with no class never applies.
func TestUpstreamChannelBadgeAppearsOnlyWhenAChannelWasObserved(t *testing.T) {
	app, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(app)

	// The row badge must be conditional on the value.
	if !strings.Contains(source, "const channelMark = h.upstream_channel") {
		t.Error("the history row must gate the channel badge on the observed value")
	}
	if !strings.Contains(source, `class="badge badge-channel"`) {
		t.Error("the channel badge needs its own class so it can be styled apart from peak")
	}
	if !strings.Contains(source, "${channelMark}") {
		t.Error("the row must render the channel badge")
	}
	// The badge is clamped so a long channel name cannot reflow the model cell,
	// which means the visible text can be cut off. The tooltip has to lead with
	// the value, or the clipped part is unrecoverable.
	if !strings.Contains(source, `escapeHtml(h.upstream_channel + ' — ' + t('history.upstreamChannelHint'))`) {
		t.Error("the channel badge tooltip must name the value before the explanation")
	}

	// The detail modal must show the channel as a labelled row, also conditional.
	if !strings.Contains(source, "record.upstream_channel ?") {
		t.Error("the detail modal must gate the channel row on the observed value")
	}
	if !strings.Contains(source, "t('detail.upstreamChannel')") {
		t.Error("the detail row must use a labelled, translated field name")
	}

	// Both locales must define both keys; a missing key renders as the raw key.
	for _, key := range []string{"'detail.upstreamChannel'", "'history.upstreamChannelHint'"} {
		if strings.Count(source, key+":") < 2 {
			t.Errorf("translation key %s must exist in both en and zh", key)
		}
	}

	css := readStyleSheet(t)
	if !strings.Contains(css, ".badge-channel") {
		t.Error("style.css must define the channel badge rule")
	}
}
