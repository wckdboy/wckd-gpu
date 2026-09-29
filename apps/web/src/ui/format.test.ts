import { describe, expect, it } from "vitest";

import { catalogAccrual, describeCountdown } from "./format";
import { startGate } from "./startGate";

describe("describeCountdown", () => {
  const now = Date.parse("2026-09-29T12:00:00Z");

  it("flashes amber inside the last 15 minutes", () => {
    const state = describeCountdown("2026-09-29T12:14:00Z", now);
    expect(state?.warn).toBe(true);
    expect(state?.elapsed).toBe(false);
    expect(state?.label).toBe("14m 00s");
  });

  it("stays quiet before T−15 and after the deadline", () => {
    expect(describeCountdown("2026-09-29T12:16:00Z", now)?.warn).toBe(false);
    const late = describeCountdown("2026-09-29T11:59:00Z", now);
    expect(late?.elapsed).toBe(true);
    expect(late?.warn).toBe(false);
  });
});

describe("catalogAccrual", () => {
  it("uses elapsed hours times the catalog rate", () => {
    const start = "2026-09-29T12:00:00Z";
    const now = Date.parse(start) + 30 * 60 * 1000;
    expect(catalogAccrual(0.34, start, now)).toBeCloseTo(0.17, 8);
    expect(catalogAccrual(0.34, "not-a-date", now)).toBe(0);
  });
});

describe("startGate", () => {
  const offer = {
    id: "runpod:community:NVIDIA GeForce RTX 4090",
    vendor: "runpod",
    sku: "NVIDIA GeForce RTX 4090",
    name: "RTX 4090",
    vram_gb: 24,
    usd_per_hr: 0.34,
    cloud: "community",
    perf_index: 45,
    perf_table: "2026-09-29",
    reliability_factor: 1,
    region_factor: 1,
    score: 1,
    estimate_usd: 1.36,
  };

  it("blocks the PWA and a missing offer", () => {
    expect(
      startGate({
        mode: "http",
        busy: false,
        offerSource: "live",
        offer,
        projectId: "hailuo-tests",
        presetId: "comfyui-minimax-h3",
        hours: 4,
      }).reason,
    ).toBe("Desktop required for Start.");
    expect(
      startGate({
        mode: "cli",
        busy: false,
        offerSource: "none",
        offer: null,
        projectId: "hailuo-tests",
        presetId: "comfyui-minimax-h3",
        hours: 4,
      }).ok,
    ).toBe(false);
  });

  it("allows a locked live offer on the desktop bridge", () => {
    expect(
      startGate({
        mode: "cli",
        busy: false,
        offerSource: "live",
        offer,
        projectId: "hailuo-tests",
        presetId: "comfyui-minimax-h3",
        hours: 4,
      }).ok,
    ).toBe(true);
  });
});
