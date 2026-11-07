# Architecture

## The shape

The service is one Go binary with three packages under `internal/`:

| Package  | Role                                                                 |
|----------|----------------------------------------------------------------------|
| `api`    | HTTP routes, JSON in and out, error shape, request logging, web page |
| `links`  | Pure rules: URL and code validation, random code generation          |
| `store`  | SQLite access: create, visit, stats, list, delete, reset             |

`cmd/app` reads `PORT` and `DB_PATH`, opens the store, and starts an
`http.Server` with timeouts and graceful shutdown.

The path of a redirect, from request to response:

1. `http.Server` accepts `GET /gofmt`.
2. `logRequests` middleware starts a timer.
3. `http.ServeMux` matches `GET /{code}` and calls `server.redirect`.
4. `store.Visit` runs one statement:
   `UPDATE links SET visits = visits + 1, last_visit_at = ? WHERE code = ? RETURNING url`.
5. No row → `404 not_found`. A row → `302` with `Location: <url>` and
   `Cache-Control: no-store`.
6. The middleware logs method, path, status and duration.

If the redirect were too slow, I would optimise step 4 first. It is the only
step that touches the disk. An in-memory `code → url` cache would remove the
read, but the visit count still needs a write.

## The data

One SQLite table:

```sql
links(id INTEGER PRIMARY KEY AUTOINCREMENT, code TEXT NOT NULL UNIQUE,
      url TEXT NOT NULL, created_at TEXT NOT NULL,
      visits INTEGER NOT NULL DEFAULT 0, last_visit_at TEXT)
```

The file is `/data/links.db` in the container, on a Docker volume. The
database runs in WAL mode with a 5-second busy timeout. Everything in the table
survives a restart. Nothing is held only in memory. `POST /reset` deletes all
rows and resets the ID sequence, so IDs start at 1 again.

## Three decisions

**1. The database decides code uniqueness.** I insert directly and treat a
`UNIQUE` violation as `409`. I rejected "check if the code exists, then
insert", because two requests can both pass the check before either inserts.
For a generated code, a collision triggers a new code and a retry, up to 5
attempts. With 64⁷ (about 4.4 × 10¹²) possible codes, a second collision is
very rare, and the limit stops an endless loop. A test runs 50 goroutines on
the same code and expects exactly one success.

**2. The visit is counted synchronously, before the redirect.** The lookup and
the increment are one atomic statement. I rejected counting in a background
goroutine through a channel. That is faster, but clicks in the buffer are lost
if the process stops, and the stats can lag behind the redirect. The cost of
my choice is one disk write on every redirect.

**3. SQLite with a pure-Go driver.** I chose `modernc.org/sqlite`, because the
Dockerfile builds with `CGO_ENABLED=0`, and `mattn/go-sqlite3` needs CGO. I
rejected PostgreSQL, because it needs a second container, and the project
must start with one `docker run`. I rejected a JSON file, because it gives no
atomic writes and no uniqueness constraint.

## What is not built yet

- The in-memory `code → url` cache from the optimisation note above.
- Pagination for `GET /links`. It returns every link.
- Rate limiting and authentication. Anyone can create or delete links.
