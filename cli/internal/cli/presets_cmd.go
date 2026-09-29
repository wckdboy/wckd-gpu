package cli

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/wckdboy/wckd-gpu/cli/internal/preset"
)

type presetPortView struct {
	Name      string `json:"name"`
	Container int    `json:"container"`
	Expose    string `json:"expose"`
}

type presetView struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Version       int              `json:"version"`
	WorkloadClass string           `json:"workload_class"`
	MinVRAMGB     int              `json:"min_vram_gb"`
	MinRAMGB      int              `json:"min_ram_gb"`
	MinDiskGB     int              `json:"min_disk_gb"`
	GPUFamilies   []string         `json:"gpu_families"`
	Reliability   string           `json:"reliability"`
	PreferRegions []string         `json:"prefer_regions"`
	Image         string           `json:"image"`
	Ports         []presetPortView `json:"ports"`
	Notes         string           `json:"notes"`
}

func (a *App) presetsCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "presets",
		Short: "List presets from the configured presets directory",
		Long: `Reads presets/*.yaml from the configured directory (WCKD_PRESETS_DIR, the
config file, or a presets/ directory walked up from the working directory).

This command does not contact RunPod or S3.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := a.open()
			if err != nil {
				return err
			}
			files, err := preset.List(d.cfg.PresetsDir)
			if err != nil {
				return err
			}
			views := make([]presetView, 0, len(files))
			for _, f := range files {
				views = append(views, presetToView(f))
			}
			if asJSON {
				return writeJSON(a.stdout(), views)
			}
			writePresets(a.stdout(), views)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print presets as JSON")
	return cmd
}

func presetToView(f preset.File) presetView {
	families := f.Spec.Constraints.GPUFamilies
	if families == nil {
		families = []string{}
	}
	regions := f.Spec.Constraints.PreferRegions
	if regions == nil {
		regions = []string{}
	}
	ports := make([]presetPortView, 0, len(f.Spec.Runtime.Ports))
	for _, port := range f.Spec.Runtime.Ports {
		ports = append(ports, presetPortView{
			Name:      port.Name,
			Container: port.Container,
			Expose:    port.Expose,
		})
	}
	reliability := f.Spec.Constraints.Reliability
	if reliability == "" {
		reliability = "any"
	}
	return presetView{
		ID:            f.Metadata.ID,
		Name:          f.Metadata.Name,
		Version:       f.Metadata.Version,
		WorkloadClass: f.Spec.WorkloadClass,
		MinVRAMGB:     f.Spec.Constraints.MinVRAMGB,
		MinRAMGB:      f.Spec.Constraints.MinRAMGB,
		MinDiskGB:     f.Spec.Constraints.MinDiskGB,
		GPUFamilies:   families,
		Reliability:   reliability,
		PreferRegions: regions,
		Image:         f.Spec.Runtime.Image,
		Ports:         ports,
		Notes:         f.Spec.Notes,
	}
}

func writePresets(w io.Writer, views []presetView) {
	if len(views) == 0 {
		fmt.Fprintln(w, "no presets")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tCLASS\tVRAM\tIMAGE")
	for _, view := range views {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n", view.ID, view.Name, view.WorkloadClass, view.MinVRAMGB, view.Image)
	}
	_ = tw.Flush()
}
