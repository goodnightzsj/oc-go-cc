package gui

import "testing"

func TestViewStateRoundTripsThroughTheHash(t *testing.T) {
	runPlatformBehavior(t, viewStateScript)
}

// The view choices are the reader's own preferences and live in localStorage;
// the hash carries only the tab name, because that one is navigation and a
// deep link into a page is worth keeping. These used to share the URL, which
// dragged one person's filters into another person's link and grew the address
// bar with state nobody reads.
const viewStateScript = platformBehaviorDOMScript + `
async function checks() {
  const get = id => document.getElementById(id);
  for (const id of VIEW_CONTROLS.platform) {
    get(id).options = ['', ...visiblePlatforms].map(value => ({value, disabled:false}));
  }
  localStorage.removeItem(VIEW_STATE_KEY);

  // Serialise the reader's choices.
  get('provider-filter').value = 'commandcode';
  get('history-search').value = 'cache';
  historyPage = 3;
  currentSort = {field:'duration_ms', dir:'asc'};
  const state = buildViewState();
  assert.equal(state.platform, 'commandcode', 'a filtered platform is remembered');
  assert.equal(state.q, 'cache', 'the search is remembered');
  assert.equal(state.page, '3', 'the page is remembered');
  assert.equal(state.sort, 'duration_ms', 'the sort field is remembered');
  assert.equal(state.dir, 'asc', 'the sort direction is remembered');

  // Defaults stay out: the store holds choices, not the panel's opening state.
  historyPage = 1;
  currentSort = {field:'start_time', dir:'desc'};
  get('provider-filter').value = defaultControlValue('provider-filter', get('provider-filter'));
  get('history-search').value = '';
  const plain = buildViewState();
  assert.equal(plain.page, undefined, 'page 1 is the default and must not be stored');
  assert.equal(plain.sort, undefined, 'the default sort must not be stored');
  assert.equal(plain.dir, undefined, 'the default direction must not be stored');

  // Write, then read back through the storage layer - not through a variable,
  // so a broken serialise/parse pair fails here.
  get('provider-filter').value = 'commandcode';
  get('history-search').value = 'cache';
  historyPage = 3;
  currentSort = {field:'duration_ms', dir:'asc'};
  syncViewState();
  const saved = readSavedView();
  assert.equal(saved.platform, 'commandcode', 'the store must round-trip the platform');
  assert.equal(saved.q, 'cache', 'the store must round-trip the search');

  // Apply: a stored view restores the whole view, not just the tab.
  for (const id of VIEW_CONTROLS.platform) get(id).value = '';
  get('history-search').value = '';
  historyPage = 1;
  currentSort = {field:'start_time', dir:'desc'};
  captureViewDefaults();

  const moved = applyViewState(readSavedView());
  assert.ok(moved, 'restoring a page must report that history pagination moved');
  assert.equal(get('provider-filter').value, 'commandcode', 'the stored filter must be restored');
  assert.equal(get('history-search').value, 'cache', 'the stored search must be restored');
  assert.equal(historyPage, 3, 'the stored page must be restored');
  assert.equal(currentSort.field, 'duration_ms', 'the stored sort must be restored');
  assert.equal(currentSort.dir, 'asc');

  // One key, every control that drives it - otherwise the history filter is
  // restored and the same platform stays unselected on four other tabs.
  for (const id of ['overview-provider','perf-provider','analytics-provider','quota-provider']) {
    assert.equal(get(id).value, 'commandcode', 'every control for one concept must follow the store: ' + id);
  }

  // The hash is navigation only.
  activeTab = 'history';
  assert.equal(buildViewHash(), '#history', 'the hash must name the tab and nothing else');
  assert.equal(syncViewHash, syncViewHash, 'syncViewHash exists');

  // A corrupt store must not stop the panel from loading.
  localStorage.setItem(VIEW_STATE_KEY, '{not json');
  assert.equal(JSON.stringify(readSavedView()), '{}', 'a corrupt store reads as empty');
  localStorage.removeItem(VIEW_STATE_KEY);
}
` + platformBehaviorRunScript

// Switching tabs has to update the address bar. The filters moved to
// localStorage, and for a while nothing wrote the tab name either, so the URL
// kept naming whichever page the panel had opened on - a reload then went back
// there instead of where the reader was.
func TestTabChangesFollowTheAddressBar(t *testing.T) {
	runPlatformBehavior(t, navigationBehaviorDOMScript+`
async function checks() {
  await bootView();
  for (const tab of ['history', 'analytics', 'quota', 'fallback', 'overview']) {
    document.getElementById('nav-' + tab).click();
    await settleView();
    assert.equal(activeTab, tab);
    assert.equal(location.hash, '#' + tab, 'the address bar must name the current tab: ' + tab);
    assert.equal(parseViewHash(location.hash).params.toString(), '', 'the hash carries the tab and nothing else');
  }

  // A programmatic switch counts too: opening a provider's settings section
  // moves the panel without a click on the tab bar.
  activateTab('settings');
  assert.equal(location.hash, '#settings', 'a programmatic tab change must update the URL as well');
}
`+platformBehaviorRunScript)
}
