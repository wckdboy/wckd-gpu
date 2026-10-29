package offer

import (
	"math"
	"strings"
	"testing"
)

func TestScoreFormula(t *testing.T) {
	got := Score(70, 0.5, ReliabilityCommunity, RegionPreferred)
	want := (70 / 0.5) * ReliabilityCommunity * RegionPreferred
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %v want %v", got, want)
	}
	if Score(70, 0, 1, 1) != 0 {
		t.Fatal("zero price should score 0")
	}
}

func TestRankOrdersByScore(t *testing.T) {
	quotes := []Quote{
		quote("NVIDIA GeForce RTX 4090", "RTX 4090", 24, "secure", 0.69, "EU-RO-1", "HIGH"),
		quote("NVIDIA GeForce RTX 4090", "RTX 4090", 24, "community", 0.34, "EU-RO-1", "HIGH"),
		quote("NVIDIA GeForce RTX 5090", "RTX 5090", 32, "community", 0.80, "EU-RO-1", "MEDIUM"),
	}
	c := Constraints{
		MinVRAMGB:     24,
		GPUFamilies:   []string{"5090", "4090", "L40S", "H100", "PRO6000"},
		Reliability:   "any",
		PreferRegions: []string{"EU"},
	}
	ranked := Rank(quotes, c, "video_dit")
	if len(ranked) != 3 {
		t.Fatalf("len %d", len(ranked))
	}
	if ranked[0].Cloud != "community" || ranked[0].Family != "4090" {
		t.Fatalf("top: %+v", ranked[0])
	}
	wantTop := Score(45, 0.34, ReliabilityCommunity, RegionPreferred)
	if math.Abs(ranked[0].Score-wantTop) > 1e-9 {
		t.Fatalf("score %v want %v", ranked[0].Score, wantTop)
	}
	if ranked[1].Family != "5090" {
		t.Fatalf("second: %+v", ranked[1])
	}
	if ranked[2].Cloud != "secure" {
		t.Fatalf("third: %+v", ranked[2])
	}
	if ranked[0].ID != "runpod:community:NVIDIA GeForce RTX 4090" {
		t.Fatalf("id %s", ranked[0].ID)
	}
	if ranked[0].PerfTable != PerfTableVersion+"/video_dit" {
		t.Fatalf("table %s", ranked[0].PerfTable)
	}
}

func TestRankFilters(t *testing.T) {
	quotes := []Quote{
		quote("NVIDIA GeForce RTX 3090", "RTX 3090", 24, "community", 0.20, "EU-RO-1", "HIGH"),
		quote("NVIDIA GeForce RTX 4090", "RTX 4090", 16, "community", 0.20, "EU-RO-1", "HIGH"),
		quote("NVIDIA GeForce RTX 4090", "RTX 4090", 24, "community", 0.30, "EU-RO-1", "NONE"),
		quote("NVIDIA GeForce RTX 4090", "RTX 4090", 24, "community", 0, "EU-RO-1", "HIGH"),
		quote("NVIDIA GeForce RTX 4090", "RTX 4090", 24, "community", 0.30, "US-KS-2", "HIGH"),
		quote("NVIDIA GeForce RTX 4090", "RTX 4090", 24, "secure", 0.50, "EU-RO-1", "HIGH"),
	}
	// The 16GB quote is mislabeled as 4090 but fails the VRAM floor.
	quotes[1].VRAMGB = 16

	c := Constraints{
		MinVRAMGB:     24,
		GPUFamilies:   []string{"4090"},
		Reliability:   "secure",
		PreferRegions: []string{"EU"},
	}
	ranked := Rank(quotes, c, "video_dit")
	if len(ranked) != 1 {
		t.Fatalf("got %#v", ranked)
	}
	if ranked[0].Cloud != "secure" || ranked[0].Region != "EU-RO-1" {
		t.Fatalf("%+v", ranked[0])
	}
}

