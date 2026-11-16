import assert from 'node:assert/strict';
import { spawn, spawnSync } from 'node:child_process';
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, readlinkSync, realpathSync, statSync, symlinkSync, unlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const dir = realpathSync(mkdtempSync(path.join(tmpdir(), 'oc-delivery-')));
function run(command, args, cwd, env = {}) {
  const result = spawnSync(command, args, { cwd, env: { ...process.env, ...env }, encoding: 'utf8', timeout: 30000 });
  assert.ifError(result.error);
  return result;
}

const formatDir = path.join(dir, 'format-gate');
mkdirSync(formatDir);
assert.equal(run('git', ['init', '-q', '-b', 'format-fixture'], formatDir).status, 0);
writeFileSync(path.join(formatDir, '.gitignore'), '.tmp/\n');
mkdirSync(path.join(formatDir, '.tmp'));
writeFileSync(path.join(formatDir, '.tmp', 'research.go'), 'not Go source');
const trackedGo = path.join(formatDir, 'tracked.go');
writeFileSync(trackedGo, 'package fixture\nvar value=1\n');
assert.equal(run('git', ['add', '.'], formatDir).status, 0);
const formatGate = () => run('bash', [path.join(root, 'scripts/check-go-format.sh')], formatDir);
assert.notEqual(formatGate().status, 0, 'tracked formatting errors were ignored');
assert.equal(run('gofmt', ['-w', trackedGo], formatDir).status, 0);
assert.equal(formatGate().status, 0, 'ignored research copy affected formatting');
const newGo = path.join(formatDir, 'new file.go');
writeFileSync(newGo, 'package fixture\nvar added=2\n');
assert.notEqual(formatGate().status, 0, 'untracked source or space-containing filename was skipped');
writeFileSync(newGo, 'not Go source');
assert.notEqual(formatGate().status, 0, 'formatter parse failure became success');
assert.equal(run('gofmt', ['-w', trackedGo], formatDir).status, 0);
unlinkSync(newGo);
writeFileSync(path.join(formatDir, 'remaining.go'), 'package fixture\n');
unlinkSync(trackedGo);
assert.equal(formatGate().status, 0, 'tracked deletion became a formatter input');
const formatFailure = run('bash', ['-c',
  'gofmt() { return 23; }; export -f gofmt; bash "$1"', 'test', path.join(root, 'scripts/check-go-format.sh')], formatDir);
assert.equal(formatFailure.status, 23, 'empty formatter output hid its failure');
const listingFailure = run('bash', ['-c',
  'git() { if [[ "$1" == ls-files ]]; then return 24; fi; command git "$@"; }; export -f git; bash "$1"', 'test', path.join(root, 'scripts/check-go-format.sh')], formatDir);
assert.equal(listingFailure.status, 24, 'source enumeration failure became success');
mkdirSync(path.join(formatDir, 'scripts'));
copyFileSync(path.join(root, 'scripts/check-go-format.sh'), path.join(formatDir, 'scripts/check-go-format.sh'));
assert.equal(run('git', ['-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', '-c', 'core.hooksPath=/dev/null', 'commit', '-qm', 'fixture'], formatDir).status, 0);
const pushHook = () => run('bash', ['-c',
  'go() { printf "post-format-step\\n"; return 25; }; export -f go; bash "$1"', 'test', path.join(root, 'scripts/git-hooks/pre-push')], formatDir);
const validHook = pushHook();
assert.equal(validHook.status, 25, 'pre-push still checked ignored research copies');
assert.match(validHook.stdout, /post-format-step/);
writeFileSync(newGo, 'package fixture\nvar added=2\n');
const rejectedHook = pushHook();
assert.equal(rejectedHook.status, 1, 'pre-push accepted unformatted project source');
assert.doesNotMatch(rejectedHook.stdout, /post-format-step/);
unlinkSync(newGo);
assert.equal(run('git', ['branch', 'main'], formatDir).status, 0);
assert.equal(run('git', ['symbolic-ref', 'HEAD', 'refs/heads/main'], formatDir).status, 0);
const protectedHook = pushHook();
assert.equal(protectedHook.status, 0, 'protected branch policy changed');
assert.doesNotMatch(protectedHook.stdout, /post-format-step/);
console.log('PASS: shared format gate checks tracked/untracked source, ignores research and propagates failures');

for (const child of ['cmd', 'internal', 'pkg']) mkdirSync(path.join(dir, child));
mkdirSync(path.join(dir,'scripts'));
copyFileSync(path.join(root,'scripts/source-fingerprint.mjs'),path.join(dir,'scripts/source-fingerprint.mjs'));
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

for (const input of ['tailwind.config.js','package.json','package-lock.json','scripts/build-css.sh','scripts/go-common.sh']) {
  const baseline = fingerprint();
  writeFileSync(path.join(dir,input),'before');
  const added = fingerprint();
  assert.notEqual(added,baseline,'CSS build input addition was not fingerprinted: '+input);
  writeFileSync(path.join(dir,input),'after');
  assert.notEqual(fingerprint(),added,'CSS build input edit was not fingerprinted: '+input);
  unlinkSync(path.join(dir,input));
  assert.equal(fingerprint(),baseline,'CSS build input removal was not fingerprinted: '+input);
}
console.log('PASS: CSS config, dependency manifests and build script edit/add/delete change the fingerprint');

// The legacy byte contract includes spaces, binary bytes, empty files and module symlinks.
writeFileSync(path.join(dir,'cmd/space name.bin'),Buffer.from([0,255,10,0]));
writeFileSync(path.join(dir,'pkg/empty'),'');
writeFileSync(path.join(dir,'module-target'),'module fixture\n');
unlinkSync(path.join(dir,'go.mod'));
symlinkSync('module-target',path.join(dir,'go.mod'));
function legacyFingerprint() { return run('bash',['-euo','pipefail','-c',`
cd "$1"
(find cmd internal pkg -type f -print; printf '%s\\n' go.mod go.sum tailwind.config.js package.json package-lock.json scripts/build-css.sh scripts/go-common.sh scripts/source-fingerprint.mjs) |
sort | while IFS= read -r name; do
  [[ -f "$name" ]] || continue
  printf '%s\\0' "$name"
  cat "$name" || exit
  printf '\\0'
done | shasum -a 256 | awk '{print $1}'
`,'test',dir],dir); }
const legacy = legacyFingerprint();
assert.equal(legacy.status,0,legacy.stderr);
assert.equal(fingerprint(),legacy.stdout.trim(),'source reader changed the legacy byte encoding');
console.log('PASS: streamed fingerprint matches legacy bytes for spaces, binary, empty and symlink inputs');

// macOS cannot create a byte-root directory; inject only Node's lossy string boundary.
const lossyRootPreload=path.join(dir,'lossy-root-preload.mjs');
writeFileSync(lossyRootPreload,`process.cwd=()=>'/unrepresentable-root-\ufffd';
if (process.argv[2]) process.argv[2]='/unrepresentable-root-\ufffd';
`);
for (const shell of ['bash','/bin/bash']) {
  const lossyRoot=run(shell,['-euo','pipefail','-c','source "$1"; source_fingerprint "$2" "shasum -a 256"','test',path.join(root,'scripts/go-common.sh'),dir],dir,{NODE_OPTIONS:'--import='+lossyRootPreload});
  assert.equal(lossyRoot.status,0,lossyRoot.stderr);
  assert.equal(lossyRoot.stdout.trim(),legacy.stdout.trim(),'full fingerprint caller depends on Node root/cwd string decoding');
}
console.log('PASS: complete fingerprint caller is independent of lossy Node root/cwd strings (boundary injection)');

