# ergotime-server

Modern reimplementation of the Ergotime web API and GUI server. Release notes are in [CHANGELOG.md](CHANGELOG.md). License: [GPLv3](LICENSE).

Tracks work time (**customers → projects → reports**), serves a Vue SPA admin UI,
and syncs with the desktop client.

## Stack

| Layer    | Technology                                                                                               |
| -------- | -------------------------------------------------------------------------------------------------------- |
| API      | Go, [Echo](https://echo.labstack.com/)                                                                   |
| ORM      | [GORM](https://gorm.io/)                                                                                 |
| DB       | PostgreSQL                                                                                               |
| Auth     | Local users table, bcrypt passwords, JWT in **httpOnly** cookie (or HTTP Basic / Bearer for API clients) |
| Frontend | Vue 3, Vite, Vue Router, Pinia, PrimeVue                                                                 |
| Deploy   | Reverse HTTPS proxy (nginx example included)                                                             |

## CI

GitHub Actions (`.github/workflows/build.yml`) runs the tests and builds one `linux/amd64` binary (`CGO_ENABLED=0`, Vue UI embedded) on every push to `main` and on pull requests. A tag `v*` publishes `ergotime-server-linux-amd64` on a GitHub release.

## Features

- **Users** stored in PostgreSQL (bootstrap admin on first start)
- **Customers** CRUD (top-level billing / work entity)
- **Projects** CRUD under a customer (optional cost center, free-text unit e.g. hours/kilometers)
- **Reports** CRUD + incremental **sync** (`/api/report/sync/:seq`) for the desktop client (scoped per user)
- **Monthly report** aggregation for the SPA (customer → project breakdown)
- **Timesheets**: per-customer PDF export for a selected month, project reallocation dialog, invoice status
- SPA served by the same process (production) or Vite dev proxy (development)
- Security: CSP, HSTS, X-Frame-Options, nosniff, CORS allowlist, login rate limit, bcrypt, cookie flags for HTTPS proxy

## Project layout

```
cmd/server/          main entrypoint
internal/
  auth/              password hashing + JWT
  config/            YAML / env configuration
  database/          GORM + migrations + bootstrap admin
  dto/               request/response types
  handlers/          HTTP handlers
  middleware/        auth, CORS, security headers, rate limit
  models/            GORM models
config/              example + debug configs
deploy/              nginx + systemd units
web/                 Vue 3 SPA (Vite) + go:embed of dist/ into the binary
```

## Quick start (development)

### 1. PostgreSQL

```bash
docker compose up -d
# or use an existing Postgres and create role/db "ergotime"
```

### 2. Config

```bash
cp config/config.debug.yaml config/config.yaml
# edit database password / jwt_secret for anything non-local
```

### 3. Backend

```bash
go build -o bin/ergotime-server ./cmd/server
./bin/ergotime-server -config config/config.debug.yaml
```

Default bootstrap user (only if the `users` table is empty):

- username: `admin`
- password: `admin` — **change immediately**

### 4. Frontend (dev with HMR)

Requires Node.js 20+.

```bash
cd web
npm install
npm run dev
```

Open http://127.0.0.1:5173 — Vite proxies `/api` to the Go server on `:8080`.

### 5. Production / release binary (SPA embedded)

The Vue build is **embedded into the server binary** via `go:embed`, so a release
install is a single executable (plus config + Postgres). No separate `web/dist`
on disk is required at runtime.

```bash
make release
# equivalent: make frontend && go build -ldflags="-s -w" -o bin/ergotime-server ./cmd/server
./bin/ergotime-server -config config/config.yaml
```

Or: `make all` then `make run`.

For local frontend iteration without recompiling Go, set `static.dir: web/dist`
(as in `config.debug.yaml`) so the server reads files from disk instead of the embed.

## Configuration

Config file search order:

1. `-config` flag
2. `ERGOTIME_CONFIG` env
3. `./config.yaml`, `./config/config.yaml`, `/etc/ergotime/config.yaml`

Environment overrides use prefix `ERGOTIME_` and `_` for nesting, e.g.:

```bash
export ERGOTIME_DATABASE_PASSWORD=secret
export ERGOTIME_AUTH_JWT_SECRET=$(openssl rand -base64 48)
```

See [config/config.example.yaml](config/config.example.yaml) for all keys.

**Behind HTTPS reverse proxy**, set:

- `auth.cookie_secure: true`
- `auth.cookie_samesite: lax` (or `none` only if cross-site)
- `server.host: 127.0.0.1` (do not expose Go directly)
- `cors.allowed_origins` only if the SPA is on a different origin (same-origin is preferred)

Example nginx config: [deploy/nginx-ergotime.conf](deploy/nginx-ergotime.conf).

## HTTP API overview

All JSON endpoints under `/api` require authentication except:

| Method | Path               | Description                                      |
| ------ | ------------------ | ------------------------------------------------ |
| POST   | `/api/auth/login`  | JSON `{username,password}` → sets session cookie |
| POST   | `/api/auth/logout` | Clears session cookie                            |
| GET    | `/healthz`         | Liveness                                         |
| GET    | `/readyz`          | DB readiness                                     |

Authenticated (cookie JWT, `Authorization: Bearer <jwt>`, or HTTP Basic):

| Method         | Path                                 | Notes                                                |
| -------------- | ------------------------------------ | ---------------------------------------------------- |
| GET            | `/api/auth/me`                       | Current user                                         |
| GET/POST       | `/api/users`                         | Admin list/create                                    |
| GET/PUT/DELETE | `/api/users/:id`                     | Admin; self can change own password                  |
| GET/POST…      | `/api/customers`                     | Mutations require admin                              |
| GET/POST…      | `/api/projects`                      | Mutations require admin; `?customer_id=`             |
| GET/POST…      | `/api/reports`                       | Time entries (`project_id`); non-admins see own only |
| POST           | `/api/reports/batch`                 | Desktop batch push (deletes/creates/updates)         |
| GET            | `/api/reports/monthly?month=YYYY-MM` | Aggregated SPA report                                |
| GET            | `/api/timesheets?month=YYYY-MM`      | Customers with time in month                         |
| GET            | `/api/timesheets/projects`           | Project totals for customer/month                    |
| PUT            | `/api/timesheets/reallocate`         | Move time between projects (sum preserved)           |
| GET            | `/api/timesheets/pdf`                | Timesheet PDF (project summary at end)               |
| PUT            | `/api/timesheets/invoiced`           | Mark customer/month invoiced                         |
| GET            | `/api/activity`                      | Legacy alias (desktop client)                        |
| GET            | `/api/report/sync/:seq`              | Incremental report sync (pull)                       |
| POST/PUT       | `/api/report`                        | Legacy form-urlencoded support                       |

List responses use `{ "data": [ ... ] }` for compatibility with the old server.

Legacy activity/report JSON for the Python desktop client includes `_id`, `activityid`, and integer `active`/`deleted` flags when the client is detected (or `?legacy=1`).

## Security notes

- Passwords are **bcrypt** (cost 12); hashes are never returned in JSON.
- Session JWT is stored in an **httpOnly**, **Secure** (in release), **SameSite** cookie — mitigates XSS token theft.
- Vue renders user data as text bindings (not `v-html`) — mitigates stored XSS.
- CSP + `X-Frame-Options: DENY` + `X-Content-Type-Options: nosniff`.
- CORS is an **allowlist**; empty list means no cross-origin credentialed access (same-origin SPA is fine).
- Login endpoint is **rate limited** per IP.
- Body size limited to 1 MiB.
- Prefer terminating TLS at nginx/Caddy and binding Go to localhost.

## Relation to legacy `ergotime/server`

This project replaces the Flask + server-rendered UI with a Go API and Vue SPA.
Domain concepts (users, activity, project, report + `seq` sync) match the original schema intent.
Table names stay `users`, `activity`, `project`, `report` with primary key column `_id` for easier migration from an existing database.

If you migrate an old database, re-hash plaintext passwords (legacy stored plaintext) before production use.

## License

GPLv3 — same project family as Ergotime.
