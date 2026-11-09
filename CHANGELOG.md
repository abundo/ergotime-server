# Changelog

All notable changes to the ErgoTime server are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versions are tagged `v*` on GitHub. That tag builds the Linux x86_64 binary and attaches it to a GitHub release.

## [1.0.0] - 2026-11-09

First release of the Go server. It replaces the Python Flask server. PostgreSQL remains the database.

### Added

- HTTP API on Go and Echo, with GORM against PostgreSQL.
- Vue 3 single-page admin UI (Vite, Vue Router, Pinia, PrimeVue), embedded in the release binary. The same process serves the API and the UI.
- Users stored in PostgreSQL, with bcrypt passwords. A bootstrap admin is created on first start when the users table is empty (`admin` / `admin` until it is changed).
- Customers, projects under a customer (optional cost center and free-text unit), and reports.
- Incremental report sync at `/api/report/sync/:seq`, scoped to the authenticated user, for the desktop client.
- Monthly report aggregation in the UI, and per-customer timesheet PDF export with project reallocation and invoice status.
- Authentication by an httpOnly JWT cookie, a Bearer token, or HTTP Basic (the desktop client uses Basic). Admin-only routes for user and catalog management.
- Security headers (CSP, HSTS, `X-Frame-Options`, nosniff), a CORS allowlist, a login rate limit, and a 1 MiB body limit.
- Health endpoints `/healthz` and `/readyz`.
- Example nginx and systemd units, plus a debug config and a Docker Compose Postgres service for local development.
- GitHub Actions workflow that tests the server, builds the Vue UI, and produces a `linux/amd64` binary on every push to `main` and on every `v*` tag. A tag also publishes a GitHub release.

### Fixed

- A JWT cookie or Bearer token is accepted only while that user still exists and is active. Admin rights are read from the database on each request, so deactivating a user or removing admin takes effect before the token expires.

### Changed

- The admin UI is Vue and PrimeVue instead of the Flask pages.
- Session auth for the browser is a JWT in an httpOnly cookie. API clients authenticate with HTTP Basic or Bearer.
- The production deploy is one static binary with the UI embedded, instead of a Python WSGI process and a separate static tree.

### Removed

- The Python Flask application, its Jinja templates, and the bundled Bootstrap and w2ui assets.
