// Package e2e_test exercises the whole API over HTTP against a real
// PostgreSQL. By default it starts one with pgtest and serves the router
// in-process; set E2E_BASE_URL (e.g. http://localhost:8000 after `make up`) to
// test a running deployment instead.
package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/IlliaVern/go-todo-app/internal/api"
	"github.com/IlliaVern/go-todo-app/internal/identity/token"
	"github.com/IlliaVern/go-todo-app/internal/platform/postgres/pgtest"
)

const e2eSigningKey = "e2e-signing-key-0123456789abcdef"

func startAPI(t *testing.T) string {
	t.Helper()
	if baseURL := os.Getenv("E2E_BASE_URL"); baseURL != "" {
		return strings.TrimSuffix(baseURL, "/")
	}

	db := pgtest.Start(t)
	handler, err := api.NewHandler(db, api.Options{
		SigningKey:            e2eSigningKey,
		AuthRequestsPerMinute: 1000, // the suite signs up many users from one address
		Logger:                slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv.URL
}

type client struct {
	t     *testing.T
	base  string
	token string
}

type response struct {
	status int
	header http.Header
	body   []byte
}

// do sends a request without a body.
func (c client) do(method, path string) response {
	c.t.Helper()
	return c.raw(method, path, "")
}

// raw sends body as is, so tests can send malformed JSON.
func (c client) raw(method, path, body string) response {
	c.t.Helper()
	req, err := http.NewRequestWithContext(c.t.Context(), method, c.base+path, strings.NewReader(body))
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		// Fail on redirects instead of following them: the API never redirects.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	return response{status: resp.StatusCode, header: resp.Header, body: data}
}

// send marshals body to JSON and sends it.
func send[T any](c client, method, path string, body T) response {
	c.t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		c.t.Fatal(err)
	}
	return c.raw(method, path, string(data))
}

func (c client) expect(r response, status int) {
	c.t.Helper()
	if r.status != status {
		c.t.Fatalf("status = %d, want %d; body: %s", r.status, status, r.body)
	}
}

// decode asserts the status and returns the JSON body as a T.
func decode[T any](c client, r response, status int) T {
	c.t.Helper()
	c.expect(r, status)
	var out T
	if err := json.Unmarshal(r.body, &out); err != nil {
		c.t.Fatalf("decode %s: %v", r.body, err)
	}
	return out
}

func ptr[T any](v T) *T { return &v }

var userSeq atomic.Int64

// signUp registers a fresh user and returns a client authenticated as them.
func signUp(t *testing.T, base string) client {
	t.Helper()
	username := fmt.Sprintf("user-%d-%d", time.Now().UnixNano(), userSeq.Add(1))
	anon := client{t: t, base: base}
	anon.expect(send(anon, "POST", "/auth/sign-up", map[string]string{"name": "Test", "username": username, "password": "s3cret"}), http.StatusCreated)
	return signIn(t, base, username, "s3cret")
}

func signIn(t *testing.T, base, username, password string) client {
	t.Helper()
	anon := client{t: t, base: base}
	body := decode[tokenBody](anon, send(anon, "POST", "/auth/sign-in", map[string]string{"username": username, "password": password}), http.StatusOK)
	if body.Token == "" {
		t.Fatal("empty token")
	}
	return client{t: t, base: base, token: body.Token}
}

type tokenBody struct{ Token string }

type idBody struct{ ID int64 }

type statusBody struct{ Status string }

type listBody struct {
	ID          int64
	Title       string
	Description string
}

type itemBody struct {
	ID          int64
	Title       string
	Description string
	Done        bool
}

type listsBody struct{ Data []listBody }

type itemsBody struct{ Data []itemBody }

type messageBody struct{ Message string }

type itemPatch struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	Done        *bool   `json:"done,omitempty"`
}

