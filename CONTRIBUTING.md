# Contributing

This project is library-first: site-specific behavior lives under `site/<name>/`, while `cmd/msd/` is a thin CLI wrapper around the shared engine.

## Development

Run the default checks before sending changes:

```bash
make test
make lint
```

For a smaller loop:

```bash
go test ./site/<name> ./engine
```

Live integration tests use the `integration` build tag:

```bash
make smoke-one SITE=<name>
```

Do not make unit tests depend on the network. Use `httptest` and fixtures for normal tests.

## Adding a Site Handler

1. Create `site/<name>/<name>.go`.
2. Implement `site.Site`.
3. Register the handler with `func init() { site.Register(&YourSite{}) }`.
4. Add a blank import in `cmd/msd/main.go`.
5. Add unit tests using `httptest`.
6. Add an integration test behind `//go:build integration` when a stable public sample exists.
7. Document site-specific credentials, limits, or flags in `README.md`.

The handler should return typed errors from `site/site.go` whenever possible:

| Error | Use when |
|---|---|
| `site.ErrNotFound` | URL, album, folder, or file does not exist. |
| `site.ErrAuthRequired` | Password, token, premium account, or other credential is required. |
| `site.ErrRateLimited` | The remote site returns rate-limit or anti-abuse throttling. |
| `site.ErrSiteChanged` | Parsing failed because the site structure changed. |

## Handler Guidelines

- Prefer structured APIs over scraping when available.
- Keep expiring CDN URLs out of `Album.Files`; resolve them in `DownloadRequest`.
- Store per-resolve download links on the handler when the site requires it.
- Set realistic `DefaultConcurrency`, `DefaultResolveDelay`, and `DefaultDownloadDelay` values.
- Preserve original filenames when possible; let the engine sanitize filesystem names.
- Keep auth tokens out of logs and tests.
- Put network-dependent tests behind the `integration` build tag.

## CLI Guidelines

- The CLI should translate typed site errors into actionable messages.
- Keep site-specific flags rare. Prefer config/env vars for credentials.
- `--dry-run` must resolve metadata only and never create downloads.
- Debug logging goes to stderr and should not interfere with stdout usage.
