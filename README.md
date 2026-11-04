# [Project title]

[One paragraph: what this program does and who would use it. Replace this
whole file — it is your project's front door, and it is marked.]

- **Student:** [Surname Firstname], group [group]
- **Project:** [number and title from the project description]
- **Language:** Go

## What it does

[Two or three paragraphs. What problem it solves, what it does not do, and
what state it is in.]

## How to run it

```
docker run --rm -p 8080:8080 ghcr.io/[user]/[repo]:latest
```

To build and run without Docker:

```
go run ./cmd/app
go run ./cmd/app --version
```

## What you should see

[What a successful run looks like. If it is a service, one request and its
response. If it is a tool, one command and its output. A reader who has never
seen your project must be able to tell whether it worked.]

---

## Before you delete these notes

**Fill in `atad.json` first.** Name, group, student id, email, project number
and title, language, and the command that runs your project. The marking script reads this file. A field left as
`[placeholder]` counts as missing.

**`--version` is part of the contract.** It must print one line of JSON with
`app`, `version`, `commit` and `built_at`, and exit 0. The `commit` value is
stamped into the binary by the Dockerfile from the SHA that CI supplies, which
is how it is proved that the published image was built from the code you
submitted. Do not print it yourself, and do not remove the test that checks it.

**The four contract routes are not optional.** `/health`, `/version`, `/` and
`POST /reset` must answer exactly as the project document specifies, in every
project. They are checked automatically, and a right answer with the wrong
status code is a wrong answer.

**Layout.** `cmd/app` is the entry point, `internal/` is your own packages,
and `docs/` holds your documentation. Add packages under `internal/` as the
project grows; a single 800-line `main.go` is not a design.

**`docs/` has fixed names.** `docs/architecture.md` is due in week 7 and
`docs/report.docx` at the end. `docs/README.md` explains both — read it, and do
not edit or delete it.

**Do not commit** `coverage/`, build output, or anything with a credential in
it. `.gitignore` already covers the usual cases.
