package gui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/provider"
)

func TestClinePassChannelPinConfigSave(t *testing.T) {
	t.Setenv("CLINE_PIN_TEST_KEY", "synthetic-secret")
	srv, path := configTestServer(t, `{"cline_pass":{"api_key":"${CLINE_PIN_TEST_KEY}","base_url":"https://example.invalid/chat/completions"}}`)
	for _, enabled := range []string{"true", "false"} {
		rec := httptest.NewRecorder()
		patch := `{"cline_pass":{"channel_pin_enabled":` + enabled + `,"channel_probe_enabled":` + enabled + `,"channel_pin":"deepseek"}}`
		srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodPost, "/api/proxy/config", strings.NewReader(patch)))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("save: %d %s", rec.Code, rec.Body)
		}
		cfg := srv.atomicCfg.Get().ClinePass
		if cfg.ChannelPinEnabled != (enabled == "true") || cfg.ChannelProbeEnabled != (enabled == "true") || cfg.ChannelPin != "deepseek" || cfg.BaseURL != "https://example.invalid/chat/completions" {
			t.Fatal("save lost settings")
		}
		saved, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(saved, []byte("${CLINE_PIN_TEST_KEY}")) || bytes.Contains(saved, []byte("synthetic-secret")) {
			t.Fatal("save changed secret placeholder")
		}
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodPost, "/api/proxy/config", strings.NewReader(`{"cline_pass":{"channel_pin_enabled":true,"channel_pin":"deepseek/alibaba"}}`)))
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest || !bytes.Equal(before, after) || srv.atomicCfg.Get().ClinePass.ChannelPinEnabled {
		t.Fatal("invalid save changed disk or runtime")
	}
	for _, target := range []string{"", "   "} {
		rec = httptest.NewRecorder()
		srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodPost, "/api/proxy/config", strings.NewReader(`{"cline_pass":{"channel_pin_enabled":true,"channel_pin":"`+target+`"}}`)))
		if rec.Code != http.StatusNoContent || srv.atomicCfg.Get().ClinePass.ChannelPin != "deepseek" {
			t.Fatalf("empty target did not use default: %d", rec.Code)
		}
	}
}