unlinkSync(path.join(dir,'scripts/source-fingerprint.mjs'));
const missingReader=run('bash',['-euo','pipefail','-c','source "$1"; source_fingerprint "$2" "shasum -a 256"','test',path.join(root,'scripts/go-common.sh'),dir],dir);
assert.notEqual(missingReader.status,0,'missing fingerprint reader silently yielded an empty hash');
assert.equal(missingReader.stdout,'');
copyFileSync(path.join(root,'scripts/source-fingerprint.mjs'),path.join(dir,'scripts/source-fingerprint.mjs'));

writeFileSync(path.join(dir,'scripts/source-fingerprint.mjs'),'');
for (const shell of ['bash','/bin/bash']) {
  const emptyReader=run(shell,['-euo','pipefail','-c','source "$1"; source_fingerprint "$2" "shasum -a 256"','test',path.join(root,'scripts/go-common.sh'),dir],dir);
  assert.notEqual(emptyReader.status,0,'empty fingerprint reader silently yielded an empty hash');
  assert.equal(emptyReader.stdout,'');
  assert.match(emptyReader.stderr,/empty fingerprint reader/);
}
copyFileSync(path.join(root,'scripts/source-fingerprint.mjs'),path.join(dir,'scripts/source-fingerprint.mjs'));

// APFS rejects non-UTF-8 names. Map only the fs path boundary; read real file bytes.
const rawName=Buffer.concat([Buffer.from('cmd/raw-'),Buffer.from([255]),Buffer.from('.txt')]);
const rawBacking=path.join(dir,'raw-name-content');
const rawPathReader=`import fs from 'node:fs';
import { syncBuiltinESMExports } from 'node:module';
import { pathToFileURL } from 'node:url';
const expected=Buffer.from('${rawName.toString('hex')}','hex');
const stat=fs.statSync, read=fs.createReadStream;
fs.statSync=(file)=>{
  if (Buffer.isBuffer(file) && file.equals(expected)) return stat(process.argv[3]);
  const error=new Error('unknown byte path');error.code='ENOENT';throw error;
};
fs.createReadStream=(file)=>{
  if (!Buffer.isBuffer(file) || !file.equals(expected)) throw new Error('wrong byte path');
  return read(process.argv[3]);
};
syncBuiltinESMExports();
await import(pathToFileURL(process.argv[1]));
`;
for (const contents of ['before','after']) {
  writeFileSync(rawBacking,contents);
  const result=spawnSync('node',['--input-type=module','-e',rawPathReader,path.join(root,'scripts/source-fingerprint.mjs'),dir,rawBacking],
    {input:Buffer.concat([rawName,Buffer.from('\n')]),timeout:30000});
  assert.ifError(result.error);
  assert.equal(result.status,0,String(result.stderr));
  assert.deepEqual(result.stdout,Buffer.concat([rawName,Buffer.from([0]),Buffer.from(contents),Buffer.from([0])]),'non-UTF-8 filename was omitted or rewritten');
}
console.log('PASS: raw filename bytes and changed content survive the filesystem path boundary');

if (process.platform === 'linux') {
  const rawFile=Buffer.concat([Buffer.from(dir+'/'),rawName]);
  let previous;
  for (const contents of ['before','after']) {
    writeFileSync(rawFile,contents);
    const legacy=legacyFingerprint();
    assert.equal(legacy.status,0,legacy.stderr);
    const current=fingerprint();
    assert.equal(current,legacy.stdout.trim(),'Linux byte filename differs from the legacy hash');
    assert.notEqual(current,previous,'Linux byte filename content edit was not fingerprinted');
    previous=current;
  }
  console.log('PASS: actual Linux non-UTF-8 filename edit changes the legacy-compatible hash');

  const rawRoot=Buffer.concat([Buffer.from(dir+'/byte-root-'),Buffer.from([255])]);
  const rawChild=name=>Buffer.concat([rawRoot,Buffer.from('/'+name)]);
  for (const name of ['cmd','internal','pkg','scripts']) mkdirSync(rawChild(name),{recursive:true});
  copyFileSync(path.join(root,'scripts/source-fingerprint.mjs'),rawChild('scripts/source-fingerprint.mjs'));
  let previousRoot;
  for (const contents of ['before','after']) {
    writeFileSync(rawChild('cmd/source.txt'),contents);
    const result=run('bash',['-euo','pipefail','-c',`
raw_root=$(printf '%s/byte-root-\\377' "$2")
source "$1"
source_fingerprint "$raw_root" "shasum -a 256"
cd "$raw_root"
printf 'cmd/source.txt\\0'; cat cmd/source.txt; printf '\\0'
printf 'scripts/source-fingerprint.mjs\\0'; cat scripts/source-fingerprint.mjs; printf '\\0'
`,'test',path.join(root,'scripts/go-common.sh'),dir],dir);
    assert.equal(result.status,0,result.stderr);
    const split=result.stdout.indexOf('\n');
    const hash=result.stdout.slice(0,split);
    const legacy=spawnSync('shasum',['-a','256'],{input:Buffer.from(result.stdout.slice(split+1)),encoding:'utf8',timeout:30000});
    assert.ifError(legacy.error);
    assert.equal(legacy.status,0,legacy.stderr);
    assert.equal(hash,legacy.stdout.split(' ')[0],'actual Linux byte-root caller changed legacy bytes');
    assert.notEqual(hash,previousRoot,'actual Linux byte-root content edit did not change hash');
    previousRoot=hash;
  }
  console.log('PASS: complete fingerprint caller in actual Linux non-UTF-8 root preserves encoding and detects edits');
}

function fingerprintReadFailure(cwd) {
  const preload = path.join(cwd,'fingerprint-read-failure.mjs');
  writeFileSync(preload,`import fs from 'node:fs';
import path from 'node:path';
import { Readable } from 'node:stream';
import { syncBuiltinESMExports } from 'node:module';
const read = fs.createReadStream;
fs.createReadStream = function(file, ...args) {
  if ((!process.env.HASH_READ_FAILURE_MARKER || fs.existsSync(process.env.HASH_READ_FAILURE_MARKER)) &&
      (!process.env.SOURCE_CHANGE || path.resolve(String(file)) === process.env.SOURCE_CHANGE)) {
    return new Readable({read() { this.destroy(new Error('injected fingerprint failure')); }});
  }
  return read(file, ...args);
};
syncBuiltinESMExports();
`);
  return {NODE_OPTIONS:'--import='+preload};
}
for (const tool of ['node','find']) {
  const inject = tool === 'find' ? 'find() { printf "injected fingerprint failure\\n" >&2; return 23; }; ' : '';
  const result = run('bash',['-euo','pipefail','-c',
    'source "$1"; ' + inject + 'value=$(source_fingerprint "$2" "shasum -a 256"); printf "accepted:%s" "$value"',
    'test',path.join(root,'scripts/go-common.sh'),dir],dir,tool === 'node' ? fingerprintReadFailure(dir) : {});
  assert.notEqual(result.status,0,tool+' failure returned a successful fingerprint');
  assert.match(result.stderr,/injected fingerprint failure/);
  assert.doesNotMatch(result.stdout,/accepted:/);
}
console.log('PASS: fingerprint enumeration and input read errors propagate');

