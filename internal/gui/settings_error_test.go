package gui

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/routatic/proxy/internal/config"
)

func checkSettingsError(t *testing.T, rec *httptest.ResponseRecorder, status int, code, field string, saved bool) {
	t.Helper()
	var response struct {
		Error settingsError `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	got := response.Error
	if rec.Code != status || got.Code != code || got.Field != field || got.Saved != saved {
		t.Fatalf("error = %d %+v; want %d %s %s saved=%v", rec.Code, got, status, code, field, saved)
	}
	if got.Message != settingsErrorMessages[code] || got.Message == "" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing safe Chinese message or no-store")
	}
	for _, secret := range []string{"private-sentinel", "synthetic-key", "UNSET_SETTINGS_TEST", "/private/path"} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Fatalf("error leaked %s", secret)
		}
	}
}

func TestSettingsValidationErrorsPreserveState(t *testing.T) {
	for _, tc := range []struct{ patch, code, field string }{
		{`{`, "invalid_json", ""},
		{`null`, "object_required", ""},
		{`{"port":65536}`, "port_range", "port"},
		{`{"cline_pass":{"timeout_ms":"private-sentinel"}}`, "invalid_type", "cline_pass.timeout_ms"},
		{`{"cline_pass":{"timeout_ms":-1}}`, "non_negative", "cline_pass.timeout_ms"},
		{`{"cline_pass":{"base_url":"https://user:private-sentinel@example.invalid"}}`, "plain_url", "cline_pass.base_url"},
		{`{"cline_pass":{"channel_pin":"BAD/private-sentinel"}}`, "channel_slug", "cline_pass.channel_pin"},
		{`{"cline_pass":{"api_keys":[""]}}`, "empty_key", "cline_pass.api_keys[0]"},
		{`{"cline_pass":{"api_key":"private-sentinel${UNSET_SETTINGS_TEST}"}}`, "unresolved_env", "cline_pass.api_key"},
		{`{"cline_pass":{"api_keys":["` + keyMask + `","private-sentinel"]}}`, "masked_keys_mixed", "cline_pass.api_keys"},
		{`{"active_site":"private-sentinel"}`, "unknown_provider", "active_site"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			const original = `{"api_key":"synthetic-key","port":3456}`
			srv, path := configTestServer(t, original)
			before := srv.atomicCfg.Get()
			rec := httptest.NewRecorder()
			srv.handleProxyConfig(rec, httptest.NewRequest(http.MethodPost, "/api/proxy/config", strings.NewReader(tc.patch)))
			checkSettingsError(t, rec, 400, tc.code, tc.field, false)
			after, err := os.ReadFile(path)
			if err != nil || string(after) != original || srv.atomicCfg.Get() != before {
				t.Fatal("failed save changed state")
			}
		})
	}
}

func TestSettingsIOErrorsAndSavedMarker(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{
		{&os.PathError{Op: "write", Path: "/private/path", Err: os.ErrPermission}, "file_permission"},
		{&os.PathError{Op: "open", Path: "/private/path", Err: os.ErrNotExist}, "file_missing"},
		{&os.PathError{Op: "write", Path: "/private/path", Err: syscall.ENOSPC}, "disk_full"},
	} {
		rec := httptest.NewRecorder()
		writeConfigError(rec, tc.err)
		checkSettingsError(t, rec, 500, tc.code, "", false)
	}
	for _, saved := range []bool{false, true} {
		rec := httptest.NewRecorder()
		writeSettingsError(rec, 500, "redaction_failed", "", saved)
		checkSettingsError(t, rec, 500, "redaction_failed", "", saved)
	}
}

func TestSettingsAutostartFailureDoesNotPublishSuccess(t *testing.T) {
	srv := &Server{cfg: Config{}, proxyPort: 3456}
	srv.setAutostart = func(enabled bool, port int) error {
		if !enabled || port != 3456 {
			t.Fatal("wrong autostart request")
		}
		return errors.New("private-sentinel /private/path")
	}
	rec := httptest.NewRecorder()
	srv.handleConfig(rec, httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(`{"autostart":true,"notify":true}`)))
	checkSettingsError(t, rec, 500, "autostart_failed", "autostart", false)
	if srv.cfg.Autostart || srv.cfg.Notify {
		t.Fatal("failed operation published state")
	}
	srv.setAutostart = func(bool, int) error { return nil }
	rec = httptest.NewRecorder()
	srv.handleConfig(rec, httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(`{"autostart":true,"notify":true}`)))
	if rec.Code != 204 || !srv.cfg.Autostart || !srv.cfg.Notify {
		t.Fatal("successful operation not published")
	}
}

func TestSettingsImportUsesSameValidation(t *testing.T) {
	srv, _ := configTestServer(t, `{"api_key":"synthetic-key"}`)
	for _, apply := range []string{"false", "true"} {
		rec := httptest.NewRecorder()
		srv.handleConfigImport(rec, httptest.NewRequest(http.MethodPost, "/api/config/import", strings.NewReader(`{"config":{"cline_pass":{"channel_pin":"BAD/private-sentinel"}},"apply":`+apply+`}`)))
		checkSettingsError(t, rec, 400, "channel_slug", "cline_pass.channel_pin", false)
	}
	_, err := config.LoadJSON([]byte(`{"api_key":"private-sentinel${UNSET_SETTINGS_TEST}"}`))
	var validation *config.ValidationError
	if !errors.As(err, &validation) || validation.Code != "unresolved_env" || strings.Contains(err.Error(), "private-sentinel") {
		t.Fatal("diagnostic lost type or leaked key")
	}
}
