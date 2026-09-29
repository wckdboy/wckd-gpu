# Deploy the PWA on Coolify

The Coolify app is the installable web shell in `apps/web`. It is a static build behind nginx. Tauri is not in this image. Start, status, stop, offers, and config check still need the desktop app, which runs the `wckd` CLI on a machine that holds the keys. The deployed site keeps saying **Desktop required for Start**. The catalog cost ticker in that shell is `$/hr × elapsed`, not an invoice.

No RunPod, S3, or other provider secret belongs in the image, the Compose file, or Coolify env. The shell does not read them.

## What Coolify runs

`docker-compose.yml` at the repo root has one service, `web`:

- Build context is the repo root (`apps/web/Dockerfile`) so the image can bundle `presets/*.yaml`
- nginx listens on port **80** inside the container
- `expose: 80` — Coolify’s proxy maps the public port and terminates HTTPS
- `restart: unless-stopped`
- Healthcheck is `curl` against `http://127.0.0.1/`

There is no GPU service, no RunPod sidecar, and no S3 sidecar. Do not add a bind mount for `.env`.

## Create the app

1. Coolify → new resource → **Docker Compose**.
2. Point it at this repository and branch. Compose path: `docker-compose.yml` (repo root).
3. Leave build-arg `VITE_PUBLIC_BASE_URL` empty. The UI does not read any `VITE_*` variable. The arg exists so a future public flag can be passed at build time. It is not a place for keys.
4. Set the domain on the `web` service. Coolify issues the certificate. The container stays on port 80; do not publish that port yourself unless you are smoking the image outside Coolify.
5. Deploy. A healthy check is HTTP 200 on `/` and on a client route such as `/index.html`. Unknown paths also return `index.html` so the PWA shell loads.

Local check, from the repo root:

```bash
docker compose config
docker compose build web
```

## Env

None required. Do not set `RUNPOD_API_KEY`, `AWS_*`, or `S3_*` on this resource. Those stay in the desktop machine’s `.env` or process environment, next to the `wckd` binary.
