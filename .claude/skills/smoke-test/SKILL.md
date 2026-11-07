---
name: smoke-test
description: Smoke-test the running API end to end against the Docker stack. Use after changing handlers, services, repositories, schema/ migrations or Docker files, or when asked to verify the API works.
---

1. Start the stack with `make up`. Ready when `docker compose -f docker/compose.yaml ps -a` shows `postgres` healthy, `migrate` exited with code 0 and `app` running.
2. Run `python3 -I .claude/skills/smoke-test/smoke.py`. For an API on another address, set `BASE_URL` (e.g. `BASE_URL=http://localhost:8001` for `PORT=8001 make run`).
3. Done when the last line reads `FAILURES: 0`. On a `FAIL`, read `make logs` for the matching `request failed` entry before changing code.

When an endpoint's contract changes on purpose, update its expected status in `smoke.py` in the same commit. A new endpoint gets a check in the same flow.
