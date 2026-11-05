package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"atad-project/internal/links"
	"atad-project/internal/store"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "links.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(NewHandler(st, log, time.Now()))
	t.Cleanup(srv.Close)
	return srv
}

func noRedirectClient() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

func do(t *testing.T, srv *httptest.Server, method, path, body string) *http.Response {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, srv.URL+path, r)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := noRedirectClient().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return v
}

func wantStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s status = %d, want %d, body %s", resp.Request.Method, resp.Request.URL.Path, resp.StatusCode, want, body)
	}
}

func wantError(t *testing.T, resp *http.Response, status int, code string) {
	t.Helper()
	wantStatus(t, resp, status)
	body := decode[map[string]string](t, resp)
	if body["error"] != code || body["message"] == "" {
		t.Errorf("error body = %v, want error %q and a message", body, code)
	}
}

func TestProjectScenario(t *testing.T) {
	srv := newTestServer(t)

	wantStatus(t, do(t, srv, http.MethodPost, "/reset", ""), http.StatusNoContent)

	resp := do(t, srv, http.MethodPost, "/links", `{"url":"https://go.dev/doc/effective_go","code":"gofmt"}`)
	wantStatus(t, resp, http.StatusCreated)
	created := decode[linkResponse](t, resp)
	if created.ID != 1 || created.Code != "gofmt" || created.URL != "https://go.dev/doc/effective_go" || created.CreatedAt.IsZero() {
		t.Errorf("created = %+v", created)
	}

	wantError(t, do(t, srv, http.MethodPost, "/links", `{"url":"https://example.org","code":"gofmt"}`), http.StatusConflict, "conflict")

	resp = do(t, srv, http.MethodGet, "/gofmt", "")
	wantStatus(t, resp, http.StatusFound)
	if loc := resp.Header.Get("Location"); loc != "https://go.dev/doc/effective_go" {
		t.Errorf("Location = %q", loc)
	}

	for i := range 2 {
		resp := do(t, srv, http.MethodGet, "/links/gofmt/stats", "")
		wantStatus(t, resp, http.StatusOK)
		st := decode[statsResponse](t, resp)
		if st.Visits != 1 || st.LastVisitAt == nil {
			t.Errorf("stats read %d = %+v, want 1 visit and a last visit time", i+1, st)
		}
	}
}

func TestCreateGeneratesCode(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, http.MethodPost, "/links", `{"url":"https://example.org"}`)
	wantStatus(t, resp, http.StatusCreated)
	created := decode[linkResponse](t, resp)
	if len(created.Code) != links.GeneratedCodeLength {
		t.Errorf("generated code %q has length %d, want %d", created.Code, len(created.Code), links.GeneratedCodeLength)
	}
	if err := links.ValidateCode(created.Code); err != nil {
		t.Errorf("generated code %q is invalid: %v", created.Code, err)
	}
}

