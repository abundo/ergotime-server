# Development

Go API and Vue SPA. Release notes are in `CHANGELOG.md`. User-facing setup is
in `README.md`. Deploy examples are in `deploy/`.

# Prerequisites

- Go 1.26.5 (the version in `go.mod`).
- Node.js 20 or newer, for the Vue app in `web/`.
- PostgreSQL. Local development uses the Compose file in this directory
  (Postgres 16). Docker or Podman is enough. An existing server works if the
  role and database are both named `ergotime`.
- `make`.

# Build and run

Start Postgres and the API:

```bash
docker compose up -d
# or: podman compose up -d
make run
```

`make run` builds `bin/ergotime-server` and starts it with
`config/config.debug.yaml`. That file listens on `127.0.0.1:8080`, uses the
Compose database (`ergotime` / `ergotime`), and serves `web/dist` from disk.
The bootstrap user, created only when `users` is empty, is `admin` / `admin`.

Frontend with hot reload (Vite proxies `/api` to port 8080):

```bash
cd web && npm install && npm run dev
```

Open http://127.0.0.1:5173.

A release binary embeds the built SPA:

```bash
make release
```

Tests:

```bash
make test
```

`make test` is `go test ./...`. The Vue app has no separate test script.
