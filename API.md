# godrive HTTP API (token auth)

Machine-friendly API for listing, uploading, downloading, renaming, moving, and deleting files. Authenticate as a user with a personal API token; every request then runs with that user’s groups and path ACLs.

Base URL examples below use `https://godrive.zip` — replace with your instance.

<details>
<summary><strong>Contents</strong></summary>

- [Authentication](#authentication)
  - [Create a token](#create-a-token)
  - [Who am I?](#who-am-i)
- [Path model](#path-model)
  - [Content negotiation](#content-negotiation)
- [Permissions](#permissions)
- [Errors](#errors)
- [File operations](#file-operations)
  - [List a directory](#list-a-directory)
  - [Download a file](#download-a-file)
  - [Create a folder](#create-a-folder)
  - [Upload a small file](#upload-a-small-file)
  - [Update / rename / move a file](#update--rename--move-a-file)
  - [Move into a folder](#move-into-a-folder)
  - [Delete](#delete)
- [Resumable uploads (large files)](#resumable-uploads-large-files)
  - [Upload config](#upload-config)
  - [Preflight (optional)](#preflight-optional)
  - [Create session → PATCH chunks → complete](#create-session--patch-chunks--complete)
- [Optional: path ACLs & shares](#optional-path-acls--shares)
  - [Get / set ACL on a path](#get--set-acl-on-a-path)
  - [Shares](#shares)
- [Quick reference](#quick-reference)
- [curl cookbook](#curl-cookbook)

</details>

## Authentication

Send the raw token on every request:

```http
Authorization: Bearer <token>
```

Tokens act as the issuing user (same home, groups, and ACL effective permissions). The user must have app access ([OIDC](https://openid.net/developers/how-connect-works/) `access` / `admin` group mapping). Owners get full access on their files. Admins bypass ACLs only in [sudo mode](#who-am-i).

### Create a token

Log in to the web UI (or use an existing session cookie) and call `POST /api/tokens`. Store `token` securely — it is only returned once. List/revoke with `GET /api/tokens` and `DELETE /api/tokens/{token_hash}`.

<details>
<summary>Example request (optional body)</summary>

```http
POST /api/tokens
Cookie: X-Session-ID=<session>
```

```http
POST /api/tokens
Content-Type: application/json
Cookie: X-Session-ID=<session>

{
  "description": "ci deploy — github actions godrive.zip"
}
```

</details>

<details>
<summary>Example response <code>201</code></summary>

```json
{
  "token": "dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4",
  "token_prefix": "dGhpcy1p",
  "token_hash": "a1b2c3d4e5f6…",
  "description": "ci deploy — github actions godrive.zip",
  "created_at": "2026-10-08T00:00:00Z"
}
```

</details>

### Who am I?

`GET /api/me` with the Bearer token.

<details>
<summary>Example request</summary>

```http
GET /api/me
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Accept: application/json
```

</details>

<details>
<summary>Example response <code>200</code></summary>

```json
{
  "authenticated": true,
  "auth_enabled": true,
  "guests_allowed": false,
  "id": "oidc-sub-alice",
  "username": "alice",
  "email": "alice@example.com",
  "avatar": "https://www.gravatar.com/avatar/…",
  "home": "/home/alice",
  "groups": ["godrive", "editors"],
  "available_groups": ["admin", "godrive", "editors", "viewer"],
  "is_admin": false,
  "is_access": true,
  "is_guest": false,
  "session_expires_at": null
}
```

Admins also get `"sudo": false|true`. Sudo is **off by default**: without it, admins see only what ACL/ownership allows. Turn it on with `PATCH /api/me` `{"sudo": true}` (sets a session-only HttpOnly cookie; cleared on logout / browser close) or send `X-Godrive-Sudo: 1` on API requests. Not stored on the user. Admin settings APIs still work without sudo.

</details>

## Path model

Files and folders are addressed by absolute logical paths on the **public** URL space (not under `/api`):

| Path | Meaning |
|------|---------|
| `/` | Root |
| `/home/alice` | Folder |
| `/home/alice/report.pdf` | File |

Path mutations require the matching ACL bit (see [Permissions](#permissions)). Reserved prefixes such as `/api`, `/s`, `/_nuxt` cannot be used as storage paths.

### Content negotiation

`GET` on a path:

- `Accept: application/json` → directory listing
- otherwise → stream file bytes (files) or SPA/HTML for directories in browsers

Always send `Accept: application/json` when listing from scripts.

## Permissions

Effective permission bitfield (`permissions` / `effective` in JSON):

| Bit | Name | Value |
|-----|------|------:|
| Read | list / download | `1` |
| Create | upload / mkdir into folder | `2` |
| Update | overwrite / rename metadata | `4` |
| Delete | delete | `8` |
| UpdatePermissions | edit ACL | `16` |
| Share | create share links | `32` |

All bits combined = `63`.

## Errors

Common statuses: `400` bad input, `401` missing/invalid auth, `403` ACL denied, `404` missing, `409` conflict (exists / upload_offset).

<details>
<summary>Example error bodies</summary>

```json
{
  "message": "forbidden",
  "status": 403,
  "path": "/home/alice/docs"
}
```

```json
{
  "message": "unauthorized",
  "status": 401,
  "path": "/api/me"
}
```

```json
{
  "message": "file exists",
  "status": 409,
  "path": "/home/alice/docs"
}
```

```json
{
  "message": "forbidden: /home/alice/secret.txt",
  "status": 403,
  "path": "/home/alice/secret.txt"
}
```

</details>

---

## File operations

### List a directory

`GET /{path}` with `Accept: application/json`. Requires **Read** on the directory (entries without Read are omitted).

<details>
<summary>Example request</summary>

```http
GET /home/alice
Accept: application/json
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
```

</details>

<details>
<summary>Example response <code>200</code></summary>

```json
{
  "path": "/home/alice",
  "permissions": 63,
  "files": [
    {
      "path": "/home/alice/docs",
      "name": "docs",
      "is_dir": true,
      "size": 4096,
      "date": "2026-10-08T12:00:00Z",
      "permissions": 7
    },
    {
      "path": "/home/alice/archive",
      "name": "archive",
      "is_dir": true,
      "size": 0,
      "date": "2026-10-07T09:30:00Z",
      "permissions": 63
    },
    {
      "path": "/home/alice/note.txt",
      "name": "note.txt",
      "is_dir": false,
      "size": 12,
      "content_type": "text/plain; charset=utf-8",
      "description": "scratch pad",
      "owner_id": "oidc-sub-alice",
      "owner": "alice",
      "date": "2026-10-08T11:45:00Z",
      "permissions": 63
    },
    {
      "path": "/home/alice/photo.jpg",
      "name": "photo.jpg",
      "is_dir": false,
      "size": 245760,
      "content_type": "image/jpeg",
      "description": "",
      "owner_id": "oidc-sub-alice",
      "owner": "alice",
      "date": "2026-10-06T18:00:00Z",
      "permissions": 63
    }
  ]
}
```

</details>

### Download a file

`GET /{path}` streams bytes. Supports `Range`. For a folder zip, use `GET /path?dl=1` with Read on that folder.

| Query | Effect |
|-------|--------|
| `dl=1` or `download=1` | `Content-Disposition: attachment` |
| `preview=1` | image preview variant when applicable |

<details>
<summary>Example request</summary>

```http
GET /home/alice/note.txt?dl=1
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
```

```http
GET /home/alice/photo.jpg
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Range: bytes=0-1023
```

</details>

<details>
<summary>Example curl</summary>

```bash
curl -fsSL -H "Authorization: Bearer $TOKEN" \
  -o note.txt "https://godrive.zip/home/alice/note.txt?dl=1"
```

</details>

### Create a folder

`POST /{path}` with **no body** creates that directory (and missing parents). Requires **Create**. Response `201`.

Multipart `POST` is still file upload into a parent directory — only non-multipart POSTs create folders.

<details>
<summary>Example request (no body)</summary>

```http
POST /home/alice/docs
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
```

</details>

<details>
<summary>Example response <code>201</code></summary>

```json
{
  "path": "/home/alice/docs",
  "is_dir": true
}
```

</details>

### Upload a small file

Use when `Content-Length ≤ chunk_size` (see [Upload config](#upload-config)). Larger files must use the resumable session API.

`POST /{path}` with a **raw body** writes the file at that path. Set `Content-Type` to the MIME type (optional; sniffed from the name). Response `204`.

Requires **Create** on the parent for new files, **Update** to overwrite. Empty body still means mkdir (see above).

For a plain put this is enough. If you need extra fields (especially `description`), use **multipart** instead.

<details>
<summary>Example — raw body (simple)</summary>

```http
POST /home/alice/docs/hello.txt
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Content-Type: text/plain; charset=utf-8
Content-Length: 11

hello world
```

```bash
curl -fsS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: text/plain" \
  --data-binary @hello.txt \
  "https://godrive.zip/home/alice/docs/hello.txt"
```

</details>

<details>
<summary>Example — multipart (with metadata)</summary>

`POST /{parent}` as `multipart/form-data`:

1. Part `json` — `name`, `size`, optional `description`
2. Part `file` — raw bytes (`size` must match)

```http
POST /home/alice/docs
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Content-Type: multipart/form-data; boundary=----godrive

------godrive
Content-Disposition: form-data; name="json"
Content-Type: application/json

{
  "name": "report.pdf",
  "size": 248193,
  "description": "Q3 finance pack — confidential"
}
------godrive
Content-Disposition: form-data; name="file"; filename="report.pdf"
Content-Type: application/pdf

%PDF-1.7 …
------godrive--
```

```bash
JSON='{"name":"report.pdf","size":248193,"description":"Q3 finance pack — confidential"}'
curl -fsS -H "Authorization: Bearer $TOKEN" \
  -F "json=$JSON;type=application/json" \
  -F "file=@report.pdf;type=application/pdf" \
  "https://godrive.zip/home/alice/docs"
```

</details>

### Update / rename / move a file

`PATCH /{path}` updates an existing entry. Use JSON for metadata / rename / move, or `multipart/form-data` with a `file` part to replace bytes. Content replace and description changes need **Update**. Rename/move needs delete+create style authorization on source and destination. Response `204`.

| Field | Effect |
|-------|--------|
| `name` | rename within the same parent |
| `dir` | move into another directory (optional `name` to rename at the same time) |
| `description` | update description |
| `size` | byte length of the new body when replacing content |

#### Replace file contents

Send multipart with parts `json` + `file` (same shape as small upload). `size` in `json` should match the new body length.

<details>
<summary>Example — replace content in place</summary>

```http
PATCH /home/alice/docs/report.pdf
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Content-Type: multipart/form-data; boundary=----godrive

------godrive
Content-Disposition: form-data; name="json"
Content-Type: application/json

{
  "size": 251004,
  "description": "Q3 finance pack — revised"
}
------godrive
Content-Disposition: form-data; name="file"; filename="report.pdf"
Content-Type: application/pdf

%PDF-1.7 … (new bytes)
------godrive--
```

</details>

<details>
<summary>Example — replace content and rename</summary>

```http
PATCH /home/alice/docs/draft.pdf
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Content-Type: multipart/form-data; boundary=----godrive

------godrive
Content-Disposition: form-data; name="json"
Content-Type: application/json

{
  "name": "report-final.pdf",
  "size": 251004,
  "description": "final PDF after review"
}
------godrive
Content-Disposition: form-data; name="file"; filename="report-final.pdf"
Content-Type: application/pdf

%PDF-1.7 …
------godrive--
```

</details>

<details>
<summary>Example — replace content while moving to another folder</summary>

```http
PATCH /home/alice/docs/report.pdf
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Content-Type: multipart/form-data; boundary=----godrive

------godrive
Content-Disposition: form-data; name="json"
Content-Type: application/json

{
  "dir": "/home/alice/published",
  "name": "report-2026-q3.pdf",
  "size": 251004,
  "description": "published revision"
}
------godrive
Content-Disposition: form-data; name="file"; filename="report-2026-q3.pdf"
Content-Type: application/pdf

%PDF-1.7 …
------godrive--
```

</details>

<details>
<summary>Example curl — replace content</summary>

```bash
SIZE=$(wc -c < ./report-v2.pdf)
JSON="{\"size\":$SIZE,\"description\":\"Q3 finance pack — revised\"}"
curl -fsS -X PATCH -H "Authorization: Bearer $TOKEN" \
  -F "json=$JSON;type=application/json" \
  -F "file=@report-v2.pdf;type=application/pdf" \
  "https://godrive.zip/home/alice/docs/report.pdf"
```

</details>

For files larger than `chunk_size`, use a [resumable upload](#create-session--patch-chunks--complete) with `"replace": true` instead of multipart `PATCH`.

#### Metadata / rename / move (JSON)

<details>
<summary>Example — rename in place</summary>

```http
PATCH /home/alice/docs/old-name.txt
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Content-Type: application/json

{
  "name": "new-name.txt"
}
```

</details>

<details>
<summary>Example — move into another folder (keep name)</summary>

```http
PATCH /home/alice/docs/report.pdf
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Content-Type: application/json

{
  "dir": "/home/alice/archive"
}
```

</details>

<details>
<summary>Example — move and rename + description</summary>

```http
PATCH /home/alice/docs/draft.md
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Content-Type: application/json

{
  "dir": "/home/alice/published",
  "name": "announcement.md",
  "description": "Published 2026-10-08"
}
```

</details>

<details>
<summary>Example — description only</summary>

```http
PATCH /home/alice/note.txt
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Content-Type: application/json

{
  "description": "updated scratch pad"
}
```

</details>

### Move into a folder

`PUT /{path}` with header `Destination: /target/dir`. Needs **Create** on the destination and **Delete** on each source. Response `204`.

<details>
<summary>Example — move one path (no body)</summary>

```http
PUT /home/alice/note.txt
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Destination: /home/alice/archive
```

Moves `/home/alice/note.txt` → `/home/alice/archive/note.txt`.

</details>

<details>
<summary>Example — move several children (JSON body)</summary>

```http
PUT /home/alice
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Destination: /home/alice/archive
Content-Type: application/json

[
  "note.txt",
  "docs",
  "photo.jpg"
]
```

Body names are relative to the request path.

</details>

### Delete

`DELETE /{path}`. No body deletes that path. Optional JSON array of relative names deletes children instead. Cascades to children, ACL rows, and shares. Needs **Delete**. Response `204`.

<details>
<summary>Example — delete one path (no body)</summary>

```http
DELETE /home/alice/note.txt
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
```

</details>

<details>
<summary>Example — delete several children of a folder</summary>

```http
DELETE /home/alice/archive
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Content-Type: application/json

[
  "note.txt",
  "photo.jpg",
  "old-reports"
]
```

</details>

<details>
<summary>Example curl</summary>

```bash
curl -fsS -X DELETE -H "Authorization: Bearer $TOKEN" \
  "https://godrive.zip/home/alice/note.txt"

# batch-delete siblings in one request
curl -fsS -X DELETE -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '["note.txt","photo.jpg","old-reports"]' \
  "https://godrive.zip/home/alice/archive"
```

</details>

---

## Resumable uploads (large files)

### Upload config

`GET /api/upload/config` (no auth required). All values are enforced by the server:

| Field | Enforcement |
|-------|-------------|
| `max_size` | Total file size on create / simple `POST` |
| `chunk_size` | Max `Content-Length` per resumable `PATCH`; also the simple-upload ceiling |
| `session_ttl` | Session `expires_at`; expired sessions cannot continue |
| `max_parallel` | Max concurrent active sessions per user (or per share for share uploads); create returns `429` when exceeded |

<details>
<summary>Example response <code>200</code></summary>

```json
{
  "max_size": 53687091200,
  "chunk_size": 16777216,
  "session_ttl": "2h0m0s",
  "max_parallel": 6
}
```

</details>

### Preflight (optional)

`POST /api/uploads/preflight` checks path, size, and ACL before creating a session.

<details>
<summary>Example request</summary>

```http
POST /api/uploads/preflight
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Content-Type: application/json

{
  "dir": "/home/alice/docs",
  "name": "dataset-2026-10.parquet",
  "size": 8589934592,
  "content_type": "application/octet-stream",
  "replace": false
}
```

| Field | Required | Notes |
|-------|----------|--------|
| `dir` | yes | parent directory |
| `name` | yes | basename |
| `size` | yes | total bytes |
| `content_type` | no | MIME hint |
| `replace` | no | if `true`, require **Update** when the file already exists |
| `share_id` | no | for share-link uploads (not needed with a user token) |

</details>

<details>
<summary>Example responses</summary>

Success:

```json
{
  "ok": true,
  "path": "/home/alice/docs/dataset-2026-10.parquet"
}
```

Failure:

```json
{
  "ok": false,
  "path": "/home/alice/docs/dataset-2026-10.parquet",
  "errors": [
    "file exists",
    "file too large"
  ]
}
```

</details>

### Create session → PATCH chunks → complete

#### Create session

<details>
<summary>Example request — new file</summary>

```http
POST /api/uploads
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Content-Type: application/json

{
  "dir": "/home/alice/docs",
  "name": "dataset-2026-10.parquet",
  "size": 8589934592,
  "content_type": "application/vnd.apache.parquet",
  "description": "monthly export",
  "replace": false
}
```

</details>

<details>
<summary>Example request — replace existing large file</summary>

```http
POST /api/uploads
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Content-Type: application/json

{
  "dir": "/home/alice/docs",
  "name": "dataset-2026-10.parquet",
  "size": 9126805504,
  "content_type": "application/vnd.apache.parquet",
  "description": "monthly export — revised",
  "replace": true
}
```

Requires **Update** on the existing file (and **Create** on the parent). Then `PATCH` chunks and `complete` as usual; the final object overwrites the path.

</details>

<details>
<summary>Example response <code>201</code></summary>

```json
{
  "id": "u_01JABC9XYZUPLOADSESS",
  "path": "/home/alice/docs/dataset-2026-10.parquet",
  "size": 8589934592,
  "upload_offset": 0,
  "chunk_size": 16777216,
  "expires_at": "2026-10-11T00:00:00Z"
}
```

</details>

#### Upload a chunk

`Upload-Offset` must equal the session’s current `upload_offset`. Response includes the new `upload_offset`. Mismatch → `409`. Chunk body larger than `chunk_size` → `413`.

<details>
<summary>Example request</summary>

```http
PATCH /api/uploads/u_01JABC9XYZUPLOADSESS
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Upload-Offset: 0
Content-Type: application/octet-stream
Content-Length: 16777216

<16 MiB raw bytes>
```

Later chunk:

```http
PATCH /api/uploads/u_01JABC9XYZUPLOADSESS
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Upload-Offset: 16777216
Content-Type: application/octet-stream
Content-Length: 16777216

<next 16 MiB>
```

</details>

<details>
<summary>Example response <code>200</code></summary>

```json
{
  "upload_offset": 16777216
}
```

</details>

#### Complete

Optional `acl` on complete (same shape as [permission rules](#get--set-acl-on-a-path)) if the user may set ACLs. Abort with `DELETE /api/uploads/{id}`. Poll with `GET /api/uploads/{id}`.

<details>
<summary>Example request — complete (no body)</summary>

```http
POST /api/uploads/u_01JABC9XYZUPLOADSESS/complete
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
```

</details>

<details>
<summary>Example request — complete with description</summary>

```http
POST /api/uploads/u_01JABC9XYZUPLOADSESS/complete
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Content-Type: application/json

{
  "description": "monthly export — finalized"
}
```

</details>

<details>
<summary>Example request — complete with ACL</summary>

```http
POST /api/uploads/u_01JABC9XYZUPLOADSESS/complete
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Content-Type: application/json

{
  "description": "monthly export — team readable",
  "acl": [
    {
      "principal_type": "group",
      "principal_id": "editors",
      "allow": 7,
      "deny": 0
    },
    {
      "principal_type": "guest",
      "principal_id": "*",
      "allow": 0,
      "deny": 0
    }
  ]
}
```

</details>

<details>
<summary>Example response <code>200</code></summary>

```json
{
  "path": "/home/alice/docs/dataset-2026-10.parquet"
}
```

</details>

---

## Optional: path ACLs & shares

Same token auth. Useful when automating publish/share flows.

### Get / set ACL on a path

`GET /api/acl/{path}` needs **Read** or **UpdatePermissions**.  
`PUT /api/acl/{path}` needs **UpdatePermissions** (or admin sudo) and **replaces** local rules on that path.  
`PATCH /api/acl/{path}` needs **UpdatePermissions** (or admin sudo) and **merges**: `upsert` rules and/or `remove` principals; other local rules stay. Root is `GET/PUT/PATCH /api/acl`.

Principal types: `user`, `group`, `everyone`, `guest`, `share`.

<details>
<summary>Example — get permissions</summary>

```http
GET /api/acl/home/alice/docs
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Accept: application/json
```

```json
{
  "path": "/home/alice/docs",
  "effective": 63,
  "acl": [
    {
      "path": "/home/alice/docs",
      "principal_type": "user",
      "principal_id": "oidc-sub-alice",
      "allow": 63,
      "deny": 0
    }
  ],
  "inherited": [
    {
      "path": "/",
      "principal_type": "everyone",
      "principal_id": "*",
      "allow": 63,
      "deny": 0
    },
    {
      "path": "/",
      "principal_type": "guest",
      "principal_id": "*",
      "allow": 1,
      "deny": 0
    }
  ],
  "available_groups": ["admin", "godrive", "editors", "viewer"],
  "available_users": [
    {"id": "oidc-sub-alice", "username": "alice", "email": "alice@example.com"},
    {"id": "oidc-sub-bob", "username": "bob", "email": "bob@example.com"}
  ]
}
```

</details>

<details>
<summary>Example — set ACL (replace local rules)</summary>

```http
PUT /api/acl/home/alice/docs
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Content-Type: application/json

{
  "acl": [
    {
      "principal_type": "user",
      "principal_id": "oidc-sub-alice",
      "allow": 63,
      "deny": 0
    },
    {
      "principal_type": "user",
      "principal_id": "oidc-sub-bob",
      "allow": 5,
      "deny": 0
    },
    {
      "principal_type": "group",
      "principal_id": "editors",
      "allow": 15,
      "deny": 0
    },
    {
      "principal_type": "everyone",
      "principal_id": "*",
      "allow": 1,
      "deny": 0
    },
    {
      "principal_type": "guest",
      "principal_id": "*",
      "allow": 1,
      "deny": 0
    },
    {
      "principal_type": "everyone",
      "principal_id": "*",
      "allow": 0,
      "deny": 8
    }
  ]
}
```

Notes:

- `allow: 5` = Read (`1`) + Update (`4`)
- `allow: 15` = Read + Create + Update + Delete
- `allow: 63` = all bits
- A deny of `8` strips Delete even if inherited allow includes it
- For `everyone` / `guest`, `principal_id` is normalized to `*`
- `share` rules need an existing share id

Response `204`. Replace runs in a transaction (invalid rules leave the previous ACL intact).

</details>

<details>
<summary>Example — patch ACL (upsert / remove)</summary>

```http
PATCH /api/acl/home/alice/docs
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Content-Type: application/json

{
  "upsert": [
    {
      "principal_type": "user",
      "principal_id": "oidc-sub-bob",
      "allow": 5,
      "deny": 0
    },
    {
      "principal_type": "guest",
      "principal_id": "*",
      "allow": 1,
      "deny": 0
    }
  ],
  "remove": [
    {
      "principal_type": "user",
      "principal_id": "oidc-sub-carol"
    }
  ]
}
```

Notes:

- At least one of `upsert` or `remove` is required
- `remove` is idempotent (missing rules are fine)
- Other local rules on the path are left unchanged
- Response `204`

</details>

### Shares

Needs **Share**. Default `allow` is Read (`1`). List/delete: `GET /api/shares`, `DELETE /api/shares/{id}`. Public browse URLs are `/s/{id}/…` (not token-authenticated).

<details>
<summary>Example — create share (relative expiry)</summary>

```http
POST /api/shares
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Content-Type: application/json

{
  "path": "/home/alice/docs",
  "expires_in": "7d",
  "allow": 1,
  "deny": 0
}
```

`expires_in` accepts Go durations (`72h`, `30m`) or day shorthand (`7d`).

</details>

<details>
<summary>Example — create share (absolute expiry + write bits)</summary>

```http
POST /api/shares
Authorization: Bearer dGhpcy1pcy1hLXNhbXBsZS1yYXctdG9rZW4
Content-Type: application/json

{
  "path": "/home/alice/inbox",
  "expires_at": "2026-12-31T23:59:59Z",
  "allow": 3,
  "deny": 0
}
```

`allow: 3` = Read + Create (drop-box style if you omit Read and only set Create: `"allow": 2`).

</details>

<details>
<summary>Example response <code>201</code></summary>

```json
{
  "id": "AbCdEfGhIjKl",
  "path": "/home/alice/docs",
  "url": "/s/AbCdEfGhIjKl",
  "created_at": "2026-10-08T00:00:00Z",
  "expires_at": "2026-10-15T00:00:00Z",
  "allow": 1,
  "deny": 0
}
```

Public URL: `https://godrive.zip/s/AbCdEfGhIjKl`.

</details>

---

## Quick reference

| Action | Method | URL |
|--------|--------|-----|
| List | `GET` | `/{path}` + `Accept: application/json` |
| Download | `GET` | `/{path}` or `?dl=1` |
| Mkdir | `POST` | `/{path}` (no body) |
| Upload (small) | `POST` | `/{path}` + raw body |
| Patch / rename / replace | `PATCH` | `/{path}` (JSON or multipart) |
| Move | `PUT` | `/{path}` + `Destination` (no body) |
| Delete | `DELETE` | `/{path}` (no body) |
| Upload config | `GET` | `/api/upload/config` |
| Resumable upload | `POST/PATCH/POST` | `/api/uploads…` |
| Me | `GET` / `PATCH` | `/api/me` (`home`, admin `sudo`) |
| Tokens | `GET/POST/DELETE` | `/api/tokens` |
| ACL | `GET/PUT/PATCH` | `/api/acl/{path}` |
| Shares | `GET/POST/DELETE` | `/api/shares` |

## curl cookbook

<details>
<summary>Expand cookbook</summary>

```bash
export BASE=https://godrive.zip
export TOKEN=…

# list
curl -fsS -H "Authorization: Bearer $TOKEN" -H "Accept: application/json" "$BASE/home/alice"

# mkdir
curl -fsS -X POST -H "Authorization: Bearer $TOKEN" "$BASE/home/alice/inbox"

# upload small file
curl -fsS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: text/plain" \
  --data-binary @hello.txt \
  "$BASE/home/alice/inbox/hello.txt"

# rename
curl -fsS -X PATCH -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"hello-renamed.txt"}' "$BASE/home/alice/inbox/hello.txt"

# replace file contents (small) — POST again also works with Update
curl -fsS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: text/plain" \
  --data-binary @hello-v2.txt \
  "$BASE/home/alice/inbox/hello-renamed.txt"

# move into archive
curl -fsS -X PUT -H "Authorization: Bearer $TOKEN" \
  -H "Destination: /home/alice/archive" \
  "$BASE/home/alice/inbox"

# download
curl -fsSL -H "Authorization: Bearer $TOKEN" -o hello.txt \
  "$BASE/home/alice/archive/inbox/hello-renamed.txt?dl=1"

# delete
curl -fsS -X DELETE -H "Authorization: Bearer $TOKEN" \
  "$BASE/home/alice/archive/inbox/hello-renamed.txt"

# grant guest read (leave other local rules alone)
curl -fsS -X PATCH -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{
    "upsert": [
      {"principal_type":"guest","principal_id":"*","allow":1,"deny":0}
    ]
  }' "$BASE/api/acl/home/alice/published"

# create share
curl -fsS -X POST -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"path":"/home/alice/published","expires_in":"7d","allow":1}' \
  "$BASE/api/shares"
```

</details>
