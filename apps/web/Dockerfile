# Installable PWA shell for Coolify. No CLI, no GPU, no secrets.
# Node 22 matches CI. nginx listens on 80; the master drops to the nginx
# user via cap_net_bind_service so the process is not root.

FROM node:22-bookworm-slim AS build

WORKDIR /src

ENV COREPACK_ENABLE_DOWNLOAD_PROMPT=0
RUN corepack enable && corepack prepare pnpm@10.33.3 --activate

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json apps/web/package.json
COPY apps/desktop/package.json apps/desktop/package.json

RUN pnpm install --frozen-lockfile --filter @wckd/web...

COPY presets presets
COPY apps/web apps/web

# Compose passes this as a build-arg (${VITE_PUBLIC_BASE_URL:-}).
# ARG is in scope for the next RUN; ENV exports it into the Vite process
# so a later import.meta.env.VITE_PUBLIC_BASE_URL read is inlined here.
# The UI does not read it yet. Never pass secrets. This stage is not the runtime image.
ARG VITE_PUBLIC_BASE_URL=
ENV VITE_PUBLIC_BASE_URL=${VITE_PUBLIC_BASE_URL}

RUN pnpm --filter @wckd/web build

# Runtime is static files. No ARG, no ENV, no secrets. nginx listens on 80.
FROM nginx:1.27-alpine

RUN apk add --no-cache curl libcap \
  && setcap 'cap_net_bind_service=+ep' /usr/sbin/nginx \
  && mkdir -p /tmp/nginx/client_temp /tmp/nginx/proxy_temp /tmp/nginx/fastcgi_temp /tmp/nginx/uwsgi_temp /tmp/nginx/scgi_temp \
  && chown -R nginx:nginx /tmp/nginx /usr/share/nginx/html /var/cache/nginx

COPY apps/web/nginx.conf /etc/nginx/nginx.conf
COPY --from=build --chown=nginx:nginx /src/apps/web/dist /usr/share/nginx/html

USER nginx
EXPOSE 80

ENTRYPOINT ["nginx"]
CMD ["-g", "daemon off;"]
