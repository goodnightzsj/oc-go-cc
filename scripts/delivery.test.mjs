import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, unlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const dir = mkdtempSync(path.join(tmpdir(), 'oc-delivery-'));
function run(command, args, cwd, env = {}) {
  const result = spawnSync(command, args, { cwd, env: { ...process.env, ...env }, encoding: 'utf8', timeout: 30000 });
  assert.ifError(result.error);
  return result;
}
for (const child of ['cmd', 'internal', 'pkg']) mkdirSync(path.join(dir, child));
writeFileSync(path.join(dir, 'go.mod'), 'module fixture\n\ngo 1.25.0\n');
writeFileSync(path.join(dir, 'go.sum'), '');
writeFileSync(path.join(dir, 'cmd/main.go'), 'package main\nimport (_ "embed"; "fmt")\n//go:embed value.txt\nvar value string\nfunc main() { fmt.Print(value) }\n');
function fingerprint() {
  const result = run('bash', ['-euo', 'pipefail', '-c', 'source "$1"; source_fingerprint "$2" "shasum -a 256"', 'test', path.join(root, 'scripts/go-common.sh'), dir], dir);
  assert.equal(result.status, 0, result.stderr);
  return result.stdout.trim();
}
const hashes = [];
for (const value of ['before', 'after']) {
  writeFileSync(path.join(dir, 'cmd/value.txt'), value);
  hashes.push(fingerprint());
  const binary = path.join(dir, 'app');
  const built = run('go', ['build', '-o', binary, './cmd'], dir, { GOTOOLCHAIN: 'local' });
  assert.equal(built.status, 0, built.stderr);
  assert.equal(run(binary, [], dir).stdout, value);
}
assert.notEqual(hashes[0], hashes[1]);
writeFileSync(path.join(dir, 'cmd/extra.txt'), 'new');
assert.notEqual(fingerprint(), hashes[1]);
unlinkSync(path.join(dir, 'cmd/extra.txt'));
assert.equal(fingerprint(), hashes[1]);
writeFileSync(path.join(dir, 'README.md'), 'outside watched trees');
assert.equal(fingerprint(), hashes[1]);
console.log('PASS: embedded input edit/add/delete and actual binary output');

if (process.argv.includes('--dist')) {
  for (const fail of ['amd64', 'arm64', 'none']) {
    const cwd = path.join(dir, fail);
    const stub = path.join(cwd, 'stub');
    mkdirSync(stub, { recursive: true });
    writeFileSync(path.join(stub, 'go'), '#!/bin/sh\nif [ "$GOARCH" = "$FAIL_ARCH" ]; then exit 23; fi\nwhile [ "$#" -gt 0 ]; do\nif [ "$1" = -o ]; then shift; printf fixture > "$1"; exit 0; fi\nshift\ndone\nexit 29\n', { mode: 0o755 });
    const result = run('make', ['-f', path.join(root, 'Makefile'), '-o', 'clean', 'dist', 'VERSION=test', 'PLATFORMS=linux-amd64 linux-arm64'], cwd, { PATH: `${stub}:${process.env.PATH}`, FAIL_ARCH: fail });
    const sums = path.join(cwd, 'dist/checksums.txt');
    if (fail === 'none') {
      assert.equal(result.status, 0, result.stderr);
      assert.match(readFileSync(sums, 'utf8'), /linux-amd64/);
      assert.match(readFileSync(sums, 'utf8'), /linux-arm64/);
    } else {
      assert.notEqual(result.status, 0);
      assert.equal(existsSync(sums), false);
    }
  }
  console.log('PASS: each target failure stops dist; all targets produce checksums');
}