// Exercise the production entry with isolated tools; never contact a service,
// account, or repository remote. A forced restart failure also checks rollback.
function deployFixture(name, compiler = 'success') {
  const cwd = path.join(dir, 'deploy-css', name);
  const stub = path.join(cwd, 'stub');
  for (const child of ['stub', 'scripts', 'cmd', 'pkg', 'internal/gui/assets', 'node_modules/.bin', '.tmp/prod/releases/old']) mkdirSync(path.join(cwd, child), {recursive:true});
  for (const name of ['prod-deploy.sh', 'go-common.sh', 'source-fingerprint.mjs', 'build-css.sh']) copyFileSync(path.join(root, 'scripts', name), path.join(cwd, 'scripts', name));
  writeFileSync(path.join(cwd, 'go.mod'), 'module fixture\n');
  writeFileSync(path.join(cwd, 'go.sum'), '');
  const css = path.join(cwd, 'internal/gui/assets/compiled-tailwind.css');
  writeFileSync(css, 'old-css');
  const old = path.join(cwd, '.tmp/prod/releases/old');
  const current = path.join(cwd, '.tmp/prod/current');
  symlinkSync(old, current);
  const log = path.join(cwd, 'actions.log');
  writeFileSync(log, '');
  writeFileSync(path.join(stub, 'git'), '#!/bin/sh\nprintf fixture\n', {mode:0o755});
  writeFileSync(path.join(stub, 'systemctl'), '#!/bin/sh\nprintf "service:%s\\n" "$*" >> "$ACTION_LOG"\nexit 1\n', {mode:0o755});
  writeFileSync(path.join(stub, 'curl'), '#!/bin/sh\nexit 1\n', {mode:0o755});
  writeFileSync(path.join(stub, 'go'), `#!/bin/sh
printf 'build\\n' >> "$ACTION_LOG"
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then
    shift
    printf '#!/bin/sh\\nprintf "catalog\\\\n" >> "$ACTION_LOG"\\n' > "$1"
    chmod +x "$1"
    if [ -n "\${SOURCE_CHANGE:-}" ]; then printf changed > "$SOURCE_CHANGE"; fi
    if [ -n "\${COMMIT_CHANGE:-}" ]; then printf changed > "$COMMIT_CHANGE"; fi
    if [ -n "\${HASH_READ_FAILURE_MARKER:-}" ]; then touch "$HASH_READ_FAILURE_MARKER"; fi
    exit 0
  fi
  shift
done
exit 29
`, {mode:0o755});
  if (compiler !== 'missing') {
    writeFileSync(path.join(cwd, 'node_modules/.bin/tailwindcss'), `#!/bin/sh
if [ -n "\${DEPLOY_HOLD:-}" ]; then
  printf entered > "$DEPLOY_HOLD"
  while [ ! -f "$DEPLOY_HOLD.release" ]; do sleep 0.05; done
fi
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then
    shift
    ${compiler === 'empty' ? ':' : 'printf new-css'} > "$1"
    exit ${compiler === 'failure' ? 23 : 0}
  fi
  shift
done
exit 29
`, {mode:0o755});
  }
  const env = {
    PATH:`${stub}:${process.env.PATH}`, ACTION_LOG:log, OC_GO_CC_GO_BIN:path.join(stub,'go'),
    OC_GO_CC_ALLOW_STALE:'1', OC_GO_CC_START_TIMEOUT:'0',
    OC_GO_CC_SERVICE_NAME:'oc-go-cc.service', OC_GO_CC_HEALTH_URL:'http://127.0.0.1:3456/health',
  };
  return {cwd, stub, css, old, current, log, env};
}
for (const input of ['index.html', 'app.js', 'config']) {
  const f = deployFixture('css-input-change-' + input);
  writeFileSync(path.join(f.cwd, 'internal/gui/assets/index.html'), '<div class="text-red-500"></div>');
  writeFileSync(path.join(f.cwd, 'internal/gui/assets/app.js'), 'const style = "text-red-500";');
  writeFileSync(path.join(f.cwd, 'internal/gui/assets/tailwind-input.css'), '@tailwind utilities;');
  writeFileSync(path.join(f.cwd, 'tailwind.config.js'), "module.exports={content:['./internal/gui/assets/**/*.{html,js}']};");
  const changedFile = input === 'config' ? 'tailwind.config.js' : 'internal/gui/assets/' + input;
  const changedText = input === 'config' ? "module.exports={content:[],safelist:['bg-blue-500']};" : 'bg-blue-500';
  writeFileSync(path.join(f.cwd, 'edit-at-write.cjs'), `const fs=require('node:fs');
const write=fs.promises.writeFile.bind(fs.promises);
fs.promises.writeFile=async function(file,...args) {
  if (String(file).includes('.compiled-tailwind.')) {
    fs.writeFileSync(${JSON.stringify(changedFile)},${JSON.stringify(changedText)});
    fs.writeFileSync('edit-observed','yes');
  }
  return write(file,...args);
};
`);
  writeFileSync(path.join(f.cwd, 'node_modules/.bin/tailwindcss'), '#!/bin/sh\nexec node "$REAL_TAILWIND" "$@"\n', {mode:0o755});
  const result = run('bash', ['scripts/prod-deploy.sh'], f.cwd, {
    ...f.env, NODE_OPTIONS:'--require=' + path.join(f.cwd, 'edit-at-write.cjs'),
    REAL_TAILWIND:path.join(root, 'node_modules/tailwindcss/lib/cli.js'),
  });
  assert.equal(readFileSync(path.join(f.cwd, changedFile), 'utf8'), changedText, 'input mutation did not run');
  assert.equal(readFileSync(path.join(f.cwd, 'edit-observed'), 'utf8'), 'yes');
  assert.notEqual(result.status, 0, 'changed CSS inputs reached deployment');
  assert.match(result.stderr, /inputs changed during CSS compilation/);
  assert.equal(readFileSync(f.css, 'utf8'), 'old-css', 'inconsistent CSS replaced the last good asset');
  assert.equal(readFileSync(f.log, 'utf8'), '', 'inconsistent CSS reached Go or service operations');
  assert.equal(readlinkSync(f.current), f.old);
  assert.deepEqual(readdirSync(path.join(f.cwd, '.tmp/prod/releases')), ['old']);
}
console.log('PASS: actual Tailwind input changes during successful compilation block CSS publication and deployment');

function editAfterCSSPublish(f) {
  writeFileSync(path.join(f.stub, 'mv'), `#!/bin/sh
/bin/mv "$@" || exit
if [ ! -f css-published ]; then
  printf changed > cmd/source.txt
  touch css-published
fi
`, {mode:0o755});
}

{
  const f = deployFixture('edit-after-css');
  writeFileSync(path.join(f.cwd, 'cmd/source.txt'), 'initial');
  editAfterCSSPublish(f);
  const result = run('bash', ['scripts/prod-deploy.sh'], f.cwd, f.env);
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /source changed during build/);
  assert.equal(readFileSync(path.join(f.cwd, 'cmd/source.txt'), 'utf8'), 'changed');
  assert.equal(readFileSync(f.log, 'utf8'), 'build\n', 'post-CSS edit reached catalog or cutover');
  assert.equal(readlinkSync(f.current), f.old);
}

