# Architecture

This document describes MSD internals for contributors. For user-facing commands, see [usage.md](usage.md). For adding a site, see [CONTRIBUTING.md](../CONTRIBUTING.md).

## Overview

```text
cmd/msd/main.go
  |
  |-- loads config
  |-- matches URL with site.Match()
  |-- applies site-specific config
  |-- calls Site.Resolve()
  |-- passes Album to engine.Engine.Download()

site/
  |
  |-- registry.go       global site registry
  |-- site.go           Site interface and typed errors
  |-- <name>/           site-specific resolver/downloader

engine/
  |
  |-- engine.go         album orchestration, concurrency, post-links
  |-- download.go       HTTP download, resume, progress callbacks
  |-- retry.go          retry policy
```

## Site Registry

Site handlers register themselves at package init time:

```go
func init() { site.Register(&Gofile{}) }
```

The CLI blank-imports site packages so their `init` functions run. At runtime:

```go
s := site.Match(rawURL)
```

`site.Match` iterates registered handlers and returns the first one whose `Match(url)` method accepts the URL.

## Site Interface

All handlers implement:

```go
type Site interface {
    Name() string
    Match(url string) bool
    Resolve(ctx context.Context, url string, password string) (*Album, error)
    DownloadRequest(ctx context.Context, file File) (*DownloadRequest, error)
    DefaultConcurrency() int
    DefaultResolveDelay() time.Duration
    DefaultDownloadDelay() time.Duration
}
```

`Resolve` turns a user URL into an `Album`:

```go
type Album struct {
    ID        string
    Name      string
    Files     []File
    PostLinks []string
}
```

`DownloadRequest` turns a `File` into a final HTTP request. This split lets handlers defer short-lived URL signing until the download starts.

## Engine

The engine is site-agnostic:

1. Creates the output directory.
2. Writes `post-links.txt` when `Album.PostLinks` is populated.
3. Applies concurrency and download delay defaults.
4. Starts one worker per file, limited by concurrency.
5. Calls `site.DownloadRequest`.
6. Downloads to `.part`, resumes when possible, then renames to the final path.

The engine does not parse site HTML/API responses and does not know about site credentials.

## Config Flow

`internal/config` starts with built-in defaults, including the user's Downloads
directory, then loads YAML from the platform config path and applies environment
overrides. Download paths support `~`, normal environment variables, and the
special `${XDG_DOWNLOAD_DIR}` value.

Current precedence:

```text
CLI flags > environment variables > config file > built-in defaults
```

The CLI applies site-specific config after URL matching:

```go
applySiteConfig(s, cfg)
```

Today this wires `sites.gofile.account_token` into the Gofile handler. Future site credentials should follow the same pattern: config owns parsing and precedence, site handlers expose small setters for resolved values.

## Typed Errors

Handlers should wrap the typed errors from `site/site.go`:

| Error | Use when |
|---|---|
| `site.ErrNotFound` | URL, album, folder, or file does not exist. |
| `site.ErrAuthRequired` | Password, token, premium account, or another credential is required. |
| `site.ErrRateLimited` | The remote site returns throttling or anti-abuse responses. |
| `site.ErrSiteChanged` | Parsing failed because HTML/API shape changed. |

The CLI maps these to shorter user-facing messages.

## Handler Guidelines

- Prefer structured APIs over HTML scraping when available.
- Keep expiring CDN URLs out of persistent `Album.Files` when they can be regenerated later.
- Store per-resolve download links on the handler when the site requires it.
- Preserve original filenames where possible; the engine sanitizes filesystem names.
- Keep tokens out of logs and test fixtures.
- Unit tests must not depend on the live network. Use `httptest` and fixtures.
- Live network tests belong behind the `integration` build tag.

## Adding a Site

1. Create `site/<name>/<name>.go`.
2. Implement `site.Site`.
3. Register with `func init() { site.Register(&YourSite{}) }`.
4. Add a blank import in `cmd/msd/main.go`.
5. Add unit tests with local HTTP fixtures.
6. Add integration tests behind `//go:build integration` if a stable public sample exists.
7. Update [sites.md](sites.md), [usage.md](usage.md), and the README if user-facing behavior changes.
