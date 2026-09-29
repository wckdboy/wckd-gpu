# Deploy the PWA on Coolify

The Coolify app is the installable web shell in `apps/web`. It is a static build behind nginx. Tauri is not in this image. Start, status, stop, offers, and config check still need the desktop app, which runs the `wckd` CLI on a machine that holds the keys. The deployed site keeps saying **Desktop required for Start**. The catalog cost ticker in that shell is `$/hr × elapsed`, not an invoice.

No RunPod, S3, AWS, or other provider secret belongs in the image, the Compose file, or Coolify env. The shell does not read them.

## What Coolify runs

`docker-compose.yaml` at the repo root has one service, `web`:

- Build context is the repo root (`apps/web/Dockerfile`) so the image can bundle `presets/*.yaml`
- nginx listens on port **80** inside the container (`apps/web/nginx.conf`, Dockerfile `EXPOSE 80`)
- `expose: ${PORT:-80}` — Coolify’s proxy terminates HTTPS and forwards to container port 80. The default is 80, so expose matches the listener
- `restart: unless-stopped`
- Healthcheck is `curl` against `http://127.0.0.1/` (port 80, where nginx listens)
- No `environment:` and no `env_file:`. nginx does not read variables. Do not publish port 80 yourself unless you are smoking the image outside Coolify

There is no GPU service, no RunPod sidecar, and no S3 sidecar. Do not add a bind mount for `.env`.

## Variables

Coolify reads `${VAR}` and `${VAR:-default}` in the Compose file and creates matching entries under the resource’s Environment Variables. A literal (for example `VITE_PUBLIC_BASE_URL: ""`) is not a substitution, so the UI cannot replace it. Every build-arg in this file uses substitution. There is no `env_file` key and no `environment:` block: Coolify does not need a committed env file, and the nginx process is not given variables.

| Name | Required | Default | Phase | Where it goes |
| --- | --- | --- | --- | --- |
| `VITE_PUBLIC_BASE_URL` | Optional | empty | Build only | `build.args` → Dockerfile `ARG` / `ENV` → Vite at `pnpm build` |
| `PORT` | Optional | `80` | Runtime / proxy | `expose` → Coolify proxy → container port 80 |

`${VITE_PUBLIC_BASE_URL:-}` is the optional form (empty fallback, not `${VITE_PUBLIC_BASE_URL:?}`). Coolify creates the variable with an empty initial value and keeps a value you later type in the UI. The current UI does not read `VITE_*`. The arg exists so a future public base URL can be inlined at build time. Leave it empty, or set a public origin such as `https://gpu.example.com`.

`${PORT:-80}` is the same optional form, with default 80. Coolify’s predefined application variable `PORT` is the container port: the first exposed port, when `PORT` is not already set. For Docker Compose, the proxy uses container port 80 when the service domain has no port suffix (`https://gpu.example.com`, not `https://gpu.example.com:3000`). A literal `expose: "80"` does not create an Environment Variables entry. The substitution does, the same way `VITE_PUBLIC_BASE_URL` does.

nginx does not read `PORT`. There is still no `environment:` block, so the value is not injected into the container. The healthcheck calls `http://127.0.0.1/` (port 80). Leave `PORT` at 80 so expose, the listener, and the proxy stay on the same port. Do not append a port to the domain.

### How Coolify maps them

1. After the resource loads `docker-compose.yaml`, Coolify lists `VITE_PUBLIC_BASE_URL` and `PORT` because the file references them. Set values under **Configuration → Environment Variables** only if you need to. `PORT` arrives as `80`. A runtime checkbox does not put either variable in the container: this service has no `environment:` section.
2. On deploy, Compose substitutes `${VITE_PUBLIC_BASE_URL:-}` from that value. Unset or empty stays empty. `${PORT:-80}` becomes `80` when unset or empty.
3. The substituted base URL is the Docker build-arg. Coolify can also forward build-scoped variables with **Advanced → Inject Build Args to Dockerfile**. The Dockerfile declares `ARG VITE_PUBLIC_BASE_URL=` and `ENV VITE_PUBLIC_BASE_URL=...` immediately before `pnpm --filter @wckd/web build`, so the Vite process sees the value either way. `PORT` is not a build-arg.
4. The runtime stage is a new nginx image. It does not receive the `ARG`, the `ENV`, or `PORT`. Changing the URL means a new image build, not a container restart. The proxy still forwards to container port 80.

Build-args can show up in build logs. Do not mark a provider key as a build variable.

### Forbidden on this resource

Do not define these in Coolify, in `docker-compose.yaml`, or as image build-args. The PWA does not use them. They stay in the desktop machine’s `.env` or process environment, next to the `wckd` binary.

- `RUNPOD_API_KEY`, `WCKD_RUNPOD_API_KEY`, any `RUNPOD_*` or `WCKD_RUNPOD_*`
- `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, any `AWS_*`
- `S3_*`, `WCKD_S3_*`, or any other object-storage credential

## Create the app

1. Coolify → new resource → **Docker Compose**.
2. Point it at this repository and branch. Compose path: `docker-compose.yaml` (repo root).
3. Environment Variables: `PORT` (optional, default 80 — leave it) and optional `VITE_PUBLIC_BASE_URL`, as in the table above. Do not add RunPod, AWS, or S3 keys.
4. Set the domain on the `web` service with no port suffix. Coolify issues the certificate and forwards to container port 80.
5. Deploy. A healthy check is HTTP 200 on `/` and on a client route such as `/index.html`. Unknown paths also return `index.html` so the PWA shell loads.

Local check, from the repo root. Compose also interpolates a project `.env`, so leave `VITE_PUBLIC_BASE_URL` unset there when you want the empty case. `PORT` may be unset or `80`; both resolve expose to `80`.

```bash
docker compose config
PORT=80 docker compose config
VITE_PUBLIC_BASE_URL=https://gpu.example.com docker compose config
docker compose build web
```

The first command resolves the build-arg to empty and `expose` to `"80"`. The second keeps expose on 80. The third resolves the build-arg to `https://gpu.example.com`. A literal `""` in the compose file would stay empty in every case.
