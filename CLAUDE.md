# MSD — Multi Site Downloader

Go CLI tool and library for downloading albums from file-hosting sites (pixeldrain, filester.me, gofile.io).

## Build & Test

```bash
make build          # go build -o msd ./cmd/msd/
make test           # go test -race -count=1 ./...
make lint           # golangci-lint run (v2.11.4)
make smoke          # go test -race -count=1 -tags=integration ./...
go vet ./...        # included in CI
```

## Architecture

Library-first design: `cmd/msd/` is a thin CLI wrapper. All logic lives in importable packages.

```
cmd/msd/          CLI entrypoint (cobra). Imports site packages via blank imports.
engine/           Download orchestrator. Concurrency, resume, retries, rate limiting, progress.
site/             Site interface, registry, typed errors. No implementation here.
site/pixeldrain/  Pixeldrain handler (REST API).
site/filester/    Filester handler (HTML scraping + CDN API).
site/gofile/      Gofile handler (guest token + website token derivation).
internal/config/  Config loading (XDG), YAML parsing, CLI flag merge. Not part of public API.
internal/fsutil/  Filename sanitization (cross-platform). Not part of public API.
```

### Key interfaces

**`site.Site`** — every site handler implements this:
- `Name()`, `Match(url)` — identification and URL routing
- `Resolve(ctx, url, password)` — fetch album metadata, return `*Album` with `[]File`
- `DownloadRequest(ctx, file)` — return `*DownloadRequest{URL, Headers, Cookies}` just before download (handles expiring CDN URLs)
- `DefaultConcurrency()`, `DefaultResolveDelay()`, `DefaultDownloadDelay()` — site-specific defaults

**`engine.ProgressReporter`** — CLI implements with progress bars, library users provide their own or use `NoopReporter`.

### Site registration

Sites self-register via `init()` → `site.Register()`. The CLI blank-imports all sites. Library consumers import only what they need.

### Typed errors

`site.ErrSiteChanged`, `site.ErrRateLimited`, `site.ErrAuthRequired`, `site.ErrNotFound` — use these for site handler errors. The CLI maps them to user-friendly messages.

## Conventions

- Module path: `github.com/Wasylq/MSD`
- Config location: `$XDG_CONFIG_HOME/msd/config.yaml` (Linux: `~/.config/msd/`, macOS: `~/Library/Application Support/msd/`, Windows: `%APPDATA%\msd\`)
- Downloads go to `.part` file first, fsync, then atomic rename
- Filename sanitization always goes through `internal/fsutil/` — never inline in engine
- Rate limiting: split between resolve (API/scraping) and download (CDN) — some sites only limit one
- Retry: exponential backoff with jitter for 429, 5xx, disconnects
- Integration tests use `//go:build integration` build tag, run via `make smoke`
- Unit tests use `httptest` servers and `testdata/` fixtures — no network calls

## CLI flags

```
msd <url> [flags]
  -o, --output         Download directory
  -c, --concurrency    Max concurrent downloads
  --request-delay      Delay between requests (e.g., "5s")
  --password           Album password
  --no-resume          Don't resume partial downloads
  --dry-run            List files without downloading
  -d, --debug          Debug output to stderr
  --version            Version info
```

## Adding a new site handler

1. Create `site/<name>/<name>.go`
2. Implement `site.Site` interface
3. Add `func init() { site.Register(&YourSite{}) }` 
4. Add blank import in `cmd/msd/main.go`
5. Add unit tests with `httptest` + fixtures in `testdata/`
6. Add integration test behind `//go:build integration`