for (const compiler of ['missing', 'failure', 'empty', 'success']) {
  const {cwd, css, old, current, log, env} = deployFixture(compiler, compiler);
  const result = run('bash', ['scripts/prod-deploy.sh'], cwd, env);
  assert.notEqual(result.status,0,'fixture must stop at CSS or forced restart failure');
  assert.equal(readlinkSync(current),old,'failed deployment changed the active release');
  const actions=readFileSync(log,'utf8');
  const releases=readdirSync(path.join(cwd,'.tmp/prod/releases'));
  if (compiler === 'success') {
    assert.equal(readFileSync(css,'utf8'),'new-css');
    assert.match(actions,/build\ncatalog\nservice:restart /,'valid CSS did not reach build and restart');
    const hash=run('bash',['-euo','pipefail','-c','source "$1"; source_fingerprint "$2" "shasum -a 256"','test',path.join(cwd,'scripts/go-common.sh'),cwd],cwd);
    assert.equal(hash.status,0,hash.stderr);
    assert.ok(releases.some(name=>name.includes('-'+hash.stdout.trim().slice(0,12)+'.')),'release fingerprint predates CSS generation');
  } else {
    assert.equal(actions,'','CSS failure reached build, catalog sync or service mutation');
    assert.equal(readFileSync(css,'utf8'),'old-css','CSS failure replaced the last good asset');
    assert.deepEqual(releases,['old'],'CSS failure created a release');
  }
}
console.log('PASS: production CSS gate precedes fingerprint/build/service changes and preserves last good assets');

{
  const f=deployFixture('empty-fingerprint-reader');
  writeFileSync(path.join(f.cwd,'scripts/source-fingerprint.mjs'),'');
  const source=path.join(f.cwd,'cmd/input.txt');
  writeFileSync(source,'before');
  const result=run('bash',['scripts/prod-deploy.sh'],f.cwd,{...f.env,SOURCE_CHANGE:source});
  assert.notEqual(result.status,0);
  assert.match(result.stderr,/empty fingerprint reader/);
  assert.equal(readFileSync(f.log,'utf8'),'','empty reader reached build, catalog or service');
  assert.equal(readFileSync(source,'utf8'),'before');
  assert.equal(readlinkSync(f.current),f.old);
  assert.deepEqual(readdirSync(path.join(f.cwd,'.tmp/prod/releases')),['old']);
  assert.equal(existsSync(path.join(f.cwd,'.tmp/prod/deploy.lock')),false);
}
console.log('PASS: empty fingerprint reader fails before build or release activation');

{
  const f = deployFixture('css-embed-boundary');
  writeFileSync(path.join(f.cwd,'internal/gui/embed.go'),'package gui\nimport "embed"\n//go:embed assets/*\nvar assets embed.FS\n');
  writeFileSync(path.join(f.cwd,'node_modules/.bin/tailwindcss'),`#!/bin/sh
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then shift; output=$1; break; fi
  shift
done
printf partial-css > "$output"
printf '%s' "$output" > css-stage.path
while [ ! -f css-stage.release ]; do /bin/sleep 0.02; done
printf new-css > "$output"
`,{mode:0o755});
  const child=spawn('bash',['scripts/build-css.sh'],{cwd:f.cwd});
  let output='',closed=false;
  child.stdout.on('data',data=>output+=data);
  child.stderr.on('data',data=>output+=data);
  const finished=new Promise(resolve=>child.on('close',code=>{closed=true;resolve(code);}));
  try {
    for (let i=0;i<500 && !closed && !existsSync(path.join(f.cwd,'css-stage.path'));i++) await new Promise(resolve=>setTimeout(resolve,10));
    assert.ok(existsSync(path.join(f.cwd,'css-stage.path')),output);
    const result=run('go',['list','-json','./internal/gui'],f.cwd,{GOTOOLCHAIN:'local'});
    assert.equal(result.status,0,result.stderr);
    assert.deepEqual(JSON.parse(result.stdout).EmbedFiles,['assets/compiled-tailwind.css'],'Go embedded in-progress CSS staging output');
    assert.equal(readFileSync(f.css,'utf8'),'old-css','CSS published before compile completion');
    writeFileSync(path.join(f.cwd,'css-stage.release'),'release');
    assert.equal(await finished,0,output);
    assert.equal(readFileSync(f.css,'utf8'),'new-css');
    assert.equal(existsSync(readFileSync(path.join(f.cwd,'css-stage.path'),'utf8')),false,'CSS staging file leaked');
  } finally {
    if (!closed) writeFileSync(path.join(f.cwd,'css-stage.release'),'cleanup owned compiler');
    await finished;
  }
}
console.log('PASS: actual Go embed excludes in-progress CSS staging files and publication remains atomic');

for (const scenario of ['before-INT', 'before-TERM', 'after-INT', 'after-TERM', 'first-TERM', 'restart-failure', 'health-failure', 'first-stop-failure', 'healthy-TERM']) {
  const f = deployFixture('rollback-' + scenario);
  if (scenario.startsWith('first-')) unlinkSync(f.current);
  writeFileSync(path.join(f.stub, 'systemctl'), `#!/bin/sh
printf 'service:%s\\n' "$*" >> "$ACTION_LOG"
if [ "$1" = restart ]; then
  if [ "$ROLLBACK_CASE" != healthy-TERM ] && { [ "$(readlink "$CURRENT_PATH")" != "$OLD_PATH" ] || [ "$ROLLBACK_CASE" = restart-failure ]; }; then exit 1; fi
fi
if [ "$1" = stop ] && [ "$ROLLBACK_CASE" = first-stop-failure ]; then exit 1; fi
exit 0
`, {mode:0o755});
  writeFileSync(path.join(f.stub, 'curl'), '#!/bin/sh\nprintf "health\\n" >> "$ACTION_LOG"\nif [ "$ROLLBACK_CASE" = health-failure ]; then exit 1; fi\nexit 0\n', {mode:0o755});
  const command = `
systemctl() {
  if [[ "$1" == restart && ( "$ROLLBACK_CASE" == after-* || "$ROLLBACK_CASE" == first-TERM ) ]]; then
    printf 'service:%s\\n' "$*" >> "$ACTION_LOG"
    ROLLBACK_SIGNAL=\${ROLLBACK_CASE##*-}
    ROLLBACK_CASE=rollback
    kill -"$ROLLBACK_SIGNAL" "$$"
  fi
  command systemctl "$@"
}
echo() {
  builtin echo "$@"
  if [[ "$ROLLBACK_CASE" == before-* && "$*" == '[prod] syncing model catalog' ]]; then kill -\${ROLLBACK_CASE#before-} "$$"; fi
  if [[ "$ROLLBACK_CASE" == healthy-TERM && "$*" == '[prod] deploy succeeded' ]]; then kill -TERM "$$"; fi
}
source scripts/prod-deploy.sh
`;
  const result = run('bash', ['-c', command], f.cwd, {...f.env, CURRENT_PATH:f.current, OLD_PATH:f.old, ROLLBACK_CASE:scenario, OC_GO_CC_START_TIMEOUT:'1'});
  const signalled = scenario.endsWith('-INT') || scenario.endsWith('-TERM');
  assert.equal(result.status, signalled ? scenario.endsWith('-INT') ? 130 : 143 : 1, scenario + ': original failure status changed');
  assert.equal(existsSync(path.join(f.cwd, '.tmp/prod/deploy.lock')), false, scenario + ': deployment lock leaked');
  const actions = readFileSync(f.log, 'utf8');
  if (scenario === 'healthy-TERM') {
    assert.notEqual(readlinkSync(f.current), f.old, 'confirmed healthy deployment was rolled back');
    assert.equal((actions.match(/service:restart /g) || []).length, 1);
    assert.doesNotMatch(result.stderr, /rolling back/);
  } else if (scenario.startsWith('before-')) {
    assert.equal(readlinkSync(f.current), f.old);
    assert.doesNotMatch(actions, /service:/, 'pre-cutover signal restarted the old service');
  } else if (scenario.startsWith('first-')) {
    assert.equal(existsSync(f.current), false, 'first deployment left the failed candidate active');
    assert.match(actions, /service:stop /);
  } else {
    assert.equal(readlinkSync(f.current), f.old, scenario + ': old release was not restored');
    assert.equal((actions.match(/service:restart /g) || []).length, 2);
  }
  if (scenario.endsWith('-failure')) {
    assert.match(result.stderr, /CRITICAL.*rollback.*failed/, scenario + ': recovery failure was hidden');
    assert.doesNotMatch(result.stderr, /rollback succeeded/);
  } else if (scenario.startsWith('after-') || scenario.startsWith('first-')) {
    assert.match(result.stderr, /rollback succeeded/, scenario + ': recovery result missing');
  }
}
assert.doesNotMatch(readFileSync(path.join(root, 'scripts/prod-watch.sh'), 'utf8'), /keeping previous healthy binary/, 'watcher assumes recovery succeeded');
console.log('PASS: cutover signals restore old release or stop first deployment, recovery failures are explicit, healthy activation is retained');

