# syntax=docker/dockerfile:1
# Frontend assets and fonts are embedded into the stripped Go binary.
# Keep build toolchains in separate stages: the runtime only needs the CGO
# SQLite/WebP dependencies, CA certificates, and timezone data.
FROM node:22-alpine3.23 AS frontend-builder
ENV NODE_OPTIONS=--max-old-space-size=768
WORKDIR /src/frontend
RUN npm install --global pnpm@10.17.1
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm run build

FROM golang:1.26-alpine3.23 AS backend-builder
ENV GOMAXPROCS=2 GOMEMLIMIT=768MiB GOGC=50 CGO_ENABLED=1
RUN apk add --no-cache gcc g++ musl-dev libwebp-dev
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY backend/ ./backend/
COPY main.go ./
COPY --from=frontend-builder /src/frontend/dist ./frontend/dist
COPY --from=frontend-builder /src/frontend/src/assets/fonts ./frontend/src/assets/fonts
# 版本号在构建时注入，避免为了改版本去动源码。
# 构建时可用 --build-arg VERSION=v3.5.8 覆盖；不传则为 dev。
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build go build -p 1 -trimpath -ldflags="-s -w -X oneimg/backend/config.Version=${VERSION}" -o /out/oneimg ./main.go

FROM alpine:3.23
# CGO_ENABLED=1 is required by SQLite and WebP; do not switch to scratch
# or drop libwebp without verifying database and image-processing features.
RUN apk add --no-cache ca-certificates tzdata libwebp \
    && mkdir -p /app/data /app/uploads
WORKDIR /app
COPY --from=backend-builder /out/oneimg /app/oneimg
COPY ca/ ./ca/
# No frontend/dist copy: main.go embeds it into /app/oneimg at build time.
EXPOSE 8080
CMD ["/app/oneimg"]
