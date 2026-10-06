# godrive

Self-hosted file drive with path ACLs, share links (`/s/{id}`), API tokens, and a Nuxt SPA embedded in a single Go binary.

## Docker

```bash
cp example.config.toml config.toml
# edit [server] / database / storage / auth as needed
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

With `-tags dev`, HTML that hits Go is redirected to `[server].frontend_url` — use the Nuxt dev server for the UI.

### Auth (Authelia / OIDC)

Enable `[auth]` with `enabled = true` in `config.toml` pointing at your Authelia issuer (see `example.config.toml`). Restart the Go API after changing it.

For local Nuxt (`http://localhost:3000`), register `http://localhost:3000/api/callback` as an allowed redirect URI on the Authelia client. Production uses `https://godrive.zip/api/callback`.

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

### Uploads

Large files use resumable chunked sessions (`[upload]` in config). Defaults: `max_size = 50GB`, `chunk_size = 16MB`, `session_ttl = 72h`, `max_parallel = 2`. Reverse proxies in front of godrive must allow request bodies at least as large as `chunk_size` and keep connections open long enough for slow multi-GB transfers.

### Share links

Share URLs (`/s/{id}`) are capability links. Permissions come from `path_acl` rows with `principal_type = share` (not from normal user/guest rules on the same path). Creating a share upserts a default Read grant for that share id; any allow/deny combination is valid (for example Create without Read for drop-box style links).

## Features

- SQLite or Postgres (sqlc + [gomigrate](https://github.com/topi314/gomigrate))
- Path ACLs with inheritance; `everyone` = logged-in users, `guest` = anonymous (publish via `guest` + read); `share` = capability URL only
- OIDC groups mapped to godrive groups in `[auth.groups.map]`; `admin` bypasses ACLs, `access` gates who can use the app
- Short opaque sessions (`session_lifespan`, default 15m) plus a long-lived refresh cookie (`refresh_token_lifespan`, default 30d); groups refresh on renewal
- OIDC authorization code + PKCE; PAR / introspection / revocation / RP logout when the IdP advertises them
- Short share links at `/s/{id}` with configurable share-principal ACL bits
- Resumable large-file uploads via the upload dialog (chunked PATCH sessions)
- Instant pickup of files dropped into local storage (fsnotify) or via S3 notifications
- Open Graph previews for public paths; private URLs get a generic teaser card
- S3-compatible object storage via AWS SDK v2
