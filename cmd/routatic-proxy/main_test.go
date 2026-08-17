package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/spf13/cobra"
)

func TestDefaultConfigValidWithGlobalAPIKey(t *testing.T) {
	t.Setenv("ROUTATIC_PROXY_API_KEY", "test-key")
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(getDefaultConfig()), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFromPath(path)
	if err != nil {
		t.Fatalf("generated default config is invalid: %v", err)
	}
	if cfg.AnthropicFirst.BaseURL != "https://api.anthropic.com" {
		t.Fatalf("AnthropicFirst=%+v", cfg.AnthropicFirst)
	}
}

// The legacy "serve" name is an alias of "start" and must still resolve there
// and report itself via CalledAs(), which is how RunE forces headless mode.
func TestServeAliasResolvesToStartAndReportsCalledAs(t *testing.T) {
	start := startCmd()
	for _, name := range []string{"config", "port", "background", "headless", "_daemonize"} {
		if start.Flags().Lookup(name) == nil {
			t.Fatalf("start is missing flag --%s", name)
		}
	}

	var calledAs string
	start.RunE = func(cmd *cobra.Command, args []string) error {
		calledAs = cmd.CalledAs()
		return nil
	}

	root := &cobra.Command{Use: appName}
	root.AddCommand(start)
	root.SetArgs([]string{"serve"})
	root.SetOut(os.Stderr)
	if err := root.Execute(); err != nil {
		t.Fatalf("serve alias failed to execute: %v", err)
	}
	if calledAs != "serve" {
		t.Fatalf("CalledAs()=%q, want %q (headless would not be forced)", calledAs, "serve")
	}
}

func TestServeReturnsProxyListenError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	port := listener.Addr().(*net.TCPAddr).Port

	configPath := filepath.Join(t.TempDir(), "config.json")
	databasePath := filepath.Join(t.TempDir(), "data.db")
	configJSON := fmt.Sprintf(`{
		"api_key":"test-key",
		"host":"127.0.0.1",
		"port":%d,
		"hot_reload":false,
		"catalog":{"enabled":false},
		"storage":{"database_path":%q,"retention_days":1}
	}`, port, databasePath)
	if err := os.WriteFile(configPath, []byte(configJSON), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ROUTATIC_PROXY_CONFIG", configPath)

	root := &cobra.Command{Use: appName, SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(startCmd())
	root.SetArgs([]string{"serve", "--port", strconv.Itoa(port)})
	err = root.Execute()
	if err == nil {
		t.Fatal("serve returned nil while its listen port was occupied")
	}
	if !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("serve error = %q, want listen failure", err)
	}
}
