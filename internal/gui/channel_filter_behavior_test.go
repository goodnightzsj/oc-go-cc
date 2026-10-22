package gui

import (
	"strings"
	"testing"
)

// The channel filter is offered only where a channel can exist. Its options
// come from what this instance recorded, so the platform it is scoped to is
// what decides both whether it appears and what it offers.
//
// The behavior script runs the shipped app.js against a synthetic DOM and
// network, so the assertions are about what the code actually does rather than
// about the shape of the source.
func TestChannelFilterBehavior(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  const get = id => document.getElementById(id);
  const channels = get('channel-filter');

  // Every fetch is answered per endpoint. The channel endpoint reports what the
  // platform recorded, which is what the control has to offer.
  const observed = {'cline-pass': ['deepseek', 'openai-compatible-private'], 'opencode-go': []};
  const historyUrls = [];
  const channelQueries = [];
  fetch = async raw => {
    const url = new URL(raw, 'http://synthetic.invalid');
    if (url.pathname === '/api/history/channels') {
      channelQueries.push(url.searchParams.get('provider'));
      return {ok:true, json:async()=>({channels: observed[url.searchParams.get('provider')] || []})};
    }
    if (url.pathname === '/api/history') {
      historyUrls.push(url);
      return {ok:true, json:async()=>({items:[], total:0})};
    }
    if (url.pathname === '/api/history/summary') return {ok:true, json:async()=>({total_requests:0, success_rows:0})};
    if (url.pathname === '/api/metrics') return {ok:true, json:async()=>({proxy_running:true, model_counts:{}})};
    if (url.pathname === '/api/prices') return {ok:true, json:async()=>({tables:{}})};
    if (url.pathname === '/api/cline-pass/channels') return {ok:true, json:async()=>({channels:{}})};
    return {ok:true, json:async()=>({})};
  };

  // Nothing selected yet: no channel to filter by, so no control.
  assert.equal(channels.hidden, true, 'the channel filter must start hidden');
  await syncChannelFilter();
  assert.equal(channels.hidden, true, 'no platform means no channel filter');

  // The endpoint answers for every platform; the control stays hidden until a
  // concrete one is chosen, because "all platforms" is not a platform whose
  // channel this could be. The unscoped request must not even be made.
  assert.equal(JSON.stringify(channelQueries), JSON.stringify([]),
    'an unscoped platform must not query for channels');

  // A platform that records channels offers exactly the ones it recorded.
  get('provider-filter').value = 'cline-pass';
  await syncChannelFilter();
  assert.equal(channels.hidden, false, 'ClinePass must offer the channel filter');
  // Compared as JSON: the option values are built inside the vm context, so
  // deepStrictEqual fails on prototype identity rather than on content.
  assert.equal(JSON.stringify(channels.options.map(o => o.value)),
    JSON.stringify(['', 'deepseek', 'openai-compatible-private']));
  assert.equal(channels.options[0].textContent, t('filter.allChannels'));

  // Selecting a channel puts it in the query.
  channels.value = 'deepseek';
  historyUrls.length = 0;
  await refreshHistory();
  assert.equal(historyUrls.at(-1).searchParams.get('channel'), 'deepseek');
  assert.equal(historyUrls.at(-1).searchParams.get('provider'), 'cline-pass');

  // A platform that records none withdraws the control and stops sending the
  // channel: keeping it would filter that platform's rows by a channel it never
  // writes, which reads as an empty page rather than an inapplicable filter.
  get('provider-filter').value = 'opencode-go';
  await syncChannelFilter();
  assert.equal(channels.hidden, true, 'a platform with no channels must not offer the filter');
  assert.equal(channels.value, '', 'the stale channel must be cleared');
  historyUrls.length = 0;
  await refreshHistory();
  assert.equal(historyUrls.at(-1).searchParams.get('channel'), null, 'a withdrawn channel must not be sent');

  // Switching back re-offers it, and the filter counts as a filter for the
  // empty-state wording.
  get('provider-filter').value = 'cline-pass';
  await syncChannelFilter();
  assert.equal(channels.hidden, false);
  channels.value = 'deepseek';
  assert.equal(historyHasFilters(), true, 'a selected channel must count as a filter');
  resetHistoryFilters(false);
  assert.equal(channels.value, '', 'reset must clear the channel');

  // A failing first lookup for a platform leaves the control withdrawn rather
  // than offering a filter that cannot be applied.
  fetch = async raw => new URL(raw, 'http://synthetic.invalid').pathname === '/api/history/channels'
    ? {ok:false, status:503, text:async()=>'unavailable'}
    : {ok:true, json:async()=>({})};
  get('provider-filter').value = 'opencode-go';
  await syncChannelFilter();
  assert.equal(channels.hidden, true, 'a first failed lookup must not offer a channel filter');

  // A later failure for the same platform does not withdraw options that a
  // real answer already produced: the tab polls the channel list every three
  // seconds, and one flaky poll must not blink the control out from under a
  // reader who is using it.
  fetch = async raw => {
    const path = new URL(raw, 'http://synthetic.invalid').pathname;
    if (path === '/api/history/channels') return {ok:true, json:async()=>({channels:['fireworks','deepseek']})};
    return {ok:true, json:async()=>({})};
  };
  get('provider-filter').value = 'cline-pass';
  await syncChannelFilter();
  assert.equal(channels.hidden, false);
  fetch = async raw => new URL(raw, 'http://synthetic.invalid').pathname === '/api/history/channels'
    ? {ok:false, status:503, text:async()=>'unavailable'}
    : {ok:true, json:async()=>({})};
  await syncChannelFilter();
  assert.equal(channels.hidden, false, 'a transient failure must keep the options already offered');
  assert.equal(JSON.stringify(channels.options.map(o => o.value)),
    JSON.stringify(['', 'fireworks', 'deepseek']),
    'the offered options must survive a transient failure');

  // A channel this instance starts recording shows up on the next poll, not
  // only on the next page load: the list is derived from the same rows the
  // table shows, so a plain refresh has to re-read it.
  fetch = async raw => {
    const path = new URL(raw, 'http://synthetic.invalid').pathname;
    if (path === '/api/history/channels') return {ok:true, json:async()=>({channels:['deepseek','fireworks','newly-observed']})};
    if (path === '/api/history') return {ok:true, json:async()=>({items:[], total:0})};
    if (path === '/api/history/summary') return {ok:true, json:async()=>({total_requests:0, success_rows:0})};
    return {ok:true, json:async()=>({})};
  };
  channels.value = 'fireworks';
  await refreshHistory();
  assert.equal(JSON.stringify(channels.options.map(o => o.value)),
    JSON.stringify(['', 'deepseek', 'fireworks', 'newly-observed']),
    'a channel observed since the last sync must appear on the next history refresh');
  assert.equal(channels.value, 'fireworks', 'and the selection must survive the rebuild');
}
`+platformBehaviorRunScript)
}

// The two locales must both define the channel filter's labels; a missing key
// renders as the raw key in the dropdown.
func TestChannelFilterTranslationsExist(t *testing.T) {
	app, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(app)
	for _, key := range []string{"'filter.channel'", "'filter.allChannels'"} {
		if strings.Count(source, key+":") < 2 {
			t.Errorf("translation key %s must exist in both en and zh", key)
		}
	}
	page, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	// The control must ship in the markup, hidden, so the platform can reveal
	// it: a control created only by script has no place in the filter band.
	if !strings.Contains(string(page), `id="channel-filter"`) || !strings.Contains(string(page), `id="channel-filter" aria-label`) {
		t.Error("index.html must declare the channel filter")
	}
	if !strings.Contains(string(page), `id="channel-filter" aria-label="Upstream channel" data-i18n-aria-label="filter.channel" hidden`) {
		t.Error("the channel filter must ship hidden")
	}
}
