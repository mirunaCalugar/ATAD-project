# URL Shortener with Analytics

A web service that turns a long URL into a short code, redirects visitors of
that code to the original address, and counts each visit. It is for anyone who
wants short, shareable links and a simple view of how often each one is used.

- **Student:** Calugar Miruna, group DMWD
- **Project:** 1 — URL Shortener with Analytics
- **Language:** Go

## What it does

`POST /links` accepts an `http` or `https` URL and returns a short code. You can
choose the code (4 to 10 characters from `A-Z a-z 0-9 - _`), or the service
generates a random 7-character one. A visit to `/{code}` answers `302` with the
original URL in the `Location` header and counts one visit.
`GET /links/{code}/stats` reports the number of visits and the time of the last
one. Reading the stats does not count as a visit.

Links live in a SQLite database, so they survive a restart. Code uniqueness is
enforced by the database, so two simultaneous requests for the same code create
one link and get one `201` and one `409`. The visit is counted in the same
statement that reads the URL, before the redirect is sent, so no visit is lost
if the process stops.

A web page on `/` lets you create, copy, open and delete links and watch the
visit counts update. The service does not have user accounts, link expiry or
per-visitor analytics. Design notes are in [docs/architecture.md](docs/architecture.md).

## How to run it

```
docker run --rm -p 8080:8080 -v atad-data:/data ghcr.io/mirunacalugar/atad-project:latest
```

The `-v atad-data:/data` volume keeps the links when the container is removed.
Without it, data survives a restart of the same container only.

The image is built for `linux/amd64`. On an Apple Silicon Mac, add
`--platform linux/amd64`.

To build and run without Docker:

```
go run ./cmd/app
go run ./cmd/app --version
```

| Variable  | Default    | Meaning                     |
|-----------|------------|-----------------------------|
| `PORT`    | `8080`     | Port the service listens on |
| `DB_PATH` | `links.db` | SQLite database file        |

The Docker image sets `DB_PATH=/data/links.db`.

To run the tests:

```
go test -race ./...
```

## What you should see

Open <http://localhost:8080/> for the web page, or use `curl`:

```
$ curl -i -H "Content-Type: application/json" \
    -d '{"url":"https://go.dev/doc/effective_go","code":"gofmt"}' \
    http://localhost:8080/links
HTTP/1.1 201 Created
Content-Type: application/json

{"id":1,"code":"gofmt","url":"https://go.dev/doc/effective_go","created_at":"2026-11-05T12:15:02.86151371Z"}

$ curl -i http://localhost:8080/gofmt
HTTP/1.1 302 Found
Location: https://go.dev/doc/effective_go

$ curl http://localhost:8080/links/gofmt/stats
{"code":"gofmt","visits":1,"last_visit_at":"2026-11-05T12:15:28.7703675Z"}
```

`visits` stays `1` however many times you read the stats.

## Routes

| Route                       | Method | Success                                       | Errors     |
|-----------------------------|--------|-----------------------------------------------|------------|
| `/health`                   | GET    | `200 {"status","uptime_seconds"}`             |            |
| `/version`                  | GET    | `200 {"app","version","commit","built_at"}`   |            |
| `/`                         | GET    | `200`, web page                               |            |
| `/reset`                    | POST   | `204`, deletes all links                      |            |
| `/links`                    | POST   | `201 {"id","code","url","created_at"}`        | `400`, `409` |
| `/links`                    | GET    | `200`, all links, newest first                |            |
| `/{code}`                   | GET    | `302` to the original URL                     | `404`      |
| `/links/{code}/stats`       | GET    | `200 {"code","visits","last_visit_at"}`       | `404`      |
| `/links/{code}`             | DELETE | `204`                                         | `404`      |

Errors have the shape `{"error":"<code>","message":"<explanation>"}`.
