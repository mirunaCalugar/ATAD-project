package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"atad-project/internal/links"
	"atad-project/internal/store"
	"atad-project/internal/version"
)

const maxBodyBytes = 1 << 20

const identityPage = `<!doctype html><html lang="en"><meta charset="utf-8">
<title>%[1]s</title><body>
<h1>%[1]s</h1>
<p>Calugar Miruna, group [group]</p>
<p>Project 1: URL Shortener with Analytics</p>
</body></html>`

type server struct {
	store   *store.Store
	log     *slog.Logger
	started time.Time
}

type linkResponse struct {
	ID        int64     `json:"id"`
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

type statsResponse struct {
	Code        string     `json:"code"`
	Visits      int64      `json:"visits"`
	LastVisitAt *time.Time `json:"last_visit_at"`
}

type createRequest struct {
	URL  string  `json:"url"`
	Code *string `json:"code"`
}

func NewHandler(st *store.Store, log *slog.Logger, started time.Time) http.Handler {
	s := &server{store: st, log: log, started: started}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /version", s.version)
	mux.HandleFunc("GET /{$}", s.identity)
	mux.HandleFunc("POST /reset", s.reset)

	mux.HandleFunc("POST /links", s.createLink)
	mux.HandleFunc("GET /links", s.listLinks)
	mux.HandleFunc("GET /links/{code}/stats", s.stats)
	mux.HandleFunc("DELETE /links/{code}", s.deleteLink)
	mux.HandleFunc("GET /{code}", s.redirect)

	return logRequests(log, mux)
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"uptime_seconds": int(time.Since(s.started).Seconds()),
	})
}

func (s *server) version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, version.Current())
}

func (s *server) identity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, identityPage, version.Current().App)
}

func (s *server) reset(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Reset(r.Context()); err != nil {
		s.internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) createLink(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "validation", "body must be a JSON object with \"url\" and an optional \"code\": "+err.Error())
		return
	}
	if err := links.ValidateURL(req.URL); err != nil {
		writeError(w, http.StatusBadRequest, "validation", err.Error())
		return
	}

	var (
		link store.Link
		err  error
	)
	if req.Code != nil {
		if err := links.ValidateCode(*req.Code); err != nil {
			writeError(w, http.StatusBadRequest, "validation", err.Error())
			return
		}
		link, err = s.store.Create(r.Context(), *req.Code, req.URL)
	} else {
		link, err = s.store.CreateGenerated(r.Context(), req.URL)
	}

	if errors.Is(err, store.ErrCodeTaken) {
		writeError(w, http.StatusConflict, "conflict", fmt.Sprintf("code %q is already taken", *req.Code))
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toLinkResponse(link))
}

func (s *server) listLinks(w http.ResponseWriter, r *http.Request) {
	all, err := s.store.List(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	resp := make([]linkResponse, len(all))
	for i, l := range all {
		resp[i] = toLinkResponse(l)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *server) stats(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	st, err := s.store.Stats(r.Context(), code)
	if errors.Is(err, store.ErrNotFound) {
		writeNotFound(w, code)
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, statsResponse{Code: st.Code, Visits: st.Visits, LastVisitAt: st.LastVisitAt})
}

func (s *server) deleteLink(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	err := s.store.Delete(r.Context(), code)
	if errors.Is(err, store.ErrNotFound) {
		writeNotFound(w, code)
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) redirect(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	target, err := s.store.Visit(r.Context(), code)
	if errors.Is(err, store.ErrNotFound) {
		writeNotFound(w, code)
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *server) internalError(w http.ResponseWriter, err error) {
	s.log.Error("request failed", "err", err)
	writeError(w, http.StatusInternalServerError, "internal", "internal server error")
}

func toLinkResponse(l store.Link) linkResponse {
	return linkResponse{ID: l.ID, Code: l.Code, URL: l.URL, CreatedAt: l.CreatedAt}
}

func writeNotFound(w http.ResponseWriter, code string) {
	writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("no link with code %q", code))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": code, "message": message})
}
