# wckd-gpu

One-button cloud GPU sessions for creative and agent workloads.

Press start → pick a preset + hours → best price/perf on RunPod → provision → sync the project to S3-compatible storage → clean shutdown.

**Status:** P1 CLI, plus an early P3 client (installable PWA and Tauri 2 desktop) that shells out to that CLI. The control plane is not started. Scope and deferrals: [docs/MVP.md](docs/MVP.md).

## Why Go

The CLI is a single static binary in `cli/`. Go keeps the HTTP adapters, the session state machine, and the tests in one module with no runtime to install on the machine that has to kill a pod at the deadline. The architecture notes already list Go as the control-plane language, so the RunPod port can move later without a rewrite.

## Docs

- [Product & architecture spec](docs/ARCHITECTURE.md)
- [MVP scope](docs/MVP.md)
- [Preset schema](docs/PRESETS.md)
- [Threat model](docs/THREAT-MODEL.md)
- [Pod sidecar](sidecar/README.md)

## Build

Requires Go 1.22+.

```bash
make build          # bin/wckd
make test
make check          # tests, CLI help, bash -n on the sidecar
```

Or:

```bash
cd cli && go test ./... && CGO_ENABLED=0 go build -o ../bin/wckd ./cmd/wckd
./bin/wckd --help
```

## Configure

Copy [.env.example](.env.example) to `.env` and fill in your own keys. `.env` is gitignored. The CLI reads `./.env` and does not override variables that are already exported.

```bash
cp .env.example .env
# edit .env — RunPod API key, R2 (or any S3) endpoint, bucket, access key, secret
set -a && source .env && set +a
```

An optional YAML file works too. Copy [wckd.yaml.example](wckd.yaml.example) to `./wckd.yaml` or `~/.wckd/config.yaml`. Environment variables win over the file. Do not commit either file once it contains secrets.

| Variable | Purpose |
| --- | --- |
| `RUNPOD_API_KEY` or `WCKD_RUNPOD_API_KEY` | RunPod API key |
| `WCKD_S3_ENDPOINT` | S3-compatible endpoint, for example `https://<account>.r2.cloudflarestorage.com` |
| `WCKD_S3_BUCKET` | Bucket name |
| `WCKD_S3_ACCESS_KEY` / `WCKD_S3_SECRET_KEY` | S3 credentials |
| `WCKD_S3_REGION` | `auto` for R2 |
| `WCKD_SESSION_HOURS` | Default length, 0–24 |
| `WCKD_PROJECT_ID` | Default project id (`projects/<id>/` in the bucket) |
| `WCKD_PRESET_ID` | Default preset, `comfyui-minimax-h3` |
| `WCKD_IMAGE` | Optional image override (the sidecar image you built) |

`wckd config check` calls RunPod and the bucket. It does not create a pod.

## Commands

```bash
wckd config check
wckd config check --json          # no secrets in the JSON
wckd offers --preset comfyui-minimax-h3 --hours 1
wckd offers --preset comfyui-minimax-h3 --hours 1 --json
wckd presets --json               # presets/*.yaml, no network
wckd projects                     # local ids, plus ids seen on sessions
wckd projects add hailuo-tests    # remembers the id; does not write S3
wckd start --hours 1 --project hailuo-tests --preset comfyui-minimax-h3
wckd start --dry-run --hours 1 --project hailuo-tests --preset comfyui-minimax-h3
wckd status --json
wckd stop --json
wckd stop --force                 # delete the pod even if drain did not finish
```

`start` prints a session id (`sess_…`). State is stored in `~/.wckd` (or `WCKD_STATE_DIR`). `status` and `stop` use that session when you omit the id.

Offer rank:

```
score = (perf_index / usd_per_hr) * reliability_factor * region_factor
```

`perf_index` is a versioned estimate (`2026-09-29`) for the preset workload class, not a live benchmark. Secure cloud uses reliability 1.0, community 0.85. A data center outside `prefer_regions` uses region factor 0.90. Pass `--offer` with an id from `wckd offers` to skip the ranking and the failover list.

Phases: `pending` → `hydrating` → `ready` → `draining` → `terminated`. `ready` means the sidecar wrote `hydrate-ok.json`. `stop` writes `control.json` with `action=drain`, waits up to two minutes for `drain-ok.json`, then deletes the pod. A missing marker leaves the pod running.

