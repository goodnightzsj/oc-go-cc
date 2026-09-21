package gui

import "testing"

func TestCustomSelectShowsDisabledReason(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  // A disabled choice must say why. CustomSelect replaces the native select, so
  // the reason has to survive into the rendered list: an inert option with no
  // visible cause reads as a broken control rather than an unmet precondition.
  // The page's DOMContentLoaded never fires in this shim, so the enhancement
  // that listens for it is invoked directly.
  window.CustomSelect.enhance(document.getElementById('cfg-active-site'));
  const select = document.getElementById('cfg-active-site');
  const option = document.createElement('option');
  option.value = 'cline-pass';
  option.textContent = 'ClinePass';
  option.disabled = true;
  option.title = 'Add an API key in Settings first.';
  select.options.push(option);

  const state = window.CustomSelect.instances.get(select);
  assert.ok(state, 'CustomSelect did not enhance the active-site selector');
  state.select.options = select.options;
  window.CustomSelect.render(state);

  const item = state.list.children[state.list.children.length - 1];
  assert.equal(item.disabled, true, 'the unconfigured platform must stay disabled');
  const text = item.children.map(child => child.textContent).join(' ');
  assert.ok(text.includes('Add an API key in Settings first.'),
    'the disabled option renders no reason, so nothing explains why: ' + JSON.stringify(text));
  assert.equal(item.title, 'Add an API key in Settings first.', 'the pointer tooltip must carry the reason too');

  // A selectable option must not be decorated with a reason.
  const live = document.createElement('option');
  live.value = 'opencode-go';
  live.textContent = 'OpenCode Go';
  live.disabled = false;
  live.title = '';
  select.options.push(live);
  window.CustomSelect.render(state);
  const last = state.list.children[state.list.children.length - 1];
  assert.equal(last.children.length, 0, 'a selectable option must carry no reason');
  assert.ok(!last.disabled, 'a configured platform must stay selectable');
}
`+platformBehaviorRunScript)
}
