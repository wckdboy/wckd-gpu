package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/wckdboy/wckd-gpu/cli/internal/serve"
	"github.com/wckdboy/wckd-gpu/cli/internal/session"
)

func (a *App) serveCmd() *cobra.Command {
	var (
		listen           string
		keyFile          string
		interval         time.Duration
		allowNonLoopback bool
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the local rental API on loopback",
		Long: `Listens on 127.0.0.1 by default and requires a bearer token from a mode-0600 key file.

The key is created on first run when the file is missing. It is never printed.
Pass --api-key-file (a path). There is no flag that puts the key on the command line.

Invincible supervises this process and proxies /v1/rent/* to it. The engine stays
a separate process. See docs/API.md.

POST /v1/rent/sessions returns 409 with the catalog cost estimate until the body
sets confirm to true or the request sends X-Wckd-Confirm: true. Nothing is
provisioned before that. There is no flag that turns the check off.

An in-process sweeper drains and terminates sessions past their deadline, including
sessions loaded from the state directory after this process restarts. wckd serve
does not spawn a second sweeper process.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := a.open()
			if err != nil {
				return err
			}
			if err := requireCredentials(d.cfg); err != nil {
				return err
			}
			if err := os.MkdirAll(d.cfg.StateDir, 0o700); err != nil {
				return err
			}
			if !cmd.Flags().Changed("listen") {
				if v := os.Getenv("WCKD_SERVE_LISTEN"); v != "" {
					listen = v
				}
			}
			if !cmd.Flags().Changed("api-key-file") {
				if v := os.Getenv("WCKD_SERVE_KEY_FILE"); v != "" {
					keyFile = v
				}
			}
			if !cmd.Flags().Changed("sweep-interval") {
				if v := os.Getenv("WCKD_SWEEP_INTERVAL"); v != "" {
					parsed, err := time.ParseDuration(v)
					if err != nil {
						return fmt.Errorf("WCKD_SWEEP_INTERVAL: %w", err)
					}
					interval = parsed
				}
			}
			if interval <= 0 {
				return fmt.Errorf("sweep interval must be > 0")
			}
			if keyFile == "" {
				keyFile = filepath.Join(d.cfg.StateDir, "serve.key")
			}
			key, created, err := serve.LoadOrCreateKey(keyFile)
			if err != nil {
				return err
			}
			if created {
				fmt.Fprintf(a.stdout(), "key_file: %s (created, mode 0600, value not printed)\n", keyFile)
			} else {
				fmt.Fprintf(a.stdout(), "key_file: %s\n", keyFile)
			}
			ctx := cmd.Context()
			if ctx == nil {
				var stop context.CancelFunc
				ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
				defer stop()
			}
			ready := make(chan struct{})
			srv := &serve.Server{
				Addr:             listen,
				AllowNonLoopback: allowNonLoopback,
				Service:          d.svc,
				PresetsDir:       d.cfg.PresetsDir,
				Image:            d.cfg.Image,
				Storage: session.StorageCreds{
					Endpoint:  d.cfg.S3.Endpoint,
					Bucket:    d.cfg.S3.Bucket,
					AccessKey: d.cfg.S3.AccessKey,
					SecretKey: d.cfg.S3.SecretKey,
					Region:    d.cfg.S3.Region,
				},
				Key:        key,
				SweepEvery: interval,
				Log:        log.New(a.stderr(), "", log.LstdFlags),
				Redact:     []string{d.cfg.RunPod.APIKey},
				Ready:      ready,
			}
			errCh := make(chan error, 1)
			go func() {
				errCh <- srv.Run(ctx)
			}()
			select {
			case <-ready:
			case err := <-errCh:
				return err
			}
			fmt.Fprintf(a.stdout(), "listening: %s\n", srv.URL)
			fmt.Fprintf(a.stdout(), "sweep_interval: %s\n", interval)
			return <-errCh
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:8787", "loopback address to bind")
	cmd.Flags().StringVar(&keyFile, "api-key-file", "", "bearer key file (mode 0600; default $WCKD_STATE_DIR/serve.key)")
	cmd.Flags().DurationVar(&interval, "sweep-interval", 15*time.Second, "time between deadline sweeps")
	cmd.Flags().BoolVar(&allowNonLoopback, "allow-non-loopback", false, "bind a non-loopback address (breaks the engine contract)")
	return cmd
}