{
  const f = deployFixture('collision');
  writeFileSync(f.css, 'new-css');
  const hash = run('bash', ['-euo','pipefail','-c','source "$1"; source_fingerprint "$2" "shasum -a 256"','test',path.join(f.cwd,'scripts/go-common.sh'),f.cwd], f.cwd);
  assert.equal(hash.status,0,hash.stderr);
  const stamp = '20261116180000';
  writeFileSync(path.join(f.stub,'date'), `#!/bin/sh\nif [ "$1" = -u ]; then printf fixture; else printf ${stamp}; fi\n`, {mode:0o755});
  const old = path.join(f.cwd,'.tmp/prod/releases',stamp+'-'+hash.stdout.trim().slice(0,12));
  mkdirSync(old);
  const sentinel = path.join(old,'routatic-proxy');
  writeFileSync(sentinel,'healthy original binary');
  unlinkSync(f.current);
  symlinkSync(old,f.current);
  const result = run('bash',['scripts/prod-deploy.sh'],f.cwd,f.env);
  assert.notEqual(result.status,0);
  assert.equal(readFileSync(sentinel,'utf8'),'healthy original binary','same-second release name overwrote the rollback binary');
  assert.equal(readlinkSync(f.current),old);
  assert.equal(existsSync(path.join(f.cwd,'.tmp/prod/deploy.lock')),false,'failed deploy leaked its lock');
}

{
  const f = deployFixture('concurrent');
  const marker = path.join(f.cwd,'held');
  const first = spawn('bash',['scripts/prod-deploy.sh'],{cwd:f.cwd,env:{...process.env,...f.env,DEPLOY_HOLD:marker},timeout:30000});
  let output = '';
  first.stdout.on('data',data=>output+=data);
  first.stderr.on('data',data=>output+=data);
  const finished = new Promise((resolve,reject)=>{first.on('error',reject);first.on('close',resolve)});
  try {
    for(let i=0;i<200 && !existsSync(marker);i++) await new Promise(resolve=>setTimeout(resolve,10));
    assert.ok(existsSync(marker),'first deployment never entered the compiler');
    const second = run('bash',['scripts/prod-deploy.sh'],f.cwd,f.env);
    assert.equal(second.status,75,'concurrent deployment must report retryable lock contention');
    assert.match(second.stderr,/deployment lock/);
    assert.equal(readFileSync(f.log,'utf8'),'','contending deploy mutated build or service');
    assert.ok(existsSync(path.join(f.cwd,'.tmp/prod/deploy.lock')),'contender removed the owner lock');
  } finally {
    writeFileSync(marker+'.release','release');
    assert.notEqual(await finished,0,output);
  }
  assert.equal(existsSync(path.join(f.cwd,'.tmp/prod/deploy.lock')),false,'owner did not release its lock');
  assert.equal(readlinkSync(f.current),f.old);
}
console.log('PASS: same-second deploy preserves the rollback binary and concurrent deploy fails before CSS/build/service mutations');

{
  const f = deployFixture('invalid-lock');
  const lock = path.join(f.cwd,'.tmp/prod/deploy.lock');
  writeFileSync(lock,'not a lock directory');
  const result = run('bash',['scripts/prod-deploy.sh'],f.cwd,f.env);
  assert.equal(result.status,1,'invalid lock path must not be reported as temporary contention');
  assert.match(result.stderr,/cannot create deployment lock/);
  assert.equal(readFileSync(f.log,'utf8'),'');
  assert.equal(readFileSync(lock,'utf8'),'not a lock directory');
}

for (const changed of ['cmd/main.go','tailwind.config.js','HEAD']) {
  const f = deployFixture('source-change-'+changed.replaceAll('/','-'));
  const source = path.join(f.cwd,changed);
  writeFileSync(source,'fixture');
  const env = {...f.env};
  if (changed === 'HEAD') {
    writeFileSync(path.join(f.stub,'git'),'#!/bin/sh\nif [ "$3" = rev-parse ]; then cat "$COMMIT_CHANGE"; else printf fixture; fi\n',{mode:0o755});
    env.COMMIT_CHANGE=source;
  } else env.SOURCE_CHANGE=source;
  const result = run('bash',['scripts/prod-deploy.sh'],f.cwd,env);
  assert.notEqual(result.status,0);
  assert.equal(readFileSync(source,'utf8'),'changed','fixture did not change source during the build');
  assert.equal(readFileSync(f.log,'utf8'),'build\n','changed build reached catalog or service');
  assert.match(result.stderr,/source changed during build/);
  assert.equal(readlinkSync(f.current),f.old);
  assert.equal(existsSync(path.join(f.cwd,'.tmp/prod/deploy.lock')),false);
}
console.log('PASS: source/CSS/HEAD changes during build stop before catalog and cutover');

{
  const f = deployFixture('post-build-read-error');
  const marker = path.join(f.cwd,'read-failure');
  const source = path.join(f.cwd,'internal/source.txt');
  writeFileSync(source,'before');
  const result = run('bash',['scripts/prod-deploy.sh'],f.cwd,{...f.env,...fingerprintReadFailure(f.cwd),SOURCE_CHANGE:source,HASH_READ_FAILURE_MARKER:marker});
  assert.notEqual(result.status,0);
  assert.match(result.stderr,/cannot verify source fingerprint after build/);
  assert.equal(readFileSync(source,'utf8'),'changed');
  assert.equal(readFileSync(f.log,'utf8'),'build\n');
  assert.equal(readlinkSync(f.current),f.old);
  assert.equal(existsSync(path.join(f.cwd,'.tmp/prod/deploy.lock')),false);
}
console.log('PASS: post-build fingerprint errors refuse catalog and cutover');

