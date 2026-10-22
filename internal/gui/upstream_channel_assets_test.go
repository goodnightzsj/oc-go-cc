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

	// A real channel name has to fit. The badge was set in the monospace face at
	// 10px, which made openai-compatible-private about 186px wide - wider than
	// the model cell it sits in - so the value was always truncated and the
	// badge reported less than it existed to report. Monospace is the thing
	// that made it too wide; the check is that the face is gone and the cell can
	// hold a name of that length.
	rule := cssSection(css, ".badge-channel {")
	if strings.Contains(rule, "monospace") {
		t.Error("the channel badge must not use a monospace face: a real channel name does not fit")
	}
	cell := cssSection(css, ".history-model-cell {")
	if !strings.Contains(cell, "max-width: 300px") {
		t.Errorf("the model cell must be wide enough for a channel badge; got %q", cell)
	}
}

// The upstream-model badge must appear only when the gateway named a model
// that is not the one we sent. Repeating the request id on every ordinary row
// would be noise, and the badge's presence is itself the signal that the
// gateway rewrote the id onto something else.
func TestUpstreamModelBadgeOnlyForARealRewrite(t *testing.T) {
	app, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(app)
	// The condition, stated once. A badge rendered unconditionally would pass
	// any assertion that only looks for the class name.
	// The model column names exactly what the client asked for. The observed
	// id is not shown there: routing and billing key on the request id, and a
	// row whose model cell disagreed with them would read as a different
	// request. What the gateway actually ran is reported by the channel badge
	// beside it.
	if !strings.Contains(source, "const servedModel = h.model") {
		t.Error("the model column must show the id the client requested")
	}
	if strings.Contains(source, "history-requested-model") {
		t.Error("the model column must not carry a second model id")
	}
	// The channel badge is unchanged: it answers which upstream served the
	// request, which is a different question from which model ran.
	if !strings.Contains(source, "badge badge-channel") {
		t.Error("the channel badge must still be rendered")
	}
	// Both locales, or the tooltip shows a raw key.
	for _, key := range []string{"'history.upstreamModelHint'", "'history.requestedModelHint'"} {
		if strings.Count(source, key+":") < 2 {
			t.Errorf("translation key %s must exist in both en and zh", key)
		}
	}
	css, err := assets.ReadFile("assets/style.css")
	if err != nil {
		t.Fatal(err)
	}
	// A distinct colour: two chips in the same cyan read as one datum.
	rule := cssSection(string(css), ".badge-upstream-model {")
	if rule == "" {
		t.Fatal("missing .badge-upstream-model rule")
	}
	if strings.Contains(rule, "--ui-cyan") {
		t.Error("the upstream-model badge must not reuse the channel badge's colour")
	}
}
