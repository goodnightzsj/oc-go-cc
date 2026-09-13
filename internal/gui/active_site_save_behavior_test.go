package gui

import "testing"

func TestSavingActiveSiteMovesEveryView(t *testing.T) {
	runPlatformBehavior(t, activeSiteSaveScript)
}

// The platform selectors on the other tabs are set from /api/sites, which was
// only read at page load. Saving a new active platform therefore left every
// other tab filtering by the platform it had rendered on entry, and the change
// only appeared after a manual refresh. This drives the real save path and
// asserts the rest of the dashboard follows the save it just acknowledged.
const activeSiteSaveScript = platformBehaviorDOMScript + `
context.Event = class { constructor(type, init) { this.type = type; Object.assign(this, init || {}); } };
async function checks() {
  const get = id => document.getElementById(id);
  const views = ['overview-provider','provider-filter','perf-provider','analytics-provider','quota-provider'];
  const option = value => ({value, disabled:false});
  const choices = [option(''), option('opencode-go'), option('commandcode')];
  for (const id of [...views, 'cfg-active-site']) {
    const select = get(id);
    select.options = choices;
    // The shim's nodes expose emit(); the selector code dispatches a real event.
    select.dispatchEvent = event => Promise.resolve(select.emit(event.type));
  }
  get('overview-provider').value = '';
  get('provider-filter').value = '';
  get('perf-provider').value = '';
  get('analytics-provider').value = '';
  get('quota-provider').value = '';

  const stored = {active_site:'opencode-go'};
  const sites = {sites:[{id:'opencode-go',selectable:true},{id:'commandcode',selectable:true}], active:stored.active_site};
  fetch = async (raw, init) => {
    const url = new URL(raw, 'http://synthetic.invalid');
    if (url.pathname === '/api/sites') {
      return {ok:true, json:async()=>Object.assign({}, sites, {active:stored.active_site})};
    }
    if (url.pathname === '/api/proxy/config') {
      if ((init || {}).method === 'POST') {
        Object.assign(stored, JSON.parse(init.body));
        return {ok:true, text:async()=>''};
      }
      return {ok:true, json:async()=>stored};
    }
    return {ok:false, status:503, text:async()=>'synthetic unavailable response'};
  };

  await loadProxyConfig();
  await applyActiveSite();
  for (const id of views) {
    assert.equal(get(id).value, 'opencode-go', 'views open on the stored platform: ' + id);
  }

  get('cfg-active-site').value = 'commandcode';
  await saveProxyConfig();

  assert.equal(stored.active_site, 'commandcode', 'the save must reach the server');
  for (const id of views) {
    assert.equal(get(id).value, 'commandcode',
      'saving a new platform must move this view without a reload: ' + id);
  }
}
` + platformBehaviorRunScript
