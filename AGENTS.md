# AGENTS.md

REST API for per-user todo lists and items. Go 1.27, gin, PostgreSQL via sqlx + pgx, JWT auth. Commands live in the `Makefile` (`make help`).

## Architecture

Layered, dependencies point inward only:

```
cmd/main.go          wiring: config, DB, server lifecycle
pkg/handler          HTTP (gin): binding, auth middleware, status codes
pkg/service          use cases: validation, password hashing, JWT
pkg/repository       raw SQL against PostgreSQL
todo (repo root)     domain types, update inputs and their Validate(), domain errors
```

- The root package `todo` imports no project package. Handlers call only services; services call only repository interfaces.
- Each layer exposes interfaces (`pkg/service/service.go`, `pkg/repository/repository.go`) aggregated into a `Service` / `Repository` struct. A new operation is a vertical slice: repository interface + `*_postgres.go` impl → service interface + impl → handler + route in `pkg/handler/handler.go`.
- Every service and repository method takes `ctx context.Context` first; handlers pass `c.Request.Context()`, repositories use the `*Context` sqlx methods.
- Business rules (validation, hashing, ownership checks before writes) belong in `pkg/service` or on domain types, not in handlers.

## Invariants

- **Ownership isolation**: lists belong to users through `users_lists`, items to lists through `lists_items`. Every list or item query joins down to `users_lists.user_id` and filters on the caller's id. A query without that join leaks other users' data.
- Placeholders are positional (`$1`, `$2`); in dynamic `UPDATE` builders, append args in the same order the placeholders are numbered.
- Table names come from the constants in `pkg/repository/postgres.go`.
- Schema changes are a new migration pair in `schema/` (`00000N_name.up.sql` + `.down.sql`, golang-migrate format). Applied migrations stay unchanged.
- Secrets (`DB_PASSWORD`, `AUTH_SIGNING_KEY`) come only from the environment. Non-secret settings live in `configs/config.yml`; any key there is overridable by env as UPPER_SNAKE_CASE (`db.host` → `DB_HOST`). Document new variables in `.env.example`.
- HTTP status codes and JSON shapes are the API contract. Change them deliberately and say so in the commit body.

## Gotchas

- Collection routes end with a slash (`/api/lists/`, `/api/lists/:id/items/`); gin redirects the slashless form.
- Not-found and validation failures currently return 500 (only sign-in maps to 401). This is existing behaviour, not a pattern to copy.
- `docker compose -f docker/compose.yaml` resolves `.env` from `docker/`, so the root `.env` is used only via `--env-file .env`; `make` targets pass it. PostgreSQL sets its password on first volume init only: after changing `DB_PASSWORD`, reset with `make down ARGS=-v`.
- Passwords are bcrypt hashes; the `password_hash` column is compared in the service, never in SQL.

## Verification

1. `make check` (gofmt, `go vet`, `go test -race`). There are no Go tests yet; new logic gets table-driven tests next to it.
2. For changes to handlers, services, repositories, schema or Docker files: `make up`, then run `.claude/skills/smoke-test/smoke.py` (see its `SKILL.md`). Done means every check prints `PASS`.

## Commits

One logical change per commit. Imperative subject up to 72 characters; the body explains what changed and why, in terms of the code.
