package cli

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

func (a *App) sweeperCmd() *cobra.Command {
	var (
		once     bool
		interval time.Duration
	)
	cmd := &cobra.Command{
		Use:   "sweeper",
		Short: "Stop sessions whose deadline has passed",
		Long: `Polls the local session directory and drains plus terminates anything past its deadline.

wckd start spawns this in the background. Run it yourself under systemd or cron
(--once) when the machine that started the session must enforce the timer
without keeping that shell open. A dead laptop cannot run this process; the
pod sidecar still drains and exits on its own deadline, and the next sweeper
run deletes the pod.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := a.open()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(d.cfg.StateDir, 0o700); err != nil {
				return err
			}
			_ = os.WriteFile(pidPath(d.cfg.StateDir), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600)
			ctx := cmd.Context()
			if ctx == nil {
				var stop context.CancelFunc
				ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
				defer stop()
			}
			log.SetOutput(a.stderr())
			if once {
				return d.svc.Sweep(ctx)
			}
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			log.Printf("sweeper watching %s every %s", d.cfg.StateDir, interval)
			for {
				if err := d.svc.Sweep(ctx); err != nil {
					log.Printf("sweep: %v", err)
				}
				select {
				case <-ctx.Done():
					return nil
				case <-ticker.C:
				}
			}
		},
	}
	cmd.Flags().BoolVar(&once, "once", false, "sweep one time and exit")
	cmd.Flags().DurationVar(&interval, "interval", 15*time.Second, "time between sweeps")
	return cmd
}