`start` detaches `wckd sweeper`, which enforces the deadline while this machine stays up. `wckd sweeper --once` is the cron form.

`stop` waits up to 2 minutes for the drain marker (`WCKD_DRAIN_WAIT`, a Go duration such as `5m`). Poll interval is `WCKD_DRAIN_POLL` (default `5s`).

## Smoke test with your own keys

This spends RunPod money. CI does not run it.

1. `cp .env.example .env` and fill in a RunPod API key plus an R2 (or S3) bucket you can write.
2. `set -a && source .env && set +a`
3. `make build`
4. `./bin/wckd config check`
5. `./bin/wckd offers --preset comfyui-minimax-h3 --hours 1`
6. `./bin/wckd start --dry-run --hours 1 --project hailuo-tests --preset comfyui-minimax-h3`
7. To actually rent a GPU:

```bash
./bin/wckd start --hours 1 --project hailuo-tests --preset comfyui-minimax-h3
./bin/wckd status
./bin/wckd stop
```

The committed preset uses the public image `runpod/pytorch:1.0.2-cu1281-torch280-ubuntu2404`. That image will boot, but it will not run the sidecar, so `stop` will fail the drain and keep the pod. Finish with `wckd stop --force` if you only wanted to prove placement.

For hydrate and drain, build [sidecar/Dockerfile](sidecar/Dockerfile), push it, and set `WCKD_IMAGE` to that reference before `start`. Then:

- the pod pulls `projects/hailuo-tests/workspace/` on boot
- `wckd stop` asks it to push that prefix back and only then deletes the pod
- a second `start` on the same project hydrates the previous outputs

Confirm the pod is gone in the RunPod console or with their API after `stop` prints `phase: terminated`.

## Web and desktop

One UI in [`apps/web`](apps/web) (Vite, React, TypeScript). It is an installable PWA (manifest + service worker for the shell). [`apps/desktop`](apps/desktop) is a Tauri 2 window around that same build for macOS, Linux, and Windows.

```bash
pnpm install
pnpm test
pnpm lint
pnpm build          # apps/web → apps/web/dist
pnpm dev            # Vite on http://127.0.0.1:1420
pnpm --filter @wckd/desktop dev    # Tauri. Needs Rust 1.90+ and the platform webkit libraries.
```

The UI uses a `ControlClient`:

- **CliBridge** (Tauri) runs `wckd` with `--json`. Settings are the binary path, an optional `--config` file, and the working directory that holds `.env`. The process environment is inherited, so exported keys work too.
- **HttpBridge** (the PWA, and the later control plane) does not shell out. Presets come from the YAML bundled at build time. Project ids live in local storage. Start, status, stop, and config check show that the desktop app is required until the API exists.

Nothing in the client stores a RunPod or S3 secret. Do not put keys in the settings screen.

The console palette, type, and screen rules are in [docs/UI.md](docs/UI.md). Coolify serves this same static shell from [`docker-compose.yaml`](docker-compose.yaml) on container port `PORT` (optional, default 80); see [docs/DEPLOY-COOLIFY.md](docs/DEPLOY-COOLIFY.md). The desktop binary is not part of that image.

In the desktop app: Settings → Config check, then Offers → Rank offers. **Dry run** calls `wckd start --dry-run` and does not create a pod. **Start** does, and it stays disabled until an offer is locked. The PWA shows “Desktop required for Start” and can preview a labeled sample rank. Session shows the phase, the countdown to `deadline_at` (amber inside T−15), a catalog cost ticker, Open UI, and a stub Extend. Stop confirms drain → S3 sync → terminate. If the drain marker never arrives, the pod stays up; Force requires the typed word `DESTROY`.

`start` still detaches `wckd sweeper` unless `WCKD_NO_SWEEPER=1`.

## Platforms (target)

PWA · macOS · Linux · Windows now. Android and iOS store apps later.

The P1 CLI runs on the machine that holds the API keys (Linux and macOS are what `make check` covers). The Tauri shell is the same machine: it cannot see keys that are not in that environment.

## Owner

WCKD / BLXMP — self-host and SMB AI tooling.
