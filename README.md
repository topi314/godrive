# godrive

Self-hosted file drive: path ACLs, share links, API tokens, and a [Nuxt](https://nuxt.com) SPA in a single [Go](https://go.dev) binary.

<details>
<summary><strong>Contents</strong></summary>

- [Features](#features)
- [Quick start (Docker)](#quick-start-docker)
- [Configuration](#configuration)
  - [Auth (OIDC)](#auth-oidc)
  - [Storage](#storage)
  - [Uploads](#uploads)
  - [Share links](#share-links)
- [HTTP API](#http-api)
- [Development](#development)
- [Production build](#production-build)
- [License](#license)
- [Contributing](#contributing)
- [Contact](#contact)

</details>

## Features

- Path ACLs with inheritance (`everyone`, `guest`, `user`, `group`, `share`)
- [OIDC](https://openid.net/developers/how-connect-works/) ([Authelia](https://www.authelia.com) and other IdPs): PKCE, optional PAR / introspection / RP logout
- Group mapping; `admin` bypasses ACLs, `access` gates who can use the app
- Short sessions plus long-lived refresh cookies; groups refresh on renewal
- Capability share links at `/s/{id}` with share-principal ACL bits
- Personal API tokens for scripted file management ([API.md](API.md))
- Resumable chunked uploads; reverse proxies must allow bodies ≥ `chunk_size`
- Local disk ([fsnotify](https://github.com/fsnotify/fsnotify)) or [S3](https://aws.amazon.com/s3/)-compatible storage ([MinIO](https://min.io) notify / poll sync)
- [SQLite](https://www.sqlite.org) or [Postgres](https://www.postgresql.org); [Open Graph](https://ogp.me) cards for public paths

## Quick start (Docker)

```bash
cp example.config.toml config.toml
# edit listen addr, storage, and [auth] as needed
docker compose up --build
```

See [`compose.yml`](compose.yml) and [`example.config.toml`](example.config.toml). Uses [Docker Compose](https://docs.docker.com/compose/).

## Configuration

TOML only. Copy the example and point the binary at it:

```bash
cp example.config.toml config.toml
./godrive -config config.toml
```

Environment overrides use the `GODRIVE_` prefix (for example `GODRIVE_DATABASE_PASSWORD`).

| Area | Notes |
|------|--------|
| `[server]` | `listen_addr`, `frontend_url` ([Nuxt](https://nuxt.com) origin in `-tags dev`) |
| `[database]` | `sqlite` or `postgres` |
| `[storage]` | `local` or `s3`; optional `sync_interval` (defaults 15m local / 1m s3) |
| `[auth]` | [OIDC](https://openid.net/developers/how-connect-works/); `enabled = false` disables auth and ACLs (open admin mode) |
| `[upload]` | `max_size`, `chunk_size`, `session_ttl`, `max_parallel` |
| `[otel]` | Optional traces + [Prometheus](https://prometheus.io) `/metrics` |

### Auth (OIDC)

Set `[auth] enabled = true` with your issuer, client, and redirect URI (see the example config). Restart after changes.

Register the callback on the IdP client:

| Environment | Redirect URI |
|-------------|--------------|
| Production | `https://godrive.zip/api/callback` |
| Local [Nuxt](https://nuxt.com) | `http://localhost:3000/api/callback` |

`[auth.groups]`: `admin`, `access`, optional `guest` browsing, and `[auth.groups.map]` for extra OIDC → ACL group names. On first start with an empty ACL table, `/` is seeded with everyone → all and guest → read.

### Storage

- **Local** — files under `[storage.local].path`; [fsnotify](https://github.com/fsnotify/fsnotify) picks up drops; umask applies to new modes.
- **[S3](https://aws.amazon.com/s3/)** — [MinIO](https://min.io) and other S3 APIs via [AWS SDK for Go v2](https://aws.github.io/aws-sdk-go-v2/); optional webhook notify (`Authorization: Bearer …`) or rely on `sync_interval`.

### Uploads

Defaults: `max_size = 50GB`, `chunk_size = 16MB`, `session_ttl = 2h`, `max_parallel = 6`. Proxies in front of godrive must allow request bodies at least as large as `chunk_size` and keep connections open for slow multi-GB transfers.

### Share links

`/s/{id}` URLs are capability links. Permissions come from `path_acl` rows with `principal_type = share` (not normal user/guest rules on the same path). Creating a share upserts a default Read grant; other allow/deny mixes work (for example Create without Read for drop-boxes).

## HTTP API

Token-authenticated list / upload / download / rename / move / delete and resumable uploads: **[API.md](API.md)**.

## Development

Run the API without embedding the SPA (`-tags dev`) and [Nuxt](https://nuxt.com) with hot reload:

```bash
# terminal 1 — API (http://localhost:8090)
go run -tags dev . -config config.toml

# terminal 2 — UI (http://localhost:3000, proxies API to Go)
cd frontend
npm install
npm run dev
```

Override the API proxy target with `GODRIVE_API` (default `http://localhost:8090`). HTML that hits Go in this mode redirects to `[server].frontend_url` — use the Nuxt origin for the UI.

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

[Docker](https://www.docker.com) builds the frontend, copies it into `frontend/dist`, then embeds it (no `-tags dev`).

---

## License

godrive is licensed under the [Apache License 2.0](LICENSE).

---

## Contributing

Contributions are always welcome! Just open a pull request or discussion and I will take a look at it.

---

## Contact

- [Discord](https://discord.gg/sD3ABd5)
- [Twitter](https://twitter.com/topi314)
- [Email](mailto:git@topi.wtf)
