package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wckdboy/wckd-gpu/cli/internal/config"
	"github.com/wckdboy/wckd-gpu/cli/internal/preset"
)

func (a *App) offersCmd() *cobra.Command {
	var (
		presetID string
		hours    float64
		asJSON   bool
		limit    int
	)
	cmd := &cobra.Command{
		Use:   "offers",
		Short: "List ranked RunPod offers for a preset",
		Long: `Lists RunPod community and secure pod SKUs that match the preset, then ranks them.

score = (perf_index / usd_per_hr) * reliability_factor * region_factor

This command does not create a pod.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := a.open()
			if err != nil {
				return err
			}
			if d.cfg.RunPod.APIKey == "" {
				return fmt.Errorf("missing RunPod API key (WCKD_RUNPOD_API_KEY or RUNPOD_API_KEY)")
			}
			id := d.cfg.Defaults.PresetID
			if cmd.Flags().Changed("preset") {
				id = presetID
			}
			if err := config.ValidatePresetID(id); err != nil {
				return err
			}
			h := d.cfg.Defaults.Hours
			if cmd.Flags().Changed("hours") {
				h = hours
			}
			if err := config.ValidateHours(h); err != nil {
				return err
			}
			p, err := preset.LoadID(d.cfg.PresetsDir, id)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = cmdContext()
			}
			offers, err := d.svc.Ranked(ctx, p)
			if err != nil {
				return err
			}
			if len(offers) == 0 {
				return fmt.Errorf("no RunPod offers matched the preset constraints")
			}
			if limit > 0 && limit < len(offers) {
				offers = offers[:limit]
			}
			if asJSON {
				return writeJSON(a.stdout(), offers)
			}
			writeOffers(a.stdout(), offers, h)
			return nil
		},
	}
	cmd.Flags().StringVar(&presetID, "preset", "", "preset id (default from config)")
	cmd.Flags().Float64Var(&hours, "hours", 0, "hours used only for the estimate column")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print offers as JSON")
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum rows to print")
	return cmd
}
