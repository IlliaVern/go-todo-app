# go-todo-app

A REST API for personal todo lists, written in Go. Users sign up, get a JWT, and manage their own lists and the items in them. Every user sees only their own data.

**Stack:** Go 1.27 · [gin](https://github.com/gin-gonic/gin) · PostgreSQL 18 via [sqlx](https://github.com/jmoiron/sqlx) + [pgx](https://github.com/jackc/pgx) · [golang-jwt](https://github.com/golang-jwt/jwt) · bcrypt · `log/slog` · [viper](https://github.com/spf13/viper) · [golang-migrate](https://github.com/golang-migrate/migrate) · [testcontainers-go](https://golang.testcontainers.org) · Docker Compose

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
| `POST` | `/auth/sign-up` | `{"name", "username", "password"}` | `{"id"}` |
| `POST` | `/auth/sign-in` | `{"username", "password"}` | `{"token"}` (valid for 12 h) |
| `POST` | `/api/lists/` | `{"title", "description"?}` | `{"id"}` |
| `GET` | `/api/lists/` | | `{"data": [list]}` |
| `GET` | `/api/lists/:id` | | `list` |
| `PUT` | `/api/lists/:id` | `{"title"?, "description"?}` | `{"status": "ok"}` |
| `DELETE` | `/api/lists/:id` | | `{"status": "ok"}` (deletes its items too) |
| `POST` | `/api/lists/:id/items/` | `{"title", "description"?}` | `{"id"}` |
| `GET` | `/api/lists/:id/items/` | | `[item]` |
| `GET` | `/api/items/:id` | | `item` |
| `PUT` | `/api/items/:id` | `{"title"?, "description"?, "done"?}` | `{"status": "ok"}` |
| `DELETE` | `/api/items/:id` | | `{"status": "ok"}` |

`list` is `{"id", "title", "description"}` and `item` is `{"id", "title", "description", "done"}`. Empty collections come back as `[]`.

Errors come back as `{"message": "..."}`:

| Status | When |
|---|---|
| 400 | Malformed JSON, invalid id, failed validation: blank title, a field over 255 characters, password empty or over 72 bytes, or an update with no fields |
| 401 | Missing or invalid token, or wrong username or password |
| 404 | The list or item doesn't exist or belongs to another user |
| 409 | Username already taken |
| 500 | Unexpected failure. Details are logged, not returned |

Every response carries an `X-Request-ID` header. The server reuses yours if you send one (printable ASCII, up to 128 characters) and otherwise generates one. The one exception is gin's automatic trailing-slash redirect (`/api/lists` → `/api/lists/`), which is answered before any middleware runs.

```sh
curl -s localhost:8000/auth/sign-up -d '{"name":"Ada","username":"ada","password":"s3cret"}'
TOKEN=$(curl -s localhost:8000/auth/sign-in -d '{"username":"ada","password":"s3cret"}' | jq -r .token)
curl -s localhost:8000/api/lists/ -H "Authorization: Bearer $TOKEN" -d '{"title":"Groceries"}'
```

## Local development

To run the API on your machine with only the database in Docker:

```sh
cp .env.example .env   # set DB_PASSWORD and AUTH_SIGNING_KEY
make infra             # PostgreSQL on localhost:5436 + schema
make run               # go run ./cmd/api
```

Tests:

```sh
make test        # unit tests + end-to-end tests (starts postgres:18.4 via testcontainers; needs Docker)
make test-unit   # unit tests only
make e2e         # end-to-end tests against the running `make up` stack
make check       # gofmt, go vet and all tests; run before pushing
make vuln        # govulncheck
```

Run `make help` to list all targets.

## Configuration

Non-secret settings live in [`configs/config.yml`](configs/config.yml). You can override any key there with an environment variable: write the key in upper case and replace dots with underscores (`db.host` → `DB_HOST`). The app loads `.env` from the working directory if one exists.

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `8000` | HTTP port |
| `DB_HOST` / `DB_PORT` | `localhost` / `5436` | PostgreSQL address |
| `DB_USERNAME` / `DB_DBNAME` / `DB_SSLMODE` | `postgres` / `postgres` / `disable` | PostgreSQL connection |
| `DB_PASSWORD` | none | PostgreSQL password (required) |
| `AUTH_SIGNING_KEY` | none | HS256 token signing key, at least 32 bytes (required). Generate one with `openssl rand -base64 48` |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `GIN_MODE` | `release` | Set to `debug` for gin's route and request debug output (plain text, not JSON) |

The Compose stack supplies local-only defaults for `DB_PASSWORD` and `AUTH_SIGNING_KEY` when `.env` doesn't set them. Never reuse those values outside your machine. An `AUTH_SIGNING_KEY=` line left empty is passed through, and the app refuses to start.

## Logging

The app writes JSON lines to stdout, one per request (trailing-slash redirects excepted):

```json
{"time":"…","level":"INFO","msg":"request","method":"GET","route":"/api/items/:id","status":200,"duration_ms":0.4,"bytes":55,"client_ip":"192.168.107.1","request_id":"7F4PEA5TR3MYXRCQJMKVHCHKTR"}
```

- 4xx lines are logged at `WARN` and include the rejection `reason`.
- 5xx lines are logged at `ERROR` and include the internal `error`.
- `client_ip` is the direct peer. Proxy headers are not trusted.

In Compose, every container uses Docker's `local` log driver and keeps at most 5 compressed files of 10 MB each. Docker rotates logs by size, not by age.

## Architecture

The code follows domain-driven design with two bounded contexts:

```
cmd/api/              entry point
internal/
  api/                composition root: wires everything into one router
  identity/           users, sign-up/sign-in, access tokens
    domain/           User aggregate, invariants, repository port
    app/              use cases; hasher and token ports
    password/ token/  bcrypt and JWT adapters
    postgres/         repository adapter
    httpapi/          gin handlers and the auth middleware
  todo/               lists and items
    domain/           List and Item aggregates, invariants, repository ports
    app/              use cases, all scoped to the calling owner
    postgres/         repository adapters
    httpapi/          gin handlers and DTOs
  platform/           config, logging, postgres, httpserver, httpx (shared HTTP helpers)
  e2e/                end-to-end HTTP tests
schema/               SQL schema (golang-migrate format)
configs/              default configuration
docker/               Dockerfile and Compose stack
```

The domain packages depend on nothing else in the project. Infrastructure plugs in through ports, and the two contexts never import each other. [`AGENTS.md`](AGENTS.md) documents the conventions and invariants for contributors and AI coding agents.

## Changes from the 2022 version

- Code is reorganised from `pkg/handler|service|repository` into the bounded contexts above.
- The schema stores the owner directly: `todo_lists.user_id` and `todo_items.list_id` replace the `users_lists` and `lists_items` join tables. Deleting a list also deletes its items.
- Protected routes live under `/api/...`. Before, they were under `/auth/api/...` by mistake.
- Errors use proper status codes (see the error table above) instead of a blanket 500.
- Passwords are hashed with bcrypt instead of SHA-1, and `AUTH_SIGNING_KEY` replaces the hard-coded signing key.
