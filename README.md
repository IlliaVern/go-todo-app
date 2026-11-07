# go-todo-app

A REST API for personal todo lists, written in Go. Users sign up, get a JWT, and manage their own lists and the items in them. Every user sees only their own data.

**Stack:** Go 1.27 · [gin](https://github.com/gin-gonic/gin) · PostgreSQL 18 via [sqlx](https://github.com/jmoiron/sqlx) + [pgx](https://github.com/jackc/pgx) · [golang-jwt](https://github.com/golang-jwt/jwt) · bcrypt · `log/slog` · [viper](https://github.com/spf13/viper) · [golang-migrate](https://github.com/golang-migrate/migrate) · Docker Compose

## Quick start

Requires Docker with Compose v2 or later.

```sh
docker compose -f docker/compose.yaml up --build
```

This starts PostgreSQL, applies the migrations in `schema/`, and serves the API on <http://localhost:8000>. Without a `.env` file it uses local-only default credentials.

To use your own values, run `cp .env.example .env`, edit `.env`, then start with `make up`. `make up` passes `.env` to Compose and runs the stack in the background.

```sh
make up      # build and start everything
make logs    # follow logs
make down    # stop (make down ARGS=-v also deletes the database volume)
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
| `DELETE` | `/api/lists/:id` | | `{"status": "ok"}` |
| `POST` | `/api/lists/:id/items/` | `{"title", "description"?}` | `{"id"}` |
| `GET` | `/api/lists/:id/items/` | | `[item]` |
| `GET` | `/api/items/:id` | | `item` |
| `PUT` | `/api/items/:id` | `{"title"?, "description"?, "done"?}` | `{"status": "ok"}` |
| `DELETE` | `/api/items/:id` | | `{"status": "ok"}` |

Errors come back as `{"message": "..."}`.

```sh
curl -s localhost:8000/auth/sign-up -d '{"name":"Ada","username":"ada","password":"s3cret"}'
TOKEN=$(curl -s localhost:8000/auth/sign-in -d '{"username":"ada","password":"s3cret"}' | jq -r .token)
curl -s localhost:8000/api/lists/ -H "Authorization: Bearer $TOKEN" -d '{"title":"Groceries"}'
```

## Local development

To run the API on your machine with only the database in Docker:

```sh
cp .env.example .env   # set DB_PASSWORD and AUTH_SIGNING_KEY
make infra             # PostgreSQL on localhost:5436 + migrations
make run               # go run ./cmd
```

Run `make help` to list all targets. Before you push, run `make check` (gofmt, `go vet`, tests) and `make vuln` (govulncheck).

## Configuration

Non-secret settings live in [`configs/config.yml`](configs/config.yml). You can override any key there with an environment variable: write the key in upper case and replace dots with underscores (`db.host` → `DB_HOST`). The app loads `.env` from the working directory if one exists.

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `8000` | HTTP port |
| `DB_HOST` / `DB_PORT` | `localhost` / `5436` | PostgreSQL address |
| `DB_USERNAME` / `DB_DBNAME` / `DB_SSLMODE` | `postgres` / `postgres` / `disable` | PostgreSQL connection |
| `DB_PASSWORD` | none | PostgreSQL password (required) |
| `AUTH_SIGNING_KEY` | none | HS256 token signing key, at least 32 bytes (required). Generate one with `openssl rand -base64 48` |
| `GIN_MODE` | `debug` (`release` in the Docker image) | gin mode |

## Project layout

```
cmd/              entry point: config, database, HTTP server lifecycle
pkg/handler/      HTTP layer: routes, request binding, auth middleware
pkg/service/      application logic: validation, password hashing, JWT
pkg/repository/   PostgreSQL access with sqlx
*.go (root)       domain types and errors (package todo)
schema/           golang-migrate SQL migrations
configs/          default configuration
docker/           Dockerfile and Compose stack
```

Requests flow `handler → service → repository`, and each layer depends only on the interfaces of the layer below it. [`AGENTS.md`](AGENTS.md) covers conventions and invariants for contributors and AI coding agents.

## Upgrading from the 2022 version

- Passwords are hashed with bcrypt instead of SHA-1. Accounts created before this change can no longer sign in and must be created again.
- Protected routes now live under `/api/...`. Before, they were under `/auth/api/...` by mistake.
- `AUTH_SIGNING_KEY` is now required. The signing key is no longer hard-coded.