func TestAPI(t *testing.T) {
	base := startAPI(t)

	t.Run("probes", func(t *testing.T) {
		anon := client{t: t, base: base}
		anon.expect(anon.do("GET", "/healthz"), http.StatusOK)
		if id := anon.do("GET", "/healthz").header.Get("X-Request-ID"); id == "" {
			t.Error("probe response has no X-Request-ID")
		}
		ready := decode[statusBody](anon, anon.do("GET", "/readyz"), http.StatusOK)
		if ready.Status != "ready" {
			t.Errorf("readyz status = %q", ready.Status)
		}
	})

	t.Run("auth", func(t *testing.T) {
		anon := client{t: t, base: base}
		username := fmt.Sprintf("auth-%d", time.Now().UnixNano())
		creds := map[string]string{"name": "Ada", "username": username, "password": "s3cret"}

		created := decode[idBody](anon, send(anon, "POST", "/auth/sign-up", creds), http.StatusCreated)
		if created.ID <= 0 {
			t.Fatalf("id = %d", created.ID)
		}

		msg := decode[messageBody](anon, send(anon, "POST", "/auth/sign-up", creds), http.StatusConflict)
		if msg.Message != "username is already taken" {
			t.Errorf("duplicate message = %q", msg.Message)
		}

		anon.expect(send(anon, "POST", "/auth/sign-up", map[string]string{"name": "B", "username": username + "-b", "password": ""}), http.StatusBadRequest)
		anon.expect(send(anon, "POST", "/auth/sign-up", map[string]string{"name": "B", "username": username + "-b", "password": strings.Repeat("p", 1025)}), http.StatusBadRequest)
		anon.expect(anon.raw("POST", "/auth/sign-up", `{"name":`), http.StatusBadRequest)
		anon.expect(send(anon, "POST", "/auth/sign-up", map[string]string{"name": "B\x00", "username": username + "-c", "password": "x"}), http.StatusBadRequest)
		anon.expect(send(anon, "POST", "/auth/sign-in", map[string]string{"username": "a\x00b", "password": "x"}), http.StatusUnauthorized)

		wrongPassword := decode[messageBody](anon, send(anon, "POST", "/auth/sign-in", map[string]string{"username": username, "password": "wrong"}), http.StatusUnauthorized)
		unknownUser := decode[messageBody](anon, send(anon, "POST", "/auth/sign-in", map[string]string{"username": username + "-nobody", "password": "wrong"}), http.StatusUnauthorized)
		if unknownUser.Message != wrongPassword.Message {
			t.Errorf("unknown user and wrong password must look identical: %q vs %q", unknownUser.Message, wrongPassword.Message)
		}
	})

	t.Run("authentication is required", func(t *testing.T) {
		user := signUp(t, base)
		for _, tc := range []struct {
			name   string
			header string
		}{
			{"missing header", ""},
			{"forged token", "Bearer a.b.c"},
			{"wrong scheme", "Basic " + user.token},
			{"no token", "Bearer "},
		} {
			t.Run(tc.name, func(t *testing.T) {
				req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, base+"/api/lists/", nil)
				if tc.header != "" {
					req.Header.Set("Authorization", tc.header)
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				resp.Body.Close()
				if resp.StatusCode != http.StatusUnauthorized {
					t.Errorf("status = %d, want 401", resp.StatusCode)
				}
			})
		}

		msg := decode[messageBody](user, user.do("GET", "/auth/api/lists/"), http.StatusNotFound)
		if msg.Message != "route not found" {
			t.Errorf("unknown route message = %q, want JSON error", msg.Message)
		}
	})

	t.Run("token for a user that no longer exists", func(t *testing.T) {
		if os.Getenv("E2E_BASE_URL") != "" {
			t.Skip("needs the in-process server's signing key")
		}
		issuer, err := token.NewJWT(e2eSigningKey, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		tok, err := issuer.Issue(987654321)
		if err != nil {
			t.Fatal(err)
		}
		ghost := client{t: t, base: base, token: string(tok)}
		ghost.expect(send(ghost, "POST", "/api/lists/", map[string]string{"title": "x"}), http.StatusUnauthorized)
		ghost.expect(ghost.do("GET", "/api/lists/"), http.StatusUnauthorized)
		ghost.expect(ghost.do("GET", "/api/items/1"), http.StatusUnauthorized)
	})

	t.Run("lists and items", func(t *testing.T) {
		user := signUp(t, base)

		for _, path := range []string{"/api/lists/", "/api/lists"} {
			if r := user.do("GET", path); r.status != http.StatusOK {
				t.Errorf("GET %s = %d, want 200 with or without trailing slash", path, r.status)
			}
		}
		if r := user.do("GET", "/api/lists/"); strings.TrimSpace(string(r.body)) != `{"data":[]}` {
			t.Errorf("empty lists body = %s, want {\"data\":[]}", r.body)
		}

		list := decode[idBody](user, send(user, "POST", "/api/lists/", map[string]string{"title": "Groceries", "description": "weekly"}), http.StatusCreated)
		lists := decode[listsBody](user, user.do("GET", "/api/lists/"), http.StatusOK)
		if len(lists.Data) != 1 || lists.Data[0] != (listBody{ID: list.ID, Title: "Groceries", Description: "weekly"}) {
			t.Errorf("lists = %+v", lists.Data)
		}

		listPath := fmt.Sprintf("/api/lists/%d", list.ID)
		user.expect(send(user, "PATCH", listPath, map[string]string{"title": "Food"}), http.StatusOK)
		got := decode[listBody](user, user.do("GET", listPath), http.StatusOK)
		if got != (listBody{ID: list.ID, Title: "Food", Description: "weekly"}) {
			t.Errorf("updated list = %+v", got)
		}

		msg := decode[messageBody](user, user.raw("PATCH", listPath, `{}`), http.StatusBadRequest)
		if msg.Message != "update structure has no values" {
			t.Errorf("empty update message = %q", msg.Message)
		}
		user.expect(send(user, "PATCH", listPath, map[string]string{"title": " "}), http.StatusBadRequest)
		user.expect(send(user, "POST", "/api/lists/", map[string]string{"title": ""}), http.StatusBadRequest)
		user.expect(send(user, "POST", "/api/lists/", map[string]string{"title": "a\x00b"}), http.StatusBadRequest)
		user.expect(user.do("GET", "/api/lists/abc"), http.StatusBadRequest)
		user.expect(user.do("GET", "/api/lists/999999999"), http.StatusNotFound)

		itemsPath := listPath + "/items/"
		if r := user.do("GET", itemsPath); r.status != http.StatusOK || strings.TrimSpace(string(r.body)) != `{"data":[]}` {
			t.Errorf("empty items = %d %s, want 200 {\"data\":[]}", r.status, r.body)
		}

		item := decode[idBody](user, send(user, "POST", itemsPath, map[string]string{"title": "Milk"}), http.StatusCreated)
		itemPath := fmt.Sprintf("/api/items/%d", item.ID)

		user.expect(send(user, "PATCH", itemPath, itemPatch{Done: ptr(true), Title: ptr("Oat milk")}), http.StatusOK)
		gotItem := decode[itemBody](user, user.do("GET", itemPath), http.StatusOK)
		if gotItem != (itemBody{ID: item.ID, Title: "Oat milk", Done: true}) {
			t.Errorf("updated item = %+v", gotItem)
		}
		items := decode[itemsBody](user, user.do("GET", itemsPath), http.StatusOK)
		if len(items.Data) != 1 || items.Data[0] != gotItem {
			t.Errorf("items = %+v", items)
		}
		user.expect(user.raw("PATCH", itemPath, `{}`), http.StatusBadRequest)
		user.expect(send(user, "PATCH", itemPath+"/", itemPatch{Done: ptr(false)}), http.StatusOK)
		user.expect(user.raw("PATCH", itemPath, `{"done":true,"colour":"red"}`), http.StatusBadRequest)
		user.expect(user.raw("PATCH", itemPath, `{"done":true} {"done":false}`), http.StatusBadRequest)
		notAllowed := decode[messageBody](user, send(user, "PUT", itemPath, itemPatch{Done: ptr(true)}), http.StatusMethodNotAllowed)
		if notAllowed.Message != "method not allowed" {
			t.Errorf("405 message = %q", notAllowed.Message)
		}

		user.expect(user.do("DELETE", itemPath), http.StatusOK)
		user.expect(user.do("GET", itemPath), http.StatusNotFound)
		user.expect(user.do("DELETE", itemPath), http.StatusNotFound)

		second := decode[idBody](user, send(user, "POST", itemsPath, map[string]string{"title": "Bread"}), http.StatusCreated)
		user.expect(user.do("DELETE", listPath), http.StatusOK)
		user.expect(user.do("GET", listPath), http.StatusNotFound)
		user.expect(user.do("GET", fmt.Sprintf("/api/items/%d", second.ID)), http.StatusNotFound)
		user.expect(user.do("DELETE", listPath), http.StatusNotFound)
	})

	t.Run("concurrent patches to different fields all apply", func(t *testing.T) {
		user := signUp(t, base)
		list := decode[idBody](user, send(user, "POST", "/api/lists/", map[string]string{"title": "L"}), http.StatusCreated)
		item := decode[idBody](user, send(user, "POST", fmt.Sprintf("/api/lists/%d/items/", list.ID), map[string]string{"title": "old"}), http.StatusCreated)
		itemPath := fmt.Sprintf("/api/items/%d", item.ID)

		patches := []itemPatch{{Title: ptr("new")}, {Done: ptr(true)}, {Description: ptr("d")}}
		for _, status := range concurrently(user, "PATCH", itemPath, patches) {
			if status != http.StatusOK {
				t.Errorf("concurrent PATCH status = %d, want 200", status)
			}
		}

		got := decode[itemBody](user, user.do("GET", itemPath), http.StatusOK)
		if got != (itemBody{ID: item.ID, Title: "new", Description: "d", Done: true}) {
			t.Errorf("item after concurrent patches = %+v, want all three applied", got)
		}
	})

	t.Run("heavy contention answers 200 or 409, never 500", func(t *testing.T) {
		user := signUp(t, base)
		list := decode[idBody](user, send(user, "POST", "/api/lists/", map[string]string{"title": "L"}), http.StatusCreated)
		listPath := fmt.Sprintf("/api/lists/%d", list.ID)

		patches := make([]map[string]string, 20)
		for i := range patches {
			patches[i] = map[string]string{"title": fmt.Sprintf("t-%d", i)}
		}
		ok := 0
		for _, status := range concurrently(user, "PATCH", listPath, patches) {
			switch status {
			case http.StatusOK:
				ok++
			case http.StatusConflict:
			default:
				t.Errorf("status = %d, want 200 or 409", status)
			}
		}
		if ok == 0 {
			t.Error("no concurrent update succeeded")
		}
	})

	t.Run("users cannot touch each other's data", func(t *testing.T) {
		alice, bob := signUp(t, base), signUp(t, base)

		list := decode[idBody](alice, send(alice, "POST", "/api/lists/", map[string]string{"title": "Alice's"}), http.StatusCreated)
		listPath := fmt.Sprintf("/api/lists/%d", list.ID)
		item := decode[idBody](alice, send(alice, "POST", listPath+"/items/", map[string]string{"title": "secret"}), http.StatusCreated)
		itemPath := fmt.Sprintf("/api/items/%d", item.ID)

		bobLists := decode[listsBody](bob, bob.do("GET", "/api/lists/"), http.StatusOK)
		if len(bobLists.Data) != 0 {
			t.Errorf("bob sees lists %+v", bobLists.Data)
		}
		bob.expect(bob.do("GET", listPath), http.StatusNotFound)
		bob.expect(send(bob, "PATCH", listPath, map[string]string{"title": "hacked"}), http.StatusNotFound)
		bob.expect(bob.do("GET", listPath+"/items/"), http.StatusNotFound)
		bob.expect(send(bob, "POST", listPath+"/items/", map[string]string{"title": "x"}), http.StatusNotFound)
		bob.expect(bob.do("GET", itemPath), http.StatusNotFound)
		bob.expect(send(bob, "PATCH", itemPath, itemPatch{Done: ptr(true)}), http.StatusNotFound)
		bob.expect(bob.do("DELETE", itemPath), http.StatusNotFound)
		bob.expect(bob.do("DELETE", listPath), http.StatusNotFound)

		gotList := decode[listBody](alice, alice.do("GET", listPath), http.StatusOK)
		gotItem := decode[itemBody](alice, alice.do("GET", itemPath), http.StatusOK)
		if gotList.Title != "Alice's" || gotItem.Title != "secret" || gotItem.Done {
			t.Errorf("alice's data changed: %+v %+v", gotList, gotItem)
		}
	})
}

// concurrently sends one request per body at the same time and returns the statuses.
func concurrently[T any](c client, method, path string, bodies []T) []int {
	statuses := make([]int, len(bodies))
	var wg sync.WaitGroup
	for i, body := range bodies {
		wg.Go(func() { statuses[i] = send(c, method, path, body).status })
	}
	wg.Wait()
	return statuses
}
