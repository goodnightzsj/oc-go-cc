package gui

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestClinePassChannelPinConfigSave(t *testing.T) {
	t.Setenv("CLINE_PIN_TEST_KEY", "synthetic-secret")
	srv, path := configTestServer(t, `{"cline_pass":{"api_key":"${CLINE_PIN_TEST_KEY}","base_url":"https://example.invalid/chat/completions"}}`)
	for _, enabled := range []string{"true", "false"} {
		rec := httptest.NewRecorder()
		patch := `{"cline_pass":{"channel_pin_enabled":` + enabled + `,"channel_pin":"deepseek"}}`
		srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodPost, "/api/proxy/config", strings.NewReader(patch)))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("save: %d %s", rec.Code, rec.Body)
		}
		cfg := srv.atomicCfg.Get().ClinePass
		if cfg.ChannelPinEnabled != (enabled == "true") || cfg.ChannelPin != "deepseek" || cfg.BaseURL != "https://example.invalid/chat/completions" {
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
  for(const lang of ['en','zh']) {
    currentLang=lang;
    for(const key of ['clinepass.channelPin','clinepass.channelTarget','clinepass.channelHint']) assert.notEqual(t(key),key);
  }
  assert.ok(page.includes('aria-describedby="cline-pass-channel-hint"'));
}
`+platformBehaviorRunScript)
}
