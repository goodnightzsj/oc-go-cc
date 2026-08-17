package main

import (
	"os"
	"path/filepath"
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
