# godrive

Self-hosted file drive with path ACLs, share links (`/s/{id}`), API tokens, and a Nuxt SPA embedded in a single Go binary.

## Build

```bash
cd web && npm ci && npm run generate && cd ..
go build -o godrive .
```

Docker (multi-stage) builds the frontend then embeds it.

## Config

TOML only — see [`example.godrive.toml`](example.godrive.toml).

```bash
./godrive -config ./godrive.toml
```

Env overrides use the `GODRIVE_` prefix (e.g. `GODRIVE_DATABASE_PASSWORD`).

## Features

- SQLite or Postgres (sqlc + [gomigrate](https://github.com/topi314/gomigrate))
- Path ACLs with inheritance; publish via `everyone` + read
- Short share links at `/s/{id}`
- Instant pickup of files dropped into local storage (fsnotify) or via S3 notifications
- Open Graph previews for public paths; private URLs get a generic teaser card
- S3-compatible object storage via AWS SDK v2
