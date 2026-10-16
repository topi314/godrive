# syntax=docker/dockerfile:1

# Static frontend is arch-independent — build once on the runner, not under QEMU.
FROM --platform=$BUILDPLATFORM node:22-bookworm-slim AS frontend
WORKDIR /app

COPY frontend/package.json frontend/package-lock.json* ./
RUN --mount=type=cache,target=/root/.npm \
    if [ -f package-lock.json ]; then npm ci; else npm install; fi

COPY frontend/ ./
ENV NUXT_PUBLIC_API_BASE=
RUN npm run generate

# Cross-compile Go from the native runner for each TARGETARCH.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /build

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
COPY --from=frontend /app/.output/public ./frontend/dist

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

# git is required for Go to stamp vcs.revision / vcs.time into the binary.
RUN apk add --no-cache git

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH \
    GOARM=${TARGETVARIANT#v} \
    go build -trimpath -o godrive .

FROM alpine:3.21

COPY --from=build /build/godrive /bin/godrive

EXPOSE 80
ENTRYPOINT ["/bin/godrive"]
CMD ["-config", "/var/lib/godrive/config.toml"]
