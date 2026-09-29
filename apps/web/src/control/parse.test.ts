import { describe, expect, it } from "vitest";

import {
  estimateUsd,
  isSlug,
  parseConfigCheck,
  parseOffers,
  parseProject,
  parseProjects,
  parseSession,
  parseStartResult,
  presetFromYaml,
} from "./parse";
import { isDrainFailure, phaseActive } from "./types";

const offerJson = `[
  {
    "id": "runpod:community:NVIDIA GeForce RTX 4090",
    "vendor": "runpod",
    "sku": "NVIDIA GeForce RTX 4090",
    "name": "RTX 4090",
    "family": "4090",
    "vram_gb": 24,
    "usd_per_hr": 0.34,
    "cloud": "community",
    "region": "EU-RO-1",
    "data_center_ids": ["EU-RO-1"],
    "availability": "HIGH",
    "perf_index": 45,
    "perf_table": "2026-09-29/video_dit",
    "reliability_factor": 0.85,
    "region_factor": 1,
    "score": 112.5
  }
]`;

const sessionJson = `{
  "id": "sess_abc123def456",
  "project_id": "hailuo-tests",
  "preset_id": "comfyui-minimax-h3",
  "hours": 4,
  "created_at": "2026-09-29T12:00:00Z",
  "deadline_at": "2026-09-29T16:00:00Z",
  "phase": "ready",
  "offer": ${offerJson.slice(1, offerJson.lastIndexOf("]"))},
  "instance_id": "pod123",
  "pod_status": "RUNNING",
  "data_center": "EU-RO-1",
  "endpoints": [{ "name": "ui", "url": "https://pod123-8188.proxy.runpod.net" }],
  "cost_estimate_usd": 1.36,
  "drain_ok": false,
  "forced": false
}`;

describe("estimateUsd", () => {
  it("matches the CLI catalog product", () => {
    expect(estimateUsd(0.34, 4)).toBeCloseTo(1.36, 10);
    expect(estimateUsd(Number.NaN, 1)).toBe(0);
  });
});

describe("parseOffers", () => {
  it("reads ranked JSON and attaches the estimate", () => {
    const offers = parseOffers(offerJson, 4);
    expect(offers).toHaveLength(1);
    expect(offers[0]?.id).toBe("runpod:community:NVIDIA GeForce RTX 4090");
    expect(offers[0]?.score).toBe(112.5);
    expect(offers[0]?.estimate_usd).toBeCloseTo(1.36, 10);
    expect(offers[0]?.vram_gb).toBe(24);
  });

  it("reads the space-aligned table", () => {
    const table = [
      "score = (perf_index / usd_per_hr) * reliability_factor * region_factor",
      "RANK  SCORE  USD/HR  EST    VRAM  CLOUD      AVAIL  REGION   GPU       OFFER",
      "1     112.50  0.340   1.360  24    community  HIGH   EU-RO-1  RTX 4090  runpod:community:NVIDIA GeForce RTX 4090",
    ].join("\n");
    const offers = parseOffers(table, 4);
    expect(offers[0]?.id).toBe("runpod:community:NVIDIA GeForce RTX 4090");
    expect(offers[0]?.name).toBe("RTX 4090");
    expect(offers[0]?.cloud).toBe("community");
    expect(offers[0]?.usd_per_hr).toBeCloseTo(0.34, 5);
    expect(offers[0]?.estimate_usd).toBeCloseTo(1.36, 5);
  });
});

describe("parseSession", () => {
  it("reads session JSON", () => {
    const session = parseSession(sessionJson);
    expect(session.id).toBe("sess_abc123def456");
    expect(session.phase).toBe("ready");
    expect(session.project_id).toBe("hailuo-tests");
    expect(session.cost_estimate_usd).toBeCloseTo(1.36, 5);
    expect(session.deadline_at).toBe("2026-09-29T16:00:00Z");
    expect(session.endpoints[0]?.url).toBe("https://pod123-8188.proxy.runpod.net");
    expect(session.offer.estimate_usd).toBeCloseTo(1.36, 10);
    expect(phaseActive(session.phase)).toBe(true);
  });

  it("ignores the Go zero timestamp", () => {
    const session = parseSession(
      sessionJson.replace(
        "2026-09-29T16:00:00Z",
        "0001-01-01T00:00:00Z",
      ),
    );
    expect(session.deadline_at).toBeUndefined();
  });

  it("reads the text status block", () => {
    const text = [
      "session_id: sess_abc123def456",
      "phase: hydrating",
      "project: hailuo-tests",
      "preset: comfyui-minimax-h3",
      "offer: runpod:community:NVIDIA GeForce RTX 4090",
      "gpu: RTX 4090 24GB community $0.340/hr",
      "estimate_usd: 1.3600",
      "deadline: 2026-09-29T16:00:00Z",
      "remaining: 3h59m",
      "pod: pod123",
      "pod_status: RUNNING",
      "data_center: EU-RO-1",
      "endpoint_ui: https://pod123-8188.proxy.runpod.net",
      "error: drain did not finish; pod left running",
    ].join("\n");
    const session = parseSession(text);
    expect(session.phase).toBe("hydrating");
    expect(session.instance_id).toBe("pod123");
    expect(session.offer.name).toBe("RTX 4090");
    expect(session.offer.usd_per_hr).toBeCloseTo(0.34, 5);
    expect(session.cost_estimate_usd).toBeCloseTo(1.36, 5);
    expect(session.endpoints[0]?.name).toBe("ui");
    expect(isDrainFailure(session.error ?? "")).toBe(true);
  });
});

