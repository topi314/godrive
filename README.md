# godrive

Self-hosted file drive with path ACLs, share links (`/s/{id}`), API tokens, and a Nuxt SPA embedded in a single Go binary.

## Docker

```bash
cp example.config.toml config.toml
# edit listen_addr / database / storage / auth as needed
docker compose up --build
```

## Development

Run the Go API without embedding the SPA (`-tags dev`), and the Nuxt app with hot reload:

```bash
# terminal 1 — API (default http://localhost:8090)
go run -tags dev . -config config.toml

# terminal 2 — UI (http://localhost:3000, proxies API routes to Go)
cd frontend
npm install
npm run dev
```

Override the API target with `GODRIVE_API` (default `http://localhost:8090`).

With `-tags dev`, HTML that hits Go is redirected to `frontend_url` — use the Nuxt dev server for the UI.

### Auth (Authelia / OIDC)

Enable `[auth]` in `config.toml` pointing at your Authelia issuer (see `example.config.toml` / `example.production.toml`). Restart the Go API after changing it.

For local Nuxt (`http://localhost:3000`), register `http://localhost:3000/api/callback` as an allowed redirect URI on the Authelia client. Production can keep `https://godrive.zip/callback` — bare `/callback` is still registered alongside `/api/callback`.

## Production build

```bash
cd frontend
NUXT_PUBLIC_API_BASE= npm run generate
# copy .output/public → frontend/dist (Docker does this automatically)
# PowerShell:
Copy-Item -Recurse -Force .output\public\* dist\
cd ..
go build -o godrive .
```

Docker builds the frontend, copies `.output/public` into `frontend/dist`, then embeds it (no `-tags dev`).

## Config

TOML only — see [`example.config.toml`](example.config.toml).

```bash
cp example.config.toml config.toml
./godrive -config config.toml
```

Env overrides use the `GODRIVE_` prefix (e.g. `GODRIVE_DATABASE_PASSWORD`).

## Features

- SQLite or Postgres (sqlc + [gomigrate](https://github.com/topi314/gomigrate))
- Path ACLs with inheritance; publish via `everyone` + read
- Short share links at `/s/{id}`
- Instant pickup of files dropped into local storage (fsnotify) or via S3 notifications
- Open Graph previews for public paths; private URLs get a generic teaser card
- S3-compatible object storage via AWS SDK v2
