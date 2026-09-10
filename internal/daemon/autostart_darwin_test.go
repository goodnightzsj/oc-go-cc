//go:build darwin

package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestEnableDisableAutostart_Darwin(t *testing.T) {
	// Setup temporary home directory
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	// HOME alone does not isolate launchctl's real gui/<uid> service domain.
	binDir := t.TempDir()
	launchctlLog := filepath.Join(tempHome, "launchctl.log")
	stub := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$TEST_LAUNCHCTL_LOG\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "launchctl"), []byte(stub), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("TEST_LAUNCHCTL_LOG", launchctlLog)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "ROUTATIC_PROXY_") {
			t.Setenv(key, "")
		}
	}
	// Set a ROUTATIC_PROXY_* var with characters that need XML escaping.
	t.Setenv("ROUTATIC_PROXY_API_KEY", "sk-test&key<value>")

	configPath := "/tmp/mock-config.json"
	port := 9999

	// Enable autostart
	err := EnableAutostart(configPath, port)
	if err != nil {
		t.Fatalf("EnableAutostart failed: %v", err)
	}

	// Verify plist file was created
	plistPath := filepath.Join(tempHome, "Library", "LaunchAgents", LaunchAgent+".plist")
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		t.Fatalf("Expected plist file to exist at %s, but it does not", plistPath)
	}

	// Read and verify plist contents
	contentBytes, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("Failed to read plist file: %v", err)
	}
	content := string(contentBytes)

	// Check for key elements in the plist
	if !strings.Contains(content, "<key>Label</key>\n    <string>com.routatic.proxy</string>") {
		t.Errorf("Plist missing correct Label string")
	}
	if !strings.Contains(content, "<string>start</string>") {
		t.Errorf("Plist missing start command")
	}
	if !strings.Contains(content, "<string>--background</string>") {
		t.Errorf("Plist missing --background flag")
	}
	if !strings.Contains(content, "<string>--config</string>\n        <string>/tmp/mock-config.json</string>") {
		t.Errorf("Plist missing config path arguments")
	}
	if !strings.Contains(content, "<string>--port</string>\n        <string>9999</string>") {
		t.Errorf("Plist missing port arguments")
	}

	// Verify ROUTATIC_PROXY_* env var is present and XML-escaped.
	if !strings.Contains(content, "<key>ROUTATIC_PROXY_API_KEY</key>") {
		t.Errorf("Plist missing ROUTATIC_PROXY_API_KEY env var")
	}
	// The value should be XML-escaped: & -> &amp;, < -> &lt;, > -> &gt;
	if !strings.Contains(content, "<string>sk-test&amp;key&lt;value&gt;</string>") {
		t.Error("Plist missing XML-escaped synthetic API key value")
	}

	// Disable autostart
	err = DisableAutostart()
	if err != nil {
		t.Fatalf("DisableAutostart failed: %v", err)
	}

	// Verify plist file was removed
	if _, err := os.Stat(plistPath); !os.IsNotExist(err) {
		t.Errorf("Expected plist file to be deleted, but it still exists")
	}
	commands, err := os.ReadFile(launchctlLog)
	if err != nil {
		t.Fatal(err)
	}
	domain := "gui/" + strconv.Itoa(os.Getuid())
	target := domain + "/" + LaunchAgent
	wantCommands := "bootout " + target + "\nbootstrap " + domain + " " + plistPath + "\nbootout " + target + "\n"
	if string(commands) != wantCommands {
		t.Errorf("stub launchctl calls = %q, want %q", commands, wantCommands)
	}
}
