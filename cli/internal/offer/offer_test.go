package offer

import (
	"math"
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
