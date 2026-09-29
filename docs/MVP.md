# MVP scope (P1)

**Goal:** Prove the lifecycle without a mobile app.

## P1 implementation status

The CLI lives in `cli/` (Go module `github.com/wckdboy/wckd-gpu/cli`). Operator instructions are in the repo [README](../README.md). Sidecar behavior is in [sidecar/README.md](../sidecar/README.md).

### Implemented

- [x] `wckd config check` — RunPod `GET /v2/pods` and S3 `HeadBucket`, no GPU created
- [x] `wckd offers` — RunPod pod catalog, community and secure, ranked with the architecture score
- [x] `wckd start --hours --project --preset [--offer] [--dry-run]` — top offer, or a locked offer, with failover across the next candidates (default 3)
- [x] `wckd status [session]` — phase, remaining time, pod id, proxy URL
- [x] `wckd stop [session] [--force]` — drain marker, then terminate; failed drain leaves the pod up
- [x] `wckd sweeper` — spawned by `start`; stops sessions whose deadline has passed
- [x] Phases `pending → hydrating → ready → draining → terminated`, plus `failed`
- [x] Local session files and S3 project / session / receipt objects
- [x] Preset `presets/comfyui-minimax-h3.yaml`
- [x] Sidecar scripts and `sidecar/Dockerfile` (rclone hydrate and drain)
- [x] Unit tests for scoring, the state machine, and config parsing; HTTP tests mock RunPod and S3
- [x] CI runs `make check` and does not call paid APIs

### Deferred

- [ ] `wckd auth`, `wckd project create`, `wckd session extend` — the project prefix is written on start; keys are BYOK env, not a login
- [ ] Always-on control plane, so a dead laptop still deletes the pod (the sidecar drains and exits on its own clock; the pod record remains until a sweeper runs)
- [ ] Hydrate on the stock RunPod PyTorch image (API v2 cannot replace the entrypoint; use `sidecar/Dockerfile`)
- [ ] Preset HTTP healthcheck from the CLI (`ready` is the hydrate marker)
- [ ] Quarantine hold with a capped extra bill — P1 leaves the pod running and fails the stop
- [ ] Short-lived prefix-scoped S3 credentials (the user key is injected into the pod)
- [ ] Network volumes, Vast, Lambda, mobile store apps, multi-user
- [ ] Control plane. The P3 PWA + Tauri shell is in `apps/web` and `apps/desktop` and talks to this CLI (`CliBridge`). `HttpBridge` waits on the API.
- [ ] A published ComfyUI + MiniMax H3 image (the workload command is recorded; a missing binary holds the session open)

### Divergences from the spec

- Commands are `wckd config|offers|start|status|stop|sweeper`, not `wckd session start` and not `wckd auth`.
- Phases follow the P1 brief (`pending`, `hydrating`, `ready`, `draining`, `terminated`) rather than `quoted → provisioning → running`. `running` in the architecture doc is `ready` here. `failed` is extra, for provision errors. `failed → draining` exists so a pod that was created can still be cleaned up.
- Offer ranking uses the architecture formula, including a versioned perf table. The original "out of scope" note below allowed sorting by $/hr inside a VRAM floor. Single vendor only: RunPod.
- Host RAM is not in the GPU catalog. It is sent as `minRamPerGpu` at create time. VRAM, family, reliability, and region are filtered before create.
- Repo layout is `cli/`, `presets/`, `sidecar/`, `apps/web`, and `apps/desktop`. The vendor port is `cli/internal/vendor`. The client is not under `apps/cli`.
- Receipts multiply the catalog $/hr by elapsed wall time. They are not the RunPod invoice.
- Sessions are capped at 24 hours.
- S3 credentials live in the environment on the machine that runs the CLI, and are copied into the pod env so rclone can run. The threat model’s “keys only in the control plane” mitigation is P2.

## In scope

- CLI: `wckd auth`, `wckd project create`, `wckd session start|status|stop|extend`
- Vendor: **RunPod only** (Community + Secure)
- Storage: **S3-compatible** (document R2 as recommended)
- Preset: **one** — `comfyui-minimax-h3` (or stub `comfyui-base` if H3 image not ready)
- Hard stop timer + drain via rclone
- Cost estimate + receipt JSON

## Out of scope for MVP

- Control-plane API (the desktop shell calls the CLI instead)
- Mobile store apps
- Vast/Lambda
- Multi-user
- Fancy scoring (single-vendor sort by $/hr within VRAM floor is enough)

The Flutter client in the original sketch was replaced by the PWA + Tauri shell.

## Acceptance tests

1. Start 1h session on cheapest RunPod offer matching preset constraints.
2. Kill laptop mid-session → control/CLI sweeper still stops at deadline.
3. Stop early → files appear under S3 prefix within 2 minutes.
4. Second start hydrates previous outputs.
5. No instance left running after stop (verify via RunPod API).

## Suggested repo layout (implementation)

```
/apps/cli
/apps/control-api
/packages/offers-runpod
/packages/session-fsm
/packages/rclone-agent
/presets/
/docs/
```
