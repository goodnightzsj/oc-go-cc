async (page) => {
  // The caller first opens TestMultiPlatformBrowserServer's BROWSER_SMOKE_URL.
  // No services are started or stopped here, and the caller's context is untouched.
  const initialURL = new URL(page.url());
  if (initialURL.protocol !== 'http:' || !['127.0.0.1', 'localhost', '[::1]'].includes(initialURL.hostname)) {
    throw new Error('Open the loopback synthetic fixture before running this script.');
  }
  const origin = initialURL.origin;
  const providers = ['opencode-go', 'opencode-zen', 'aws-bedrock', 'openrouter', 'commandcode'];
  const labels = ['OpenCode Go', 'OpenCode Zen', 'AWS Bedrock', 'OpenRouter', 'CommandCode'];
  const tabs = ['overview', 'history', 'performance', 'fallback', 'analytics', 'quota', 'settings'];
  const outputDir = '/tmp/oc-go-cc-multiplatform-' + Date.now() + '-' + Math.random().toString(16).slice(2);
  const result = {
    ok: false, origin, outputDir, scopes: [], patches: [], layouts: [], screenshots: [],
    injectedFailures: [], unexpectedRequests: [], pageErrors: [], checks: 0,
  };
  const context = await page.context().browser().newContext({
    viewport: {width: 1440, height: 900}, locale: 'en-US', timezoneId: 'UTC', serviceWorkers: 'block',
  });
  const tab = await context.newPage();
  tab.setDefaultTimeout(10000);
  let phase = 'fixture-identity';
  let expectedPatch = null;
  let fixtureConfirmed = false;
  let injection = null;
  const traffic = [];
  const readPaths = new Set([
    '/api/metrics', '/api/config', '/api/proxy/config', '/api/catalog/lock',
    '/api/history', '/api/history/summary', '/api/analytics/summary',
    '/api/analytics/tokens/trend', '/api/perf/models', '/api/perf/aggregate', '/api/quota',
  ]);
  const canonical = value => Array.isArray(value) ? value.map(canonical)
    : value && typeof value === 'object'
      ? Object.fromEntries(Object.keys(value).sort().map(key => [key, canonical(value[key])])) : value;
  const same = (left, right) => JSON.stringify(canonical(left)) === JSON.stringify(canonical(right));
  function check(condition, message) {
    result.checks++;
    if (!condition) throw new Error(phase + ': ' + message);
  }
  tab.on('pageerror', error => result.pageErrors.push({phase, message: error.message}));
  await context.route('**/*', async route => {
    const request = route.request();
    const url = new URL(request.url());
    const method = request.method();
    const event = {phase, method, path: url.pathname, provider: url.searchParams.get('provider') || ''};
    const block = async reason => {
      result.unexpectedRequests.push({...event, origin: url.origin, reason});
      await route.abort('blockedbyclient');
    };
    if (url.origin !== origin) return block('external origin');
    if (!['GET', 'HEAD'].includes(method)) {
      if (method !== 'POST' || url.pathname !== '/api/proxy/config' || !fixtureConfirmed || !expectedPatch) {
        return block('unapproved write');
      }
      let patch;
      try { patch = request.postDataJSON(); } catch (_) { return block('invalid config patch'); }
      if (!same(patch, expectedPatch)) return block('config patch exceeds the selected timeout field');
      result.patches.push({phase, patch});
      expectedPatch = null; // Each explicit save permits exactly one request.
    } else if (url.pathname.startsWith('/api/') && !readPaths.has(url.pathname)) {
      return block('unexpected API');
    }
    traffic.push(event);
    if (injection && method === 'GET' && url.pathname === injection.path && event.provider === injection.provider) {
      result.injectedFailures.push({...event, status: 503, expected: true});
      injection = null;
      return route.fulfill({status: 503, contentType: 'text/plain', body: 'synthetic browser failure'});
    }
    return route.continue();
  });

  const selected = provider => provider ? [provider] : providers;
  const totals = provider => {
    const indices = selected(provider).map(item => providers.indexOf(item) + 1);
    return {
      requests: indices.length * 2, known: indices.length,
      tokens: indices.reduce((sum, index) => sum + 2 * (index * 10 + 6), 0),
      cost: indices.reduce((sum, index) => sum + index / 10, 0),
      displayCost: provider ? '$' + (indices[0] / 10).toFixed(3) + ' + ?' : '$1.50 + ?',
    };
  };
  const expectProviders = (rows, provider, label) => check(
    same(rows.map(row => row.provider).sort(), [...selected(provider)].sort()), label + ' crossed provider boundaries',
  );
  function expectSummary(data, provider) {
    const expected = totals(provider);
    check(data.provider === provider, 'summary provider echo');
    check(data.summary.total_requests === expected.requests, 'summary request count');
    check(data.summary.known_requests === expected.known, 'unknown details counted as known');
    check(data.summary.unknown_cost_requests === expected.known, 'unknown prices lost');
    check(Math.abs(data.summary.est_cost_usd - expected.cost) < 1e-9, 'known cost subtotal');
    const tokens = ['input_tokens', 'output_tokens', 'cache_read_tokens', 'cache_creation_tokens']
      .reduce((sum, key) => sum + data.summary[key], 0);
    check(tokens === expected.tokens, 'cache-inclusive token total');
    expectProviders(data.models, provider, 'same-name models');
    expectProviders(data.providers, provider, 'provider breakdown');
    check(data.models.every(row => row.model === 'shared-model' && row.requests === 2), 'same-name models merged');
  }
  function expectTrend(data, provider) {
    check(data.provider === provider, 'trend provider echo');
    check(data.trend.reduce((sum, point) => sum + point.requests, 0) === totals(provider).requests, 'trend scope');
  }
  async function api(path) {
    return tab.evaluate(async path => {
      const response = await fetch(path);
      if (!response.ok) throw new Error(path + ': HTTP ' + response.status);
      return response.json();
    }, path);
  }
  async function openTab(name) {
    await tab.locator('.tab[data-tab="' + name + '"]').click();
    await tab.locator('#tab-' + name + '.active').waitFor({state: 'visible'});
  }
  async function choose(id, provider) {
    const control = tab.locator('.theme-select:has(#' + id + ')');
    await control.locator('.theme-select-trigger').click();
    const name = provider ? labels[providers.indexOf(provider)] : 'All platforms';
    await control.getByRole('option', {name, exact: true}).click();
  }
  function waitResponse(path, provider) {
    return tab.waitForResponse(response => {
      const url = new URL(response.url());
      return url.origin === origin && url.pathname === path && (url.searchParams.get('provider') || '') === provider;
    });
  }
  async function selectAndRead(view, id, provider, paths) {
    phase = view + '/' + (provider || 'all');
    if (view === 'quota') await tab.waitForFunction(() => !document.getElementById('btn-refresh-quota').disabled);
    const waiting = paths.map(path => waitResponse(path, provider));
    await choose(id, provider);
    const responses = await Promise.all(waiting);
    for (const response of responses) check(response.ok(), 'unexpected HTTP ' + response.status());
    result.scopes.push({
      view, provider: provider || 'all', ...totals(provider),
      requestURLs: responses.map(response => new URL(response.url()).pathname + new URL(response.url()).search),
    });
    return Promise.all(responses.map(response => response.json()));
  }
  async function expectText(id, text) {
    await tab.waitForFunction(({id, text}) => document.getElementById(id)?.textContent === text, {id, text});
  }
  async function expectModelRows(id, provider) {
    await tab.waitForFunction(({id, providers}) => {
      const rows = [...document.querySelectorAll('#' + id + ' tr')];
      const actual = rows.map(row => row.querySelector('td:first-child small')?.textContent).sort();
      return JSON.stringify(actual) === JSON.stringify([...providers].sort());
    }, {id, providers: selected(provider)});
  }

  try {
    await tab.goto(origin + '/', {waitUntil: 'networkidle'});
    const fixture = await api('/api/history?page=1&size=50');
    const fixtureIDs = providers.flatMap(provider => [0, 1].map(index => 'synthetic-' + provider + '-' + index));
    check(fixture.total === 10 && same(fixture.items.map(row => row.id).sort(), fixtureIDs.sort()),
      'server is not the ten-row synthetic fixture; refusing writes');
    const initialConfig = await api('/api/proxy/config');
    const goURL = new URL(initialConfig.opencode_go.base_url);
    check(['127.0.0.1', 'localhost', '[::1]'].includes(goURL.hostname) && goURL.pathname === '/go/chat/completions',
      'fixture upstream is not loopback');
    check(/^•+$/.test(initialConfig.openrouter.management_api_key), 'management key is not masked');
    fixtureConfirmed = true;

    await openTab('overview');
    for (const provider of [...providers, '']) {
      const [summary, trend, latency] = await selectAndRead('overview', 'overview-provider', provider,
        ['/api/analytics/summary', '/api/analytics/tokens/trend', '/api/perf/aggregate']);
      expectSummary(summary, provider);
      expectTrend(trend, provider);
      check(summary.retained.total_requests === totals(provider).requests, 'retained comparison scope');
      check(latency.total_requests === totals(provider).known, 'unknown requests counted in latency');
      await expectText('m-total', String(totals(provider).requests));
      await expectText('m-tokens', String(totals(provider).tokens));
      await expectText('m-cost', totals(provider).displayCost);
      await expectText('m-success', '100.0%');
      check(await tab.locator('#overview-error').isHidden(), 'overview error remained visible');
    }

    await openTab('history');
    for (const provider of [...providers, '']) {
      const [history, summary] = await selectAndRead('history', 'provider-filter', provider,
        ['/api/history', '/api/history/summary']);
      check(history.total === totals(provider).requests && history.items.length === history.total, 'history count');
      check(history.items.every(row => selected(provider).includes(row.provider)), 'history mixed platforms');
      check(history.items.filter(row => row.details_known).length === totals(provider).known, 'history unknown details');
      check(summary.total_requests === totals(provider).requests, 'history summary scope');
      await expectText('history-summary-requests', String(totals(provider).requests));
      await expectText('history-summary-tokens', String(totals(provider).tokens));
      await expectText('history-summary-cost', totals(provider).displayCost);
      await tab.waitForFunction(count => document.querySelectorAll('#history-tbody tr[data-id]').length === count,
        totals(provider).requests);
      check(await tab.locator('#history-tbody .badge-unknown').count() === totals(provider).known, 'unknown history badges');
      const rowProviders = await tab.locator('#history-tbody .history-model-cell small').allTextContents();
      check(rowProviders.every(item => selected(provider).includes(item)), 'history DOM mixed platforms');
    }

    await openTab('performance');
    for (const provider of [...providers, '']) {
      const [rows] = await selectAndRead('performance', 'perf-provider', provider, ['/api/perf/models']);
      expectProviders(rows, provider, 'performance');
      check(rows.every(row => row.model === 'shared-model' && row.count === 1 && row.success === 1 && row.failed === 0),
        'unknown history contaminated performance');
      check(rows.every(row => row.avg_ms === (providers.indexOf(row.provider) + 1) * 100), 'latency mixed platforms');
      await expectModelRows('perf-tbody', provider);
      check(await tab.locator('#perf-error').isHidden(), 'performance error remained visible');
    }

    await openTab('analytics');
    for (const provider of [...providers, '']) {
      const [summary, trend] = await selectAndRead('analytics', 'analytics-provider', provider,
        ['/api/analytics/summary', '/api/analytics/tokens/trend']);
      expectSummary(summary, provider);
      expectTrend(trend, provider);
      await expectText('kpi-requests', String(totals(provider).requests));
      await expectText('kpi-tokens', String(totals(provider).tokens));
      await expectText('kpi-cost', totals(provider).displayCost);
      await expectModelRows('analytics-model-tbody', provider);
    }

    await selectAndRead('analytics', 'analytics-provider', 'commandcode',
      ['/api/analytics/summary', '/api/analytics/tokens/trend']);
    await expectText('kpi-requests', '2');
    phase = 'analytics/injected-503';
    injection = {path: '/api/analytics/summary', provider: 'commandcode'};
    await tab.locator('#btn-refresh-analytics').click();
    await tab.locator('#analytics-error').waitFor({state: 'visible'});
    await expectText('kpi-requests', '—');
    check((await tab.locator('#analytics-error').innerText()).includes('503'), '503 was hidden or converted to zero');
    check(result.injectedFailures.length === 1, '503 injection did not happen exactly once');
    const recovery = waitResponse('/api/analytics/summary', 'commandcode');
    await tab.locator('#btn-refresh-analytics').click();
    const recovered = await recovery;
    check(recovered.ok(), 'real API did not recover after injected 503');
    expectSummary(await recovered.json(), 'commandcode');
    await expectText('kpi-requests', '2');
    check(await tab.locator('#analytics-error').isHidden(), 'failure did not clear after successful refresh');
    result.injectedFailures[0].recoveredFromRealAPI = true;
    await selectAndRead('analytics', 'analytics-provider', '', ['/api/analytics/summary', '/api/analytics/tokens/trend']);

    await openTab('quota');
    for (const provider of providers) {
      const [account, local] = await selectAndRead('quota', 'quota-provider', provider,
        ['/api/quota', '/api/analytics/summary']);
      expectSummary(local, provider);
      check(account.provider === provider, 'account scope');
      await expectText('quota-local-requests', '2');
      await expectText('quota-local-tokens', String(totals(provider).tokens));
      await expectText('quota-local-cost', totals(provider).displayCost);
      await expectText('quota-local-unknown', '1');
      await expectModelRows('quota-local-model-tbody', provider);
      await tab.waitForFunction(() => !document.getElementById('btn-refresh-quota').disabled);
      check((await tab.locator('#quota-local-note').innerText()).includes('not an account bill or balance'),
        'local ledger was described as an account balance');
      if (provider === 'opencode-go') {
        check(account.status === 'available' && account.source === 'upstream_api', 'Go account lookup');
        check(account.accounts.length === 1 && account.accounts[0].report.weekly.used_percent === 10, 'Go quota values');
        check(await tab.locator('#quota-accounts .quota-card').count() === 3, 'Go windows not rendered');
      } else if (provider === 'openrouter') {
        check(account.status === 'partial' && account.credits_status === 'available', 'OpenRouter partial status');
        check(account.accounts.length === 2 && account.accounts.filter(item => item.error?.includes('401')).length === 1,
          'one rejected key hid valid OpenRouter data');
        const good = account.accounts.find(item => item.openrouter)?.openrouter;
        check(good?.usage === 2 && good.byok_usage === 3 && good.limit_remaining === 8, 'OpenRouter key fields');
        check(account.credits.total_credits - account.credits.total_usage === -1, 'negative account balance');
        check((await tab.locator('#quota-openrouter-credits').innerText()).includes('$-1.00'), 'negative balance not shown');
        const text = await tab.locator('#quota-openrouter-accounts').innerText();
        check(text.includes('401') && text.includes('$2.00') && text.includes('$3.00'), 'key errors or BYOK usage absent');
      } else {
        check(account.status === 'unavailable' && account.source === 'none', 'unavailable account status');
        check(account.reason === (provider === 'aws-bedrock' ? 'aws_billing_auth_required' : 'no_public_account_api'),
          'account capability reason');
        check(await tab.locator('#quota-unavailable').isVisible() && await tab.locator('#quota-go').isHidden(),
          'unavailable platform displayed Go quota');
        check(await tab.locator('#quota-links a').count() > 0, 'official console links absent');
      }
    }
    await selectAndRead('quota', 'quota-provider', 'openrouter', ['/api/quota', '/api/analytics/summary']);

    phase = 'fallback/provider-identity';
    await openTab('fallback');
    const chainProviders = await tab.locator('#fallback-chain .model-meta').allTextContents();
    check(same(chainProviders, providers.slice(1)), 'fallback chain lost provider identity');
    check(await tab.locator('#fallback-chain .model-name').allTextContents()
      .then(names => names.length === 4 && names.every(name => name === 'shared-model')), 'fallback fixture model names');
    check(await tab.locator('#fallback-add-model option[value="opencode-go/shared-model"]').count() === 1,
      'same-name Go model cannot be distinguished from the existing chain');

    await openTab('settings');
    const sections = ['opencode_go', 'opencode_zen', 'aws_bedrock', 'openrouter', 'commandcode'];
    const fieldPrefixes = ['go', 'zen', 'bedrock', 'openrouter', 'commandcode'];
    for (let index = 0; index < providers.length; index++) {
      phase = 'settings/' + providers[index];
      const before = await api('/api/proxy/config');
      const section = sections[index];
      const timeout = (before[section].timeout_ms || 0) + 101;
      const inputID = 'cfg-' + fieldPrefixes[index] + '-timeout';
      await tab.locator('#' + inputID).fill(String(timeout));
      const expected = JSON.parse(JSON.stringify(before));
      expected[section].timeout_ms = timeout;
      expectedPatch = {[section]: {timeout_ms: timeout}};
      const patchCount = result.patches.length;
      const saved = tab.waitForResponse(response =>
        new URL(response.url()).pathname === '/api/proxy/config' && response.request().method() === 'POST');
      await tab.locator('#btn-save-cfg').click();
      check((await saved).ok(), 'temporary config save failed');
      await tab.waitForFunction(() => !document.getElementById('btn-save-cfg').disabled);
      const after = await api('/api/proxy/config');
      check(result.patches.length === patchCount + 1, 'save did not produce exactly one allowed patch');
      check(same(after, expected), 'saving one timeout changed another config field');
      check(/^•+$/.test(after.openrouter.management_api_key), 'management key leaked in API');
      check(/^•+$/.test(await tab.locator('#cfg-openrouter-management-key').inputValue()), 'management key leaked in form');
    }

    phase = 'layout';
    for (const [width, height] of [[1440, 900], [768, 1024], [390, 844]]) {
      await tab.setViewportSize({width, height});
      for (const name of tabs) {
        await openTab(name);
        const counter = {overview: 'm-total', history: 'history-summary-requests', analytics: 'kpi-requests'}[name];
        if (counter) await expectText(counter, '10');
        if (name === 'performance') await expectModelRows('perf-tbody', '');
        if (name === 'quota') {
          await expectText('quota-local-requests', '2');
          await tab.waitForFunction(() => !document.getElementById('btn-refresh-quota').disabled);
        }
        await tab.locator('#tab-' + name).evaluate(panel => { panel.scrollTop = 0; });
        const bounds = await tab.evaluate(() => {
          const panel = document.querySelector('.tab-content.active');
          return {viewport: innerWidth, page: document.documentElement.scrollWidth,
            panel: panel.scrollWidth, panelWidth: panel.clientWidth};
        });
        result.layouts.push({width, tab: name, ...bounds});
        check(bounds.page <= bounds.viewport + 1 && bounds.panel <= bounds.panelWidth + 1,
          name + ' horizontally overflows at ' + width + 'px');
        if (width === 1440) {
          const path = outputDir + '/desktop-' + name + '.png';
          await tab.screenshot({path, fullPage: true});
          result.screenshots.push({tab: name, width, path});
        }
      }
    }
    for (const width of [390, 320]) {
      await tab.setViewportSize({width, height: 844});
      await openTab('quota');
      for (const provider of providers) {
        await selectAndRead('quota-layout', 'quota-provider', provider, ['/api/quota', '/api/analytics/summary']);
        await tab.waitForFunction(() => !document.getElementById('btn-refresh-quota').disabled);
        const bounds = await tab.locator('#tab-quota').evaluate(panel => ({
          viewport: innerWidth, page: document.documentElement.scrollWidth,
          panel: panel.scrollWidth, panelWidth: panel.clientWidth,
        }));
        result.layouts.push({width, tab: 'quota', provider, ...bounds});
        check(bounds.page <= bounds.viewport + 1 && bounds.panel <= bounds.panelWidth + 1,
          provider + ' quota horizontally overflows at ' + width + 'px');
      }
    }
    check(result.unexpectedRequests.length === 0, 'unexpected requests were blocked; inspect the first violation');
    check(result.pageErrors.length === 0, 'uncaught page errors');
    result.browser = context.browser().version();
    result.networkRequests = traffic.length;
    result.ok = true;
    return result;
  } catch (error) {
    result.failure = {phase, message: error.message};
    const path = outputDir + '/first-failure.png';
    try {
      await tab.screenshot({path, fullPage: true});
      result.screenshots.push({tab: 'first-failure', path});
    } catch (screenshotError) {
      result.failure.screenshotError = screenshotError.message;
    }
    // No retries: preserve the first failed assertion and all evidence collected so far.
    throw new Error('Multi-platform browser smoke failed: ' + JSON.stringify(result));
  } finally {
    await context.close();
  }
}
