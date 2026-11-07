"""End-to-end smoke test for the todo API.

Usage: python3 -I .claude/skills/smoke-test/smoke.py   (BASE_URL defaults to http://localhost:8000)

Expected status codes encode the API's current contract, including the 500s it
returns for validation failures and missing entities. Exits non-zero on any FAIL.
"""
import json, os, random, sys, urllib.request, urllib.error
B = os.environ.get("BASE_URL", "http://localhost:8000")
fails = 0
def req(method, path, body=None, token=None):
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(B + path, data=data, method=method, headers={"Content-Type": "application/json"})
    if token: r.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(r) as resp: code, raw = resp.status, resp.read()
    except urllib.error.HTTPError as e: code, raw = e.code, e.read()
    try: return code, json.loads(raw)
    except Exception: return code, raw.decode()
def check(name, got, want):
    global fails
    ok = got[0] == want
    fails += not ok
    print(f"{'PASS' if ok else 'FAIL'} {name:<28} {got[0]} {got[1]}")
    return got[1]
u = f"smoke{random.randint(0, 10**9)}"
check("sign-up", req("POST", "/auth/sign-up", {"name": "S", "username": u, "password": "pw123456"}), 200)
check("sign-in wrong password", req("POST", "/auth/sign-in", {"username": u, "password": "nope"}), 401)
check("sign-in unknown user", req("POST", "/auth/sign-in", {"username": u + "x", "password": "nope"}), 401)
t = check("sign-in", req("POST", "/auth/sign-in", {"username": u, "password": "pw123456"}), 200)["token"]
check("no token", req("GET", "/api/lists/"), 401)
check("forged token", req("GET", "/api/lists/", token="a.b.c"), 401)
check("legacy /auth/api path", req("GET", "/auth/api/lists/", token=t), 404)
l = check("create list", req("POST", "/api/lists/", {"title": "L1", "description": "d"}, t), 200)["id"]
check("get lists", req("GET", "/api/lists/", token=t), 200)
check("get list", req("GET", f"/api/lists/{l}", token=t), 200)
check("update list", req("PUT", f"/api/lists/{l}", {"title": "L1-upd"}, t), 200)
check("get updated list", req("GET", f"/api/lists/{l}", token=t), 200)
check("update list empty", req("PUT", f"/api/lists/{l}", {}, t), 500)
i = check("create item", req("POST", f"/api/lists/{l}/items/", {"title": "I1"}, t), 200)["id"]
check("get items", req("GET", f"/api/lists/{l}/items/", token=t), 200)
check("update item", req("PUT", f"/api/items/{i}", {"done": True, "title": "I1-upd"}, t), 200)
got = check("get updated item", req("GET", f"/api/items/{i}", token=t), 200)
if not (got.get("done") is True and got.get("title") == "I1-upd"): fails += 1; print("FAIL item update not applied")
check("update item empty", req("PUT", f"/api/items/{i}", {}, t), 500)
check("delete item", req("DELETE", f"/api/items/{i}", token=t), 200)
check("get deleted item", req("GET", f"/api/items/{i}", token=t), 500)
check("delete list", req("DELETE", f"/api/lists/{l}", token=t), 200)
check("get deleted list", req("GET", f"/api/lists/{l}", token=t), 500)
print("FAILURES:", fails); sys.exit(1 if fails else 0)