for (const alias of [false,true]) {
  const f = deployFixture(alias ? 'symlink-prune' : 'healthy-prune');
  const releases = path.join(f.cwd,'.tmp/prod/releases');
  const oldDirs = ['11111111111111-a','11111111111111-b','11111111111111-c'];
  for (const name of oldDirs) mkdirSync(path.join(releases,name));
  const outside = path.join(f.cwd,'not-a-release');
  mkdirSync(outside);
  writeFileSync(path.join(outside,'sentinel'),'preserve');
  symlinkSync(outside,path.join(releases,'00000000000000-link'));
  writeFileSync(path.join(f.stub,'systemctl'),'#!/bin/sh\nprintf "service:%s\\n" "$*" >> "$ACTION_LOG"\nexit 0\n',{mode:0o755});
  writeFileSync(path.join(f.stub,'curl'),'#!/bin/sh\nprintf "health\\n" >> "$ACTION_LOG"\nexit 0\n',{mode:0o755});
  const env={...f.env,OC_GO_CC_START_TIMEOUT:'3',OC_GO_CC_KEEP_RELEASES:alias ? '1' : '2'};
  let result;
  if (alias) {
    const checkout=path.join(dir,'checkout-link');
    symlinkSync(f.cwd,checkout);
    writeFileSync(path.join(f.stub,'date'),'#!/bin/sh\nif [ "$1" = -u ]; then printf fixture; else printf 00000000000000; fi\n',{mode:0o755});
    result=run('/bin/bash',['-euo','pipefail','-c','cd "$1"; /bin/bash scripts/prod-deploy.sh','test',checkout],dir,env);
  } else result=run('/bin/bash',['scripts/prod-deploy.sh'],f.cwd,env);
  assert.equal(result.status,0,result.stderr);
  assert.match(result.stdout,/deploy succeeded/);
  assert.equal(readFileSync(f.log,'utf8'),'build\ncatalog\nservice:restart oc-go-cc.service\nservice:is-active --quiet oc-go-cc.service\nhealth\n');
  const current = readlinkSync(f.current);
  assert.notEqual(current,f.old);
  assert.ok(existsSync(path.join(current,'routatic-proxy')),'pruning removed the current binary');
  assert.equal(statSync(current).mode & 0o755,0o755,'unique release directory is not readable by the service user');
  assert.equal(existsSync(f.old),!alias,'old release retention did not match the limit');
  for (const name of oldDirs) assert.equal(existsSync(path.join(releases,name)),false,'old release was not pruned: '+name);
  assert.equal(readFileSync(path.join(outside,'sentinel'),'utf8'),'preserve','pruning followed a symlink outside releases');
  assert.equal(readdirSync(releases).filter(name=>statSync(path.join(releases,name)).isDirectory() && name!=='00000000000000-link').length,Number(env.OC_GO_CC_KEEP_RELEASES));
  assert.equal(existsSync(path.join(f.cwd,'.tmp/prod/deploy.lock')),false);
}
console.log('PASS: successful health/cutover retains current and bounded old releases, including symlink checkouts, without following release symlinks');

for (const scenario of ['generated', 'during-deploy', 'css-failure', 'fixed-during-css', 'lock-contention', 'deploy-failure']) {
  const cwd = path.join(dir, 'watch', scenario);
  for (const child of ['scripts', 'stub', 'cmd', 'pkg', 'internal/gui/assets']) mkdirSync(path.join(cwd, child), {recursive:true});
  for (const name of ['prod-watch.sh', 'go-common.sh', 'source-fingerprint.mjs']) copyFileSync(path.join(root, 'scripts', name), path.join(cwd, 'scripts', name));
  writeFileSync(path.join(cwd, 'go.mod'), 'module fixture\n');
  writeFileSync(path.join(cwd, 'go.sum'), '');
  writeFileSync(path.join(cwd, 'internal/source.txt'), 'initial');
  writeFileSync(path.join(cwd, 'internal/gui/assets/compiled-tailwind.css'), 'initial');
  writeFileSync(path.join(cwd, 'deploy.log'), '');
  writeFileSync(path.join(cwd, 'attempt.log'), '');
  writeFileSync(path.join(cwd, 'stub/sleep'), `#!/bin/sh
count=0
if [ -f tick ]; then count=$(cat tick); fi
count=$((count + 1))
printf '%s' "$count" > tick
if [ "$count" = 1 ]; then printf changed > internal/source.txt; fi
if [ "$count" = 2 ]; then touch lock-released; fi
if [ "$count" -ge 3 ]; then exit 77; fi
`, {mode:0o755});
  writeFileSync(path.join(cwd, 'scripts/build-css.sh'), `#!/bin/sh
if [ "$WATCH_CASE" = css-failure ]; then exit 23; fi
if [ "$WATCH_CASE" = fixed-during-css ] && [ "$(cat internal/source.txt)" = changed ]; then
  printf corrected > internal/source.txt
  exit 23
fi
cp internal/source.txt internal/gui/assets/compiled-tailwind.css
`, {mode:0o755});
  writeFileSync(path.join(cwd, 'scripts/prod-deploy.sh'), `#!/bin/sh
set -e
printf 'attempt\\n' >> attempt.log
if [ "$WATCH_CASE" = lock-contention ] && [ ! -f lock-released ]; then exit 75; fi
if [ "$WATCH_CASE" = deploy-failure ]; then exit 23; fi
bash scripts/build-css.sh
printf 'deploy\\n' >> deploy.log
if [ "$WATCH_CASE" = during-deploy ]; then printf later > internal/source.txt; fi
`, {mode:0o755});
  const result = run('bash', ['scripts/prod-watch.sh'], cwd, {PATH:`${path.join(cwd,'stub')}:${process.env.PATH}`, WATCH_CASE:scenario});
  assert.equal(result.status,77, result.stderr);
  const expected = scenario === 'during-deploy' ? 2 : ['css-failure','deploy-failure'].includes(scenario) ? 0 : 1;
  assert.equal(readFileSync(path.join(cwd,'deploy.log'),'utf8'), 'deploy\n'.repeat(expected), scenario);
  const attempts = ['during-deploy','lock-contention'].includes(scenario) ? 2 : scenario === 'css-failure' ? 0 : 1;
  assert.equal(readFileSync(path.join(cwd,'attempt.log'),'utf8'), 'attempt\n'.repeat(attempts), scenario+' retry policy');
  if (scenario === 'css-failure' || scenario === 'fixed-during-css') assert.match(result.stderr,/CSS build failed/);
}
console.log('PASS: watcher ignores its own CSS generation without swallowing concurrent source edits or retrying unchanged failures');

