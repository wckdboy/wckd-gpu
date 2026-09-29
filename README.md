# wckd-gpu

One-button cloud GPU sessions for creative and agent workloads.

Press start → pick a preset + hours → best price/perf on RunPod → provision → sync the project to S3-compatible storage → clean shutdown.

**Status:** P1 CLI. Control plane and the Flutter client are not started. Scope and deferrals: [docs/MVP.md](docs/MVP.md).

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
wckd offers --preset comfyui-minimax-h3 --hours 1
wckd start --hours 1 --project hailuo-tests --preset comfyui-minimax-h3
wckd status
wckd stop
wckd stop --force          # delete the pod even if drain did not finish
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

## Platforms (target)

Android · iOS · macOS · Linux · Windows

The P1 CLI runs on the machine that holds the API keys (Linux and macOS are what `make check` covers).

## Owner

WCKD / BLXMP — self-host and SMB AI tooling.
