// Read-only deployment checks. Requires an existing Playwright installation and
// an explicit loopback SSH tunnel; never run the synthetic fixture's write tests here.
const assert = require('node:assert/strict');
const {mkdir, writeFile} = require('node:fs/promises');
const path = require('node:path');

const [origin, playwrightPath, outputDir] = process.argv.slice(2);
assert(origin && playwrightPath && outputDir, 'usage: node production-smoke.cjs <origin> <playwright-path> <output-dir>');
const target = new URL(origin);
assert(target.protocol === 'http:' && ['127.0.0.1', 'localhost', '[::1]'].includes(target.hostname), 'loopback tunnel required');
assert(target.origin === origin && !target.username && !target.password, 'use an origin without credentials or path');
const {chromium} = require(playwrightPath);
const providers = ['opencode-go', 'opencode-zen', 'aws-bedrock', 'openrouter', 'commandcode'];
const labels = ['OpenCode Go', 'OpenCode Zen', 'AWS Bedrock', 'OpenRouter', 'CommandCode'];
const tabs = ['overview', 'history', 'performance', 'fallback', 'analytics', 'quota', 'settings'];
const readPaths = new Set([
  '/api/metrics', '/api/config', '/api/proxy/config', '/api/catalog/lock', '/api/history',
  '/api/history/summary', '/api/analytics/summary', '/api/analytics/tokens/trend',
  '/api/perf/models', '/api/perf/aggregate', '/api/quota',
]);
const result = {ok: false, checks: 0, scopes: [], layouts: [], accountStates: [], pageErrors: [], blocked: [], requests: 0};
let phase = 'launch';
function check(value, message) {
  result.checks++;
  assert(value, phase + ': ' + message);
}

