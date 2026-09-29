export const PHASES = [
  "pending",
  "hydrating",
  "ready",
  "draining",
  "terminated",
  "failed",
] as const;

export type Phase = (typeof PHASES)[number];

export interface Endpoint {
  name: string;
  url: string;
}

export interface Offer {
  id: string;
  vendor: string;
  sku: string;
  name: string;
  family?: string;
  vram_gb: number;
  usd_per_hr: number;
  cloud: string;
  region?: string;
  data_center_ids?: string[];
  availability?: string;
  perf_index: number;
  perf_table: string;
  reliability_factor: number;
  region_factor: number;
  score: number;
  estimate_usd: number;
}

export interface Session {
  id: string;
  project_id: string;
  preset_id: string;
  hours: number;
  created_at?: string;
  deadline_at?: string;
  phase: string;
  offer: Offer;
  instance_id?: string;
  pod_status?: string;
  data_center?: string;
  endpoints: Endpoint[];
  cost_estimate_usd: number;
  cost_actual_usd?: number;
  drain_ok: boolean;
  forced: boolean;
  error?: string;
  warning?: string;
  ended_at?: string;
}

export interface PresetPort {
  name: string;
  container: number;
  expose: string;
}

export interface Preset {
  id: string;
  name: string;
  version: number;
  workload_class: string;
  min_vram_gb: number;
  min_ram_gb: number;
  min_disk_gb: number;
  gpu_families: string[];
  reliability: string;
  prefer_regions: string[];
  image: string;
  ports: PresetPort[];
  notes: string;
}

export interface Project {
  id: string;
  saved: boolean;
  sessions: number;
}

export interface CheckProbe {
  ok: boolean;
  skipped?: boolean;
  error?: string;
  bucket?: string;
}

export interface ConfigCheck {
  ok: boolean;
  missing: string[];
  runpod: CheckProbe;
  s3: CheckProbe;
  state_dir?: string;
  presets_dir?: string;
  defaults: {
    hours: number;
    project_id?: string;
    preset_id?: string;
  };
}

export interface DryRun {
  dry_run: true;
  offer: Offer;
  estimate_usd: number;
  image: string;
  failover?: number;
}

export type StartResult =
  | { kind: "session"; session: Session }
  | { kind: "dry_run"; dryRun: DryRun };

export interface OffersQuery {
  presetId: string;
  hours: number;
  limit?: number;
}

export interface StartInput {
  projectId: string;
  presetId: string;
  hours: number;
  offerId?: string;
  dryRun?: boolean;
}

export interface StopInput {
  sessionId?: string;
  force?: boolean;
}

export interface ControlClient {
  readonly mode: "cli" | "http";
  configCheck(): Promise<ConfigCheck>;
  listOffers(query: OffersQuery): Promise<Offer[]>;
  start(input: StartInput): Promise<StartResult>;
  status(sessionId?: string): Promise<Session | null>;
  stop(input: StopInput): Promise<Session>;
  listPresets(): Promise<Preset[]>;
  listProjects(): Promise<Project[]>;
  addProject(id: string): Promise<Project>;
  openUrl(url: string): Promise<void>;
}

export class ControlError extends Error {
  readonly drainFailed: boolean;
  readonly session?: Session;

  constructor(
    message: string,
    options?: { drainFailed?: boolean; session?: Session },
  ) {
    super(message);
    this.name = "ControlError";
    this.drainFailed = options?.drainFailed ?? isDrainFailure(message);
    this.session = options?.session;
  }
}

export function isDrainFailure(message: string): boolean {
  return /drain did not finish/i.test(message);
}

export function isPhase(value: string): value is Phase {
  return (PHASES as readonly string[]).includes(value);
}

export function phaseActive(phase: string): boolean {
  switch (phase) {
    case "pending":
    case "hydrating":
    case "ready":
    case "draining":
      return true;
    case "terminated":
    case "failed":
      return false;
    default:
      return false;
  }
}
