# wckd-gpu — Product & Architecture Spec

**Version:** 0.1  
**Date:** 2026-09-29 (Europe/Copenhagen)  
**Status:** Spec only — not implemented  
**Owner:** WCKD / BLXMP

---

## 1. Problem

Renting cloud GPUs (RunPod, Vast, Lambda, …) for a few hours a day is cheaper than buying H100/H200 iron for burst video and agent work — but the UX is terrible:

- Manual pod pick, image, volume, IP, SSH
- Easy to forget a running GPU ($/hr leaks)
- Project files scatter; next day you start from zero
- Price shopping across vendors is a spreadsheet chore

**wckd-gpu** turns that into: one button, N hours, a named preset, automatic S3 backup, guaranteed spin-down.

---

## 2. Goals

### 2.1 Must

1. Start a GPU session with **one primary action** (preset + duration already chosen or defaulted).
2. Support **timed sessions** (e.g. 1 / 2 / 4 / 8 hours) with hard auto-stop.
3. Apply a **preconfigured task preset** (image, env, startup, ports, sync paths).
4. Persist all session artifacts to **S3-compatible object storage** (AWS S3, Cloudflare R2, Backblaze B2, MinIO).
5. **Clean shutdown:** stop workload → flush uploads → terminate instance → record cost receipt.
6. Fetch live offers from **multiple stable vendors**, score **price/performance** for the preset’s constraints, recommend a pick (user can override).
7. Resume a project another day from S3 (download or hydrate on next boot).
8. Clients on **Android, iOS, macOS, Linux, Windows**.

### 2.2 Must not (v1)

- Become a marketplace or reseller of GPU time (user’s own API keys / accounts).
- Store provider secrets in the mobile app in plaintext.
- Promise 100% stock availability; fail closed with alternatives.
- Support every niche provider on day one.

### 2.3 Non-goals (near term)

- Full IDE / ComfyUI UI inside the app (deep-link / embed / tunnel to app UI instead).
- Fine-grained multi-user org RBAC (single user / small team later).
- Training clusters / multi-node (single GPU or single-node multi-GPU only in v1).

---

## 3. User journeys

### 3.1 Happy path — “4 hours of H3”

1. Open app → Project **hailuo-tests** (S3 prefix `s3://bucket/projects/hailuo-tests/`).
2. Preset: **MiniMax H3 / ComfyUI**.
3. Duration: **4 hours**.
4. App queries vendors → shows top 3 offers (e.g. RunPod 5090 Community $0.69/hr, Secure $0.99/hr, Vast 4090 $0.40/hr) with estimated total = hours × rate + storage delta.
5. User taps **Start** (or confirms recommended).
6. Control plane provisions → health check → syncs project down from S3 → starts preset entrypoint → pushes session status + deep link (ComfyUI URL / SSH / Jupyter).
7. Soft warning at T−15 min; optional extend; at T=0 (or user Stop): drain script → rclone sync up → terminate → receipt.

### 3.2 Resume next day

1. Open same project → Start again.
2. New instance hydrates from S3 prefix (last checkpoint / outputs / workflows).
3. No local disk required on phone; desktop clients may cache.

### 3.3 Failure path

- Provision fails → try next offer in ranked list (user-configurable).
- Mid-session disconnect → control plane still owns timer; stop proceeds server-side.
- Sync fails at end → keep instance in **quarantine hold** (short, capped $/min) and alert; never silent data loss.

---

## 4. System architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Clients (Flutter preferred)                                  │
│  Android · iOS · macOS · Linux · Windows                       │
│  — auth, projects, start/stop, live status, deep links        │
└───────────────────────────┬─────────────────────────────────┘
                            │ HTTPS (REST + WebSocket)
┌───────────────────────────▼─────────────────────────────────┐
│  Control plane (always-on, small)                             │
│  API · Scheduler · Offer broker · Session FSM · Secrets vault │
└───────┬─────────────────┬─────────────────┬─────────────────┘
        │                 │                 │
        ▼                 ▼                 ▼
   GPU vendors      S3-compatible      Observability
   RunPod, Vast,    R2 / S3 / B2 /     logs, metrics,
   Lambda, …        MinIO              cost ledger
        │
        ▼
   Ephemeral GPU VM/pod
   Docker preset + agent sidecar (rclone, heartbeat, drain)
