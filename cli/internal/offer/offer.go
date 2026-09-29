// Package offer ranks GPU quotes with the architecture score:
//
//	score = (perf_index / usd_per_hr) * reliability_factor * region_factor
//
// perf_index values are versioned, labeled estimates for a workload class.
// They are not live benchmarks.
package offer

import (
	"fmt"
	"sort"
	"strings"
)

const (
	// PerfTableVersion labels the estimate table shipped with this CLI.
	PerfTableVersion = "2026-09-29"

	// ReliabilitySecure is the factor for RunPod Secure cloud.
	ReliabilitySecure = 1.0
	// ReliabilityCommunity discounts community hosts.
	ReliabilityCommunity = 0.85
	// RegionPreferred is used when the quote has a data center in prefer_regions,
	// and when the preset states no region preference.
	RegionPreferred = 1.0
	// RegionOther discounts quotes that miss prefer_regions.
	RegionOther = 0.90
)

// Constraints are the preset fields that filter quotes.
type Constraints struct {
	MinVRAMGB     int
	GPUFamilies   []string
	Reliability   string
	PreferRegions []string
}

// Query is the ranking request passed to a vendor adapter.
type Query struct {
	Constraints   Constraints
	WorkloadClass string
}

// DataCenter is one placement candidate.
type DataCenter struct {
	ID           string
	Name         string
	Availability string
}

// Quote is a vendor price before scoring.
type Quote struct {
	GPUID        string
	Name         string
	VRAMGB       int
	Cloud        string
	USDPerHour   float64
	DataCenters  []DataCenter
	Availability string
}

// Offer is a ranked, scored quote.
type Offer struct {
	ID                string   `json:"id"`
	Vendor            string   `json:"vendor"`
	SKU               string   `json:"sku"`
	Name              string   `json:"name"`
	Family            string   `json:"family,omitempty"`
	VRAMGB            int      `json:"vram_gb"`
	USDPerHour        float64  `json:"usd_per_hr"`
	Cloud             string   `json:"cloud"`
	Region            string   `json:"region,omitempty"`
	DataCenterIDs     []string `json:"data_center_ids,omitempty"`
	Availability      string   `json:"availability,omitempty"`
	PerfIndex         float64  `json:"perf_index"`
	PerfTable         string   `json:"perf_table"`
	ReliabilityFactor float64  `json:"reliability_factor"`
	RegionFactor      float64  `json:"region_factor"`
	Score             float64  `json:"score"`
}

// Score applies the published formula. Non-positive price scores 0.
func Score(perfIndex, usdPerHour, reliability, region float64) float64 {
	if usdPerHour <= 0 || perfIndex <= 0 || reliability <= 0 || region <= 0 {
		return 0
	}
	return (perfIndex / usdPerHour) * reliability * region
}

