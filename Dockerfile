# syntax=docker/dockerfile:1.7

# GO_VERSION mirrors the `go` directive in go.mod. CI workflows override this
# from go.mod via scripts/go-version.sh; the default below is the fallback for
# `docker build .` invocations that don't pass --build-arg, and a lint guard
# in ci.yml checks that the two stay in sync.
ARG GO_VERSION=1.26.3

FROM golang:${GO_VERSION}-alpine AS builder

ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /msd ./cmd/msd/

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY --from=builder /msd /usr/local/bin/msd
ENTRYPOINT ["msd"]