```

### 4.1 Components

| Component | Responsibility |
|-----------|----------------|
| **Client** | UX, local prefs, push notifications, never holds long-lived provider keys (optional BYOK advanced mode). |
| **API** | Auth (OIDC / magic link), projects, presets, sessions CRUD, offer query. |
| **Offer broker** | Normalize vendor SKUs → `Offer{vendor, sku, vram_gb, ram_gb, $/hr, region, reliability_tier}`. |
| **Session FSM** | States: `quoted → provisioning → hydrating → running → draining → terminated | failed`. |
| **Scheduler** | Hard stop at deadline; extend; orphan sweeper. |
| **Agent sidecar** | On GPU: heartbeat, rclone sync, run entrypoint, drain hook, expose ports via reverse tunnel or vendor proxy. |
| **Object store** | Source of truth for projects; session manifests; receipts. |
| **Secrets** | Provider API keys, S3 creds — KMS/sealed; injected at provision time as short-lived env or IAM role where possible. |

### 4.2 Session state machine

```
quoted
  → provisioning (create instance)
  → hydrating (pull S3 → disk)
  → running (entrypoint up; timer armed)
  → draining (stop entrypoint; push S3; verify)
  → terminated (destroy instance; write receipt)
  ↘ failed (any step; cleanup best-effort; alert)
```

Idempotent transitions; every transition logged to `sessions/{id}/events.jsonl` in S3.

---

## 5. Data model (logical)

### 5.1 Project

```yaml
id: proj_...
name: hailuo-tests
s3:
  endpoint: https://....r2.cloudflarestorage.com
  bucket: wckd-gpu
  prefix: projects/hailuo-tests/
owner_id: user_...
default_preset_id: preset_minimax_h3
```

### 5.2 Preset

See [PRESETS.md](PRESETS.md). Captures GPU constraints, container image, startup, ports, sync globs, healthcheck.

### 5.3 Session

```yaml
id: sess_...
project_id: proj_...
preset_id: preset_minimax_h3
duration_hours: 4
deadline_at: 2026-09-29T16:00:00+02:00
offer: { vendor: runpod, sku: RTX_5090, usd_per_hr: 0.69, ... }
instance_ref: { vendor_id: "...", region: "EU-RO-1" }
state: running
cost_estimate_usd: 2.76
cost_actual_usd: null  # filled on terminate
s3_manifest: s3://.../sessions/sess_.../manifest.json
```

### 5.4 S3 layout

```
s3://bucket/
  projects/{project_id}/
    workspace/           # user files, workflows, outputs
    .wckd/
      project.json
      last_session.json
  sessions/{session_id}/
    manifest.json
    events.jsonl
    receipt.json
    logs/
