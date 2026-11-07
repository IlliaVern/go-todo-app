# AGENTS.md

REST API for per-user todo lists and items, structured by DDD bounded contexts. Go 1.27, gin, PostgreSQL via sqlx + pgx, JWT auth. Commands live in the `Makefile` (`make help`).

## Architecture

```
cmd/api              entry point: config, logger, DB, server lifecycle
internal/api         composition root: wires repositories, services, adapters into the router
internal/identity    bounded context: users, sign-up/sign-in, access tokens
internal/todo        bounded context: lists and their items
internal/platform    shared infrastructure: config, logging, postgres, httpserver, httpx
internal/e2e         end-to-end HTTP tests
```

Each bounded context has the same packages, and dependencies point inward only:

- `domain`: aggregates with unexported state, constructors, `Apply(patch)` methods enforcing invariants, domain errors and repository ports. Imports nothing from the project.
- `app`: use cases. Depends on `domain` plus ports it declares itself (`PasswordHasher`, `TokenManager`).
- `postgres`: repository adapters. Private row structs map to domain types through `Restore*` constructors.
- `httpapi`: gin handlers, request/response DTOs and the mapping from domain errors to status codes.
- Identity-only adapters: `password` (bcrypt) and `token` (JWT).

Rules:

- Contexts never import each other. The todo context learns the caller only as the `int64` user id that `identity/httpapi.Authenticate` stores via `httpx.SetUserID`.
- Invariants live on the aggregate. A use case loads, calls the domain method, then saves; handlers only bind, call one use case and map the result.
- Domain types carry no JSON or DB tags; DTOs live in `httpapi`, row structs in `postgres`.
- Every use case and repository method takes `ctx context.Context` first, except pure CPU work (`Authenticate`, token and hash adapters).
- A new operation is a vertical slice: domain method/port → use case → adapter → route, with tests at each layer it touches.

## Invariants

- **Ownership isolation**: a list belongs to `todo_lists.user_id`, an item to `todo_items.list_id`. Every todo repository query filters by the owner's user id, joining `todo_lists` for items. Another owner's data is reported as not found, never as forbidden.
- Schema lives in `schema/000001_init.*.sql` (golang-migrate format). Until a production database exists, change it in place; Compose and the e2e suite both apply it from scratch.
- Secrets (`DB_PASSWORD`, `AUTH_SIGNING_KEY`) come only from the environment. Non-secret settings live in `configs/config.yml`; any key there is overridable by env as UPPER_SNAKE_CASE (`db.host` → `DB_HOST`). Document new variables in `.env.example` and the README configuration table.
- Error mapping: `*domain.ValidationError` → 400, invalid credentials or token → 401, `Err*NotFound` → 404, `ErrUsernameTaken` → 409, anything else → `httpx.InternalError` (generic 500 body). Rejections go through `httpx.Error`, which hands the reason to the access log. Status codes and JSON shapes are the API contract: change them deliberately and say so in the commit body.
- Collections are returned as `[]`, never `null`. Unknown routes return a JSON 404.

## Logging

- `httpx.RequestLogger` writes exactly one `"msg":"request"` line per request (gin's trailing-slash redirects bypass middleware and are not logged) (ERROR for 5xx, WARN for 4xx, INFO otherwise) carrying `request_id`. Handlers record outcomes through `httpx.Error` / `httpx.InternalError`; they do not log requests themselves.
- Other logs use `slog.*Context(ctx, ...)` with the request context so `request_id` is attached.

## Gotchas

- Collection routes end with a slash (`/api/lists/`, `/api/lists/:id/items/`); gin redirects the slashless form.
- `docker compose -f docker/compose.yaml` resolves `.env` from `docker/`, so the root `.env` is used only via `--env-file .env`; `make` targets pass it. PostgreSQL sets its password and runs the schema on first volume init only: after changing either, reset with `make down ARGS=-v`.
- The e2e suite and Compose both pin `postgres:18.4`; change them together.

## Verification

1. `make check`: gofmt, `go vet`, and `go test -race ./...`, including `internal/e2e`, which starts PostgreSQL with testcontainers (needs Docker; `make test-unit` skips it).
2. For Docker, Compose or schema changes, also run `make up` then `make e2e`, which runs the same suite against the container.

Done means both pass with no skipped e2e test.

## Commits

One logical change per commit. Imperative subject up to 72 characters; the body explains what changed and why, in terms of the code.