// Rank filters quotes and sorts by score descending, then cheaper rate, then id.
func Rank(quotes []Quote, c Constraints, workloadClass string) []Offer {
	var out []Offer
	for _, q := range quotes {
		o, ok := scoreQuote(q, c, workloadClass)
		if ok {
			out = append(out, o)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].USDPerHour != out[j].USDPerHour {
			return out[i].USDPerHour < out[j].USDPerHour
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// FormatID builds the stable id used by --offer.
func FormatID(cloud, gpuID string) string {
	return "runpod:" + strings.ToLower(cloud) + ":" + gpuID
}

// ParseID splits an offer id into cloud and GPU id.
func ParseID(id string) (cloud, gpuID string, err error) {
	const prefix = "runpod:"
	if !strings.HasPrefix(id, prefix) {
		return "", "", fmt.Errorf("offer id %q must start with %s", id, prefix)
	}
	rest := strings.TrimPrefix(id, prefix)
	cloud, gpuID, ok := strings.Cut(rest, ":")
	if !ok || cloud == "" || gpuID == "" {
		return "", "", fmt.Errorf("offer id %q must look like runpod:<cloud>:<gpu id>", id)
	}
	switch cloud {
	case "community", "secure":
	default:
		return "", "", fmt.Errorf("offer id cloud %q is not community or secure", cloud)
	}
	return cloud, gpuID, nil
}

func scoreQuote(q Quote, c Constraints, workloadClass string) (Offer, bool) {
	if q.USDPerHour <= 0 || q.VRAMGB <= 0 {
		return Offer{}, false
	}
	if c.MinVRAMGB > 0 && q.VRAMGB < c.MinVRAMGB {
		return Offer{}, false
	}
	if strings.EqualFold(q.Availability, "NONE") {
		return Offer{}, false
	}
	cloud := strings.ToLower(q.Cloud)
	switch cloud {
	case "community", "secure":
	default:
		return Offer{}, false
	}
	switch strings.ToLower(strings.TrimSpace(c.Reliability)) {
	case "", "any":
	case "community":
		if cloud != "community" {
			return Offer{}, false
		}
	case "secure":
		if cloud != "secure" {
			return Offer{}, false
		}
	default:
		return Offer{}, false
	}

	family, matched := matchFamily(q.GPUID+" "+q.Name, c.GPUFamilies)
	if len(c.GPUFamilies) > 0 && !matched {
		return Offer{}, false
	}

	dcs := usableDataCenters(q.DataCenters)
	region, regionFactor, dcIDs := place(dcs, c.PreferRegions)
	perf, table := PerfIndex(workloadClass, family, q.VRAMGB)
	rel := ReliabilityCommunity
	if cloud == "secure" {
		rel = ReliabilitySecure
	}
	return Offer{
		ID:                FormatID(cloud, q.GPUID),
		Vendor:            "runpod",
		SKU:               q.GPUID,
		Name:              q.Name,
		Family:            family,
		VRAMGB:            q.VRAMGB,
		USDPerHour:        q.USDPerHour,
		Cloud:             cloud,
		Region:            region,
		DataCenterIDs:     dcIDs,
		Availability:      q.Availability,
		PerfIndex:         perf,
		PerfTable:         table,
		ReliabilityFactor: rel,
		RegionFactor:      regionFactor,
		Score:             Score(perf, q.USDPerHour, rel, regionFactor),
	}, true
}

func place(dcs []DataCenter, prefer []string) (region string, factor float64, ids []string) {
	var preferred []DataCenter
	var others []DataCenter
	for _, dc := range dcs {
		if regionMatch(dc, prefer) {
			preferred = append(preferred, dc)
		} else {
			others = append(others, dc)
		}
	}
	if len(prefer) == 0 {
		for _, dc := range dcs {
			ids = append(ids, dc.ID)
		}
		region = firstRegion(dcs)
		return region, RegionPreferred, ids
	}
	if len(preferred) > 0 {
		for _, dc := range preferred {
			ids = append(ids, dc.ID)
		}
		return preferred[0].ID, RegionPreferred, ids
	}
	region = firstRegion(others)
	return region, RegionOther, nil
}

func firstRegion(dcs []DataCenter) string {
	if len(dcs) == 0 {
		return ""
	}
	return dcs[0].ID
}

func usableDataCenters(dcs []DataCenter) []DataCenter {
	var out []DataCenter
	for _, dc := range dcs {
		if strings.EqualFold(dc.Availability, "NONE") || dc.ID == "" {
			continue
		}
		out = append(out, dc)
	}
	return out
}

func regionMatch(dc DataCenter, prefer []string) bool {
	if len(prefer) == 0 {
		return true
	}
	hay := strings.ToUpper(dc.ID + " " + dc.Name)
	for _, p := range prefer {
		p = strings.ToUpper(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if strings.Contains(hay, p) {
			return true
		}
	}
	return false
}

func matchFamily(gpu string, allow []string) (string, bool) {
	candidates := allow
	if len(candidates) == 0 {
		candidates = knownFamilies
	}
	norm := normalize(gpu)
	best := ""
	for _, f := range candidates {
		fn := normalize(f)
		if fn == "" || !strings.Contains(norm, fn) {
			continue
		}
		if len(fn) > len(normalize(best)) {
			best = f
		}
	}
	if best == "" {
		return "", false
	}
	return best, true
}

func normalize(s string) string {
	s = strings.ToUpper(s)
	var b strings.Builder
	for _, c := range s {
		if (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// PerfIndex returns the labeled estimate for a family and workload class.
// Unknown families fall back to VRAM gigabytes so larger cards still rank,
// and the table name records that fallback.
func PerfIndex(workloadClass, family string, vramGB int) (float64, string) {
	tableName := PerfTableVersion + "/" + workloadOrGeneric(workloadClass)
	table := perfTable(workloadClass)
	if family != "" {
		if v, ok := table[normalize(family)]; ok {
			return v, tableName
		}
	}
	if vramGB <= 0 {
		return 0, tableName + "-none"
	}
	return float64(vramGB), tableName + "-vram-fallback"
}

func workloadOrGeneric(class string) string {
	switch class {
	case "video_dit", "llm", "generic":
		return class
	default:
		return "generic"
	}
}

func perfTable(class string) map[string]float64 {
	switch class {
	case "video_dit":
		return perfVideoDiT
	case "llm":
		return perfLLM
	case "generic":
		return perfGeneric
	default:
		return perfGeneric
	}
}

// knownFamilies is longest-token first only as a convenience; matchFamily
// already picks the longest hit.
var knownFamilies = []string{
	"PRO6000", "H200", "H100", "A100", "L40S", "A6000", "5090", "4090", "3090", "A40", "L4",
}

// Estimates are relative within a class, not tokens/sec or frames/sec.
var perfVideoDiT = map[string]float64{
	"H200":    130,
	"H100":    100,
	"PRO6000": 90,
	"5090":    70,
	"A100":    55,
	"4090":    45,
	"L40S":    40,
	"3090":    28,
}

var perfLLM = map[string]float64{
	"H200":    140,
	"H100":    100,
	"PRO6000": 70,
	"5090":    55,
	"A100":    60,
	"L40S":    40,
	"4090":    35,
	"3090":    22,
}

var perfGeneric = map[string]float64{
	"H200":    120,
	"H100":    100,
	"PRO6000": 80,
	"5090":    64,
	"A100":    70,
	"4090":    48,
	"L40S":    42,
	"3090":    30,
}
