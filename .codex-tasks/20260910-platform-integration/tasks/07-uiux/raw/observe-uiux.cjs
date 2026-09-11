// Read-only UI observations on the verified synthetic fixture, never production.
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');

const [origin, playwrightPath, outputDir] = process.argv.slice(2);
assert(origin && playwrightPath && outputDir, 'origin, Playwright path and output directory required');
const target = new URL(origin);
assert(target.origin === origin && target.protocol === 'http:' && target.hostname === '127.0.0.1', 'loopback origin required');
const providers = ['opencode-go', 'opencode-zen', 'aws-bedrock', 'openrouter', 'commandcode'];
const tabs = ['overview', 'history', 'performance', 'fallback', 'analytics', 'quota', 'settings'];
const result = {ok: false, pages: [], quotas: [], pageErrors: [], blocked: []};

(async () => {
  const response = await fetch(origin + '/api/history?limit=100');
  assert(response.ok, 'fixture history lookup failed');
  const history = await response.json();
  const expected = providers.flatMap(provider => [0, 1].map(i => `synthetic-${provider}-${i}`)).sort();
  assert.deepEqual(history.items.map(item => item.id).sort(), expected, 'not the synthetic ten-record fixture');
  await fs.mkdir(outputDir, {recursive: true, mode: 0o700});
  const {chromium} = require(playwrightPath);
  const browser = await chromium.launch({channel: 'chrome', headless: true});
  const context = await browser.newContext({viewport: {width: 1440, height: 900}, locale: 'en-US', timezoneId: 'UTC', colorScheme: 'dark', reducedMotion: 'reduce', serviceWorkers: 'block'});
  const page = await context.newPage();
  page.setDefaultTimeout(12000);
  page.on('pageerror', error => result.pageErrors.push(error.message));
  await context.route('**/*', route => {
    const request = route.request();
    if (new URL(request.url()).origin !== origin || !['GET', 'HEAD'].includes(request.method())) {
      result.blocked.push({method: request.method(), path: new URL(request.url()).pathname});
      return route.abort('blockedbyclient');
    }
    return route.continue();
  });
  const settle = () => page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
  async function openTab(tab) {
    await page.locator(`.tab[data-tab="${tab}"]`).click();
    await page.locator(`#tab-${tab}.active`).waitFor({state: 'visible'});
    await page.waitForLoadState('networkidle');
    await settle();
  }
  async function screenshot(name) {
    await page.screenshot({path: path.join(outputDir, name + '.png'), mask: [page.locator('input[type="password"]')]});
  }
  try {
    await page.goto(origin, {waitUntil: 'networkidle'});
    for (const width of [1440, 390]) {
      await page.setViewportSize({width, height: 900});
      for (const tab of tabs) {
        await openTab(tab);
        await page.locator('#tab-' + tab).evaluate(el => { el.scrollTop = 0; });
        const metrics = await page.locator('#tab-' + tab).evaluate(panel => {
          const visible = el => el.getClientRects().length && getComputedStyle(el).visibility !== 'hidden';
          const label = el => el.getAttribute('aria-label') || (el.getAttribute('aria-labelledby') || '').split(' ').map(id => document.getElementById(id)?.textContent || '').join(' ').trim() || [...(el.labels || [])].map(l => l.textContent.trim()).join('');
          const controls = [...panel.querySelectorAll('button,input,select,textarea,a[href]')].filter(visible);
          const table = panel.querySelector('table');
          return {
            width: innerWidth, tab: panel.id, scrollHeight: panel.scrollHeight, clientHeight: panel.clientHeight,
            tableOffset: table ? Math.round(table.getBoundingClientRect().top - panel.getBoundingClientRect().top + panel.scrollTop) : null,
            fieldCount: controls.filter(el => ['INPUT', 'SELECT', 'TEXTAREA'].includes(el.tagName)).length,
            unnamedFields: controls.filter(el => ['INPUT', 'SELECT', 'TEXTAREA'].includes(el.tagName) && !label(el)).map(el => el.id),
            smallTargets: controls.filter(el => {const r = el.getBoundingClientRect(); return r.width < 44 || r.height < 44;}).map(el => {const r = el.getBoundingClientRect(); return {id: el.id, text: (el.textContent || '').trim().slice(0, 50), width: Math.round(r.width), height: Math.round(r.height)};}),
            sortableHeaders: [...panel.querySelectorAll('th.sortable')].map(el => ({text: el.textContent.trim(), tabIndex: el.tabIndex, hasButton: Boolean(el.querySelector('button'))})),
            minTableFont: table ? Math.min(...[...table.querySelectorAll('th,td')].map(el => parseFloat(getComputedStyle(el).fontSize))) : null,
          };
        });
        result.pages.push(metrics);
        await screenshot(`${width}-${tab}-top`);
        if (metrics.scrollHeight > metrics.clientHeight + 10) {
          await page.locator('#tab-' + tab).evaluate(el => { el.scrollTop = el.scrollHeight; });
          await settle();
          await screenshot(`${width}-${tab}-bottom`);
        }
      }
    }
    await page.setViewportSize({width: 1440, height: 900});
    await openTab('overview');
    const scrub = page.locator('#overview-token-trend .usage-chart-scrub');
    await scrub.focus();
    const before = await scrub.getAttribute('aria-valuenow');
    await scrub.press('ArrowRight');
    result.chartKeyboard = {before, after: await scrub.getAttribute('aria-valuenow'), spokenValue: await scrub.getAttribute('aria-valuetext'), tooltip: await page.locator('#overview-token-trend-tip').innerText()};
    await fs.writeFile(path.join(outputDir, 'chart-accessibility.txt'), await page.locator('#overview-token-trend').ariaSnapshot());
    await openTab('performance');
    const header = page.locator('.perf-table th[data-sort="avg_ms"]');
    result.sortKeyboard = {before: await header.getAttribute('aria-sort')};
    await header.press('Enter');
    result.sortKeyboard.afterEnter = await header.getAttribute('aria-sort');
    await header.click();
    result.sortKeyboard.afterClick = await header.getAttribute('aria-sort');
    await openTab('fallback');
    result.fallback = await page.locator('#fallback-chain').evaluate(el => [...el.children].map(row => ({role: row.getAttribute('role'), draggable: row.draggable, tabIndex: row.tabIndex, buttons: [...row.querySelectorAll('button')].map(b => b.getAttribute('aria-label') || b.textContent.trim())})));
    await openTab('quota');
    for (const provider of providers) {
      const waiting = page.waitForResponse(r => new URL(r.url()).pathname === '/api/quota' && new URL(r.url()).searchParams.get('provider') === provider);
      await page.locator('#quota-provider').selectOption(provider, {force: true});
      await waiting;
      await page.waitForFunction(() => !document.getElementById('btn-refresh-quota').disabled);
      await settle();
      result.quotas.push({provider, text: await page.locator('#tab-quota').innerText()});
      await screenshot('quota-' + provider);
    }
    await openTab('settings');
    await page.locator('#tab-settings').evaluate(el => { el.scrollTop = 0; });
    await fs.writeFile(path.join(outputDir, 'settings-accessibility.txt'), await page.locator('#tab-settings').ariaSnapshot());
    const colors = () => page.evaluate(() => ({body: getComputedStyle(document.body).backgroundColor, text: getComputedStyle(document.body).color, dark: matchMedia('(prefers-color-scheme: dark)').matches}));
    result.theme = {dark: await colors()};
    await page.emulateMedia({colorScheme: 'light'});
    await settle();
    result.theme.light = await colors();
    await screenshot('settings-system-light');
    await page.locator('#btn-lang-toggle').click();
    await settle();
    result.language = {htmlLang: await page.locator('html').getAttribute('lang'), controls: await page.locator('#tab-settings label[for]').allTextContents()};
    await screenshot('settings-zh');
    assert.equal(result.pageErrors.length, 0, 'page errors during observations');
    assert.equal(result.blocked.length, 0, 'unexpected write or external request');
    result.ok = true;
  } catch (error) {
    result.failure = error.message;
    process.exitCode = 1;
  } finally {
    await fs.writeFile(path.join(outputDir, 'observations.json'), JSON.stringify(result, null, 2), {mode: 0o600});
    await context.close();
    await browser.close();
    // Only this already-verified synthetic fixture may be shut down.
    const stopped = await fetch(origin + '/api/proxy/stop', {method: 'POST'});
    assert(stopped.ok, 'synthetic fixture shutdown failed');
  }
  console.log(JSON.stringify({ok: result.ok, pages: result.pages.length, quotas: result.quotas.length, pageErrors: result.pageErrors, blocked: result.blocked, failure: result.failure}));
})().catch(error => {console.error(error.message); process.exitCode = 1;});
