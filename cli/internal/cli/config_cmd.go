package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (a *App) configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect local configuration",
	}
	cmd.AddCommand(a.configCheckCmd())
	return cmd
}

func (a *App) configCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Validate RunPod and S3 credentials without starting a GPU",
		Long: `Calls RunPod GET /v2/pods and S3 HeadBucket.

Neither call creates a pod. A missing credential fails before any network call.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := a.open()
			if err != nil {
				return err
			}
			out := a.stdout()
			if missing := d.cfg.MissingCredentials(); len(missing) > 0 {
				fmt.Fprintln(out, "credentials: missing")
				for _, item := range missing {
					fmt.Fprintf(out, "- %s\n", item)
				}
				return fmt.Errorf("config check failed")
			}
			failed := false
			ctx := cmd.Context()
			if ctx == nil {
				ctx = cmdContext()
			}
			if err := d.pods.Ping(ctx); err != nil {
				fmt.Fprintf(out, "runpod: fail: %s\n", err.Error())
				failed = true
			} else {
				fmt.Fprintln(out, "runpod: ok")
			}
			if err := d.objects.Check(ctx); err != nil {
				fmt.Fprintf(out, "s3: fail: %s\n", err.Error())
				failed = true
			} else {
				fmt.Fprintf(out, "s3: ok bucket=%s\n", d.cfg.S3.Bucket)
			}
			fmt.Fprintf(out, "state_dir: %s\n", d.cfg.StateDir)
			fmt.Fprintf(out, "presets_dir: %s\n", d.cfg.PresetsDir)
			project := d.cfg.Defaults.ProjectID
			if project == "" {
				project = "(unset)"
			}
			fmt.Fprintf(out, "defaults: hours=%g project=%s preset=%s\n", d.cfg.Defaults.Hours, project, d.cfg.Defaults.PresetID)
			if failed {
				return fmt.Errorf("config check failed")
			}
			return nil
		},
	}
}