func TestRegionFactor(t *testing.T) {
	eu := quote("NVIDIA GeForce RTX 4090", "RTX 4090", 24, "community", 0.40, "EU-RO-1", "HIGH")
	us := quote("NVIDIA GeForce RTX 4090", "RTX 4090", 24, "community", 0.40, "US-KS-2", "HIGH")
	c := Constraints{MinVRAMGB: 24, GPUFamilies: []string{"4090"}, PreferRegions: []string{"EU"}}
	ranked := Rank([]Quote{us, eu}, c, "llm")
	if len(ranked) != 2 {
		t.Fatal(len(ranked))
	}
	if ranked[0].Region != "EU-RO-1" || ranked[0].RegionFactor != RegionPreferred {
		t.Fatalf("eu: %+v", ranked[0])
	}
	if ranked[1].RegionFactor != RegionOther {
		t.Fatalf("us factor %v", ranked[1].RegionFactor)
	}
	if ranked[1].DataCenterIDs != nil {
		t.Fatalf("non-preferred should not pin data centers: %v", ranked[1].DataCenterIDs)
	}
	if len(ranked[0].DataCenterIDs) != 1 || ranked[0].DataCenterIDs[0] != "EU-RO-1" {
		t.Fatalf("dcs %v", ranked[0].DataCenterIDs)
	}
}

func TestPRO6000Family(t *testing.T) {
	q := quote("NVIDIA RTX PRO 6000 Blackwell", "RTX PRO 6000", 96, "secure", 1.2, "EU-NL-1", "LOW")
	c := Constraints{GPUFamilies: []string{"PRO6000"}, PreferRegions: []string{"EU"}}
	ranked := Rank([]Quote{q}, c, "video_dit")
	if len(ranked) != 1 || ranked[0].Family != "PRO6000" {
		t.Fatalf("%+v", ranked)
	}
	if ranked[0].PerfIndex != 90 {
		t.Fatalf("perf %v", ranked[0].PerfIndex)
	}
}

