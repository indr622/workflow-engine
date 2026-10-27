# syntax=docker/dockerfile:1.7
# Image web: build modeler-web lewat Nx, disajikan nginx sebagai file statis.
# Build dari root repo: docker build -f infra/docker/web.Dockerfile -t <image> .

FROM node:22-alpine AS build
ENV PNPM_HOME=/pnpm PATH=/pnpm:$PATH NX_DAEMON=false NX_NO_CLOUD=true CI=true
RUN corepack enable
WORKDIR /src

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN --mount=type=cache,id=pnpm,target=/pnpm/store pnpm install --frozen-lockfile

COPY . .
RUN pnpm nx build modeler-web --skip-nx-cache

FROM nginx:1.29-alpine
COPY infra/docker/nginx/default.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/dist/apps/modeler-web /usr/share/nginx/html
EXPOSE 80
HEALTHCHECK --interval=10s --timeout=3s --retries=5 CMD wget -qO- http://127.0.0.1/ >/dev/null || exit 1
