// Package e2e_test exercises the whole API over HTTP against a real
// PostgreSQL. By default it starts one with pgtest and serves the router
// in-process; set E2E_BASE_URL (e.g. http://localhost:8000 after `make up`) to
// test a running deployment instead.
package e2e_test

import (
	"bytes"
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

func (c client) do(method, path string, body any) response {
	c.t.Helper()
	var reader io.Reader
	switch b := body.(type) {
	case nil:
	case string: // raw, possibly malformed, JSON
		reader = strings.NewReader(b)
	default:
		data, err := json.Marshal(b)
		if err != nil {
			c.t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(c.t.Context(), method, c.base+path, reader)
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

// expect asserts the status and decodes the JSON body into out (if non-nil).
func (c client) expect(r response, status int, out any) {
	c.t.Helper()
	if r.status != status {
		c.t.Fatalf("status = %d, want %d; body: %s", r.status, status, r.body)
	}
	if out != nil {
		if err := json.Unmarshal(r.body, out); err != nil {
			c.t.Fatalf("decode %s: %v", r.body, err)
		}
	}
}

var userSeq atomic.Int64

// signUp registers a fresh user and returns a client authenticated as them.
func signUp(t *testing.T, base string) client {
	t.Helper()
	username := fmt.Sprintf("user-%d-%d", time.Now().UnixNano(), userSeq.Add(1))
	anon := client{t: t, base: base}
	anon.expect(anon.do("POST", "/auth/sign-up", map[string]string{"name": "Test", "username": username, "password": "s3cret"}), http.StatusCreated, nil)

	var signIn struct{ Token string }
	anon.expect(anon.do("POST", "/auth/sign-in", map[string]string{"username": username, "password": "s3cret"}), http.StatusOK, &signIn)
	if signIn.Token == "" {
		t.Fatal("empty token")
	}
	return client{t: t, base: base, token: signIn.Token}
}

type idBody struct{ ID int64 }

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

type messageBody struct{ Message string }

func TestAPI(t *testing.T) {
	base := startAPI(t)

	t.Run("probes", func(t *testing.T) {
		anon := client{t: t, base: base}
		anon.expect(anon.do("GET", "/healthz", nil), http.StatusOK, nil)
		if id := anon.do("GET", "/healthz", nil).header.Get("X-Request-ID"); id == "" {
			t.Error("probe response has no X-Request-ID")
		}
		var ready struct{ Status string }
		anon.expect(anon.do("GET", "/readyz", nil), http.StatusOK, &ready)
		if ready.Status != "ready" {
			t.Errorf("readyz status = %q", ready.Status)
		}
	})

	t.Run("auth", func(t *testing.T) {
		anon := client{t: t, base: base}
		username := fmt.Sprintf("auth-%d", time.Now().UnixNano())
		creds := map[string]string{"name": "Ada", "username": username, "password": "s3cret"}

		var created idBody
		anon.expect(anon.do("POST", "/auth/sign-up", creds), http.StatusCreated, &created)
		if created.ID <= 0 {
			t.Fatalf("id = %d", created.ID)
		}

		var msg messageBody
		anon.expect(anon.do("POST", "/auth/sign-up", creds), http.StatusConflict, &msg)
		if msg.Message != "username is already taken" {
			t.Errorf("duplicate message = %q", msg.Message)
		}

		anon.expect(anon.do("POST", "/auth/sign-up", map[string]string{"name": "B", "username": username + "-b", "password": ""}), http.StatusBadRequest, nil)
		anon.expect(anon.do("POST", "/auth/sign-up", map[string]string{"name": "B", "username": username + "-b", "password": strings.Repeat("p", 1025)}), http.StatusBadRequest, nil)
		anon.expect(anon.do("POST", "/auth/sign-up", `{"name":`), http.StatusBadRequest, nil)
		anon.expect(anon.do("POST", "/auth/sign-up", map[string]string{"name": "B\x00", "username": username + "-c", "password": "x"}), http.StatusBadRequest, nil)
		anon.expect(anon.do("POST", "/auth/sign-in", map[string]string{"username": "a\x00b", "password": "x"}), http.StatusUnauthorized, nil)

		anon.expect(anon.do("POST", "/auth/sign-in", map[string]string{"username": username, "password": "wrong"}), http.StatusUnauthorized, &msg)
		wrongPassword := msg.Message
		anon.expect(anon.do("POST", "/auth/sign-in", map[string]string{"username": username + "-nobody", "password": "wrong"}), http.StatusUnauthorized, &msg)
		if msg.Message != wrongPassword {
			t.Errorf("unknown user and wrong password must look identical: %q vs %q", msg.Message, wrongPassword)
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

		var msg messageBody
		user.expect(user.do("GET", "/auth/api/lists/", nil), http.StatusNotFound, &msg)
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
		ghost := client{t: t, base: base, token: tok}
		ghost.expect(ghost.do("POST", "/api/lists/", map[string]string{"title": "x"}), http.StatusUnauthorized, nil)
		ghost.expect(ghost.do("GET", "/api/lists/", nil), http.StatusUnauthorized, nil)
		ghost.expect(ghost.do("GET", "/api/items/1", nil), http.StatusUnauthorized, nil)
	})

	t.Run("lists and items", func(t *testing.T) {
		user := signUp(t, base)

		var lists struct{ Data []listBody }
		for _, path := range []string{"/api/lists/", "/api/lists"} {
			if r := user.do("GET", path, nil); r.status != http.StatusOK {
				t.Errorf("GET %s = %d, want 200 with or without trailing slash", path, r.status)
			}
		}
		if r := user.do("GET", "/api/lists/", nil); strings.TrimSpace(string(r.body)) != `{"data":[]}` {
			t.Errorf("empty lists body = %s, want {\"data\":[]}", r.body)
		}

		var list idBody
		user.expect(user.do("POST", "/api/lists/", map[string]string{"title": "Groceries", "description": "weekly"}), http.StatusCreated, &list)
		user.expect(user.do("GET", "/api/lists/", nil), http.StatusOK, &lists)
		if len(lists.Data) != 1 || lists.Data[0] != (listBody{ID: list.ID, Title: "Groceries", Description: "weekly"}) {
			t.Errorf("lists = %+v", lists.Data)
		}

		listPath := fmt.Sprintf("/api/lists/%d", list.ID)
		user.expect(user.do("PATCH", listPath, map[string]string{"title": "Food"}), http.StatusOK, nil)
		var got listBody
		user.expect(user.do("GET", listPath, nil), http.StatusOK, &got)
		if got != (listBody{ID: list.ID, Title: "Food", Description: "weekly"}) {
			t.Errorf("updated list = %+v", got)
		}

		var msg messageBody
		user.expect(user.do("PATCH", listPath, map[string]any{}), http.StatusBadRequest, &msg)
		if msg.Message != "update structure has no values" {
			t.Errorf("empty update message = %q", msg.Message)
		}
		user.expect(user.do("PATCH", listPath, map[string]string{"title": " "}), http.StatusBadRequest, nil)
		user.expect(user.do("POST", "/api/lists/", map[string]string{"title": ""}), http.StatusBadRequest, nil)
		user.expect(user.do("POST", "/api/lists/", map[string]string{"title": "a\x00b"}), http.StatusBadRequest, nil)
		user.expect(user.do("GET", "/api/lists/abc", nil), http.StatusBadRequest, nil)
		user.expect(user.do("GET", "/api/lists/999999999", nil), http.StatusNotFound, nil)

		itemsPath := listPath + "/items/"
		if r := user.do("GET", itemsPath, nil); r.status != http.StatusOK || strings.TrimSpace(string(r.body)) != `{"data":[]}` {
			t.Errorf("empty items = %d %s, want 200 {\"data\":[]}", r.status, r.body)
		}

		var item idBody
		user.expect(user.do("POST", itemsPath, map[string]string{"title": "Milk"}), http.StatusCreated, &item)
		itemPath := fmt.Sprintf("/api/items/%d", item.ID)

		user.expect(user.do("PATCH", itemPath, map[string]any{"done": true, "title": "Oat milk"}), http.StatusOK, nil)
		var gotItem itemBody
		user.expect(user.do("GET", itemPath, nil), http.StatusOK, &gotItem)
		if gotItem != (itemBody{ID: item.ID, Title: "Oat milk", Done: true}) {
			t.Errorf("updated item = %+v", gotItem)
		}
		var items struct{ Data []itemBody }
		user.expect(user.do("GET", itemsPath, nil), http.StatusOK, &items)
		if len(items.Data) != 1 || items.Data[0] != gotItem {
			t.Errorf("items = %+v", items)
		}
		user.expect(user.do("PATCH", itemPath, map[string]any{}), http.StatusBadRequest, nil)
		user.expect(user.do("PATCH", itemPath+"/", map[string]any{"done": false}), http.StatusOK, nil)
		user.expect(user.do("PATCH", itemPath, map[string]any{"done": true, "colour": "red"}), http.StatusBadRequest, nil)
		user.expect(user.do("PATCH", itemPath, `{"done":true} {"done":false}`), http.StatusBadRequest, nil)
		var notAllowed messageBody
		r := user.do("PUT", itemPath, map[string]any{"done": true})
		user.expect(r, http.StatusMethodNotAllowed, &notAllowed)
		if notAllowed.Message != "method not allowed" {
			t.Errorf("405 message = %q", notAllowed.Message)
		}

		user.expect(user.do("DELETE", itemPath, nil), http.StatusOK, nil)
		user.expect(user.do("GET", itemPath, nil), http.StatusNotFound, nil)
		user.expect(user.do("DELETE", itemPath, nil), http.StatusNotFound, nil)

		var second idBody
		user.expect(user.do("POST", itemsPath, map[string]string{"title": "Bread"}), http.StatusCreated, &second)
		user.expect(user.do("DELETE", listPath, nil), http.StatusOK, nil)
		user.expect(user.do("GET", listPath, nil), http.StatusNotFound, nil)
		user.expect(user.do("GET", fmt.Sprintf("/api/items/%d", second.ID), nil), http.StatusNotFound, nil)
		user.expect(user.do("DELETE", listPath, nil), http.StatusNotFound, nil)
	})

	t.Run("concurrent patches to different fields all apply", func(t *testing.T) {
		user := signUp(t, base)
		var list, item idBody
		user.expect(user.do("POST", "/api/lists/", map[string]string{"title": "L"}), http.StatusCreated, &list)
		user.expect(user.do("POST", fmt.Sprintf("/api/lists/%d/items/", list.ID), map[string]string{"title": "old"}), http.StatusCreated, &item)
		itemPath := fmt.Sprintf("/api/items/%d", item.ID)

		patches := []map[string]any{{"title": "new"}, {"done": true}, {"description": "d"}}
		for _, status := range concurrently(user, "PATCH", itemPath, patches) {
			if status != http.StatusOK {
				t.Errorf("concurrent PATCH status = %d, want 200", status)
			}
		}

		var got itemBody
		user.expect(user.do("GET", itemPath, nil), http.StatusOK, &got)
		if got != (itemBody{ID: item.ID, Title: "new", Description: "d", Done: true}) {
			t.Errorf("item after concurrent patches = %+v, want all three applied", got)
		}
	})

	t.Run("heavy contention answers 200 or 409, never 500", func(t *testing.T) {
		user := signUp(t, base)
		var list idBody
		user.expect(user.do("POST", "/api/lists/", map[string]string{"title": "L"}), http.StatusCreated, &list)
		listPath := fmt.Sprintf("/api/lists/%d", list.ID)

		patches := make([]map[string]any, 20)
		for i := range patches {
			patches[i] = map[string]any{"title": fmt.Sprintf("t-%d", i)}
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

		var list, item idBody
		alice.expect(alice.do("POST", "/api/lists/", map[string]string{"title": "Alice's"}), http.StatusCreated, &list)
		listPath := fmt.Sprintf("/api/lists/%d", list.ID)
		alice.expect(alice.do("POST", listPath+"/items/", map[string]string{"title": "secret"}), http.StatusCreated, &item)
		itemPath := fmt.Sprintf("/api/items/%d", item.ID)

		var bobLists struct{ Data []listBody }
		bob.expect(bob.do("GET", "/api/lists/", nil), http.StatusOK, &bobLists)
		if len(bobLists.Data) != 0 {
			t.Errorf("bob sees lists %+v", bobLists.Data)
		}
		bob.expect(bob.do("GET", listPath, nil), http.StatusNotFound, nil)
		bob.expect(bob.do("PATCH", listPath, map[string]string{"title": "hacked"}), http.StatusNotFound, nil)
		bob.expect(bob.do("GET", listPath+"/items/", nil), http.StatusNotFound, nil)
		bob.expect(bob.do("POST", listPath+"/items/", map[string]string{"title": "x"}), http.StatusNotFound, nil)
		bob.expect(bob.do("GET", itemPath, nil), http.StatusNotFound, nil)
		bob.expect(bob.do("PATCH", itemPath, map[string]any{"done": true}), http.StatusNotFound, nil)
		bob.expect(bob.do("DELETE", itemPath, nil), http.StatusNotFound, nil)
		bob.expect(bob.do("DELETE", listPath, nil), http.StatusNotFound, nil)

		var gotList listBody
		alice.expect(alice.do("GET", listPath, nil), http.StatusOK, &gotList)
		var gotItem itemBody
		alice.expect(alice.do("GET", itemPath, nil), http.StatusOK, &gotItem)
		if gotList.Title != "Alice's" || gotItem.Title != "secret" || gotItem.Done {
			t.Errorf("alice's data changed: %+v %+v", gotList, gotItem)
		}
	})
}

// concurrently sends one request per body at the same time and returns the statuses.
func concurrently(c client, method, path string, bodies []map[string]any) []int {
	statuses := make([]int, len(bodies))
	var wg sync.WaitGroup
	for i, body := range bodies {
		wg.Go(func() { statuses[i] = c.do(method, path, body).status })
	}
	wg.Wait()
	return statuses
}