func TestClinePassChannelPinForm(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  const stored = {cline_pass:{channel_pin:'deepseek',channel_pin_enabled:false}};
  const patches = [];
  fetch = async (url, init) => {
    if (url === '/api/proxy/config') {
      if (init?.method === 'POST') {
        const patch = JSON.parse(init.body);
        patches.push(patch);
        Object.assign(stored.cline_pass,patch.cline_pass);
        return {ok:true};
      }
      return {ok:true,json:async()=>JSON.parse(JSON.stringify(stored))};
    }
    return {ok:true,json:async()=>({sites:[],active:''})};
  };
  await loadProxyConfig();
  const toggle = document.getElementById('cfg-cline-pass-channel-pin-enabled');
  const target = document.getElementById('cfg-cline-pass-channel-pin');
  assert.equal(toggle.checked,false);
  assert.equal(target.value,'deepseek');
  toggle.checked=true; target.value='fireworks';
  await saveProxyConfig();
  assert.deepEqual(patches[0],{cline_pass:{channel_pin_enabled:true,channel_pin:'fireworks'}});
  assert.equal(toggle.checked,true);
  toggle.checked=false;
  await saveProxyConfig();
  assert.deepEqual(patches[1],{cline_pass:{channel_pin_enabled:false}});
  assert.equal(target.value,'fireworks','turning off retains the target');
  const probe = document.getElementById('cfg-cline-pass-channel-probe-enabled');
  assert.equal(probe.checked,false);
  probe.checked=true;
  await saveProxyConfig();
  assert.deepEqual(patches[2],{cline_pass:{channel_probe_enabled:true}});
  assert.equal(toggle.checked,false,'diagnostics must not change required-channel policy');
  probe.checked=false;
  await saveProxyConfig();
  assert.deepEqual(patches[3],{cline_pass:{channel_probe_enabled:false}});
  for(const lang of ['en','zh']) {
    currentLang=lang;
    for(const key of ['clinepass.channelPin','clinepass.channelTarget','clinepass.channelHint','clinepass.channelProbe','clinepass.channelProbeHint']) assert.notEqual(t(key),key);
    assert.ok(t('clinepass.channelHint').includes('422 channel_pin_unsupported'),'required policy failure is not disclosed');
  }
  assert.ok(page.includes('aria-describedby="cline-pass-channel-hint"'));
}
`+platformBehaviorRunScript)
}

func TestClinePassChannelDiagnosticsAPIReadOnly(t *testing.T) {
	calls := 0
	srv := New(Options{ChannelDiagnostics: func() provider.ChannelDiagnosticsSnapshot {
		calls++
		return provider.ChannelDiagnosticsSnapshot{Enabled: true, Results: []provider.ChannelProbeResult{{
			Model: "synthetic", Pipeline: "planner", Verdict: "unknown", Reason: "no_alternative", Baseline: "private", Actual: "private", CanonicalSlug: "private/synthetic", Endpoint: "https://example.invalid/chat/completions", KeyHint: "safe-hint", CheckedAt: time.Unix(100, 0), ExpiresAt: time.Unix(200, 0),
		}}}
	}})
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		rec := httptest.NewRecorder()
		srv.handleChannelCatalog(rec, httptest.NewRequest(method, "/api/cline-pass/channels", nil))
		if method == http.MethodPost {
			if rec.Code != http.StatusMethodNotAllowed || calls != 1 {
				t.Fatal("non-read request accessed diagnostic owner")
			}
			continue
		}
		var result channelSnapshot
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" || result.Diagnostics == nil || result.Diagnostics.EnforcementVerified || len(result.Diagnostics.Results) != 1 || result.Diagnostics.Results[0].Reason != "no_alternative" {
			t.Fatalf("response=%+v", result)
		}
		row := result.Diagnostics.Results[0]
		if row.Baseline != "private" || row.Target != "" || row.Actual != "private" || row.CanonicalSlug != "private/synthetic" {
			t.Fatalf("diagnostic projection lost routing: %+v", row)
		}
	}
}

func TestClinePassChannelDiagnosticsRefreshAfterSave(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  const get=id=>document.getElementById(id);
  const stored={port:3456,cline_pass:{channel_probe_enabled:true,channel_pin:'deepseek',base_url:'https://old.example.invalid',api_keys:[]}};
  const ok=value=>({ok:true,json:async()=>JSON.parse(JSON.stringify(value))});
  const requests=[];
  let hold=false, rejectSave=false;
  fetch=async(url,init)=>{
    if(url==='/api/sites') return ok({sites:[],active:''});
    if(url==='/api/cline-pass/channels') {
      assert.ok(!init.method,'refresh must only read local evidence');
      const data={capture_enabled:false,channels:{},diagnostics:{enabled:stored.cline_pass.channel_probe_enabled,results:[]}};
      const request={signal:init.signal}; requests.push(request);
      return {ok:true,json:()=>hold ? new Promise(resolve=>{request.resolve=()=>resolve(data)}) : Promise.resolve(data)};
    }
    assert.equal(url,'/api/proxy/config');
    if(init?.method!=='POST') return ok(stored);
    if(rejectSave) return {ok:false,status:400,json:async()=>({error:{code:'invalid_config',message:'rejected',saved:false}})};
    const patch=JSON.parse(init.body);
    if(patch.cline_pass) Object.assign(stored.cline_pass,patch.cline_pass);
    if(patch.port) stored.port=patch.port;
    // The server returns masked key labels, never the submitted edit instructions.
    if(patch.cline_pass?.api_keys) stored.cline_pass.api_keys=['••••test'];
    return ok({});
  };
  const flush=async()=>{for(let i=0;i<12;i++) await Promise.resolve()};
  initKeyEditors();
  await loadProxyConfig();
  await loadChannelCatalog();
  const status=get('cline-pass-channel-diagnostics');
  assert.equal(status.textContent,t('clinepass.diagnosticsWaiting'));
  hold=true;
  const old=loadChannelCatalog(); await flush(); hold=false;
  get('cfg-cline-pass-channel-probe-enabled').checked=false;
  await saveProxyConfig(); await flush();
  assert.equal(requests.length,3,'successful Cline save must refresh diagnostic scope');
  assert.equal(requests[1].signal.aborted,true,'save must cancel a previous response body');
  assert.equal(status.textContent,t('clinepass.diagnosticsOff'));
  requests[1].resolve(); await old;
  assert.equal(status.textContent,t('clinepass.diagnosticsOff'),'late old scope replaced saved state');
  get('cfg-cline-pass-channel-probe-enabled').checked=true;
  await saveProxyConfig(); await flush();
  assert.equal(status.textContent,t('clinepass.diagnosticsWaiting'));
  get('cfg-cline-pass-base-url').value='https://new.example.invalid';
  await saveProxyConfig(); await flush();
  addKeyRow('cfg-cline-pass-api-keys');
  keyEditorRows('cfg-cline-pass-api-keys').at(-1).value='synthetic-test';
  await saveProxyConfig(); await flush();
  assert.equal(requests.length,6,'endpoint and key changes must refresh diagnostic scope');
  get('cfg-port').value='3457';
  await saveProxyConfig(); await flush();
  assert.equal(requests.length,6,'unrelated settings must not refresh diagnostics');
  rejectSave=true;
  get('cfg-cline-pass-channel-probe-enabled').checked=false;
  await saveProxyConfig(); await flush();
  assert.equal(requests.length,6,'rejected settings must not invalidate current evidence');
  assert.equal(status.textContent,t('clinepass.diagnosticsWaiting'));
}
`+platformBehaviorRunScript)
}