(async () => {
  await mkdir(outputDir, {recursive: true, mode: 0o700});
  const browser = await chromium.launch({channel: 'chrome', headless: true});
  const context = await browser.newContext({viewport: {width: 1440, height: 900}, locale: 'en-US', timezoneId: 'UTC', serviceWorkers: 'block'});
  const page = await context.newPage();
  page.setDefaultTimeout(15000);
  page.on('pageerror', error => result.pageErrors.push({phase, message: error.message}));
  await context.route('**/*', async route => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.origin !== origin || !['GET', 'HEAD'].includes(request.method()) ||
        (url.pathname.startsWith('/api/') && !readPaths.has(url.pathname))) {
      result.blocked.push({phase, method: request.method(), path: url.pathname});
      return route.abort('blockedbyclient');
    }
    result.requests++;
    return route.continue();
  });
  async function openTab(name) {
    await page.locator('.tab[data-tab="' + name + '"]').click();
    await page.locator('#tab-' + name + '.active').waitFor({state: 'visible'});
  }
  async function select(view, id, provider, paths) {
    phase = view + '/' + (provider || 'all');
    if (view === 'quota') await page.waitForFunction(() => !document.getElementById('btn-refresh-quota').disabled);
    const waiting = paths.map(endpoint => page.waitForResponse(response => {
      const url = new URL(response.url());
      return url.origin === origin && url.pathname === endpoint && (url.searchParams.get('provider') || '') === provider;
    }));
    const control = page.locator('.theme-select:has(#' + id + ')');
    await control.locator('.theme-select-trigger').click();
    await control.getByRole('option', {name: provider ? labels[providers.indexOf(provider)] : 'All platforms', exact: true}).click();
    const responses = await Promise.all(waiting);
    responses.forEach(response => check(response.ok(), 'HTTP ' + response.status()));
    const data = await Promise.all(responses.map(response => response.json()));
    result.scopes.push({view, provider: provider || 'all'});
    return data;
  }
  function summary(data, provider) {
    check(data.provider === provider, 'summary provider echo');
    check(Number.isFinite(data.summary.total_requests), 'numeric request count');
    for (const key of ['providers', 'models']) {
      check(Array.isArray(data[key]) && (!provider || data[key].every(row => row.provider === provider)), key + ' provider isolation');
    }
  }
  try {
    await page.goto(origin, {waitUntil: 'networkidle'});
    result.browser = browser.version();
    result.uiBuild = await page.locator('#ui-build').getAttribute('data-build');
    check(/^[a-f0-9]{12}$/.test(result.uiBuild), 'hashed deployed UI');
    for (const [view, id, endpoints] of [
      ['overview', 'overview-provider', ['/api/analytics/summary', '/api/analytics/tokens/trend', '/api/perf/aggregate']],
      ['history', 'provider-filter', ['/api/history', '/api/history/summary']],
      ['performance', 'perf-provider', ['/api/perf/models']],
      ['analytics', 'analytics-provider', ['/api/analytics/summary', '/api/analytics/tokens/trend']],
      ['quota', 'quota-provider', ['/api/quota', '/api/analytics/summary']],
    ]) {
      await openTab(view);
      for (const provider of view === 'quota' ? providers : [...providers, '']) {
        const data = await select(view, id, provider, endpoints);
        if (view === 'overview' || view === 'analytics') {
          summary(data[0], provider);
          check(data[1].provider === provider && Array.isArray(data[1].trend), 'trend scope');
        } else if (view === 'history') {
          check(Array.isArray(data[0].items) && (!provider || data[0].items.every(row => row.provider === provider)), 'history isolation');
          check(Number.isFinite(data[1].total_requests), 'history aggregate');
        } else if (view === 'performance') {
          check(Array.isArray(data[0]) && (!provider || data[0].every(row => row.provider === provider)), 'performance isolation');
        } else {
          const account = data[0];
          summary(data[1], provider);
          check(account.provider === provider, 'account scope');
          check(['available', 'partial', 'not_configured', 'unavailable', 'error'].includes(account.status), 'explicit account state');
          result.accountStates.push({provider, status: account.status, source: account.source, reason: account.reason, creditsStatus: account.credits_status});
          await page.waitForFunction(() => !document.getElementById('btn-refresh-quota').disabled);
          if (provider !== 'opencode-go') check(await page.locator('#quota-go').isHidden(), 'non-Go platform displayed Go windows');
          check((await page.locator('#quota-local-note').innerText()).includes('not an account bill or balance'), 'local data provenance');
        }
      }
    }
    phase = 'settings/read-only';
    await openTab('settings');
    for (const name of ['go', 'zen', 'bedrock', 'openrouter', 'commandcode']) {
      check(await page.locator('#cfg-' + name + '-timeout').count() === 1, name + ' independent settings');
    }
    for (const width of [1440, 768, 390]) {
      await page.setViewportSize({width, height: width === 768 ? 1024 : 900});
      for (const name of tabs) {
        phase = 'layout/' + width + '/' + name;
        await openTab(name);
        if (name === 'quota') await page.waitForFunction(() => !document.getElementById('btn-refresh-quota').disabled);
        await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
        const bounds = await page.locator('#tab-' + name).evaluate(panel => ({viewport: innerWidth, page: document.documentElement.scrollWidth, panel: panel.scrollWidth, panelWidth: panel.clientWidth}));
        result.layouts.push({width, tab: name, ...bounds});
        check(bounds.page <= bounds.viewport + 1 && bounds.panel <= bounds.panelWidth + 1, 'horizontal overflow');
        if (width !== 768) await page.screenshot({path: path.join(outputDir, width + '-' + name + '.png'), fullPage: true, mask: [page.locator('input[type="password"]')]});
      }
    }
    check(result.pageErrors.length === 0, 'uncaught page errors');
    check(result.blocked.length === 0, 'unexpected network operation attempted');
    result.ok = true;
  } catch (error) {
    result.failure = {phase, message: error.message};
    await page.screenshot({path: path.join(outputDir, 'failure.png'), fullPage: true, mask: [page.locator('input[type="password"]')]}).catch(error => { result.screenshotError = error.message; });
    process.exitCode = 1;
  } finally {
    await writeFile(path.join(outputDir, 'result.json'), JSON.stringify(result, null, 2), {mode: 0o600});
    console.log(JSON.stringify(result));
    await context.close();
    await browser.close();
  }
})().catch(error => { console.error(error.message); process.exitCode = 1; });
