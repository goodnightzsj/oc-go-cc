package gui

import "testing"

// Two platform-switch defects lived on the same path, so they are pinned
// together: the site default pinned itself into the link on first open (after
// which a server-side switch stopped reaching the session), and a link naming a
// platform this deployment no longer offers held every view on "all platforms"
// for the whole session.
func TestActiveSiteFollowsTheServerAndIsNotPinnedByItsOwnDefault(t *testing.T) {
	runPlatformBehavior(t, activeSiteFollowScript)
}

const activeSiteFollowScript = platformBehaviorDOMScript + `
async function checks() {
  const get = id => document.getElementById(id);
  const views = ['overview-provider','provider-filter','perf-provider','analytics-provider','quota-provider'];
  const option = (value, disabled) => ({value, disabled: !!disabled});
  const choices = [option(''), option('opencode-go'), option('commandcode')];
  for (const id of views) get(id).options = choices;

  // The reader picked a platform by hand at boot; the pin records which one.
  location.hash = '#history?platform=commandcode';
  const {name, params} = parseViewHash(location.hash);
  viewPlatformPinned = params.get('platform') || '';
  assert.equal(viewPlatformPinned, 'commandcode', 'the pin records the platform, not merely that one was named');

  // Applying the site default must not write itself into the link. Otherwise
  // the first open pins the site and every later server-side switch is refused.
  const sites = {sites:[{id:'opencode-go',selectable:true},{id:'commandcode',selectable:true}], active:'opencode-go'};
  fetch = async () => ({ok:true, json:async()=>sites});
  const pinBefore = viewPlatformPinned;
  await applyActiveSite();
  assert.equal(viewPlatformPinned, pinBefore, 'the site default must not pin a view');
  assert.equal(location.hash.indexOf('platform=commandcode') >= 0, true,
    'a platform the reader chose stays in the link');

  // Drop the pin the way a save does, then let the server move the site.
  viewPlatformPinned = null;
  for (const id of views) get(id).value = 'commandcode';
  sites.active = 'opencode-go';
  await applyActiveSite();
  for (const id of views) {
    assert.equal(get(id).value, 'opencode-go', 'a server-side switch must reach an open dashboard: ' + id);
  }

  // A link naming a platform that is no longer offered cannot be honoured. It
  // must fall back to the active site rather than hold every view on "all
  // platforms" while routing goes somewhere else.
  viewPlatformPinned = 'aws-bedrock';
  for (const id of views) get(id).value = '';
  sites.active = 'commandcode';
  await applyActiveSite();
  assert.equal(viewPlatformPinned, null, 'an unrepresentable pin is dropped');
  for (const id of views) {
    assert.equal(get(id).value, 'commandcode', 'the views follow the site once the stale pin is gone: ' + id);
  }
}
` + platformBehaviorRunScript
