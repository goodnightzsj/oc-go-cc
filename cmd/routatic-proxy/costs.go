package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/storage"
	"github.com/spf13/cobra"
)

type providerCostCapture struct {
	CapturedAt time.Time                    `json:"captured_at"`
	Rows       []storage.ProviderCostRecord `json:"rows"`
}

func costsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "costs",
		Short: "Reconcile provider-reported request costs",
	}
	cmd.AddCommand(costsReconcileCmd())
	cmd.AddCommand(costsImportCmd())
	cmd.AddCommand(costsSyncRequestsCmd())
	return cmd
}

func costsSyncRequestsCmd() *cobra.Command {
	var configPath string
	var apply bool
	cmd := &cobra.Command{
		Use:   "sync-requests",
		Short: "Dry-run or correct request history from the persisted usage snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCostsSyncRequests(cmd, configPath, apply)
		},
	}
	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to config file")
	cmd.Flags().BoolVar(&apply, "apply", false, "Apply the transactional request correction")
	return cmd
}

func costsImportCmd() *cobra.Command {
	var configPath, inputPath string
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Replace the sanitized OpenCode account usage snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCostsImport(cmd, configPath, inputPath)
		},
	}
	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to config file")
	cmd.Flags().StringVar(&inputPath, "input", "", "Path to sanitized provider usage JSON")
	_ = cmd.MarkFlagRequired("input")
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
	capture, db, err := openProviderCapture(configPath, inputPath)
	if err != nil {
		return err
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

func runCostsImport(cmd *cobra.Command, configPath, inputPath string) error {
	capture, db, err := openProviderCapture(configPath, inputPath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if capture.CapturedAt.IsZero() {
		return errors.New("captured_at is required")
	}

	parent := cmd.Context()
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	if err := db.ReplaceProviderUsage(ctx, capture.CapturedAt, capture.Rows); err != nil {
		return err
	}
	analytics, err := db.GetProviderUsageAnalytics(ctx, 0)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(analytics.Summary)
}

func runCostsSyncRequests(cmd *cobra.Command, configPath string, apply bool) error {
	db, err := openSyncRequestStorage(configPath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	parent := cmd.Context()
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	report, err := db.SyncProviderUsageRequests(ctx, apply)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

// sync-requests only needs the storage block. Decoding the full runtime config
// would unnecessarily require API-key environment variables for an offline
// database maintenance command.
func openSyncRequestStorage(configPath string) (*storage.Database, error) {
	if configPath == "" {
		configPath = config.ResolveConfigPath()
	}
	input, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer func() { _ = input.Close() }()
	var file struct {
		Storage *config.StorageConfig `json:"storage"`
	}
	if err := json.NewDecoder(input).Decode(&file); err != nil {
		return nil, fmt.Errorf("decode storage config: %w", err)
	}
	cfg := storage.DefaultConfig
	if file.Storage != nil {
		cfg = cfg.WithOverlay(storage.Overlay{
			DatabasePath:      file.Storage.DatabasePath,
			RetentionDays:     file.Storage.RetentionDays,
			VacuumOnStartup:   file.Storage.VacuumOnStartup,
			WALEnabled:        file.Storage.WALEnabled,
			AnalyticsBaseline: file.Storage.AnalyticsBaseline,
		})
	}
	db, err := storage.Open(cfg)
	if err != nil {
		return nil, fmt.Errorf("open storage: %w", err)
	}
	return db, nil
}

func openProviderCapture(configPath, inputPath string) (providerCostCapture, *storage.Database, error) {
	if configPath != "" {
		_ = os.Setenv("ROUTATIC_PROXY_CONFIG", configPath)
	}
	cfgPath := config.ResolveConfigPath()
	cfg, err := config.LoadFromPath(cfgPath)
	if err != nil {
		return providerCostCapture{}, nil, fmt.Errorf("load config: %w", err)
	}

	input, err := os.Open(inputPath)
	if err != nil {
		return providerCostCapture{}, nil, fmt.Errorf("open provider usage: %w", err)
	}
	defer func() { _ = input.Close() }()
	var capture providerCostCapture
	if err := json.NewDecoder(input).Decode(&capture); err != nil {
		return providerCostCapture{}, nil, fmt.Errorf("decode provider usage: %w", err)
	}

	db, err := storage.Open(storageConfig(cfg))
	if err != nil {
		return providerCostCapture{}, nil, fmt.Errorf("open storage: %w", err)
	}
	return capture, db, nil
}
