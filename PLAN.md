# Implementation plan — URL Shortener with Analytics

Language: Go 1.27. Database: SQLite (`modernc.org/sqlite`, pure Go, works with `CGO_ENABLED=0`).

## 1. Layout

```
cmd/app/main.go          starts the server, registers the routes
internal/links/          URL and code validation, code generation
internal/store/          SQLite: schema, CRUD, redirect, reset
internal/api/            HTTP handlers, writeJSON, writeError
docs/architecture.md     architecture note (week 7)
```

## 2. `internal/links`

- `ValidateURL(s)`: `url.Parse`, scheme `http` or `https`, non-empty host. Otherwise `400 validation`.
- `ValidateCode(s)`: regex `^[A-Za-z0-9_-]{4,10}$`. Otherwise `400 validation`.
- Reserved codes: `links`, `health`, `version`, `reset`. A reserved custom code → `400 validation`.
- `Generate()`: 7 characters from the allowed alphabet, with `crypto/rand`. The length is the constant `GeneratedCodeLength = 7`.

## 3. `internal/store`

Schema:

```sql
CREATE TABLE IF NOT EXISTS links (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  code          TEXT    NOT NULL UNIQUE,
  url           TEXT    NOT NULL,
  created_at    TEXT    NOT NULL,
  visits        INTEGER NOT NULL DEFAULT 0,
  last_visit_at TEXT
);
```

- On startup: `PRAGMA journal_mode=WAL`, `PRAGMA busy_timeout=5000`, `Ping`.
- The database path comes from the env var `DB_PATH`, default `/data/links.db`.
- `Create(code, url)`: one `INSERT`. A UNIQUE violation → `ErrCodeTaken`.
- `Visit(code)`: one atomic statement:
  `UPDATE links SET visits = visits + 1, last_visit_at = ? WHERE code = ? RETURNING url`.
  No row → `ErrNotFound`.
- `Stats(code)`: `SELECT` only. It does not change `visits`.
- `List()`: `ORDER BY id DESC`.
- `Delete(code)`: `DELETE`. No row → `ErrNotFound`.
- `Reset()`: `DELETE FROM links` and `DELETE FROM sqlite_sequence`, so IDs start at 1 again.

## 4. `internal/api` — routes

| Route | Success | Errors |
|-------|---------|--------|
| `POST /links` | `201 {"id","code","url","created_at"}` | `400` invalid URL, code or JSON; `409` code taken |
| `GET /{code}` | `302`, `Location` header | `404` |
| `GET /links` | `200`, list, newest first | — |
| `GET /links/{code}/stats` | `200 {"code","visits","last_visit_at"}` | `404` |
| `DELETE /links/{code}` | `204` | `404` |
| `POST /reset` | `204`, clears the database | — |

- Errors always have the shape `{"error":"<code>","message":"<explanation>"}`.
- The JSON decoder uses `DisallowUnknownFields`.
- `/health`, `/version` and `/` stay from the template. The `/` page gets the name, group and project.

## 5. Decisions (for the review and `architecture.md`)

1. **Collision on a generated code.** Insert directly. If UNIQUE fails, generate a new code. Max 5 attempts, then `500`. No "does it exist?" check before the insert.
2. **Same custom code, two simultaneous requests.** The UNIQUE constraint decides. Exactly one request gets `201`, the other gets `409`.
3. **Where the click is counted.** Synchronously, in the same statement as the lookup, before the `302`. Cost: one disk write per redirect. Gain: no lost clicks on a crash. The alternative (channel + background worker) is faster, but loses the buffered clicks.
4. **First optimisation.** An in-memory cache `code → url`. Counting stays in the database.

## 6. Tests (`go test -race ./...`)

- `links`: table-driven tests for `ValidateURL`, `ValidateCode`, and the length and alphabet of `Generate`.
- `store`, on a temporary database (`t.TempDir()`):
  - duplicate code → `ErrCodeTaken`
  - 50 goroutines request the same code → exactly 1 success, 49 `ErrCodeTaken`
  - `Visit` once, `Stats` twice → `visits = 1` both times
  - close and reopen the store → the link still exists (restart simulation)
  - `Reset` → the next ID is 1
- `api`, with `httptest`: status code and JSON shape for every route and every error.
- The template's `--version` test stays.

## 7. Docker

- In the Dockerfile: `RUN mkdir /data && chown app /data`, `VOLUME /data`, `ENV DB_PATH=/data/links.db`.
- Run with persistence:
  `docker run --rm -p 8080:8080 -v atad-data:/data <image>`

## 8. Documentation

- `README.md`: full rewrite (what it does, how to run it, one request and its response).
- `docs/architecture.md`: the shape, the data, the three decisions, what is not built yet.
- `atad.json`, `/` and `README.md`: the group, when known.

## 9. Final check

1. `go vet ./...` and `go test -race ./...` pass.
2. `docker build` and `docker run` start the service on 8080.
3. The scenario from the project document: `POST /reset`, create `gofmt`, `409` on the duplicate, `302` on the redirect, `stats` twice → `visits = 1`.
4. Restart the container → `gofmt` still redirects.

## Work order

1. `internal/links` + tests
2. `internal/store` + tests
3. `internal/api` + tests
4. Dockerfile
5. README + `architecture.md`
6. Final check
