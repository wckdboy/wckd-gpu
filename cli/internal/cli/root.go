// Package cli is the wckd command line.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/wckdboy/wckd-gpu/cli/internal/config"
	"github.com/wckdboy/wckd-gpu/cli/internal/runpod"
	"github.com/wckdboy/wckd-gpu/cli/internal/s3store"
	"github.com/wckdboy/wckd-gpu/cli/internal/session"
)

// App is the CLI entrypoint.
type App struct {
	ConfigPath string
	Stdout     io.Writer
	Stderr     io.Writer
}

// Execute runs the wckd command using process arguments.
func Execute() error {
	app := &App{Stdout: os.Stdout, Stderr: os.Stderr}
	return app.Command().Execute()
}

// Command builds the cobra tree.
func (a *App) Command() *cobra.Command {
	if a.Stdout == nil {
		a.Stdout = os.Stdout
	}
	if a.Stderr == nil {
		a.Stderr = os.Stderr
	}
	root := &cobra.Command{
		Use:   "wckd",
		Short: "Timed RunPod GPU sessions with S3 hydrate and drain",
		Long: `wckd starts a timed GPU pod on RunPod, asks the pod sidecar to hydrate a
project prefix from S3-compatible storage, and drains that prefix back before
the pod is terminated.

Score for offers is (perf_index / usd_per_hr) * reliability_factor * region_factor.
perf_index values are labeled estimates, not live benchmarks.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if os.Getenv("WCKD_DISABLE_DOTENV") == "1" {
				return nil
			}
			return config.LoadDotEnv(".env")
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentFlags().StringVar(&a.ConfigPath, "config", "", "YAML config file (env overrides the file)")
	root.SetOut(a.Stdout)
	root.SetErr(a.Stderr)
	root.AddCommand(
		a.configCmd(),
		a.offersCmd(),
		a.startCmd(),
		a.statusCmd(),
		a.stopCmd(),
		a.sweeperCmd(),
	)
	return root
}

type deps struct {
	cfg     config.Config
	svc     *session.Service
	pods    *runpod.Client
	objects *s3store.Store
}

func (a *App) open() (*deps, error) {
	cfg, err := config.Load(a.ConfigPath)
	if err != nil {
		return nil, err
	}
	pods := runpod.New(cfg.RunPod.BaseURL, cfg.RunPod.APIKey)
	objects, err := s3store.New(cfg.S3)
	if err != nil {
		return nil, err
	}
	svc := &session.Service{
		Vendor:   pods,
		Objects:  objects,
		Sessions: session.NewStore(cfg.StateDir),
	}
	if err := applyDrainTiming(svc); err != nil {
		return nil, err
	}
	return &deps{cfg: cfg, svc: svc, pods: pods, objects: objects}, nil
}

// applyDrainTiming reads optional WCKD_DRAIN_WAIT and WCKD_DRAIN_POLL (Go
// durations, for example 2m or 5s). The defaults are 2 minutes and 5 seconds.
func applyDrainTiming(svc *session.Service) error {
	if v := os.Getenv("WCKD_DRAIN_WAIT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("WCKD_DRAIN_WAIT: %w", err)
		}
		if d <= 0 {
			return fmt.Errorf("WCKD_DRAIN_WAIT must be > 0")
		}
		svc.DrainWait = d
	}
	if v := os.Getenv("WCKD_DRAIN_POLL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("WCKD_DRAIN_POLL: %w", err)
		}
		if d <= 0 {
			return fmt.Errorf("WCKD_DRAIN_POLL must be > 0")
		}
		svc.DrainPoll = d
	}
	return nil
}

func (a *App) stdout() io.Writer {
	if a.Stdout != nil {
		return a.Stdout
	}
	return os.Stdout
}

func (a *App) stderr() io.Writer {
	if a.Stderr != nil {
		return a.Stderr
	}
	return os.Stderr
}

func requireCredentials(cfg config.Config) error {
	missing := cfg.MissingCredentials()
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("missing credentials:\n- %s", joinLines(missing))
}

func joinLines(lines []string) string {
	out := ""
	for i, line := range lines {
		if i > 0 {
			out += "\n- "
		}
		out += line
	}
	return out
}

func cmdContext() context.Context {
	return context.Background()
}
