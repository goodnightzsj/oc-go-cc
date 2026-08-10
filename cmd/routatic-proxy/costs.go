package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/storage"
	"github.com/spf13/cobra"
)

type providerCostCapture struct {
	Rows []storage.ProviderCostRecord `json:"rows"`
}

func costsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "costs",
		Short: "Reconcile provider-reported request costs",
	}
	cmd.AddCommand(costsReconcileCmd())
	return cmd
}

func costsReconcileCmd() *cobra.Command {
	var configPath, inputPath string
	var apply bool
	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Dry-run or apply sanitized provider request costs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCostsReconcile(cmd, configPath, inputPath, apply)
		},
	}
	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to config file")
	cmd.Flags().StringVar(&inputPath, "input", "", "Path to sanitized provider usage JSON")
	cmd.Flags().BoolVar(&apply, "apply", false, "Apply unique exact matches after the dry run")
	_ = cmd.MarkFlagRequired("input")
	return cmd
}

func runCostsReconcile(cmd *cobra.Command, configPath, inputPath string, apply bool) error {
	if configPath != "" {
		_ = os.Setenv("ROUTATIC_PROXY_CONFIG", configPath)
	}
	cfgPath := config.ResolveConfigPath()
	cfg, err := config.LoadFromPath(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	input, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open provider usage: %w", err)
	}
	defer func() { _ = input.Close() }()
	var capture providerCostCapture
	if err := json.NewDecoder(input).Decode(&capture); err != nil {
		return fmt.Errorf("decode provider usage: %w", err)
	}

	db, err := storage.Open(storageConfig(cfg))
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	defer func() { _ = db.Close() }()

	parent := cmd.Context()
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	report, reconcileErr := db.ReconcileProviderCosts(ctx, capture.Rows, apply)
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	if reconcileErr != nil {
		return reconcileErr
	}
	return nil
}
