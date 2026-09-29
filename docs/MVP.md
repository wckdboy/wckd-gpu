# MVP scope (P1)

**Goal:** Prove the lifecycle without a mobile app.

## In scope

- CLI: `wckd auth`, `wckd project create`, `wckd session start|status|stop|extend`
- Vendor: **RunPod only** (Community + Secure)
- Storage: **S3-compatible** (document R2 as recommended)
- Preset: **one** — `comfyui-minimax-h3` (or stub `comfyui-base` if H3 image not ready)
- Hard stop timer + drain via rclone
- Cost estimate + receipt JSON

## Out of scope for MVP

- Flutter UI
- Vast/Lambda
- Multi-user
- Fancy scoring (single-vendor sort by $/hr within VRAM floor is enough)

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
