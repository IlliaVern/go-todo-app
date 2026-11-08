# go-todo-app

A REST API for personal todo lists, written in Go. Users sign up, get a JWT, and manage their own lists and the items in them. Every user sees only their own data.

**Stack:** Go 1.27 standard library HTTP · PostgreSQL 18 via [pgx](https://github.com/jackc/pgx) · [golang-jwt](https://github.com/golang-jwt/jwt) · argon2id (`golang.org/x/crypto`) · [caarlos0/env](https://github.com/caarlos0/env) · `log/slog` · [golang-migrate](https://github.com/golang-migrate/migrate) · [testcontainers-go](https://golang.testcontainers.org) · Docker Compose

## Quick start

Requires Docker with Compose v2 or later.

```sh
make up
```

This builds the image, starts PostgreSQL, applies the schema, and serves the API on <http://localhost:8000>. Both ports are bound to `127.0.0.1` only.

Without a `.env` file the stack uses local-only default credentials. To use your own, run `cp .env.example .env` and fill it in. `make up` passes `.env` to Compose when it exists. If you call Compose directly, pass it yourself: `docker compose -f docker/compose.yaml --env-file .env up --build`. Compose looks for `.env` next to the compose file, not in the repository root.

```sh
make logs           # follow logs
make down           # stop
make down ARGS=-v   # stop and delete the database volume
```

## API

All `/api` routes need the header `Authorization: Bearer <token>`.

| Method | Path | Body | Response |
|---|---|---|---|
| `POST` | `/auth/sign-up` | `{"name", "username", "password"}` | 201 `{"id"}` |
| `POST` | `/auth/sign-in` | `{"username", "password"}` | `{"token"}` (valid for 12 h) |
| `POST` | `/api/lists` | `{"title", "description"?}` | 201 `{"id"}` |
| `GET` | `/api/lists` | | `{"data": [list]}` |
| `GET` | `/api/lists/{id}` | | `list` |
| `PATCH` | `/api/lists/{id}` | `{"title"?, "description"?}` | `{"status": "ok"}` |
| `DELETE` | `/api/lists/{id}` | | `{"status": "ok"}` (deletes its items too) |
| `POST` | `/api/lists/{id}/items` | `{"title", "description"?}` | 201 `{"id"}` |
| `GET` | `/api/lists/{id}/items` | | `{"data": [item]}` |
| `GET` | `/api/items/{id}` | | `item` |
| `PATCH` | `/api/items/{id}` | `{"title"?, "description"?, "done"?}` | `{"status": "ok"}` |
| `DELETE` | `/api/items/{id}` | | `{"status": "ok"}` |
| `GET` | `/healthz` | | `{"status": "ok"}` while the process serves HTTP |
| `GET` | `/readyz` | | `{"status": "ready"}`, or 503 when PostgreSQL is unreachable |

`list` is `{"id", "title", "description"}` and `item` is `{"id", "title", "description", "done"}`. Empty collections come back as `{"data": []}`. Every path also works with a trailing slash. Bodies must be a single JSON object with only the fields shown.

Usernames are case-insensitive: they are stored lowercased, so `Ada` and `ada` are the same account. They may not start or end with whitespace.

Errors come back as `{"message": "..."}`:

| Status | When |
|---|---|
| 400 | Malformed JSON, unknown fields or trailing data, invalid id, failed validation: blank title or name, a field over 255 characters or containing a NUL character, a username with surrounding whitespace, password empty or over 1024 bytes, or an update with no fields |
| 401 | Missing or invalid token, a token for a user that no longer exists, or wrong username or password |
| 404 | The route, list or item doesn't exist, or the list or item belongs to another user |
| 405 | Wrong method for an existing path; see `Allow` |
| 409 | Username already taken, or the list or item kept changing under heavy concurrent updates (retry the request) |
| 413 | Request body over 1 MiB |
| 429 | More than `AUTH_RATE_LIMIT_PER_MINUTE` sign-up or sign-in requests from one address; see `Retry-After` |
| 500 | Unexpected failure. Details are logged, not returned |
| 503 | Password hashing is saturated; see `Retry-After` |

Every response carries an `X-Request-ID` header. The server reuses yours if you send one (printable ASCII, up to 128 characters) and otherwise generates one.

```sh
curl -s localhost:8000/auth/sign-up -d '{"name":"Ada","username":"ada","password":"s3cret"}'
TOKEN=$(curl -s localhost:8000/auth/sign-in -d '{"username":"ada","password":"s3cret"}' | jq -r .token)
curl -s localhost:8000/api/lists -H "Authorization: Bearer $TOKEN" -d '{"title":"Groceries"}'
```

## Security

- **Passwords** are hashed with argon2id at OWASP's minimum parameters (19 MiB, 2 iterations, parallelism 1) and stored as PHC strings.
- **Hashing is bounded:** at most one hash per CPU runs at once. A request that can't get a slot within 2 s gets 503 instead of queueing, so memory stays capped and latency bounded under load.
- **Sign-up and sign-in are rate-limited** per client address (IPv6 per /64), 20 requests per minute by default. The limiter is in memory and per process. Proxy headers are not trusted, so every client behind a reverse proxy shares the proxy's address. Docker port publishing behaves the same way: under Compose, all host clients appear as the Docker gateway and share one bucket.
- **Tokens of deleted users** are rejected on every request: authentication checks that the user still exists.
- **Concurrent partial updates** use optimistic locking. A server-side update that loses a race is re-applied to the fresh state, up to 5 times with jittered backoff, so concurrent changes to different fields of the same list or item all land. Under heavier contention some requests get 409. Clients editing a stale copy still get last-write-wins, because versions are not exposed as ETags.
- **The database is the boundary for per-user isolation.** Every query filters by the owner, and CHECK constraints repeat the domain's validation rules.

Large-scale DDoS protection belongs at the edge (CDN or WAF), not in the application.

## Local development

To run the API on your machine with only the database in Docker:

```sh
cp .env.example .env   # set DB_PASSWORD and AUTH_SIGNING_KEY
make infra             # PostgreSQL on localhost:5436 + schema
make run               # exports .env, then go run ./cmd/api
```

Tests and checks:

```sh
make test        # all tests; repository and end-to-end tests start postgres:18.4 via testcontainers
make test-unit   # unit tests only, no Docker
make e2e         # end-to-end tests against the running `make up` stack
make lint        # golangci-lint and actionlint
make fmt         # gofmt and import grouping
make check       # lint and all tests, failing if Docker is unavailable; run before pushing
make vuln        # govulncheck
```

Linters and govulncheck are pinned in [`tools/`](tools) and run through `go tool`, so nothing needs installing. Run `make help` to list all targets.

## CI/CD

[`.github/workflows`](.github/workflows) holds two workflows. Both are **started manually only**, from the repository's Actions tab ("Run workflow"):

- **CI** runs lint, the full test suite with Docker, govulncheck, and the end-to-end suite against the Compose stack.
- **Release image** builds a linux/amd64 + linux/arm64 image from `main` and pushes it to `ghcr.io/<owner>/<repo>:<version>`, with provenance and an SBOM.

To run CI automatically, add `push` / `pull_request` triggers to `ci.yml`.

## Configuration

Everything is configured through environment variables. `make run` and `make up` read them from `.env`. Use plain `KEY=value` lines, since `make run` sources the file with the shell.

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `8000` | HTTP port |
| `DB_HOST` / `DB_PORT` | `localhost` / `5436` | PostgreSQL address |
| `DB_USERNAME` / `DB_DBNAME` / `DB_SSLMODE` | `postgres` / `postgres` / `disable` | PostgreSQL connection |
| `DB_PASSWORD` | none | PostgreSQL password (required). Generate one with `openssl rand -hex 32`. Avoid `/`, `+` and `=`: Compose puts it in a URL for migrations |
| `AUTH_SIGNING_KEY` | none | HS256 token signing key, at least 32 bytes (required). Generate one with `openssl rand -base64 48` |
| `AUTH_RATE_LIMIT_PER_MINUTE` | `20` (`300` in Compose) | Sign-up and sign-in requests allowed per client address per minute |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

The Compose stack supplies local-only defaults for `DB_PASSWORD` and `AUTH_SIGNING_KEY` when `.env` doesn't set them. Never reuse those values outside your machine. An `AUTH_SIGNING_KEY=` line left empty is passed through, and the app refuses to start.

## Logging

The app writes JSON lines to stdout, one per request:

```json
{"time":"…","level":"INFO","msg":"request","method":"GET","route":"/api/items/{id}","status":200,"duration_ms":0.4,"bytes":56,"client_ip":"192.168.107.1","request_id":"7F4PEA5TR3MYXRCQJMKVHCHKTR"}
```

- 4xx lines are logged at `WARN` and include the rejection `reason`; for example, a token's `reason` says why it was rejected.
- 5xx lines are logged at `ERROR` and include the internal `error`.
- `client_ip` is the direct peer.
- Successful health probes are logged at `DEBUG`. A failed readiness check also logs a warning.
- A request the client abandons, for example during password hashing, is logged as status 499 at `WARN`.

In Compose, every container uses Docker's `local` log driver and keeps at most 5 compressed files of 10 MB each. Docker rotates logs by size, not by age. The app container also reports health: its image runs `todo-app healthcheck` against `/readyz`.

## Architecture

The code follows domain-driven design with two bounded contexts:

```
cmd/api/              entry point and `healthcheck` subcommand
internal/
  api/                composition root: wires everything into one handler
  identity/           users, sign-up/sign-in, access tokens
    domain/           User aggregate, Name, Username and Password value objects, repository port
    app/              use cases; hasher and token ports
    password/ token/  argon2id and JWT adapters
    postgres/         repository adapter
    httpapi/          handlers and the auth middleware
  todo/               lists and items
    domain/           List and Item aggregates, Title and Description value objects, repository ports
    app/              use cases scoped to the calling owner, with optimistic-locking retries
    postgres/         repository adapters
    httpapi/          handlers and DTOs
  platform/           config, health, httpserver, httpx (HTTP helpers, logging, rate limiting), logging, postgres, retry
  e2e/                end-to-end HTTP tests
schema/               SQL schema (golang-migrate format)
docker/               Dockerfile and Compose stack
tools/                pinned developer tools
```

The domain packages depend on nothing else in the project. Infrastructure plugs in through ports, and the two contexts never import each other. [`AGENTS.md`](AGENTS.md) documents the conventions and invariants for contributors and AI coding agents.

## Changes from the 2022 version

- Code is reorganised from `pkg/handler|service|repository` into the bounded contexts above.
- gin, sqlx, viper, godotenv and logrus are replaced by the standard library, native pgx, caarlos0/env and `log/slog`. The binary links 10 external modules, down from 36.
- The schema stores the owner directly: `todo_lists.user_id` and `todo_items.list_id` replace the `users_lists` and `lists_items` join tables. Deleting a list also deletes its items.
- Protected routes live under `/api/...`. Before, they were under `/auth/api/...` by mistake.
- Errors use proper status codes (see the error table above) instead of a blanket 500. Creates answer 201, partial updates use `PATCH` instead of `PUT`, and item lists use the same `{"data": [...]}` envelope as lists.
- Usernames are case-insensitive.
- Passwords are hashed with argon2id instead of SHA-1, and `AUTH_SIGNING_KEY` replaces the hard-coded signing key.