describe("parseStartResult", () => {
  it("reads a dry-run JSON payload", () => {
    const raw = `{
      "dry_run": true,
      "offer": ${offerJson.slice(1, offerJson.lastIndexOf("]"))},
      "estimate_usd": 0.34,
      "image": "runpod/pytorch:test"
    }`;
    const result = parseStartResult(raw, 1);
    expect(result.kind).toBe("dry_run");
    if (result.kind !== "dry_run") {
      return;
    }
    expect(result.dryRun.estimate_usd).toBeCloseTo(0.34, 5);
    expect(result.dryRun.image).toBe("runpod/pytorch:test");
    expect(result.dryRun.offer.id).toContain("4090");
  });

  it("reads a dry-run text block", () => {
    const text = [
      "dry_run: true",
      "offer: runpod:secure:NVIDIA GeForce RTX 4090",
      "gpu: RTX 4090 24GB secure $0.690/hr",
      "estimate_usd: 1.3800",
      "image: ghcr.io/wckd/example:latest",
      "failover: 3",
    ].join("\n");
    const result = parseStartResult(text, 2);
    if (result.kind !== "dry_run") {
      throw new Error("expected dry run");
    }
    expect(result.dryRun.failover).toBe(3);
    expect(result.dryRun.offer.cloud).toBe("secure");
    expect(result.dryRun.estimate_usd).toBeCloseTo(1.38, 5);
  });

  it("reads a started session", () => {
    const result = parseStartResult(sessionJson, 4);
    expect(result.kind).toBe("session");
  });
});

describe("parseConfigCheck", () => {
  it("reads JSON without treating a skipped probe as success", () => {
    const report = parseConfigCheck(`{
      "ok": false,
      "missing": ["RunPod API key (WCKD_RUNPOD_API_KEY or RUNPOD_API_KEY)"],
      "runpod": {"ok": false, "skipped": true},
      "s3": {"ok": false, "skipped": true},
      "defaults": {"hours": 1, "preset_id": "comfyui-minimax-h3"}
    }`);
    expect(report.ok).toBe(false);
    expect(report.runpod.skipped).toBe(true);
    expect(report.missing).toHaveLength(1);
    expect(report.defaults.preset_id).toBe("comfyui-minimax-h3");
  });

  it("reads the text report", () => {
    const text = [
      "runpod: ok",
      "s3: ok bucket=wckd",
      "state_dir: /home/wckd/.wckd",
      "presets_dir: /src/presets",
      "defaults: hours=1 project=hailuo-tests preset=comfyui-minimax-h3",
    ].join("\n");
    const report = parseConfigCheck(text);
    expect(report.ok).toBe(true);
    expect(report.s3.bucket).toBe("wckd");
    expect(report.state_dir).toBe("/home/wckd/.wckd");
    expect(report.defaults.project_id).toBe("hailuo-tests");
  });

  it("reads a failed text probe", () => {
    const report = parseConfigCheck(
      [
        "runpod: fail: unauthorized",
        "s3: fail: s3 head bucket wckd: denied",
        "state_dir: /tmp/state",
        "presets_dir: /tmp/presets",
        "defaults: hours=1 project=(unset) preset=comfyui-minimax-h3",
      ].join("\n"),
    );
    expect(report.ok).toBe(false);
    expect(report.runpod.error).toBe("unauthorized");
    expect(report.defaults.project_id).toBeUndefined();
  });
});

describe("projects and presets", () => {
  it("rejects a project id that is not a single prefix segment", () => {
    expect(isSlug("hailuo-tests")).toBe(true);
    expect(isSlug("../etc")).toBe(false);
    expect(() => parseProject(`{"id":"../etc","saved":true,"sessions":0}`)).toThrow(
      /invalid/,
    );
    expect(parseProjects(`[{"id":"hailuo-tests","saved":true,"sessions":2}]`)).toEqual([
      { id: "hailuo-tests", saved: true, sessions: 2 },
    ]);
  });

  it("reads a preset YAML document", () => {
    const preset = presetFromYaml(`
apiVersion: wckd.gpu/v1
kind: Preset
metadata:
  id: comfyui-minimax-h3
  name: MiniMax H3 (ComfyUI)
  version: 1
spec:
  workload_class: video_dit
  constraints:
    min_vram_gb: 24
    min_ram_gb: 64
    min_disk_gb: 100
    gpu_families: ["5090", "4090"]
    reliability: any
    prefer_regions: ["EU"]
  runtime:
    image: "runpod/pytorch:test"
    ports:
      - name: ui
        container: 8188
        expose: tunnel
  notes: |
    Peak VRAM is tight.
`);
    expect(preset.id).toBe("comfyui-minimax-h3");
    expect(preset.min_vram_gb).toBe(24);
    expect(preset.gpu_families).toEqual(["5090", "4090"]);
    expect(preset.ports[0]?.container).toBe(8188);
    expect(preset.notes).toContain("Peak VRAM");
  });
});
