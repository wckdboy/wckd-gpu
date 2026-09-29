package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// checkReport is the machine-readable config check. It never includes secrets.
type checkReport struct {
	OK         bool          `json:"ok"`
	Missing    []string      `json:"missing,omitempty"`
	RunPod     checkProbe    `json:"runpod"`
	S3         checkProbe    `json:"s3"`
	StateDir   string        `json:"state_dir,omitempty"`
	PresetsDir string        `json:"presets_dir,omitempty"`
	Defaults   checkDefaults `json:"defaults"`
}

type checkProbe struct {
	OK      bool   `json:"ok"`
	Skipped bool   `json:"skipped,omitempty"`
	Error   string `json:"error,omitempty"`
	Bucket  string `json:"bucket,omitempty"`
}

type checkDefaults struct {
	Hours     float64 `json:"hours"`
	ProjectID string  `json:"project_id,omitempty"`
	PresetID  string  `json:"preset_id,omitempty"`
}

func (a *App) configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect local configuration",
	}
	cmd.AddCommand(a.configCheckCmd())
	return cmd
}

func (a *App) configCheckCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate RunPod and S3 credentials without starting a GPU",
		Long: `Calls RunPod GET /v2/pods and S3 HeadBucket.

Neither call creates a pod. A missing credential fails before any network call.
--json prints the same result without secrets.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := a.open()
			if err != nil {
				return err
			}
			report := checkReport{
				Defaults: checkDefaults{
					Hours:     d.cfg.Defaults.Hours,
					ProjectID: d.cfg.Defaults.ProjectID,
					PresetID:  d.cfg.Defaults.PresetID,
				},
				StateDir:   d.cfg.StateDir,
				PresetsDir: d.cfg.PresetsDir,
			}
			if missing := d.cfg.MissingCredentials(); len(missing) > 0 {
				report.Missing = missing
				report.RunPod.Skipped = true
				report.S3.Skipped = true
				return a.finishCheck(asJSON, report)
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = cmdContext()
			}
			if err := d.pods.Ping(ctx); err != nil {
				report.RunPod.Error = err.Error()
			} else {
				report.RunPod.OK = true
			}
			if err := d.objects.Check(ctx); err != nil {
				report.S3.Error = err.Error()
			} else {
				report.S3.OK = true
				report.S3.Bucket = d.cfg.S3.Bucket
			}
			report.OK = report.RunPod.OK && report.S3.OK
			return a.finishCheck(asJSON, report)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the check as JSON (secrets are omitted)")
	return cmd
}

func (a *App) finishCheck(asJSON bool, report checkReport) error {
	if asJSON {
		if err := writeJSON(a.stdout(), report); err != nil {
			return err
		}
	} else {
		writeCheckText(a.stdout(), report)
	}
	if !report.OK {
		return fmt.Errorf("config check failed")
	}
	return nil
}