for (const scenario of ['generated', 'during-build', 'css-failure', 'fixed-during-css', 'during-css', 'after-css']) {
  const f = deployFixture('dev-' + scenario);
  copyFileSync(path.join(root, 'scripts/dev-watch.sh'), path.join(f.cwd, 'scripts/dev-watch.sh'));
  writeFileSync(path.join(f.cwd, 'cmd/source.txt'), 'initial');
  if (scenario === 'after-css') editAfterCSSPublish(f);
  writeFileSync(path.join(f.stub, 'go'), `#!/bin/sh
printf 'build:%s\\n' "$(cat internal/gui/assets/compiled-tailwind.css)" >> "$ACTION_LOG"
rm -f started
if [ "$DEV_CASE" = during-build ] && [ ! -f built ]; then printf changed > cmd/source.txt; fi
touch built
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then
    shift
    printf '#!/bin/sh\\nprintf "start\\\\n" >> "$ACTION_LOG"\\ntouch started\\ntrap "exit 0" INT TERM\\nwhile :; do /bin/sleep 0.05; done\\n' > "$1"
    chmod +x "$1"
    exit 0
  fi
  shift
done
exit 29
`, {mode:0o755});
  writeFileSync(path.join(f.cwd, 'node_modules/.bin/tailwindcss'), `#!/bin/sh
printf 'css\\n' >> "$ACTION_LOG"
if [ "$DEV_CASE" = css-failure ]; then exit 23; fi
if [ "$DEV_CASE" = fixed-during-css ] && [ "$(cat cmd/source.txt)" = initial ]; then
  printf corrected > cmd/source.txt
  exit 23
fi
if [ "$DEV_CASE" = during-css ] && [ "$(cat cmd/source.txt)" = initial ]; then
  printf corrected > cmd/source.txt
fi
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then shift; printf new-css > "$1"; exit 0; fi
  shift
done
exit 29
`, {mode:0o755});
  writeFileSync(path.join(f.stub, 'sleep'), `#!/bin/sh
count=0
if [ -f tick ]; then count=$(cat tick); fi
count=$((count + 1))
printf '%s' "$count" > tick
if [ "$DEV_CASE" != css-failure ]; then
  attempt=0
  while [ "$attempt" -lt 100 ] && [ ! -f started ]; do
    attempt=$((attempt + 1))
    /bin/sleep 0.01
  done
fi
if [ "$count" -ge 3 ]; then exit 77; fi
/bin/sleep 0.02
`, {mode:0o755});
  const result = run('bash', ['scripts/dev-watch.sh'], f.cwd, {...f.env, DEV_CASE:scenario});
  assert.equal(result.status, 77, result.stderr);
  const actions = readFileSync(f.log, 'utf8');
  const builds = scenario === 'css-failure' ? 0 : ['during-build', 'after-css'].includes(scenario) ? 2 : 1;
  assert.equal((actions.match(/^build:/gm) || []).length, builds, scenario + ': dev build count');
  assert.equal((actions.match(/^start$/gm) || []).length, builds, scenario + ': dev start count');
  assert.equal((actions.match(/^css$/gm) || []).length, scenario === 'css-failure' ? 1 : scenario === 'generated' ? 1 : 2, scenario + ': CSS attempt count');
  if (scenario === 'css-failure') {
    assert.equal(readFileSync(f.css, 'utf8'), 'old-css');
    assert.match(result.stderr, /CSS build failed/);
  } else {
    assert.ok((actions.match(/^build:new-css$/gm) || []).length === builds, 'dev binary used stale CSS');
    assert.equal(readFileSync(f.css, 'utf8'), 'new-css');
  }
}
console.log('PASS: dev-watch builds CSS before binaries, waits on failures and ignores its own CSS output without losing build-time edits');

for (const phase of ['running', 'waiting', 'waiting-no-edit', 'css-build']) {
  for (const signal of ['INT', 'TERM']) {
    const f = deployFixture('dev-stop-' + phase + '-' + signal);
    copyFileSync(path.join(root, 'scripts/dev-watch.sh'), path.join(f.cwd, 'scripts/dev-watch.sh'));
    writeFileSync(path.join(f.cwd, 'cmd/source.txt'), 'initial');
    writeFileSync(path.join(f.stub, 'go'), `#!/bin/sh
printf 'build\\n' >> "$ACTION_LOG"
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then
    shift
    printf '#!/bin/sh\\nprintf "start\\\\n" >> "$ACTION_LOG"\\ntouch started\\ntrap "exit 0" TERM\\nwhile :; do /bin/sleep 0.02; done\\n' > "$1"
    chmod +x "$1"
    exit 0
  fi
  shift
done
exit 29
`, {mode:0o755});
    writeFileSync(path.join(f.cwd, 'node_modules/.bin/tailwindcss'), `#!/bin/sh
printf 'css\\n' >> "$ACTION_LOG"
if { [ "$STOP_PHASE" = waiting ] || [ "$STOP_PHASE" = waiting-no-edit ]; } && [ ! -f signalled ]; then exit 23; fi
if [ "$STOP_PHASE" = css-build ] && [ ! -f signalled ]; then
  touch signalled
  kill -"$STOP_SIGNAL" "$(cat watcher.pid)"
  printf changed > cmd/source.txt
fi
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then shift; printf new-css > "$1"; exit 0; fi
  shift
done
exit 29
`, {mode:0o755});
    writeFileSync(path.join(f.stub, 'sleep'), `#!/bin/sh
count=0
if [ -f tick ]; then count=$(cat tick); fi
count=$((count + 1))
printf '%s' "$count" > tick
if [ "$count" -ge 3 ]; then exit 77; fi
if [ ! -f signalled ]; then
  if [ "$STOP_PHASE" = running ]; then
    attempt=0
    while [ "$attempt" -lt 200 ] && [ ! -f started ]; do
      attempt=$((attempt + 1)); /bin/sleep 0.01
    done
    [ -f started ] || exit 29
  fi
  touch signalled
  kill -"$STOP_SIGNAL" "$(cat watcher.pid)"
  if [ "$STOP_PHASE" != waiting-no-edit ]; then printf changed > cmd/source.txt; fi
fi
/bin/sleep 0.02
`, {mode:0o755});
    const result = run('bash', ['-c', 'printf "%s" "$$" > watcher.pid; exec bash scripts/dev-watch.sh'], f.cwd,
      {...f.env, STOP_PHASE:phase, STOP_SIGNAL:signal});
    assert.equal(result.status, signal === 'INT' ? 130 : 143, phase + '/' + signal + ': watcher did not exit on signal');
    const actions = readFileSync(f.log, 'utf8');
    const started = phase === 'running' ? 1 : 0;
    assert.equal((actions.match(/^start$/gm) || []).length, started, 'source edit after stop restarted service');
    assert.equal((actions.match(/^build$/gm) || []).length, started, 'stop allowed a subsequent build');
    assert.equal((actions.match(/^css$/gm) || []).length, 1, 'stop allowed a subsequent CSS attempt');
    if (phase === 'waiting-no-edit') {
      assert.equal(readFileSync(path.join(f.cwd,'cmd/source.txt'),'utf8'),'initial');
      assert.equal(readFileSync(path.join(f.cwd,'tick'),'utf8'),'1','watcher kept polling until another source edit or failure');
    }
  }
}
console.log('PASS: dev-watch INT/TERM exit in running, waiting, no-edit waiting and CSS-build phases without rebuilding after stop');

