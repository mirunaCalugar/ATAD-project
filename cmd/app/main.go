// Entry point for your project.
//
// Every ATAD project is a web service in a container. The four routes below are
// the common contract and are checked automatically: keep them working exactly
// as specified and add your own routes alongside them.
//
// --version must also keep working. It prints one line of JSON and exits 0.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"atad-project/internal/version"
)

// started is the moment the process came up; /health reports the difference.
var started = time.Now()

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	for _, a := range os.Args[1:] {
		if a == "--version" || a == "-v" {
			return version.WriteJSON(os.Stdout)
		}
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	mux := http.NewServeMux()

	// ---- the common contract: four routes, identical in every project -------

	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = version.WriteJSON(w)
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":         "ok",
			"uptime_seconds": int(time.Since(started).Seconds()),
		})
	})

	// The identity page. Replace the placeholders with your own details; the
	// marking script reads this page.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html><html lang="en"><meta charset="utf-8">
<title>%s</title><body>
<h1>%s</h1>
<p>[Surname Firstname], group [group]</p>
<p>Project [number]: [title]</p>
</body></html>`, version.Current().App, version.Current().App)
	})

	// Discards everything the service holds, so tests can start from a known
	// state without restarting the container. If your project persists data,
	// this clears what was persisted too.
	mux.HandleFunc("POST /reset", func(w http.ResponseWriter, r *http.Request) {
		// TODO: clear your own state here.
		w.WriteHeader(http.StatusNoContent)
	})

	// ---- your own routes go below -------------------------------------------

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Shut down cleanly on Ctrl-C or on the signal Docker sends when stopping
	// the container, so in-flight requests are allowed to finish.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", srv.Addr, "commit", version.Current().Commit)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// writeJSON is the one place that sets the content type, so every response in
// the service is consistent about it.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError produces the error shape the common contract requires. Use it for
// every failure rather than writing the JSON by hand at each call site.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": code, "message": message})
}
