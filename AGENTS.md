# AGENTS.md

REST API for per-user todo lists and items, structured by DDD bounded contexts. Go 1.27 standard library HTTP, PostgreSQL via pgx, JWT auth, argon2id passwords. Commands live in the `Makefile` (`make help`).

## Architecture

```
cmd/api              entry point: config, logger, DB pool, server lifecycle, `healthcheck` subcommand
internal/api         composition root: wires repositories, services, adapters and middleware
internal/identity    bounded context: users, sign-up/sign-in, access tokens
internal/todo        bounded context: lists and their items
internal/platform    shared infrastructure: config, health, httpserver, httpx, logging, postgres (+ pgtest), retry
internal/e2e         end-to-end HTTP tests
```

Each bounded context has the same packages, and dependencies point inward only:

- `domain`: aggregates with unexported state, typed ids (`UserID`, `OwnerID`, `ListID`, `ItemID`), typed credentials (`PasswordHash`, `AccessToken`), value objects (`Title`, `Description`, `Name`, `Username`, `Password`) with validating constructors, `Apply(patch)` methods, domain errors and repository ports. Imports nothing from the project. Aggregate constructors take value objects; use cases parse raw input first (`domain.ParseText`, `domain.NewUsername`).
- `app`: use cases. Depends on `domain`, ports it declares itself (`PasswordHasher`, `TokenManager`) and `platform/retry`. Raw input with more than one string field arrives as a named struct (`SignUpInput`, `domain.TextInput`), so arguments cannot be swapped.
- `postgres`: repository adapters on `*pgxpool.Pool`. Private row structs map to domain types through `Restore*`, which trusts stored values.
- `httpapi`: `net/http` handlers, request/response DTOs and the mapping from domain errors to status codes.
- Identity-only adapters: `password` (argon2id) and `token` (JWT). They may import `identity/domain` and, for its errors, `identity/app`.

Rules:

- Contexts never import each other. The todo context learns the caller only as the `int64` id that `identity/httpapi.Authenticate` puts in the request context (`httpx.WithCallerID` / `httpx.CallerID`) and converts it to `domain.OwnerID`.
- The one sanctioned cross-context coupling is the `todo_lists.user_id → users.id` foreign key (`ON DELETE CASCADE`), kept for referential integrity in the shared database.
- Invariants live on the aggregate and its value objects. A use case loads, calls the domain method, then saves; handlers only decode, call one use case and map the result.
- Domain types carry no JSON or DB tags; DTOs live in `httpapi`, row structs in `postgres`. Domain error text is domain language; `httpapi` supplies transport wording such as "retry the request".
- No `any` or `interface{}`: use a concrete type or a type parameter, and check every type assertion. `make lint` enforces this (`forbidigo`, `revive`'s `use-any`, `forcetypeassert`) and allows only `[T any]` constraints. Where a library's signature forces `any`, use a `//nolint:forbidigo` with the reason.
- Every use case and repository method takes `ctx context.Context` first, except pure CPU work (token signing). Repositories wrap database errors with the failing operation (`insert todo list: ...`).
- A new operation is a vertical slice: domain method/port → use case → adapter → route, with tests at each layer it touches. New todo repository methods get an owner-scoping check in `internal/todo/postgres/repository_test.go`.

## Invariants

- **Ownership isolation**: a list belongs to `todo_lists.user_id`, an item to `todo_items.list_id`. Every todo repository query, inserts included, filters by the owner's user id, joining or selecting from `todo_lists` for items. Another owner's data is reported as not found, never as forbidden.
- **Optimistic concurrency**: users, lists and items carry `version`. Repository `Update` matches the loaded version and increments it, or returns `ErrConcurrentUpdate` on a mismatch. Update use cases wrap load-apply-save in `retry.OnConflict` (5 attempts, jittered backoff); persistent contention is a 409. Any new update path does the same. The exception is `ChangePassword`, which must not retry: the password it verified may no longer be current. Optimistic locking protects concurrent server-side patches, not clients editing stale copies (no ETag/If-Match).
- Schema lives in `schema/000001_init.*.sql` (golang-migrate format). Until a production database exists, change it in place; Compose and `pgtest` both apply it from scratch. CHECK constraints mirror the domain invariants that `Restore*` trusts: change both together.
- Usernames are stored lowercased (`NewUsername`), so the plain UNIQUE constraint is case-insensitive in effect.
- Configuration comes only from environment variables, parsed by `internal/platform/config` (struct tags carry names and defaults). Document new variables in `.env.example` and the README configuration table.
- Error mapping: `*domain.ValidationError` and malformed or unknown-field JSON → 400, invalid credentials, invalid tokens and tokens of deleted users → 401, `app.ErrWrongPassword` → 403, `ErrListNotFound`, `ErrItemNotFound` and unknown routes → 404, `ErrOwnerNotFound` and `ErrUserNotFound` on an authenticated route → 401, wrong method → 405, `ErrUsernameTaken` and `ErrConcurrentUpdate` → 409, oversized body → 413, rate limit → 429, `app.ErrHasherBusy` → 503, anything else → `httpx.InternalError` (generic 500 body; client cancellation logs as 499). Status codes and JSON shapes are the API contract: change them deliberately and say so in the commit body.
- Creates answer 201; partial updates are `PATCH`; collections use the `{"data": [...]}` envelope and are `[]`, never `null`.
- Password hashing is memory-hard (19 MiB per hash): it runs in bounded slots that shed load after 2s, and every route that verifies a password or reveals whether a username is taken (`/auth/*`, `/api/users/me/password`, `PATCH /api/users/me`) is rate-limited per client address, outside authentication. Keep any new hashing call behind the `PasswordHasher` port.

## HTTP and logging

- Register routes with `httpx.Handle(mux, method, path, h)`, which also serves the trailing-slash form. Read wildcards with `httpx.PathID`; decode bodies with `httpx.DecodeJSON[T]` (1 MiB, single value, unknown fields rejected).
- Answer through `httpx.JSON`, `httpx.OK`, `httpx.Error`, `httpx.ErrorWithReason` or `httpx.InternalError`; they hand the reason or error to the access log. Handlers do not log requests themselves.
- The chain is `RequestLogger → Recovery → JSONErrors → ServeMux`. Middleware in between must pass the same `*http.Request` on, or the logged route is lost (`internal/api/api_test.go` checks this). `RequestLogger` writes one `"msg":"request"` line per request (ERROR for 5xx, WARN for 4xx, INFO otherwise, DEBUG for successful health probes) with `request_id` and the matched route.
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