func TestClinePassChannelDiagnosticsPresentationAndRequestOwnership(t *testing.T) {
	runPlatformBehavior(t, platformBehaviorDOMScript+`
async function checks() {
  currentLang='zh';
  const get=id=>document.getElementById(id);
  const status=get('cline-pass-channel-diagnostics');
  const result={enabled:true,enforcement_verified:false,results:[{model:'vmc/test<img>',pipeline:'planner',verdict:'matched',status:200,baseline:'base',target:'private',actual:'private',canonical_slug:'private/test<img>',endpoint:'https://example.invalid/chat/completions',key_hint:'safe-hint',checked_at:'2000-01-01T00:00:00Z',expires_at:'2000-01-01T06:00:00Z'}]};
  renderChannelDiagnostics(result);
  assert.ok(status.textContent.includes('vmc/test<img>'));
  assert.equal(status.innerHTML,'','metadata must not be parsed as markup');
  assert.ok(status.textContent.includes(t('clinepass.diagnosticsMatched')));
  assert.ok(status.textContent.includes(t('clinepass.diagnosticsExpired')));
  assert.ok(status.textContent.includes('safe-hint'));
  assert.ok(status.textContent.includes('private/test<img>'),'canonical model omitted or treated as markup');
  assert.ok(status.textContent.includes('基线：base · 目标：private · 实际：private'));
  renderChannelDiagnostics({...result,results:[{...result.results[0],actual:'',canonical_slug:'',verdict:'unsupported',status:403}]});
  assert.ok(status.textContent.includes('实际：—'),'failed pin must not present baseline as actual');
  assert.ok(!status.textContent.includes('private/test<img>'),'failed pin retained an old canonical model');
  renderChannelDiagnostics({enabled:false,results:[]});
  assert.equal(status.textContent,t('clinepass.diagnosticsOff'));
  renderChannelDiagnostics({enabled:true,results:[]});
  assert.equal(status.textContent,t('clinepass.diagnosticsWaiting'));
  renderChannelDiagnostics();
  assert.equal(status.textContent,t('clinepass.diagnosticsUnavailable'));
  const reasons=['timeout','cancelled','transport_error','account_refused','upstream_refused','stream_failed','response_too_large','invalid_metadata','missing_channel','no_alternative'];
  for (const lang of ['en','zh']) {
    currentLang=lang;
    renderChannelDiagnostics(result);
    const route=t('clinepass.diagnosticsRoute').replace('{baseline}','base').replace('{target}','private').replace('{actual}','private');
    assert.ok(status.textContent.includes(route),'routing labels or values changed in '+lang);
    assert.ok(status.textContent.includes(t('clinepass.diagnosticsCanonical').replace('{model}','private/test<img>')));
    const literal={...result.results[0],baseline:'base:{target}:$&',target:'private:{actual}:$$',actual:'actual:{baseline}:$&',canonical_slug:'private/$&/$$/{model}'};
    renderChannelDiagnostics({...result,results:[literal]});
    const literalRoute=lang==='zh'
      ? '基线：base:{target}:$& · 目标：private:{actual}:$$ · 实际：actual:{baseline}:$&'
      : 'Baseline: base:{target}:$& · Target: private:{actual}:$$ · Actual: actual:{baseline}:$&';
    assert.ok(status.textContent.includes(literalRoute),'routing metadata was reinterpreted in '+lang);
    assert.ok(status.textContent.includes((lang==='zh'?'规范模型：':'Canonical model: ')+'private/$&/$$/{model}'),'canonical metadata was reinterpreted in '+lang);
    for (const reason of reasons) {
      renderChannelDiagnostics({...result,results:[{...result.results[0],verdict:'unknown',reason}]});
      const key='clinepass.diagnosticReason.'+reason;
      assert.notEqual(t(key),key,'missing reason translation');
      assert.ok(status.textContent.includes(t(key)),'diagnostic omitted safe reason '+reason);
    }
    renderChannelDiagnostics({...result,results:[{...result.results[0],verdict:'unknown',reason:'<private-response-detail>'}]});
    assert.ok(!status.textContent.includes('<private-response-detail>'),'unknown reason must not echo raw detail');
    assert.ok(status.textContent.includes(t('clinepass.diagnosticReason.unknown')));
  }
  const stored=get('cfg-cline-pass-channel-pin-enabled');stored.checked=false;
  const requests=[];
  fetch=(url,init)=>new Promise(resolve=>requests.push({url,init,resolve}));
  const older=loadChannelCatalog();
  const newer=loadChannelCatalog();
  assert.equal(requests[0].init.signal.aborted,true,'new read must cancel the old read');
  const answer=diagnostics=>({ok:true,json:async()=>({capture_enabled:false,channels:{},diagnostics})});
  requests[1].resolve(answer(result));await newer;
  const latest=status.textContent;
  requests[0].resolve(answer({enabled:false,results:[]}));await older;
  assert.equal(status.textContent,latest,'late old read replaced newer evidence');
  assert.equal(stored.checked,false,'diagnostic sample enabled enforcement');
  assert.ok(requests.every(r=>r.url==='/api/cline-pass/channels' && !r.init.method),'refresh must only read local results');
  let expire;
  setTimeout=callback=>{expire=callback;return 1};clearTimeout=()=>{};
  const timeout=loadChannelCatalog();expire();
  requests[2].resolve(answer(result));await timeout;
  assert.equal(status.textContent,t('clinepass.diagnosticsFailed'),'expired read must not remain loading or publish stale data');
  assert.ok(page.includes('id="cline-pass-channel-diagnostics"'));
  assert.ok(page.includes('aria-live="polite"'));
  for (const lang of ['en','zh']) {
    currentLang=lang;
    for (const key of ['diagnostics','diagnosticsRefresh','diagnosticsMatched','diagnosticsExpired','diagnosticsRoute','diagnosticsCanonical']) assert.notEqual(t('clinepass.'+key),'clinepass.'+key);
  }
}
`+platformBehaviorRunScript)
}