```

---

## 6. Offer broker & scoring

### 6.1 Vendor adapters (v1 priority)

1. **RunPod** — pods API (Community + Secure)
2. **Vast.ai** — interruptible + on-demand filters
3. **Lambda** (if API access) — stable SKUs
4. Stub: manual “paste SSH” for BYO (later)

Each adapter implements:

```
list_offers(constraints) -> Offer[]
provision(offer, cloud_init) -> InstanceRef
status(ref) -> InstanceStatus
terminate(ref) -> void
```

### 6.2 Constraints from preset

- `min_vram_gb`, `min_ram_gb`, `min_disk_gb`
- `gpu_families` allowlist (e.g. `["5090","4090","H100","L40S"]`)
- `reliability`: `community | secure | any`
- `regions` prefer EU for BLXMP data posture when possible

### 6.3 Score (transparent)

```
score = (perf_index / usd_per_hr) * reliability_factor * region_factor
```

- `perf_index`: relative table per GPU family for the preset class (video DiT vs LLM tok/s) — **versioned, labeled estimates**, not fake benches.
- Show user: $/hr, est. total, VRAM, RAM, vendor, tier, score.
- Default: auto-pick top score; always allow override.

### 6.4 Stock & failover

If provision fails (out of stock), automatically try next N offers (default 3) unless user locked a vendor.

---

## 7. GPU agent (sidecar)

Runs beside the workload container (same pod or docker-compose):

1. **Boot:** write `session.json`, start heartbeat → control plane.
2. **Hydrate:** `rclone sync` S3 prefix → `/workspace` (bandwidth caps configurable).
3. **Start:** exec preset `entrypoint`.
4. **During:** periodic incremental sync of `outputs/` (optional).
5. **Drain (SIGTERM / deadline):**
   - SIGTERM entrypoint → wait grace
   - final `rclone sync` with checksums
   - write manifest (file list + etags)
   - report `drain_ok` or `drain_failed`
6. Control plane terminates only after `drain_ok` or quarantine policy.

**Tunneling:** prefer vendor HTTPS proxy / Cloudflare Tunnel / frp — expose ComfyUI/Jupyter without raw 0.0.0.0 on public IP when possible.

---

## 8. Client architecture

**Recommendation:** **Flutter** (one codebase, solid desktop + mobile). Alternative: Tauri (desktop) + Flutter or Kotlin Multiplatform mobile — more split.

Screens:

1. Home — active session + quick Start
2. Projects — list / create / S3 binding
3. Presets — browse / pin favorites
4. Offer sheet — ranked quotes
5. Session live — timer, cost ticker, Open UI, Stop, Extend
6. Settings — S3, vendor keys (via control plane), notifications

Push: FCM/APNs for T−15 and failures.

---

## 9. Control plane stack (suggested)

| Layer | Choice |
|-------|--------|
| API | Go or TypeScript (Hono/Fastify) on Fly.io / Hetzner / Cloudflare Workers+DO |
| DB | Postgres (sessions, users) |
| Queue | Redis / NATS for provision jobs |
| Secrets | Infisical / Doppler / cloud KMS |
| Auth | Clerk / Auth.js / Keycloak — email + passkey |
| Object | User-BYO S3; optional managed R2 for free trial |

EU deployment preferred (Hetzner Falkenstein / Fly `ams` / CF EU) for BLXMP.

---

## 10. Security & privacy

See [THREAT-MODEL.md](THREAT-MODEL.md). Highlights:

- Provider keys never in client storage (v1).
- S3 keys scoped to project prefix (IAM path or R2 token).
- Sessions isolate; no shared disks across users.
- Audit log of provision/terminate.
- GDPR: user can wipe project prefix + account.

---

## 11. Cost model (product)

- **User pays GPU vendors directly** (BYOK) — wckd-gpu is orchestration.
- Optional later: prepaid credits (compliance heavier).
- App shows **estimate before start** and **receipt after stop**.
- Storage billed by user’s S3/R2 account.

---

## 12. Phased delivery

| Phase | Deliverable |
|-------|-------------|
| **P0 — Spec** | This repo docs (done). |
| **P1 — MVP CLI** | `wckd session start\|stop` + RunPod + R2 + one preset (ComfyUI stub). |
| **P2 — Control plane** | API + FSM + scheduler + secrets. |
| **P3 — Desktop app** | Flutter Linux/macOS/Windows talking to API. |
| **P4 — Mobile** | Android/iOS; push; deep links. |
| **P5 — Multi-vendor** | Vast + scoring polish + failover. |
| **P6 — Preset gallery** | H3, Wan2.2, vLLM open-weights, generic CUDA. |

Details: [MVP.md](MVP.md).

---

## 13. Success metrics

- Time from tap Start → usable UI **&lt; 5 min** p50 (excluding cold weight download).
- **Zero** unpaid orphan pods &gt; 10 min after deadline (sweeper).
- Drain success rate **≥ 99%** when network healthy.
- User can resume project next day with **zero** manual SCP.

---

## 14. Open decisions

1. Flutter vs Tauri+mobile split.
2. Managed control plane vs fully self-hosted binary for paranoid users.
3. Whether EU-only offer filter is default for BLXMP.
4. First preset: MiniMax H3 ComfyUI vs generic PyTorch.

---

## 15. Related context (BLXMP)

- Local desk GPU CapEx paused; API + rented GPU OpEx preferred.
- €7k local ceiling still relevant later; this app is the bridge.
- Wan 3.0 is API-only (no RunPod weights); presets should not claim otherwise.
- MiniMax H3 is RunPod-viable (prefer 5090 32GB + high host RAM).