for (const phase of ['css', 'go']) {
  for (const signal of ['SIGINT', 'SIGTERM']) {
    for (const [stubborn,shell] of [[false,'bash'],[true,'bash'],[false,'/bin/bash'],[true,'/bin/bash']]) {
      const f = deployFixture('dev-inflight-' + phase + '-' + signal + '-' + stubborn + '-' + path.basename(path.dirname(shell)));
      copyFileSync(path.join(root, 'scripts/dev-watch.sh'), path.join(f.cwd, 'scripts/dev-watch.sh'));
      const worker = `printf '%s' "$$" > worker.pid
${stubborn ? "trap '' TERM" : ':'}
/bin/sh -c 'printf "%s" "$$" > grandchild.pid; ${stubborn ? 'trap "" TERM;' : ''} while :; do /bin/sleep 0.02; done' &
while [ ! -f worker-release ]; do /bin/sleep 0.02; done
touch wrote-after-stop
`;
      writeFileSync(path.join(f.cwd, 'node_modules/.bin/tailwindcss'), `#!/bin/sh
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then shift; output=$1; break; fi
  shift
done
${phase === 'css' ? worker : ':'}
printf new-css > "$output"
`, {mode:0o755});
      writeFileSync(path.join(f.stub, 'go'), '#!/bin/sh\n' + worker, {mode:0o755});
      const child = spawn(shell, ['scripts/dev-watch.sh'], {cwd:f.cwd, env:{...process.env,...f.env,WATCH_INTERVAL:'0.02'},detached:true});
      let output = '', closed = false;
      child.stdout.on('data', data => output += data);
      child.stderr.on('data', data => output += data);
      const finished = new Promise(resolve => child.on('close', (code, signal) => { closed = true; resolve({code,signal}); }));
      try {
        for (let i=0;i<500 && !closed && !existsSync(path.join(f.cwd,'grandchild.pid'));i++) await new Promise(resolve=>setTimeout(resolve,10));
        assert.ok(existsSync(path.join(f.cwd,'grandchild.pid')),output);
        child.kill(signal);
        for (let i=0;i<400 && !closed;i++) await new Promise(resolve=>setTimeout(resolve,10));
        assert.ok(closed,phase+'/'+signal+': watcher waited for blocked worker\n'+output);
        const result = await finished;
        assert.equal(result.code,signal === 'SIGINT' ? 130 : 143,output);
        assert.equal(existsSync(path.join(f.cwd,'wrote-after-stop')),false,'worker continued after cancellation');
        assert.equal(readFileSync(f.css,'utf8'),phase === 'css' ? 'old-css' : 'new-css');
        assert.equal(existsSync(path.join(f.cwd,'.tmp/dev/routatic-proxy')),false,'cancelled build published a binary');
        assert.doesNotMatch(output,/\[dev\] starting/);
      } finally {
        for (const file of closed ? [] : ['worker.pid','grandchild.pid']) {
          if (!existsSync(path.join(f.cwd,file))) continue;
          try { process.kill(Number(readFileSync(path.join(f.cwd,file),'utf8')),'SIGKILL'); }
          catch (error) { if (error.code !== 'ESRCH') throw error; }
        }
        if (!closed) {
          try { process.kill(-child.pid,'SIGKILL'); } catch (error) { if (error.code !== 'ESRCH') throw error; }
        }
        await finished;
      }
    }
  }
}
console.log('PASS: dev-watch cancels/reaps blocked CSS/Go workers and descendants, including TERM-resistant workers');

if (process.argv.includes('--dist')) {
  for (const workflow of ['ci.yml', 'release-pipeline.yml']) {
    const source = readFileSync(path.join(root, '.github/workflows', workflow), 'utf8');
    const command = source.match(/      - name: Verify native WorkBuddy updater\n        run: ([^\n]+)/)?.[1];
    assert.ok(command, workflow + ': missing native updater gate');
    const cwd = path.join(dir, 'updater-gate', workflow);
    mkdirSync(path.join(cwd, 'ops/workbuddy'), {recursive:true});
    writeFileSync(path.join(cwd, 'ops/workbuddy/update.test.sh'), '#!/bin/sh\n[ "$WB2A_TEST_PORTABLE" = 0 ] || exit 29\nexit "$TEST_GATE_STATUS"\n');
    for (const status of [0, 23, 77]) {
      const result = run('bash', ['-euo', 'pipefail', '-c', command], cwd, {WB2A_TEST_PORTABLE:'1', TEST_GATE_STATUS:String(status)});
      assert.equal(result.status, status, workflow + ': native gate ignored failure or used portable mode');
    }
  }
  console.log('PASS: CI and shared release validation require native updater checks; failures and unavailable tools fail the gate');
  const releaseWorkflow = readFileSync(path.join(root, '.github/workflows/release-pipeline.yml'), 'utf8');
  const releaseStep = releaseWorkflow
    .split('      - name: Build cross-platform binaries\n')[1].split('\n      - name:')[0];
  const releaseBuild = releaseStep.split('        run: |\n')[1].split('\n').map(line => line.slice(10)).join('\n');
  assert.match(releaseBuild, /make dist-binaries VERSION="\$VERSION"/, 'release must use the tested build entry');
  const dmgPlan = run('make', ['-n', 'dmg', 'VERSION=test'], root);
  assert.equal(dmgPlan.status, 0, dmgPlan.stderr);
  assert.equal((releaseWorkflow.match(/CGO_ENABLED=1 go build/g) || []).length + (dmgPlan.stdout.match(/CGO_ENABLED=1 go build/g) || []).length, 1, 'release and make dmg must compile the CGO binary exactly once');
  assert.match(dmgPlan.stdout, /-X main.version=test/);
  const platforms = ['darwin-amd64', 'darwin-arm64', 'linux-amd64', 'linux-arm64', 'windows-amd64', 'windows-arm64'];
  const binaryName = platform => 'routatic-proxy_' + platform + (platform.startsWith('windows-') ? '.exe' : '');
  for (const entry of ['dist', 'release']) {
    for (const fail of [...platforms, 'none']) {
      const cwd = path.join(dir, entry, fail);
      const stub = path.join(cwd, 'stub');
      mkdirSync(stub, { recursive: true });
      for (const child of ['scripts', 'node_modules/.bin', 'cmd', 'pkg', 'internal/gui/assets']) mkdirSync(path.join(cwd, child), { recursive: true });
      for (const name of ['Makefile', 'scripts/build-css.sh', 'scripts/go-common.sh', 'scripts/source-fingerprint.mjs']) copyFileSync(path.join(root, name), path.join(cwd, name));
      writeFileSync(path.join(cwd, 'node_modules/.bin/tailwindcss'), '#!/bin/sh\nwhile [ "$#" -gt 0 ]; do\nif [ "$1" = -o ]; then shift; printf fixture > "$1"; exit 0; fi\nshift\ndone\nexit 29\n', { mode: 0o755 });
      writeFileSync(path.join(stub, 'go'), '#!/bin/sh\nif [ "$GOOS-$GOARCH" = "$FAIL_PLATFORM" ]; then exit 23; fi\nwhile [ "$#" -gt 0 ]; do\nif [ "$1" = -o ]; then shift; printf fixture > "$1"; exit 0; fi\nshift\ndone\nexit 29\n', { mode: 0o755 });
      const env = { PATH: `${stub}:${process.env.PATH}`, FAIL_PLATFORM: fail, VERSION: 'test' };
      const result = entry === 'dist' ? run('make', ['dist', 'VERSION=test'], cwd, env)
        : run('bash', ['-euo', 'pipefail', '-c', releaseBuild], cwd, env);
      assert.equal(readFileSync(path.join(cwd, 'internal/gui/assets/compiled-tailwind.css'), 'utf8'), 'fixture');
      const sums = path.join(cwd, 'dist/checksums.txt');
      if (fail === 'none') {
        assert.equal(result.status, 0, result.stderr);
        for (const platform of platforms) {
          assert.equal(readFileSync(path.join(cwd, 'dist', binaryName(platform)), 'utf8'), 'fixture');
          if (entry === 'dist') assert.ok(readFileSync(sums, 'utf8').includes(binaryName(platform)));
        }
        if (entry === 'release') assert.equal(existsSync(sums), false, 'release checksums must wait for signing');
      } else {
        assert.notEqual(result.status, 0);
        assert.match(result.stderr, /Error 23/);
        for (const [index, platform] of platforms.entries()) {
          assert.equal(existsSync(path.join(cwd, 'dist', binaryName(platform))), index < platforms.indexOf(fail));
        }
        assert.equal(existsSync(sums), false);
      }
    }
  }
  console.log('PASS: all six target failures stop dist and actual release entry; checksums preserve signing order');
}
