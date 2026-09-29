import { parse as parseYaml } from "yaml";

import type {
  CheckProbe,
  ConfigCheck,
  DryRun,
  Endpoint,
  Offer,
  Preset,
  PresetPort,
  Project,
  Session,
  StartResult,
} from "./types";

const SLUG = /^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/;
const OFFER_LINE =
  /^(\d+)\s+([\d.]+)\s+([\d.]+)\s+([\d.]+)\s+(\d+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(.+?)\s+(runpod:(?:community|secure):.+)$/;

export function isSlug(value: string): boolean {
  return SLUG.test(value);
}

export function estimateUsd(usdPerHour: number, hours: number): number {
  if (
    !Number.isFinite(usdPerHour) ||
    !Number.isFinite(hours) ||
    usdPerHour < 0 ||
    hours < 0
  ) {
    return 0;
  }
  return usdPerHour * hours;
}

export function parseOffers(stdout: string, hours: number): Offer[] {
  const trimmed = stdout.trim();
  if (trimmed.startsWith("[") || trimmed.startsWith("{")) {
    const value: unknown = JSON.parse(trimmed);
    const rows = Array.isArray(value)
      ? value
      : asObject(value, "offers").offers;
    if (!Array.isArray(rows)) {
      throw new Error("offers JSON is not a list");
    }
    return rows.map((row, index) => offerFromJson(row, hours, index));
  }
  return parseOffersTable(trimmed, hours);
}

export function parseSession(stdout: string): Session {
  const trimmed = stdout.trim();
  if (!trimmed) {
    throw new Error("session output was empty");
  }
  if (trimmed.startsWith("{")) {
    return sessionFromJson(JSON.parse(trimmed));
  }
  return sessionFromText(trimmed);
}

export function parseStartResult(stdout: string, hours: number): StartResult {
  const trimmed = stdout.trim();
  if (trimmed.startsWith("{")) {
    const value = asObject(JSON.parse(trimmed), "start");
    if (value.dry_run === true) {
      return { kind: "dry_run", dryRun: dryRunFromJson(value, hours) };
    }
    return { kind: "session", session: sessionFromJson(value) };
  }
  if (/^dry_run:\s*true\b/m.test(trimmed)) {
    return { kind: "dry_run", dryRun: dryRunFromText(trimmed, hours) };
  }
  return { kind: "session", session: sessionFromText(trimmed) };
}

export function parseConfigCheck(stdout: string): ConfigCheck {
  const trimmed = stdout.trim();
  if (trimmed.startsWith("{")) {
    return configFromJson(asObject(JSON.parse(trimmed), "config check"));
  }
  return configFromText(trimmed);
}

export function parsePresets(stdout: string): Preset[] {
  const value: unknown = JSON.parse(stdout);
  if (!Array.isArray(value)) {
    throw new Error("presets JSON is not a list");
  }
  return value.map((row, index) => presetFromJson(row, index));
}

export function parseProjects(stdout: string): Project[] {
  const value: unknown = JSON.parse(stdout);
  if (!Array.isArray(value)) {
    throw new Error("projects JSON is not a list");
  }
  return value.map((row, index) => projectFromJson(row, index));
}

export function parseProject(stdout: string): Project {
  return projectFromJson(JSON.parse(stdout), 0);
}

export function presetFromYaml(text: string): Preset {
  return presetFromDocument(parseYaml(text));
}

function parseOffersTable(text: string, hours: number): Offer[] {
  const offers: Offer[] = [];
  for (const line of text.split(/\r?\n/)) {
    const match = OFFER_LINE.exec(line.trim());
    if (!match) {
      continue;
    }
    const usd = Number(match[3]);
    const vram = Number(match[5]);
    offers.push({
      id: match[10],
      vendor: "runpod",
      sku: match[10].replace(/^runpod:(?:community|secure):/, ""),
      name: match[9].trim(),
      vram_gb: vram,
      usd_per_hr: usd,
      cloud: match[6],
      region: match[8],
      availability: match[7],
      perf_index: 0,
      perf_table: "",
      reliability_factor: 0,
      region_factor: 0,
      score: Number(match[2]),
      estimate_usd: estimateUsd(usd, hours),
    });
  }
  if (offers.length === 0) {
    throw new Error("no offers in CLI output");
  }
  return offers;
}

function offerFromJson(value: unknown, hours: number, index: number): Offer {
  const row = asObject(value, `offer ${index + 1}`);
  const usd = asNumber(row.usd_per_hr, "usd_per_hr");
  return {
    id: asString(row.id, "id"),
    vendor: asString(row.vendor, "vendor"),
    sku: asString(row.sku, "sku"),
    name: asString(row.name, "name"),
    family: optionalString(row.family),
    vram_gb: asNumber(row.vram_gb, "vram_gb"),
    usd_per_hr: usd,
    cloud: asString(row.cloud, "cloud"),
    region: optionalString(row.region),
    data_center_ids: optionalStringList(row.data_center_ids),
    availability: optionalString(row.availability),
    perf_index: asNumber(row.perf_index, "perf_index"),
    perf_table: asString(row.perf_table, "perf_table"),
    reliability_factor: asNumber(row.reliability_factor, "reliability_factor"),
    region_factor: asNumber(row.region_factor, "region_factor"),
    score: asNumber(row.score, "score"),
    estimate_usd: estimateUsd(usd, hours),
  };
}

function sessionFromJson(value: unknown): Session {
  const row = asObject(value, "session");
  const offer = offerFromJson(row.offer ?? emptyOffer(), 0, 0);
  const hours = asNumber(row.hours ?? 0, "hours");
  offer.estimate_usd = estimateUsd(offer.usd_per_hr, hours);
  return {
    id: asString(row.id, "id"),
    project_id: asString(row.project_id ?? "", "project_id"),
    preset_id: asString(row.preset_id ?? "", "preset_id"),
    hours,
    created_at: recentTime(row.created_at),
    deadline_at: recentTime(row.deadline_at),
    phase: asString(row.phase, "phase"),
    offer,
    instance_id: optionalString(row.instance_id),
    pod_status: optionalString(row.pod_status),
    data_center: optionalString(row.data_center),
    endpoints: endpointsFromJson(row.endpoints),
    cost_estimate_usd: asNumber(row.cost_estimate_usd ?? 0, "cost_estimate_usd"),
    cost_actual_usd:
      row.cost_actual_usd === undefined
        ? undefined
        : asNumber(row.cost_actual_usd, "cost_actual_usd"),
    drain_ok: row.drain_ok === true,
    forced: row.forced === true,
    error: optionalString(row.error),
    warning: optionalString(row.warning),
    ended_at: recentTime(row.ended_at),
  };
}

function sessionFromText(text: string): Session {
  const fields = new Map<string, string>();
  const endpoints: Endpoint[] = [];
  for (const line of text.split(/\r?\n/)) {
    const split = line.indexOf(": ");
    if (split <= 0) {
      continue;
    }
    const key = line.slice(0, split).trim();
    const value = line.slice(split + 2).trim();
    if (key.startsWith("endpoint_")) {
      endpoints.push({ name: key.slice("endpoint_".length), url: value });
      continue;
    }
    fields.set(key, value);
  }
  const id = fields.get("session_id");
  if (!id) {
    throw new Error("session output has no session_id");
  }
  const gpu = parseGpuLine(fields.get("gpu") ?? "");
  const hours = 0;
  const offer: Offer = {
    id: fields.get("offer") ?? "",
    vendor: "runpod",
    sku: "",
    name: gpu.name,
    vram_gb: gpu.vram,
    usd_per_hr: gpu.usd,
    cloud: gpu.cloud,
    perf_index: 0,
    perf_table: "",
    reliability_factor: 0,
    region_factor: 0,
    score: 0,
    estimate_usd: estimateUsd(gpu.usd, hours),
  };
  return {
    id,
    project_id: fields.get("project") ?? "",
    preset_id: fields.get("preset") ?? "",
    hours,
    deadline_at: fields.get("deadline"),
    phase: fields.get("phase") ?? "",
    offer,
    instance_id: fields.get("pod"),
    pod_status: fields.get("pod_status"),
    data_center: fields.get("data_center"),
    endpoints,
    cost_estimate_usd: Number(fields.get("estimate_usd") ?? "0"),
    cost_actual_usd: fields.has("actual_usd")
      ? Number(fields.get("actual_usd"))
      : undefined,
    drain_ok: fields.get("drain_ok") === "true",
    forced: fields.get("forced") === "true",
    error: fields.get("error"),
    warning: fields.get("warning"),
  };
}

function parseGpuLine(line: string): {
  name: string;
  vram: number;
  cloud: string;
  usd: number;
} {
  const match = /^(.*) (\d+)GB (\S+) \$([0-9.]+)\/hr$/.exec(line);
  if (!match) {
    return { name: "", vram: 0, cloud: "", usd: 0 };
  }
  return {
    name: match[1],
    vram: Number(match[2]),
    cloud: match[3],
    usd: Number(match[4]),
  };
}

function dryRunFromJson(value: Record<string, unknown>, hours: number): DryRun {
  const offer = offerFromJson(value.offer, hours, 0);
  return {
    dry_run: true,
    offer,
    estimate_usd: asNumber(value.estimate_usd ?? offer.estimate_usd, "estimate_usd"),
    image: asString(value.image ?? "", "image"),
    failover:
      value.failover === undefined
        ? undefined
        : asNumber(value.failover, "failover"),
  };
}

function dryRunFromText(text: string, hours: number): DryRun {
  const fields = textFields(text);
  const gpu = parseGpuLine(fields.get("gpu") ?? "");
  const offer: Offer = {
    id: fields.get("offer") ?? "",
    vendor: "runpod",
    sku: "",
    name: gpu.name,
    vram_gb: gpu.vram,
    usd_per_hr: gpu.usd,
    cloud: gpu.cloud,
    perf_index: 0,
    perf_table: "",
    reliability_factor: 0,
    region_factor: 0,
    score: 0,
    estimate_usd: estimateUsd(gpu.usd, hours),
  };
  return {
    dry_run: true,
    offer,
    estimate_usd: Number(fields.get("estimate_usd") ?? offer.estimate_usd),
    image: fields.get("image") ?? "",
    failover: fields.has("failover") ? Number(fields.get("failover")) : undefined,
  };
}

function configFromJson(value: Record<string, unknown>): ConfigCheck {
  const defaults = asObject(value.defaults ?? {}, "defaults");
  return {
    ok: value.ok === true,
    missing: optionalStringList(value.missing) ?? [],
    runpod: probeFromJson(value.runpod),
    s3: probeFromJson(value.s3),
    state_dir: optionalString(value.state_dir),
    presets_dir: optionalString(value.presets_dir),
    defaults: {
      hours: asNumber(defaults.hours ?? 0, "hours"),
      project_id: optionalString(defaults.project_id),
      preset_id: optionalString(defaults.preset_id),
    },
  };
}

function configFromText(text: string): ConfigCheck {
  const missing: string[] = [];
  let runpod: CheckProbe = { ok: false };
  let s3: CheckProbe = { ok: false };
  let stateDir: string | undefined;
  let presetsDir: string | undefined;
  let hours = 0;
  let projectId: string | undefined;
  let presetId: string | undefined;
  let inMissing = false;
  for (const raw of text.split(/\r?\n/)) {
    const line = raw.trim();
    if (line === "credentials: missing") {
      inMissing = true;
      continue;
    }
    if (inMissing && line.startsWith("- ")) {
      missing.push(line.slice(2).trim());
      continue;
    }
    inMissing = false;
    if (line.startsWith("runpod: ok")) {
      runpod = { ok: true };
      continue;
    }
    if (line.startsWith("runpod: fail:")) {
      runpod = { ok: false, error: line.slice("runpod: fail:".length).trim() };
      continue;
    }
    if (line.startsWith("s3: ok")) {
      const bucket = /bucket=(\S+)/.exec(line)?.[1];
      s3 = { ok: true, bucket };
      continue;
    }
    if (line.startsWith("s3: fail:")) {
      s3 = { ok: false, error: line.slice("s3: fail:".length).trim() };
      continue;
    }
    if (line.startsWith("state_dir:")) {
      stateDir = line.slice("state_dir:".length).trim();
      continue;
    }
    if (line.startsWith("presets_dir:")) {
      presetsDir = line.slice("presets_dir:".length).trim();
      continue;
    }
    if (line.startsWith("defaults:")) {
      const parsed = /hours=([0-9.]+)\s+project=(\S+)\s+preset=(\S+)/.exec(line);
      if (parsed) {
        hours = Number(parsed[1]);
        projectId = parsed[2] === "(unset)" ? undefined : parsed[2];
        presetId = parsed[3];
      }
    }
  }
  return {
    ok: missing.length === 0 && runpod.ok && s3.ok,
    missing,
    runpod,
    s3,
    state_dir: stateDir,
    presets_dir: presetsDir,
    defaults: { hours, project_id: projectId, preset_id: presetId },
  };
}

function probeFromJson(value: unknown): CheckProbe {
  if (value === undefined) {
    return { ok: false };
  }
  const row = asObject(value, "probe");
  return {
    ok: row.ok === true,
    skipped: row.skipped === true ? true : undefined,
    error: optionalString(row.error),
    bucket: optionalString(row.bucket),
  };
}

function presetFromJson(value: unknown, index: number): Preset {
  return presetFromDocument(value, `preset ${index + 1}`);
}

function presetFromDocument(value: unknown, label = "preset"): Preset {
  const row = asObject(value, label);
  if (typeof row.apiVersion === "string" || typeof row.spec === "object") {
    const meta = asObject(row.metadata, "metadata");
    const spec = asObject(row.spec, "spec");
    const constraints = asObject(spec.constraints ?? {}, "constraints");
    const runtime = asObject(spec.runtime ?? {}, "runtime");
    return {
      id: asString(meta.id, "id"),
      name: asString(meta.name, "name"),
      version: asNumber(meta.version ?? 0, "version"),
      workload_class: asString(spec.workload_class ?? "", "workload_class"),
      min_vram_gb: asNumber(constraints.min_vram_gb ?? 0, "min_vram_gb"),
      min_ram_gb: asNumber(constraints.min_ram_gb ?? 0, "min_ram_gb"),
      min_disk_gb: asNumber(constraints.min_disk_gb ?? 0, "min_disk_gb"),
      gpu_families: optionalStringList(constraints.gpu_families) ?? [],
      reliability: optionalString(constraints.reliability) ?? "any",
      prefer_regions: optionalStringList(constraints.prefer_regions) ?? [],
      image: asString(runtime.image ?? "", "image"),
      ports: portsFromYaml(runtime.ports),
      notes: optionalString(spec.notes)?.trim() ?? "",
    };
  }
  return {
    id: asString(row.id, "id"),
    name: asString(row.name, "name"),
    version: asNumber(row.version ?? 0, "version"),
    workload_class: asString(row.workload_class ?? "", "workload_class"),
    min_vram_gb: asNumber(row.min_vram_gb ?? 0, "min_vram_gb"),
    min_ram_gb: asNumber(row.min_ram_gb ?? 0, "min_ram_gb"),
    min_disk_gb: asNumber(row.min_disk_gb ?? 0, "min_disk_gb"),
    gpu_families: optionalStringList(row.gpu_families) ?? [],
    reliability: optionalString(row.reliability) ?? "any",
    prefer_regions: optionalStringList(row.prefer_regions) ?? [],
    image: asString(row.image ?? "", "image"),
    ports: portsFromJson(row.ports),
    notes: optionalString(row.notes)?.trim() ?? "",
  };
}

function portsFromJson(value: unknown): PresetPort[] {
  if (value === undefined) {
    return [];
  }
  if (!Array.isArray(value)) {
    throw new Error("ports is not a list");
  }
  return value.map((item) => {
    const row = asObject(item, "port");
    return {
      name: asString(row.name, "name"),
      container: asNumber(row.container, "container"),
      expose: asString(row.expose ?? "", "expose"),
    };
  });
}

function portsFromYaml(value: unknown): PresetPort[] {
  if (value === undefined) {
    return [];
  }
  return portsFromJson(value);
}

function projectFromJson(value: unknown, index: number): Project {
  const row = asObject(value, `project ${index + 1}`);
  const id = asString(row.id, "id");
  if (!isSlug(id)) {
    throw new Error(`project id ${id} is invalid`);
  }
  return {
    id,
    saved: row.saved === true,
    sessions: asNumber(row.sessions ?? 0, "sessions"),
  };
}

function endpointsFromJson(value: unknown): Endpoint[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.map((item) => {
    const row = asObject(item, "endpoint");
    return {
      name: asString(row.name, "name"),
      url: asString(row.url, "url"),
    };
  });
}

function emptyOffer(): Record<string, unknown> {
  return {
    id: "",
    vendor: "",
    sku: "",
    name: "",
    vram_gb: 0,
    usd_per_hr: 0,
    cloud: "",
    perf_index: 0,
    perf_table: "",
    reliability_factor: 0,
    region_factor: 0,
    score: 0,
  };
}

function textFields(text: string): Map<string, string> {
  const fields = new Map<string, string>();
  for (const line of text.split(/\r?\n/)) {
    const split = line.indexOf(": ");
    if (split <= 0) {
      continue;
    }
    fields.set(line.slice(0, split).trim(), line.slice(split + 2).trim());
  }
  return fields;
}

function recentTime(value: unknown): string | undefined {
  const text = optionalString(value);
  if (!text) {
    return undefined;
  }
  const year = Number(text.slice(0, 4));
  if (!Number.isFinite(year) || year < 2000) {
    return undefined;
  }
  return text;
}

function asObject(value: unknown, label: string): Record<string, unknown> {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`${label} is not an object`);
  }
  return value as Record<string, unknown>;
}

function asString(value: unknown, label: string): string {
  if (typeof value !== "string") {
    throw new Error(`${label} is not a string`);
  }
  return value;
}

function asNumber(value: unknown, label: string): number {
  if (typeof value !== "number" || !Number.isFinite(value)) {
    throw new Error(`${label} is not a number`);
  }
  return value;
}

function optionalString(value: unknown): string | undefined {
  if (typeof value !== "string" || value === "") {
    return undefined;
  }
  return value;
}

function optionalStringList(value: unknown): string[] | undefined {
  if (value === undefined) {
    return undefined;
  }
  if (!Array.isArray(value) || value.some((item) => typeof item !== "string")) {
    throw new Error("expected a list of strings");
  }
  return value as string[];
}
