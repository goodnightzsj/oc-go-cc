// Runs the synthetic-only acceptance matrix using an already installed browser.
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const vm = require('node:vm');

const [origin, playwrightPath, resultPath] = process.argv.slice(2);
assert(origin && playwrightPath && resultPath, 'usage: node run-browser-smoke.cjs <fixture-origin> <playwright-path> <result-path>');
const target = new URL(origin);
assert(target.protocol === 'http:' && ['127.0.0.1', 'localhost', '[::1]'].includes(target.hostname), 'loopback fixture required');

(async () => {
  const {chromium} = require(playwrightPath);
  const browser = await chromium.launch({channel: 'chrome', headless: true});
  try {
    const page = await browser.newPage();
    await page.goto(origin, {waitUntil: 'networkidle'});
    const script = await fs.readFile(path.join(__dirname, 'multi-platform-browser-smoke.js'), 'utf8');
    const run = vm.runInThisContext('(' + script.trim() + ')');
    const result = await run(page);
    await fs.writeFile(resultPath, JSON.stringify(result, null, 2), {mode: 0o600});
    // A successful matrix has verified the ten synthetic request IDs and local
    // upstream before permitting writes. Never stop an unverified server.
    assert(result.ok, 'synthetic matrix did not complete');
    const stopped = await fetch(new URL('/api/proxy/stop', origin), {method: 'POST'});
    assert(stopped.ok, 'synthetic fixture shutdown failed: HTTP ' + stopped.status);
    console.log(JSON.stringify(result));
  } catch (error) {
    await fs.writeFile(resultPath, JSON.stringify({ok: false, error: error.message}, null, 2), {mode: 0o600});
    throw error;
  } finally {
    await browser.close();
  }
})().catch(error => {console.error(error.message); process.exitCode = 1});
