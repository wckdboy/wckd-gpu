package cli

import (
	"fmt"
	"io"
)

func writeCheckText(w io.Writer, report checkReport) {
	if len(report.Missing) > 0 {
		fmt.Fprintln(w, "credentials: missing")
		for _, item := range report.Missing {
			fmt.Fprintf(w, "- %s\n", item)
		}
		return
	}
	writeProbe(w, "runpod", report.RunPod)
	if report.S3.OK {
		fmt.Fprintf(w, "s3: ok bucket=%s\n", report.S3.Bucket)
	} else {
		writeProbe(w, "s3", report.S3)
	}
	fmt.Fprintf(w, "state_dir: %s\n", report.StateDir)
	fmt.Fprintf(w, "presets_dir: %s\n", report.PresetsDir)
	project := report.Defaults.ProjectID
	if project == "" {
		project = "(unset)"
	}
	fmt.Fprintf(w, "defaults: hours=%g project=%s preset=%s\n", report.Defaults.Hours, project, report.Defaults.PresetID)
}

func writeProbe(w io.Writer, name string, probe checkProbe) {
	if probe.OK {
		fmt.Fprintf(w, "%s: ok\n", name)
		return
	}
	fmt.Fprintf(w, "%s: fail: %s\n", name, probe.Error)
}
