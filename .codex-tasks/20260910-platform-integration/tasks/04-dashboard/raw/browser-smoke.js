async (page) => {
  const context = await page.context().browser().newContext({viewport: {width: 1440, height: 900}, locale: 'en-US'});
  const tab = await context.newPage();
  tab.setDefaultTimeout(5000);
  const origin = 'http://127.0.0.1:55039';
  const errors = [];
  const unexpected = [];
  const patches = [];
  const quotaRequests = [];
  tab.on('pageerror', error => errors.push(error.message));
  const mask = '••••••••••••••••';
  const providers = ['opencode-go', 'opencode-zen', 'aws-bedrock', 'openrouter', 'commandcode'];
  const records = providers.map((provider, index) => ({
    id: 'synthetic-' + index, provider, model: 'shared-model', requested_model: 'commandcode',
    scenario: 'default', start_time: '2026-09-10T08:00:00Z', duration_ms: 200 + index,
    input_tokens: 11, output_tokens: 7, cache_read_tokens: 90, cache_creation_tokens: 3,
    streaming: true, success: true, details_known: true, attempt: 1,
    cost_usd: index === 4 ? 0.2 : index === 0 ? 0.1 : 0,
    cost_source: index === 1 ? 'unknown' : 'estimated',
  }));
  const summary = {total_requests: 5, known_requests: 5, error_requests: 0, success_rate: 1,
    input_tokens: 55, output_tokens: 35, cache_read_tokens: 450, cache_creation_tokens: 15,
    cost_usd: 0.3, est_cost_usd: 0.3, unknown_cost_requests: 1};
  const breakdown = records.map(record => ({...record, name: record.provider, requests: 1,
    tokens: 111, unknown_cost_requests: record.cost_source === 'unknown' ? 1 : 0}));
  const config = {host: '127.0.0.1', port: 3456, hot_reload: false, catalog: {enabled: false},
    models: {default: {provider: 'commandcode', model_id: 'shared-model', max_tokens: 8192}},
    model_overrides: {commandcode: {provider: 'commandcode', model_id: 'shared-model', max_tokens: 8192}},
    fallbacks: {}, model_family_overrides: {},
    commandcode: {base_url: 'https://synthetic.invalid/chat/completions',
      anthropic_base_url: 'https://synthetic.invalid/messages', api_key: mask, api_keys: [mask],
      timeout_ms: 1000, stream_timeout_ms: 2000, streaming_timeout_ms: 3000, zero_data_retention: false}};
  await context.route('**/*', async route => {
    const request = route.request();
    const requestURL = request.url();
    if (!requestURL.startsWith(origin + '/')) {
      unexpected.push(request.url());
      return route.abort();
    }
    const pathname = requestURL.slice(origin.length).split('?')[0];
    if (!pathname.startsWith('/api/')) return route.continue();
    let data;
    switch (pathname) {
      case '/api/metrics': data = {proxy_running: true, requests_received: 5, port: 3456, model_counts: {}}; break;
      case '/api/config': data = {autostart: false, notify: false}; break;
      case '/api/catalog/lock': data = {synced: false}; break;
      case '/api/proxy/config':
        if (request.method() === 'POST') {
          const patch = request.postDataJSON();
          patches.push(patch);
          Object.assign(config.commandcode, patch.commandcode || {});
        }
        data = config;
        break;
      case '/api/analytics/summary': data = {summary, providers: breakdown, models: breakdown, generated_at: '2026-09-10T08:00:00Z'}; break;
      case '/api/analytics/tokens/trend': data = {trend: [{...summary, date: '2026-09-10', requests: 5}]}; break;
      case '/api/perf/aggregate': data = {avg_latency_ms: 200, p95_latency_ms: 220, p99_latency_ms: 230}; break;
      case '/api/perf/models': data = records.map(record => ({provider: record.provider, model: record.model, count: 1, success: 1, failed: 0, avg_ms: 200, p50_ms: 200, p95_ms: 200, p99_ms: 200})); break;
      case '/api/history': {
        const selectedProvider = (requestURL.match(/[?&]provider=([^&]*)/) || [])[1] || '';
        const filtered = records.filter(record => !selectedProvider || record.provider === selectedProvider);
        data = {items: filtered, total: filtered.length};
        break;
      }
      case '/api/history/summary': data = {...summary, total_tokens: 555, success_rows: 5, models: breakdown.map(item => ({...item, name: item.model})), providers: breakdown, scenarios: []}; break;
      case '/api/quota': quotaRequests.push(requestURL); data = {accounts: [], model_limits: {models: []}}; break;
      default: unexpected.push(pathname); return route.fulfill({status: 501, body: 'Unexpected smoke-test endpoint'});
    }
    await route.fulfill({status: 200, contentType: 'application/json', body: JSON.stringify(data)});
  });
  function check(ok, message) { if (!ok) throw new Error(message); }
  const layouts = [];
  try {
    await tab.goto(origin, {waitUntil: 'networkidle'});
    await tab.waitForFunction(() => document.getElementById('m-total').textContent === '5');
    check((await tab.locator('#m-cost').innerText()).includes('?'), 'Overview must show incomplete cost');
    await tab.locator('[data-tab="history"]').click();
    await tab.waitForFunction(() => document.querySelectorAll('#history-tbody tr').length === 5);
    const picker = tab.locator('.theme-select:has(#provider-filter)');
    await picker.locator('.theme-select-trigger').click();
    await picker.getByRole('option', {name: 'CommandCode', exact: true}).click();
    await tab.waitForFunction(() => document.querySelectorAll('#history-tbody tr').length === 1);
    check((await tab.locator('#history-tbody').innerText()).includes('commandcode'), 'Provider filter must update history');
    await tab.locator('[data-tab="quota"]').click();
    const quotaPicker = tab.locator('.theme-select:has(#quota-provider)');
    await quotaPicker.locator('.theme-select-trigger').press('End');
    await tab.keyboard.press('End');
    await tab.keyboard.press('Enter');
    check(await tab.locator('#quota-provider').inputValue() === 'commandcode', 'Keyboard selection must choose CommandCode');
    check(await tab.locator('#quota-unavailable').isVisible(), 'CommandCode quota must be explicitly unavailable');
    check(!await tab.locator('#quota-go').isVisible(), 'CommandCode must not show Go quotas');
    check(await tab.locator('#quota-commandcode-links').isVisible(), 'Official billing links must remain available');
    const quotaCount = quotaRequests.length;
    await tab.evaluate(() => QuotaModule.load(true));
    check(quotaRequests.length === quotaCount, 'CommandCode must not fetch Go quota');
    await tab.locator('[data-tab="settings"]').click();
    await tab.locator('#cfg-commandcode-timeout').fill('1500');
    await tab.locator('#cfg-commandcode-zdr').check();
    await tab.locator('#btn-save-cfg').click();
    await tab.waitForFunction(() => document.getElementById('save-status').textContent.length > 0);
    check(patches.length === 1 && JSON.stringify(patches[0]) === '{"commandcode":{"timeout_ms":1500,"zero_data_retention":true}}', 'Settings save must only send the two changed fields');
    for (const [width, height] of [[1440, 900], [768, 1024], [390, 844]]) {
      await tab.setViewportSize({width, height});
      for (const name of ['overview', 'history', 'performance', 'fallback', 'analytics', 'quota', 'settings']) {
        await tab.locator('[data-tab="' + name + '"]').click();
        const bounds = await tab.evaluate(() => {
          const panel = document.querySelector('.tab-content.active');
          return {page: document.documentElement.scrollWidth, viewport: innerWidth, panel: panel.scrollWidth, panelWidth: panel.clientWidth};
        });
        layouts.push({width, tab: name, ...bounds});
      }
    }
    await tab.locator('[data-tab="quota"]').click();
    await tab.screenshot({path: '/tmp/oc-go-cc-dashboard-mobile-quota.png', fullPage: true});
    await tab.setViewportSize({width: 1440, height: 900});
    await tab.locator('[data-tab="overview"]').click();
    await tab.screenshot({path: '/tmp/oc-go-cc-dashboard-desktop.png', fullPage: true});
    check(errors.length === 0, 'Uncaught browser errors: ' + errors.join('; '));
    check(unexpected.length === 0, 'Unexpected network requests: ' + unexpected.join('; '));
    return {browser: context.browser().version(), patches, layouts, errors, unexpected, quotaRequests: quotaRequests.length,
      screenshots: ['/tmp/oc-go-cc-dashboard-desktop.png', '/tmp/oc-go-cc-dashboard-mobile-quota.png']};
  } finally {
    await context.close();
  }
}
