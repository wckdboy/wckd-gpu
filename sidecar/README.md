# Pod sidecar

The CLI never copies files onto the GPU itself. The pod runs these scripts.

## What they do

1. `bootstrap.sh` configures rclone from `WCKD_S3_*` environment variables the CLI injects at create time.
2. It syncs each hydrate pair in `WCKD_HYDRATE_MAP` (`s3-prefix|container-path`, pairs separated by `;;`) onto the pod. The default is `projects/<project>/workspace` → `/workspace`.
3. It writes `sessions/<session>/hydrate-ok.json`. The CLI moves the session from `hydrating` to `ready` when that object exists.
4. It starts `WCKD_WORKLOAD` (the preset `runtime.entrypoint`). If that process exits immediately, the script sleeps so the deadline still applies. That is the stub behavior on an image that does not contain ComfyUI.
5. It drains when either the deadline in `WCKD_DEADLINE_AT` passes or `sessions/<session>/control.json` contains `"action":"drain"`.
6. `drain.sh` syncs `WCKD_DRAIN_MAP` (`container-path|s3-prefix`) back to the bucket with checksums, honoring comma-separated `WCKD_DRAIN_EXCLUDES`, then writes `sessions/<session>/drain-ok.json`.

`wckd stop` waits for `drain-ok.json` and only then deletes the pod. If the marker never appears it leaves the pod running. `--force` deletes it anyway.

## Why a custom image is required

P1 uses RunPod REST API v2. Create-pod accepts an image, args, env, and ports. It does not accept a replacement entrypoint. The public `runpod/pytorch` image runs `/start.sh`, which does not execute container args, then sleeps.

`sidecar/Dockerfile` installs rclone and sets `ENTRYPOINT` to `bootstrap.sh`. Build it, push it to a registry your RunPod account can pull, and point the session at it:

```bash
docker build -t ghcr.io/you/wckd-comfy-h3:dev /path/to/wckd-gpu/sidecar
# push, then:
export WCKD_IMAGE=ghcr.io/you/wckd-comfy-h3:dev
```

`WCKD_IMAGE` overrides `runtime.image` in the preset. The committed preset still names the public PyTorch image so a pod can be placed before that build exists. On that public image nothing hydrates and `wckd stop` will refuse to delete the pod until you pass `--force`.

Replacing the entrypoint means the stock image's SSH and Jupyter setup does not run. The RunPod HTTPS proxy for the preset's HTTP port still works once a process listens on that port.

## Credentials

The pod receives the same S3 access key the CLI uses. Scope that key to the bucket. P1 does not mint a short-lived prefix-scoped token; that belongs with the control plane.

## Layout the CLI writes

```
s3://bucket/projects/<project>/.wckd/project.json
s3://bucket/projects/<project>/.wckd/last_session.json
s3://bucket/projects/<project>/workspace/          # hydrate source and drain destination
s3://bucket/sessions/<session>/manifest.json
s3://bucket/sessions/<session>/events.jsonl
s3://bucket/sessions/<session>/control.json       # {"action":"drain"}
s3://bucket/sessions/<session>/hydrate-ok.json    # sidecar
s3://bucket/sessions/<session>/drain-ok.json      # sidecar
s3://bucket/sessions/<session>/drain-files.txt    # sidecar, best effort
s3://bucket/sessions/<session>/receipt.json
```

The preset HTTP healthcheck is not probed by the CLI. `ready` means the hydrate marker exists, not that ComfyUI answered.
