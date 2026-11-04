# Two-stage build. The first stage has the whole Go toolchain and is large;
# nothing from it reaches the image you ship. The second stage starts from
# almost nothing and receives only the compiled binary.
#
# You will need to change very little here. The parts to read carefully are the
# -ldflags line, which stamps the version information into the binary, and the
# ENTRYPOINT, which must still start your program.

# ---- stage 1: compile -------------------------------------------------------
FROM golang:1.27-alpine AS build
WORKDIR /src

# Copy the module files first and download dependencies as their own layer.
# Docker caches that layer, so editing your source does not re-download anything.
COPY go.mod go.sum* ./
RUN go mod download

COPY . .

# COMMIT and BUILT_AT are supplied by the CI workflow; see .github/workflows/ci.yml.
ARG COMMIT=dev
ARG BUILT_AT=unknown

# CGO_ENABLED=0 produces a static binary that runs on Alpine with no extra
# libraries. -s -w strip debug information and shrink the result.
# -X sets a string variable at link time: this is how the build stamps itself.
RUN CGO_ENABLED=0 go build \
      -ldflags="-s -w \
        -X atad-project/internal/version.Commit=${COMMIT} \
        -X atad-project/internal/version.BuiltAt=${BUILT_AT}" \
      -o /app ./cmd/app

# ---- stage 2: run -----------------------------------------------------------
FROM alpine:3.21

LABEL org.opencontainers.image.title="ATAD project" \
      org.opencontainers.image.authors="Surname Firstname <firstname.surname@student.upt.ro>" \
      org.opencontainers.image.source="https://github.com/user/repo"

# Run as an unprivileged user. A container process that does not need root
# should not have it.
RUN adduser -D -u 10001 app

COPY --from=build /app /usr/local/bin/app
USER app

# Every project is a service on 8080; the contract requires this line.
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/app"]
