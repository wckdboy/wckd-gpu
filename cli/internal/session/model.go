package session

import (
	"time"

	"github.com/wckdboy/wckd-gpu/cli/internal/offer"
	"github.com/wckdboy/wckd-gpu/cli/internal/vendor"
)

// Session is the local record of one GPU rental.
type Session struct {
	ID              string            `json:"id"`
	ProjectID       string            `json:"project_id"`
	PresetID        string            `json:"preset_id"`
	Hours           float64           `json:"hours"`
	CreatedAt       time.Time         `json:"created_at"`
	DeadlineAt      time.Time         `json:"deadline_at"`
	Phase           Phase             `json:"phase"`
	Offer           offer.Offer       `json:"offer"`
	InstanceID      string            `json:"instance_id,omitempty"`
	PodStatus       string            `json:"pod_status,omitempty"`
	DataCenter      string            `json:"data_center,omitempty"`
	Endpoints       []vendor.Endpoint `json:"endpoints,omitempty"`
	CostEstimateUSD float64           `json:"cost_estimate_usd"`
	CostActualUSD   float64           `json:"cost_actual_usd,omitempty"`
	DrainOK         bool              `json:"drain_ok"`
	Forced          bool              `json:"forced"`
	Error           string            `json:"error,omitempty"`
	Warning         string            `json:"warning,omitempty"`
	Events          []Event           `json:"events,omitempty"`
	EndedAt         *time.Time        `json:"ended_at,omitempty"`
}

// Event is one lifecycle note, also copied to sessions/{id}/events.jsonl.
type Event struct {
	At      time.Time `json:"at"`
	Phase   Phase     `json:"phase"`
	Message string    `json:"message"`
}

func (s *Session) addEvent(at time.Time, message string) {
	s.Events = append(s.Events, Event{At: at, Phase: s.Phase, Message: message})
}

// Receipt is the cost record written at terminate time.
// Amounts use the catalog hourly rate, not the vendor invoice.
type Receipt struct {
	SessionID       string    `json:"session_id"`
	ProjectID       string    `json:"project_id"`
	PresetID        string    `json:"preset_id"`
	Vendor          string    `json:"vendor"`
	SKU             string    `json:"sku"`
	Cloud           string    `json:"cloud"`
	USDPerHour      float64   `json:"usd_per_hr"`
	HoursRequested  float64   `json:"hours_requested"`
	CostEstimateUSD float64   `json:"cost_estimate_usd"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	CostActualUSD   float64   `json:"cost_actual_usd"`
	CostBasis       string    `json:"cost_basis"`
	DrainOK         bool      `json:"drain_ok"`
	Forced          bool      `json:"forced"`
	InstanceID      string    `json:"instance_id,omitempty"`
}

const costBasis = "catalog_usd_per_hr_times_elapsed"

// Estimate is hours times the catalog rate.
func Estimate(usdPerHour, hours float64) float64 {
	return usdPerHour * hours
}

// Actual is the catalog rate times elapsed wall time.
func Actual(usdPerHour float64, start, end time.Time) float64 {
	if end.Before(start) {
		return 0
	}
	return usdPerHour * end.Sub(start).Hours()
}

func receiptFrom(sess Session) Receipt {
	ended := sess.CreatedAt
	if sess.EndedAt != nil {
		ended = *sess.EndedAt
	}
	return Receipt{
		SessionID:       sess.ID,
		ProjectID:       sess.ProjectID,
		PresetID:        sess.PresetID,
		Vendor:          sess.Offer.Vendor,
		SKU:             sess.Offer.SKU,
		Cloud:           sess.Offer.Cloud,
		USDPerHour:      sess.Offer.USDPerHour,
		HoursRequested:  sess.Hours,
		CostEstimateUSD: sess.CostEstimateUSD,
		StartedAt:       sess.CreatedAt,
		EndedAt:         ended,
		CostActualUSD:   sess.CostActualUSD,
		CostBasis:       costBasis,
		DrainOK:         sess.DrainOK,
		Forced:          sess.Forced,
		InstanceID:      sess.InstanceID,
	}
}
