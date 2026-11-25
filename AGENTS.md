## Design before features

A feature must fit the architecture: in the layer that owns it, through the
existing seams (models, the database, handlers, the Vue SPA), keeping the
invariants below. If it can't be added that way without a workaround (a special
case in a layer that shouldn't know about it, duplicated logic, state kept
beside the database), don't build the workaround. Stop, before writing code or
as soon as it shows up, and tell the maintainer: what doesn't fit and why, the
refactor you'd recommend and its rough size, and any smaller part of the feature
that fits cleanly now. Go on only once they've chosen.

## Layers

- `internal/models` is the schema: users, customers, projects, reports, report
  lines. Table names and the `_id` primary key stay compatible with the legacy
  database.
- `internal/database` opens GORM and runs migrations, including the bootstrap
  admin.
- `internal/handlers` is the HTTP API. Desktop sync, the SPA, and the legacy
  form routes are handlers on those models. They do not grow a second store.
- `internal/auth` and `internal/middleware` check the user on each request.
- `web/` is the Vue SPA. It calls `/api` and does not encode sync rules.

## Invariants

- `SyncProtocol` in `internal/handlers/health.go` is the desktop contract.
  `GET /healthz` advertises it. Bump it only when a client that speaks the old
  number must be refused. It is 3: start and stop are whole minutes, and a
  closed report lasts at least one minute. Protocol 2 added report lines.
- A report is one user, one customer, and one time interval. Lines attribute
  that whole interval to projects. Do not split `start`/`stop` across projects.
  Start and stop have seconds 0. Stop is the next whole minute when the
  report is one minute long: 12:20:00–12:21:00, not 12:20:59. An open report
  may omit stop. Rows already stored stay as they are until the next edit.
  `project_id` on the report is the anchor line so older filters still match.
- `seq` is the monotonic sync cursor. Deletes are soft (`deleted`) so clients
  can pull them. `client_uuid` is the idempotency key for creates.
- `invoiced` is set by the timesheet handler, not by desktop sync.
- At most one project per customer is the default.
- Admin rights come from the `users` row on each request, not from a claim
  cached until the token expires. An inactive user is rejected.
- Passwords are bcrypt. The hash is never returned in JSON.
- Legacy `/api/report` and `/api/activity` stay beside `/api/reports`. Do not
  drop them while a desktop client still calls them.
- The tree stays GPLv3.
