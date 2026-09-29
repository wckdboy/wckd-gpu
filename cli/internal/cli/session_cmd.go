package cli

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/wckdboy/wckd-gpu/cli/internal/config"
	"github.com/wckdboy/wckd-gpu/cli/internal/preset"
	"github.com/wckdboy/wckd-gpu/cli/internal/runpod"
	"github.com/wckdboy/wckd-gpu/cli/internal/session"
	"github.com/wckdboy/wckd-gpu/cli/internal/vendor"
)

func (a *App) startCmd() *cobra.Command {
	var (
		hours     float64
		projectID string
		presetID  string
		offerID   string
		dryRun    bool
		noSweeper bool
		asJSON    bool
	)
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Pick an offer, create a pod, and arm the deadline",
		Example: `  wckd start --hours 1 --project hailuo-tests --preset comfyui-minimax-h3
  wckd start --hours 1 --project hailuo-tests --preset comfyui-minimax-h3 --offer 'runpod:community:NVIDIA GeForce RTX 4090'
  wckd start --dry-run --hours 1 --project hailuo-tests --preset comfyui-minimax-h3`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := a.open()
			if err != nil {
				return err
			}
			if err := requireCredentials(d.cfg); err != nil {
				return err
			}
			hoursVal := d.cfg.Defaults.Hours
			if cmd.Flags().Changed("hours") {
				hoursVal = hours
			}
			if err := config.ValidateHours(hoursVal); err != nil {
				return err
			}
			project := d.cfg.Defaults.ProjectID
			if cmd.Flags().Changed("project") {
				project = projectID
			}
			if err := config.ValidateProjectID(project); err != nil {
				return err
			}
			id := d.cfg.Defaults.PresetID
			if cmd.Flags().Changed("preset") {
				id = presetID
			}
			if err := config.ValidatePresetID(id); err != nil {
				return err
			}
			p, err := preset.LoadID(d.cfg.PresetsDir, id)
			if err != nil {
				return err
			}
			ports, err := presetPorts(p)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = cmdContext()
			}
			req := session.StartRequest{
				ProjectID: project,
				Preset:    p,
				Hours:     hoursVal,
				OfferID:   offerID,
				Image:     p.Image(d.cfg.Image),
				Ports:     ports,
				Storage: session.StorageCreds{
					Endpoint:  d.cfg.S3.Endpoint,
					Bucket:    d.cfg.S3.Bucket,
					AccessKey: d.cfg.S3.AccessKey,
					SecretKey: d.cfg.S3.SecretKey,
					Region:    d.cfg.S3.Region,
				},
			}
			if dryRun {
				offers, err := d.svc.Candidates(ctx, p, offerID)
				if err != nil {
					return err
				}
				chosen := offers[0]
				if asJSON {
					return writeJSON(a.stdout(), map[string]any{
						"dry_run":      true,
						"offer":        chosen,
						"estimate_usd": session.Estimate(chosen.USDPerHour, hoursVal),
						"image":        req.Image,
					})
				}
				fmt.Fprintln(a.stdout(), "dry_run: true")
				fmt.Fprintf(a.stdout(), "offer: %s\n", chosen.ID)
				fmt.Fprintf(a.stdout(), "gpu: %s %dGB %s $%.3f/hr\n", chosen.Name, chosen.VRAMGB, chosen.Cloud, chosen.USDPerHour)
				fmt.Fprintf(a.stdout(), "estimate_usd: %.4f\n", session.Estimate(chosen.USDPerHour, hoursVal))
				fmt.Fprintf(a.stdout(), "image: %s\n", req.Image)
				fmt.Fprintf(a.stdout(), "failover: %d\n", len(offers))
				return nil
			}
			sess, err := d.svc.Start(ctx, req)
			if err != nil {
				if sess.ID != "" {
					writeSession(a.stdout(), sess, time.Now())
				}
				return err
			}
			if asJSON {
				if err := writeJSON(a.stdout(), sess); err != nil {
					return err
				}
			} else {
				writeSession(a.stdout(), sess, time.Now())
			}
			if !noSweeper && os.Getenv("WCKD_NO_SWEEPER") != "1" {
				pid, serr := spawnSweeper(d.cfg.StateDir, a.ConfigPath)
				if serr != nil {
					fmt.Fprintf(a.stderr(), "warning: deadline sweeper did not start: %v\nrun: wckd sweeper\n", serr)
				} else if !asJSON {
					fmt.Fprintf(a.stdout(), "sweeper_pid: %d\n", pid)
				}
			}
			return nil
		},
	}
	cmd.Flags().Float64Var(&hours, "hours", 0, "session length in hours (max 24)")
	cmd.Flags().StringVar(&projectID, "project", "", "project id (S3 prefix projects/<id>/)")
	cmd.Flags().StringVar(&presetID, "preset", "", "preset id")
	cmd.Flags().StringVar(&offerID, "offer", "", "offer id from `wckd offers` (skip auto-pick and failover)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "rank and print the pick without creating a pod")
	cmd.Flags().BoolVar(&noSweeper, "no-sweeper", false, "do not spawn the background deadline sweeper")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the session as JSON")
	return cmd
}

