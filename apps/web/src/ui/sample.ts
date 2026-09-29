import { estimateUsd } from "../control/parse";
import type { Offer, Session } from "../control/types";

const BASE: Omit<Offer, "estimate_usd">[] = [
  {
    id: "runpod:community:NVIDIA GeForce RTX 4090",
    vendor: "runpod",
    sku: "NVIDIA GeForce RTX 4090",
    name: "RTX 4090",
    family: "4090",
    vram_gb: 24,
    usd_per_hr: 0.34,
    cloud: "community",
    region: "EU-RO-1",
    availability: "HIGH",
    perf_index: 45,
    perf_table: "2026-09-29/video_dit",
    reliability_factor: 0.85,
    region_factor: 1,
    score: 112.5,
  },
  {
    id: "runpod:secure:NVIDIA GeForce RTX 4090",
    vendor: "runpod",
    sku: "NVIDIA GeForce RTX 4090",
    name: "RTX 4090",
    family: "4090",
    vram_gb: 24,
    usd_per_hr: 0.69,
    cloud: "secure",
    region: "EU-RO-1",
    availability: "HIGH",
    perf_index: 45,
    perf_table: "2026-09-29/video_dit",
    reliability_factor: 0.95,
    region_factor: 1,
    score: 62,
  },
  {
    id: "runpod:community:NVIDIA GeForce RTX 5090",
    vendor: "runpod",
    sku: "NVIDIA GeForce RTX 5090",
    name: "RTX 5090",
    family: "5090",
    vram_gb: 32,
    usd_per_hr: 0.69,
    cloud: "community",
    region: "US-TX-3",
    availability: "MEDIUM",
    perf_index: 70,
    perf_table: "2026-09-29/video_dit",
    reliability_factor: 0.8,
    region_factor: 0.9,
    score: 73,
  },
];

export function sampleOffers(hours: number): Offer[] {
  return BASE.map((offer) => ({
    ...offer,
    estimate_usd: estimateUsd(offer.usd_per_hr, hours),
  }));
}

export function sampleSession(hours: number): Session {
  const offer = sampleOffers(hours)[0];
  if (!offer) {
    throw new Error("sample catalog is empty");
  }
  const now = Date.now();
  return {
    id: "sess_samplelayout01",
    project_id: "hailuo-tests",
    preset_id: "comfyui-minimax-h3",
    hours,
    created_at: new Date(now - 8 * 60 * 1000).toISOString(),
    deadline_at: new Date(now + 12 * 60 * 1000).toISOString(),
    phase: "ready",
    offer,
    instance_id: "pod_sample",
    pod_status: "RUNNING",
    data_center: "EU-RO-1",
    endpoints: [{ name: "ui", url: "https://pod-sample-8188.proxy.runpod.net" }],
    cost_estimate_usd: offer.estimate_usd,
    drain_ok: false,
    forced: false,
  };
}
