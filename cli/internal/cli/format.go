package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/wckdboy/wckd-gpu/cli/internal/offer"
	"github.com/wckdboy/wckd-gpu/cli/internal/session"
)

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	return nil
}

func writeOffers(w io.Writer, offers []offer.Offer, hours float64) {
	fmt.Fprintln(w, "score = (perf_index / usd_per_hr) * reliability_factor * region_factor")
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "RANK\tSCORE\tUSD/HR\tEST\tVRAM\tCLOUD\tAVAIL\tREGION\tGPU\tOFFER")
	for i, o := range offers {
		fmt.Fprintf(tw, "%d\t%.2f\t%.3f\t%.3f\t%d\t%s\t%s\t%s\t%s\t%s\n",
			i+1, o.Score, o.USDPerHour, session.Estimate(o.USDPerHour, hours), o.VRAMGB,
			o.Cloud, o.Availability, o.Region, o.Name, o.ID)
	}
	_ = tw.Flush()
}

func writeSession(w io.Writer, sess session.Session, now time.Time) {
	fmt.Fprintf(w, "session_id: %s\n", sess.ID)
	fmt.Fprintf(w, "phase: %s\n", sess.Phase)
	fmt.Fprintf(w, "project: %s\n", sess.ProjectID)
	fmt.Fprintf(w, "preset: %s\n", sess.PresetID)
	if sess.Offer.ID != "" {
		fmt.Fprintf(w, "offer: %s\n", sess.Offer.ID)
		fmt.Fprintf(w, "gpu: %s %dGB %s $%.3f/hr\n", sess.Offer.Name, sess.Offer.VRAMGB, sess.Offer.Cloud, sess.Offer.USDPerHour)
	}
	fmt.Fprintf(w, "estimate_usd: %.4f\n", sess.CostEstimateUSD)
	if sess.Phase == session.PhaseTerminated {
		fmt.Fprintf(w, "actual_usd: %.4f\n", sess.CostActualUSD)
		fmt.Fprintf(w, "drain_ok: %t\n", sess.DrainOK)
		fmt.Fprintf(w, "forced: %t\n", sess.Forced)
	}
	if !sess.DeadlineAt.IsZero() {
		fmt.Fprintf(w, "deadline: %s\n", sess.DeadlineAt.UTC().Format(time.RFC3339))
		fmt.Fprintf(w, "remaining: %s\n", formatRemaining(sess.DeadlineAt.Sub(now)))
	}
	if sess.InstanceID != "" {
		fmt.Fprintf(w, "pod: %s\n", sess.InstanceID)
	}
	if sess.PodStatus != "" {
		fmt.Fprintf(w, "pod_status: %s\n", sess.PodStatus)
	}
	if sess.DataCenter != "" {
		fmt.Fprintf(w, "data_center: %s\n", sess.DataCenter)
	}
	for _, ep := range sess.Endpoints {
		fmt.Fprintf(w, "endpoint_%s: %s\n", ep.Name, ep.URL)
	}
	if sess.Error != "" {
		fmt.Fprintf(w, "error: %s\n", sess.Error)
	}
	if sess.Warning != "" {
		fmt.Fprintf(w, "warning: %s\n", sess.Warning)
	}
}

func formatRemaining(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Truncate(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh%dm", h, m)
	case m > 0:
		return fmt.Sprintf("%dm%ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}
