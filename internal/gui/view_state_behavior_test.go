package gui

import "testing"

func TestViewStateRoundTripsThroughTheHash(t *testing.T) {
	runPlatformBehavior(t, viewStateScript)
}

// Only the tab name used to be routable, so a link could not carry a filter and
// pressing back after paging into history left the tab instead of stepping back
// one page. This pins the round trip in both directions and the rule that one
// URL key drives every control for the same concept.
const viewStateScript = platformBehaviorDOMScript + `
async function checks() {
  const get = id => document.getElementById(id);

  // Serialise: the tab, the filters, and the two pieces of state that are
  // variables rather than controls.
  get('provider-filter').value = 'commandcode';
  get('history-search').value = 'cache';
  historyPage = 3;
  currentSort = {field:'duration_ms', dir:'asc'};
  activeTab = 'history';
  let link = parseViewHash(buildViewHash());
  assert.equal(link.name, 'history', 'the link must name the tab');
  assert.equal(link.params.get('platform'), 'commandcode', 'a filtered platform belongs in the link');
  assert.equal(link.params.get('q'), 'cache', 'the search belongs in the link');
  assert.equal(link.params.get('page'), '3', 'the page belongs in the link');
  assert.equal(link.params.get('sort'), 'duration_ms', 'the sort field belongs in the link');
  assert.equal(link.params.get('dir'), 'asc', 'the sort direction belongs in the link');

  // Defaults stay out rather than being spelled out: a link should not carry
  // state the reader did not choose. (The date range is not a default - the
  // history view fills it in on first load - so it is expected here.)
  historyPage = 1;
  currentSort = {field:'start_time', dir:'desc'};
  link = parseViewHash(buildViewHash());
  assert.equal(link.params.has('page'), false, 'page 1 is the default and must be omitted');
  assert.equal(link.params.has('sort'), false, 'the default sort must be omitted');
  assert.equal(link.params.has('dir'), false, 'the default direction must be omitted');

  // Apply: a link restores the whole view, not just the tab. The controls go
  // back to their opening state and that state is captured the way boot does,
  // so an omitted parameter can be resolved back to it.
  for (const id of ['provider-filter','overview-provider','perf-provider','analytics-provider','quota-provider']) {
    get(id).value = '';
  }
  get('history-search').value = '';
  historyPage = 1;
  currentSort = {field:'start_time', dir:'desc'};
  activeTab = 'overview';
  captureViewDefaults();

  const moved = applyViewState(new URLSearchParams('platform=commandcode&q=cache&page=3&sort=duration_ms&dir=asc'));
  assert.ok(moved, 'restoring a page must report that history pagination moved');
  assert.equal(get('provider-filter').value, 'commandcode', 'the filter the link named must be restored');
  assert.equal(get('history-search').value, 'cache', 'the search the link named must be restored');
  assert.equal(historyPage, 3, 'the page the link named must be restored');
  assert.equal(currentSort.field, 'duration_ms', 'the sort the link named must be restored');
  assert.equal(currentSort.dir, 'asc');

  // One key, every control that drives it - otherwise a link would restore the
  // history filter and leave the same platform unselected on four other tabs.
  for (const id of ['overview-provider','perf-provider','analytics-provider','quota-provider']) {
    assert.equal(get(id).value, 'commandcode', 'every control for one concept must follow the link: ' + id);
  }

  // An omitted parameter means the default, so stepping back from a link that
  // named a filter to one that does not has to undo it - otherwise the panel
  // and its own URL disagree about what is being shown.
  applyViewState(new URLSearchParams('platform=opencode-go'));
  assert.equal(get('history-search').value, '', 'an omitted filter must return to its default');
  assert.equal(historyPage, 1, 'an omitted page must return to the first');
  assert.equal(get('provider-filter').value, 'opencode-go');
}
` + platformBehaviorRunScript