func TestParseID(t *testing.T) {
	cloud, gpu, err := ParseID("runpod:community:NVIDIA GeForce RTX 4090")
	if err != nil {
		t.Fatal(err)
	}
	if cloud != "community" || gpu != "NVIDIA GeForce RTX 4090" {
		t.Fatalf("%s %s", cloud, gpu)
	}
	if _, _, err := ParseID("vast:4090"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRequireRegionsIsHardFilter(t *testing.T) {
	eu := quote("NVIDIA GeForce RTX 4090", "RTX 4090", 24, "community", 0.50, "EU-RO-1", "HIGH")
	us := quote("NVIDIA GeForce RTX 4090", "RTX 4090 US", 24, "community", 0.10, "US-KS-2", "HIGH")
	mixed := quote("NVIDIA GeForce RTX 5090", "RTX 5090", 32, "community", 0.80, "EU-NL-1", "HIGH")
	mixed.DataCenters = []DataCenter{
		{ID: "US-KS-2", Name: "US Kansas", Availability: "HIGH"},
		{ID: "EU-NL-1", Name: "EU Netherlands", Availability: "HIGH"},
	}
	none := quote("NVIDIA GeForce RTX 4090", "RTX 4090 nowhere", 24, "community", 0.20, "", "HIGH")
	none.DataCenters = nil
	c := Constraints{
		MinVRAMGB:      24,
		GPUFamilies:    []string{"4090", "5090"},
		PreferRegions:  []string{"EU"},
		RequireRegions: []string{"EU"},
	}
	ranked := Rank([]Quote{us, eu, mixed, none}, c, "llm")
	if len(ranked) != 2 {
		t.Fatalf("len %d %#v", len(ranked), ranked)
	}
	for _, o := range ranked {
		if strings.Contains(o.Region, "US") {
			t.Fatalf("us region survived: %+v", o)
		}
		for _, id := range o.DataCenterIDs {
			if strings.Contains(id, "US") {
				t.Fatalf("us data center pinned: %+v", o)
			}
		}
		if len(o.DataCenterIDs) == 0 {
			t.Fatalf("hard filter did not pin a data center: %+v", o)
		}
	}
	if ranked[0].Region != "EU-RO-1" || ranked[0].USDPerHour != 0.50 {
		t.Fatalf("cheaper in-region quote should win: %+v", ranked[0])
	}
	var sawMixed bool
	for _, o := range ranked {
		if o.SKU == "NVIDIA GeForce RTX 5090" {
			sawMixed = true
			if len(o.DataCenterIDs) != 1 || o.DataCenterIDs[0] != "EU-NL-1" {
				t.Fatalf("mixed quote ids %v", o.DataCenterIDs)
			}
		}
	}
	if !sawMixed {
		t.Fatal("mixed EU/US quote was dropped")
	}
}

func TestRequireSecureDropsCommunity(t *testing.T) {
	community := quote("NVIDIA GeForce RTX 4090", "RTX 4090", 24, "community", 0.20, "EU-RO-1", "HIGH")
	secure := quote("NVIDIA GeForce RTX 4090", "RTX 4090", 24, "secure", 0.90, "EU-RO-1", "HIGH")
	c := Constraints{RequireRegions: []string{"EU"}, RequireSecure: true, GPUFamilies: []string{"4090"}}
	ranked := Rank([]Quote{community, secure}, c, "video_dit")
	if len(ranked) != 1 || ranked[0].Cloud != "secure" {
		t.Fatalf("%#v", ranked)
	}
}

func TestRegionTokenBoundary(t *testing.T) {
	if !MatchesRegion("EU-RO-1", "EU Romania", []string{"EU"}) {
		t.Fatal("EU-RO-1 should match EU")
	}
	if MatchesRegion("EUR-IS-1", "Europe Iceland", []string{"EU"}) {
		t.Fatal("EUR-IS-1 should not match the EU token")
	}
	if MatchesRegion("US-KS-2", "US Kansas", []string{"EU"}) {
		t.Fatal("US should not match EU")
	}
	c := Constraints{RequireRegions: []string{"EU"}, GPUFamilies: []string{"4090"}, MinVRAMGB: 24}
	eur := quote("NVIDIA GeForce RTX 4090", "RTX 4090", 24, "community", 0.20, "EUR-IS-1", "HIGH")
	if ranked := Rank([]Quote{eur}, c, "llm"); len(ranked) != 0 {
		t.Fatalf("EUR-IS-1 survived the hard filter: %#v", ranked)
	}
}

func TestRegionPolicyTokens(t *testing.T) {
	def, err := ResolveRequire(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if def.Secure || len(def.Regions) != 1 || def.Regions[0] != "EU" {
		t.Fatalf("default %+v", def)
	}
	secure, err := ParseRequire([]string{"EU-secure"})
	if err != nil {
		t.Fatal(err)
	}
	if !secure.Secure || len(secure.Regions) != 1 || secure.Regions[0] != "EU" {
		t.Fatalf("eu-secure %+v", secure)
	}
	if secure.Label() != "EU-secure" {
		t.Fatalf("label %s", secure.Label())
	}
	any := []string{"any"}
	cleared, err := ResolveRequire([]string{"EU"}, &any)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Secure || len(cleared.Regions) != 0 {
		t.Fatalf("opt-out %+v", cleared)
	}
	empty := []string{}
	also, err := ResolveRequire([]string{"EU"}, &empty)
	if err != nil {
		t.Fatal(err)
	}
	if len(also.Regions) != 0 || also.Secure {
		t.Fatalf("empty override %+v", also)
	}
	if _, err := ParseRequire([]string{"any", "EU"}); err == nil {
		t.Fatal("expected mixed any to fail")
	}
	parts := SplitList("EU-secure")
	if len(parts) != 1 || parts[0] != "EU-secure" {
		t.Fatalf("split %#v", parts)
	}
	if len(SplitList("")) != 0 {
		t.Fatal("empty split")
	}
}

func TestNoPreferenceRegionFactorNeutral(t *testing.T) {
	q := quote("NVIDIA H100 PCIe", "H100", 80, "secure", 2.0, "US-KS-2", "HIGH")
	ranked := Rank([]Quote{q}, Constraints{Reliability: "any"}, "generic")
	if len(ranked) != 1 {
		t.Fatal(len(ranked))
	}
	if ranked[0].RegionFactor != RegionPreferred || ranked[0].Family != "H100" {
		t.Fatalf("%+v", ranked[0])
	}
}

func quote(id, name string, vram int, cloud string, price float64, dc, avail string) Quote {
	return Quote{
		GPUID:        id,
		Name:         name,
		VRAMGB:       vram,
		Cloud:        cloud,
		USDPerHour:   price,
		Availability: avail,
		DataCenters:  []DataCenter{{ID: dc, Name: dc, Availability: avail}},
	}
}
