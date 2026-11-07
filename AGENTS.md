# AGENTS.md

REST API for per-user todo lists and items, structured by DDD bounded contexts. Go 1.27 standard library HTTP, PostgreSQL via pgx, JWT auth, argon2id passwords. Commands live in the `Makefile` (`make help`).

## Architecture

```
cmd/api              entry point: config, logger, DB pool, server lifecycle, `healthcheck` subcommand
internal/api         composition root: wires repositories, services, adapters and middleware
internal/identity    bounded context: users, sign-up/sign-in, access tokens
internal/todo        bounded context: lists and their items
internal/platform    shared infrastructure: config, health, httpserver, httpx, logging, postgres (+ pgtest)
internal/e2e         end-to-end HTTP tests
```

Each bounded context has the same packages, and dependencies point inward only:

- `domain`: aggregates with unexported state, value objects (`Title`, `Description`, `Name`, `Username`) with validating constructors, `Apply(patch)` methods, domain errors and repository ports. Imports nothing from the project.
- `app`: use cases. Depends on `domain` plus ports it declares itself (`PasswordHasher`, `TokenManager`).
- `postgres`: repository adapters on `*pgxpool.Pool`. Private row structs map to domain types through `Restore*`, which trusts stored values.
- `httpapi`: `net/http` handlers, request/response DTOs and the mapping from domain errors to status codes.
- Identity-only adapters: `password` (argon2id) and `token` (JWT). They may import `identity/app` for its errors.

Rules:

- Contexts never import each other. The todo context learns the caller only as the `int64` user id that `identity/httpapi.Authenticate` puts in the request context (`httpx.WithUserID` / `httpx.UserID`).
- Invariants live on the aggregate and its value objects. A use case loads, calls the domain method, then saves; handlers only decode, call one use case and map the result.
- Domain types carry no JSON or DB tags; DTOs live in `httpapi`, row structs in `postgres`.
- Every use case and repository method takes `ctx context.Context` first, except pure CPU work (`Authenticate`, token signing).
- A new operation is a vertical slice: domain method/port → use case → adapter → route, with tests at each layer it touches. New todo repository methods get an owner-scoping check in `internal/todo/postgres/repository_test.go`.

## Invariants

- **Ownership isolation**: a list belongs to `todo_lists.user_id`, an item to `todo_items.list_id`. Every todo repository query, inserts included, filters by the owner's user id, joining or selecting from `todo_lists` for items. Another owner's data is reported as not found, never as forbidden.
- **Optimistic concurrency**: lists and items carry `version`. Repository `Update` matches the loaded version and increments it, returning `ErrConcurrentUpdate` on a mismatch; update use cases wrap load-apply-save in `retryOnConflict`. Any new update path does the same.
- Schema lives in `schema/000001_init.*.sql` (golang-migrate format). Until a production database exists, change it in place; Compose and `pgtest` both apply it from scratch.
- Configuration comes only from environment variables, parsed by `internal/platform/config` (struct tags carry names and defaults). Document new variables in `.env.example` and the README configuration table.
- Error mapping: `*domain.ValidationError` → 400, invalid credentials or token and `ErrOwnerNotFound` → 401, `Err*NotFound` → 404, `ErrUsernameTaken` and `ErrConcurrentUpdate` → 409, oversized body → 413, rate limit → 429, `app.ErrHasherBusy` → 503, anything else → `httpx.InternalError` (generic 500 body). Status codes and JSON shapes are the API contract: change them deliberately and say so in the commit body.
- Collections are returned as `[]`, never `null`. Collection routes are registered with and without a trailing slash; unknown routes return a JSON 404.
- Password hashing is memory-hard (19 MiB per hash): it runs in bounded slots that shed load after 2s, and `/auth` routes are rate-limited per client address. Keep any new hashing call behind the `PasswordHasher` port.

## HTTP and logging

- Routes are `http.ServeMux` patterns (`"GET /api/items/{id}"`); read wildcards with `httpx.PathID`, decode bodies with `httpx.DecodeJSON` (1 MiB limit).
- Answer through `httpx.JSON`, `httpx.OK`, `httpx.Error`, `httpx.ErrorWithReason` or `httpx.InternalError`; they hand the reason or error to the access log. Handlers do not log requests themselves.
- `httpx.RequestLogger` wraps the API mux and writes one `"msg":"request"` line per request (ERROR for 5xx, WARN for 4xx, INFO otherwise) with `request_id` and the matched pattern. `/healthz` and `/readyz` sit outside it.
- Other logs use `slog.*Context(ctx, ...)` so `request_id` is attached.

## Gotchas

- `rows, _ := db.Query(...)` followed by `pgx.CollectRows` is the pgx idiom: the query error surfaces from `CollectRows`.
- Developer tools are pinned in `tools/go.mod`, and actionlint separately in `tools/actionlint/go.mod` because their dependencies conflict. Run them with `make lint` / `make vuln`, not `go install`.
- `docker compose -f docker/compose.yaml` resolves `.env` from `docker/`, so the root `.env` is used only via `--env-file .env`; `make` targets pass it, and `make run` exports it as shell variables. PostgreSQL sets its password and runs the schema on first volume init only: after changing either, reset with `make down ARGS=-v`.
- `pgtest.Image` and Compose both pin `postgres:18.4`; change them together.
- GitHub workflows run only on manual dispatch by design; keep them that way unless asked.

## Verification

1. `make check`: golangci-lint (formatting, vet and the linters in `.golangci.yml`), actionlint, and `go test -race ./...`, including the repository tests and `internal/e2e`, which start PostgreSQL through `pgtest` (testcontainers). It sets `REQUIRE_DOCKER=1`, so a missing Docker fails instead of skipping; `make test-unit` skips them deliberately.
2. For Docker, Compose or schema changes, also run `make up` then `make e2e`, which runs the same suite against the container.

Done means both pass.

## Commits

One logical change per commit. Imperative subject up to 72 characters; the body explains what changed and why, in terms of the code.