func TestCreateValidation(t *testing.T) {
	srv := newTestServer(t)
	tests := []struct {
		name string
		body string
	}{
		{"not json", `not json`},
		{"empty body", ``},
		{"unknown field", `{"url":"https://example.org","extra":1}`},
		{"missing url", `{"code":"abcd"}`},
		{"ftp url", `{"url":"ftp://example.org"}`},
		{"no scheme", `{"url":"example.org"}`},
		{"code too short", `{"url":"https://example.org","code":"abc"}`},
		{"code too long", `{"url":"https://example.org","code":"abcdefghijk"}`},
		{"code bad character", `{"url":"https://example.org","code":"ab/cd"}`},
		{"empty code", `{"url":"https://example.org","code":""}`},
		{"reserved code", `{"url":"https://example.org","code":"links"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantError(t, do(t, srv, http.MethodPost, "/links", tt.body), http.StatusBadRequest, "validation")
		})
	}
}

func TestUnknownCodeIsNotFound(t *testing.T) {
	srv := newTestServer(t)
	wantError(t, do(t, srv, http.MethodGet, "/nope", ""), http.StatusNotFound, "not_found")
	wantError(t, do(t, srv, http.MethodGet, "/links/nope/stats", ""), http.StatusNotFound, "not_found")
	wantError(t, do(t, srv, http.MethodDelete, "/links/nope", ""), http.StatusNotFound, "not_found")
}

func TestStatsBeforeFirstVisit(t *testing.T) {
	srv := newTestServer(t)
	wantStatus(t, do(t, srv, http.MethodPost, "/links", `{"url":"https://example.org","code":"fresh"}`), http.StatusCreated)

	resp := do(t, srv, http.MethodGet, "/links/fresh/stats", "")
	wantStatus(t, resp, http.StatusOK)
	raw := decode[map[string]any](t, resp)
	if v, ok := raw["last_visit_at"]; !ok || v != nil {
		t.Errorf("last_visit_at = %v (present %v), want null", v, ok)
	}
	if raw["visits"] != float64(0) {
		t.Errorf("visits = %v, want 0", raw["visits"])
	}
}

func TestListNewestFirst(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, http.MethodGet, "/links", "")
	wantStatus(t, resp, http.StatusOK)
	if empty := decode[[]linkResponse](t, resp); len(empty) != 0 {
		t.Fatalf("list on empty store = %+v", empty)
	}

	for _, code := range []string{"aaaa", "bbbb"} {
		wantStatus(t, do(t, srv, http.MethodPost, "/links", `{"url":"https://example.org","code":"`+code+`"}`), http.StatusCreated)
	}
	resp = do(t, srv, http.MethodGet, "/links", "")
	wantStatus(t, resp, http.StatusOK)
	all := decode[[]linkResponse](t, resp)
	if len(all) != 2 || all[0].Code != "bbbb" || all[1].Code != "aaaa" {
		t.Errorf("list = %+v, want bbbb then aaaa", all)
	}
}

func TestDeleteThenRedirectIsNotFound(t *testing.T) {
	srv := newTestServer(t)
	wantStatus(t, do(t, srv, http.MethodPost, "/links", `{"url":"https://example.org","code":"gone"}`), http.StatusCreated)
	wantStatus(t, do(t, srv, http.MethodDelete, "/links/gone", ""), http.StatusNoContent)
	wantError(t, do(t, srv, http.MethodGet, "/gone", ""), http.StatusNotFound, "not_found")
}

func TestResetClearsLinks(t *testing.T) {
	srv := newTestServer(t)
	wantStatus(t, do(t, srv, http.MethodPost, "/links", `{"url":"https://example.org","code":"aaaa"}`), http.StatusCreated)
	wantStatus(t, do(t, srv, http.MethodPost, "/reset", ""), http.StatusNoContent)
	wantError(t, do(t, srv, http.MethodGet, "/aaaa", ""), http.StatusNotFound, "not_found")

	resp := do(t, srv, http.MethodPost, "/links", `{"url":"https://example.org","code":"aaaa"}`)
	wantStatus(t, resp, http.StatusCreated)
	if l := decode[linkResponse](t, resp); l.ID != 1 {
		t.Errorf("ID after reset = %d, want 1", l.ID)
	}
}

func TestContractRoutes(t *testing.T) {
	srv := newTestServer(t)

	resp := do(t, srv, http.MethodGet, "/health", "")
	wantStatus(t, resp, http.StatusOK)
	health := decode[map[string]any](t, resp)
	if health["status"] != "ok" {
		t.Errorf("health status = %v", health["status"])
	}
	if _, ok := health["uptime_seconds"].(float64); !ok {
		t.Errorf("uptime_seconds = %v, want a number", health["uptime_seconds"])
	}

	resp = do(t, srv, http.MethodGet, "/version", "")
	wantStatus(t, resp, http.StatusOK)
	ver := decode[map[string]string](t, resp)
	for _, f := range []string{"app", "version", "commit", "built_at"} {
		if _, ok := ver[f]; !ok {
			t.Errorf("version is missing %q: %v", f, ver)
		}
	}

	resp = do(t, srv, http.MethodGet, "/", "")
	wantStatus(t, resp, http.StatusOK)
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("/ Content-Type = %q, want text/html", ct)
	}
	page, _ := io.ReadAll(resp.Body)
	for _, want := range []string{"Calugar Miruna", "URL Shortener with Analytics"} {
		if !strings.Contains(string(page), want) {
			t.Errorf("/ page does not contain %q", want)
		}
	}
}
