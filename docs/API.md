# `wckd serve` API

Local JSON API over the same session engine as the CLI. [Invincible](https://github.com/wckdboy/invincible) supervises this binary as a child process and proxies `/v1/rent/*` to it. Engines are separate processes and are not linked into the gateway.

The path prefix is part of the contract. The proxy forwards `/v1/rent/*` without rewriting it.

## Run locally

Requires the same RunPod and S3 credentials as `wckd start` (`.env` or the environment). This spends money only after a confirmed create.

```bash
make build
./bin/wckd serve
```

Default bind is `127.0.0.1:8787`. On first launch the process writes a bearer key to `~/.wckd/serve.key` (or `$WCKD_STATE_DIR/serve.key`) mode `0600` and prints the path. It does not print the key.

```bash
KEY=$(cat ~/.wckd/serve.key)
curl -sS -H "Authorization: Bearer $KEY" \
  http://127.0.0.1:8787/v1/rent/health
```

```text
key_file: /home/you/.wckd/serve.key
listening: http://127.0.0.1:8787
sweep_interval: 15s
```

| Flag / env | Default | Role |
| --- | --- | --- |
| `--listen`, `WCKD_SERVE_LISTEN` | `127.0.0.1:8787` | Bind address. Non-loopback is refused unless `--allow-non-loopback`. |
| `--api-key-file`, `WCKD_SERVE_KEY_FILE` | `$WCKD_STATE_DIR/serve.key` | Path to the bearer. There is no flag that passes the key itself. |
| `--sweep-interval`, `WCKD_SWEEP_INTERVAL` | `15s` | In-process deadline sweep. |

A group- or world-readable key file, or a symlink, refuses to start on Unix. Windows mode bits are not an ACL, and this binary does not set an owner-only DACL. The file is read, not taken from argv, so the key is not in the process list.

Invincible should launch it the same way it launches other engines: loopback port chosen by the node, `--api-key-file` pointing at a mode-0600 file the node created, clean environment except what the runtime needs. A request without the bearer must return 401 before the model is served.

## Auth

Every route, including `/health`, requires:

```http
Authorization: Bearer <key file contents>
```

Comparison hashes both tokens with SHA-256 and then uses a constant-time compare. Missing and wrong keys return the same body:

```json
{"error":{"code":"unauthorized","message":"missing or invalid bearer token"}}
```

HTTP 401. The body does not echo the header or the key.

## Regions

`prefer_regions` on a preset is a score factor (a miss multiplies the score by 0.90). `require_regions` is a hard filter:

| Token | Effect |
| --- | --- |
| omitted | `EU`. Quotes with no usable data center in that set are dropped. |
| `EU` | Same hard filter. Community and Secure are both allowed, subject to the preset's `reliability`. |
| `EU-secure` | EU data centers and Secure cloud only. The `-secure` suffix is the mode; the prefix is the region token. |
| `any` or `*` | Disables the hard filter. Must be the only token. |
| other strings | Boundary match against the data center id and name, same as `prefer_regions`. `EU` matches `EU-RO-1` and `EU Romania`, not `EUR-IS-1`. |

A quote that has both an EU and a US data center keeps only the EU ids, and provision is pinned to those ids. A quote with no data center list cannot satisfy the filter and is dropped.

`dataCenterIds` is a preference on RunPod, not a hard placement constraint. After create, if `require_regions` is set and the returned data center is empty or outside that set, the pod is terminated and the next candidate is tried. Soft `prefer_regions` is not checked again after placement.

The CLI flags `--require-regions` on `offers` and `start` override the preset for that command. The API accepts the same override as a query parameter (offers) or a JSON field (create). An explicit empty list clears the filter.

## Endpoints

Cost figures are `catalog_usd_per_hr_times_hours`: the offer's catalog hourly price times the requested hours. They are not a vendor invoice.

### `GET /health` and `GET /v1/rent/health`

```json
{"ok":true,"engine":"wckd"}
```

### `GET /v1/rent/offers`

Query: `preset` (required), `hours` (required, `(0, 24]`), `limit` (optional, default 20, max 100), `require_regions` (optional, comma-separated).

```json
{
  "preset_id": "comfyui-minimax-h3",
  "hours": 1,
  "require_regions": ["EU"],
  "require_secure": false,
  "cost_basis": "catalog_usd_per_hr_times_hours",
  "offers": [
    {
      "id": "runpod:community:NVIDIA GeForce RTX 4090",
      "vendor": "runpod",
      "sku": "NVIDIA GeForce RTX 4090",
      "name": "RTX 4090",
      "vram_gb": 24,
      "usd_per_hr": 0.34,
      "cloud": "community",
      "region": "EU-RO-1",
      "data_center_ids": ["EU-RO-1"],
      "estimate_usd": 0.34,
      "score": 0
    }
  ]
}
```

`offers` is empty when nothing matches. That is not an error. This call does not create a pod.

### `POST /v1/rent/sessions`

```json
{
  "project_id": "hailuo-tests",
  "preset_id": "comfyui-minimax-h3",
  "hours": 1,
  "offer_id": "runpod:community:NVIDIA GeForce RTX 4090",
  "confirm": false,
  "require_regions": ["EU"]
}
```

`offer_id` and `require_regions` are optional. `confirm` defaults to false. A query-string `confirm` is ignored. There is no configuration that skips this check.

Confirmation is either `"confirm": true` in the JSON body or the header `X-Wckd-Confirm: true` (that exact word). Anything else, including `yes`, is not confirmation.

Without confirmation the response is **409** and nothing is written: no session file, no S3 object, no provision call.

```json
{
  "error": {
    "code": "confirmation_required",
    "message": "confirmation required; resend with confirm true to rent the chosen offer. Nothing was created."
  },
  "project_id": "hailuo-tests",
  "preset_id": "comfyui-minimax-h3",
  "hours": 1,
  "cost_basis": "catalog_usd_per_hr_times_hours",
  "cost_estimate_usd": 0.34,
  "require_regions": ["EU"],
  "require_secure": false,
  "offer": {"id": "runpod:community:NVIDIA GeForce RTX 4090", "usd_per_hr": 0.34, "estimate_usd": 0.34},
  "failover": [{"id": "runpod:community:NVIDIA GeForce RTX 4090", "usd_per_hr": 0.34, "estimate_usd": 0.34}]
}
```

`offer` is the candidate that would be tried first. `cost_estimate_usd` is that offer's catalog price times `hours`. `failover` is that same window (`wckd start` tries up to 3 when `offer_id` is empty), each row shaped like `offer`. A client that must lock the price resends `offer_id` from `offer.id` with `confirm: true`. An empty `offer_id` still failovers inside that window, and a later candidate can have a different hourly price.

With confirmation the response is **201** and the body is the session object (the same JSON `wckd start --json` prints). The pod is created. S3 credentials are injected into the pod environment for the sidecar. They are not part of the response.

### `GET /v1/rent/sessions`

State-store snapshot. This does not poll the vendor. Order is newest `created_at` first.

```json
{"sessions": []}
```

### `GET /v1/rent/sessions/{id}`

Polls the vendor and promotes `hydrating` to `ready` when the hydrate marker exists. The body is one session object. Unknown ids are 404. A vendor error returns the stored session with `warning` set when the file still exists.

### `POST /v1/rent/sessions/{id}/stop`

```json
{"force": false}
```

An empty body means `force: false`.

This is the CLI stop: write `control.json` with `action=drain`, wait for `drain-ok.json` (default 2 minutes, `WCKD_DRAIN_WAIT` / `WCKD_DRAIN_POLL`), then delete the pod. If the marker never arrives the pod stays up and the response is **409**:

```json
{
  "error": {"code": "drain_failed", "message": "drain did not finish; pod left running"},
  "session": {"id": "sess_...", "phase": "draining", "drain_ok": false}
}
```

Retry the same call after the sidecar finishes. `{"force": true}` deletes the pod anyway and records `drain_ok: false`, `forced: true`. That is the data-loss path.

Success is **200** with the session object, `phase` `terminated`.

## Session object

Phases: `pending`, `hydrating`, `ready`, `draining`, `terminated`, `failed`.

Fields used by clients: `id`, `project_id`, `preset_id`, `hours`, `created_at`, `deadline_at`, `phase`, `offer`, `instance_id`, `pod_status`, `data_center`, `endpoints` (`name`, `url`), `cost_estimate_usd`, `cost_actual_usd`, `drain_ok`, `forced`, `error`, `warning`, `ended_at`.

## Errors

```json
{"error":{"code":"<code>","message":"<text>"}}
```

| HTTP | `code` | When |
| --- | --- | --- |
| 400 | `bad_request` | Validation, bad JSON, unknown preset, bad `require_regions`. |
| 401 | `unauthorized` | Missing or wrong bearer. |
| 404 | `not_found` | Unknown session or route. |
| 405 | `method_not_allowed` | Wrong method. `Allow` is set. |
| 409 | `confirmation_required` | Create without confirm. Nothing was provisioned. |
| 409 | `drain_failed` | Stop left the pod running. |
| 422 | `no_offers` | No quote matched, or `offer_id` is not in the ranked list. |
| 500 | `internal` | State directory failed. |
| 502 | `vendor_error`, `provision_failed`, `stop_failed` | Vendor or object-store failure. A body with `session` is included when a record was written. |

Messages and logs are scrubbed for the bearer, the RunPod API key, and the S3 access key and secret. Those values are never printed at startup.

## Sweeper and restart

The process sweeps immediately on startup, then on the interval. Sweep uses the same drain-before-delete stop as the HTTP call (`force` is false). A session past `deadline_at` is drained and, once `drain-ok.json` exists, terminated, even if no client is connected. If the drain marker never arrives the pod stays up and the next tick retries.

Session files live in the state directory (`WCKD_STATE_DIR`, default `~/.wckd`). A new `wckd serve` process on the same directory loads them and the startup sweep terminates anything already past its deadline.

`wckd start` still detaches `wckd sweeper` unless `--no-sweeper` or `WCKD_NO_SWEEPER=1`. `wckd serve` does not spawn that process; its own loop is the sweeper.

## What this API does not do

The PWA `HttpBridge` does not call this process. The bearer must not be stored in the browser. CliBridge remains the desktop client.

Not in this process, and not blocked by it:

- Invincible rent module that proxies `/v1/rent/*` and performs the 401 check before serving.
- Tailscale sidecar, ephemeral auth keys, no public ports.
- Short-lived R2 (EU jurisdiction) credentials scoped per project. P1 still injects the operator's S3 key into the pod.
- vLLM preset.