func (a *App) statusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status [session]",
		Short: "Show phase, remaining time, and endpoints",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := a.open()
			if err != nil {
				return err
			}
			id, err := resolveSessionID(d, args)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = cmdContext()
			}
			sess, err := d.svc.Refresh(ctx, id)
			if err != nil {
				local, loadErr := d.svc.Sessions.Load(id)
				if loadErr != nil {
					return err
				}
				local.Warning = err.Error()
				sess = local
			}
			if asJSON {
				return writeJSON(a.stdout(), sess)
			}
			writeSession(a.stdout(), sess, time.Now())
			if !sess.DeadlineAt.IsZero() && sess.Phase.Active() && !time.Now().Before(sess.DeadlineAt) {
				fmt.Fprintln(a.stdout(), "deadline_elapsed: true")
			}
			if sess.Warning != "" && err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the session as JSON")
	return cmd
}

func (a *App) stopCmd() *cobra.Command {
	var (
		force  bool
		asJSON bool
	)
	cmd := &cobra.Command{
		Use:   "stop [session]",
		Short: "Drain the project to S3, then terminate the pod",
		Long: `Writes a drain request, waits for the sidecar's drain-ok marker, then deletes the pod.

If the marker never arrives the pod is left running and the command fails.
Retry the same command. --force deletes the pod anyway and records drain_ok=false.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := a.open()
			if err != nil {
				return err
			}
			id, err := resolveSessionID(d, args)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = cmdContext()
			}
			sess, err := d.svc.Stop(ctx, id, force)
			if asJSON && sess.ID != "" {
				_ = writeJSON(a.stdout(), sess)
			} else if sess.ID != "" {
				writeSession(a.stdout(), sess, time.Now())
			}
			if errors.Is(err, session.ErrDrainFailed) {
				return fmt.Errorf("drain did not finish; pod %s is still running\nretry: wckd stop %s\ndestroy without a confirmed drain: wckd stop %s --force", sess.InstanceID, sess.ID, sess.ID)
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "terminate even if drain did not finish")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the session as JSON")
	return cmd
}

func presetPorts(p preset.File) ([]vendor.NamedPort, error) {
	var ports []vendor.NamedPort
	for _, port := range p.Spec.Runtime.Ports {
		np, err := runpod.FormatPorts(port.Name, port.Container, port.Expose)
		if err != nil {
			return nil, err
		}
		ports = append(ports, np)
	}
	return ports, nil
}

func resolveSessionID(d *deps, args []string) (string, error) {
	if len(args) == 1 {
		if !session.ValidID(args[0]) {
			return "", fmt.Errorf("invalid session id %q", args[0])
		}
		return args[0], nil
	}
	return d.svc.Sessions.Current()
}
