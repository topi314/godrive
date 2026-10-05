FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN npm ci || npm install
COPY web/ ./
RUN npm run generate

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /public ./public

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=unknown
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    CGO_ENABLED=0 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH \
    go build -ldflags="-X 'main.Version=$VERSION' -X 'main.Commit=$COMMIT' -X 'main.BuildTime=$BUILD_TIME'" -o godrive-server .

FROM alpine:3.21
COPY --from=build /build/godrive-server /bin/godrive
EXPOSE 80
ENTRYPOINT ["/bin/godrive"]
CMD ["-config", "/var/lib/godrive/godrive.toml"]
