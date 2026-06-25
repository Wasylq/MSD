# MSD

MSD is a multi-site downloader for public albums and creator archives. It resolves a supported URL into a file list, then downloads files concurrently with resume support into an organized output directory.

## Supported Sites

| Site | URL shape | Notes |
|---|---|---|
| Bunkr | `https://bunkr.cr/a/<id>`, `https://bunkr.cr/f/<id>` | Public albums and single files with short-lived CDN links. |
| CoomerFans | `https://coomerfans.com/u/<service>/<id>/<name>`, `https://coomerfans.com/p/<post>/<id>/<service>` | Creator pages and single posts. Writes `post-links.txt` beside downloaded files. |
| Pixeldrain | `https://pixeldrain.com/l/<id>` | Public lists. |
| Filester | `https://filester.me/f/<slug>` | HTML pagination plus short-lived CDN links. |
| Gofile | `https://gofile.io/d/<id>` | Guest access may be IP-blocked or rate-limited; use `MSD_GOFILE_TOKEN` or `GOFILE_TOKEN`. |
| Kemono/Pawchive | `https://kemono.cr/<service>/user/<id>`, `https://pawchive.st/<service>/user/<id>` | Writes `post-links.txt` beside downloaded files. |
| Turbo | `https://turbo.cr/a/<id>`, `https://turbo.cr/d/<id>` | Public albums and single files with short-lived CDN links. |

## Build

```bash
make build
```

The binary is written to `./msd`.

For development:

```bash
go run ./cmd/msd --help
```

## Usage

Download into the current directory:

```bash
msd 'https://pixeldrain.com/l/<id>'
```

Choose an output directory:

```bash
msd -o downloads 'https://pawchive.st/patreon/user/11111111'
```

Preview files without downloading:

```bash
msd --dry-run 'https://pawchive.st/patreon/user/11111111'
```

Download multiple URLs in one run:

```bash
msd -o downloads 'https://pixeldrain.com/l/<id>' 'https://filester.me/f/<slug>'
```

Use a password for protected albums:

```bash
msd --password 'album-password' 'https://gofile.io/d/<id>'
```

Use a Gofile account token when guest access is blocked or the content requires an account:

```bash
MSD_GOFILE_TOKEN='your-token' msd 'https://gofile.io/d/<id>'
```

Or place it in your config file:

```yaml
sites:
  gofile:
    account_token: your-token
```

Download Kemono/Pawchive thumbnails instead of full attachment files:

```bash
msd --kemono-thumbnails 'https://pawchive.st/patreon/user/11111111'
```

Enable debug logging:

```bash
msd -d 'https://pixeldrain.com/l/<id>'
```

## Output Layout

Albums are saved under the configured output directory. If the site provides an album name, MSD creates a subdirectory with a sanitized version of that name.

Kemono/Pawchive filenames use:

```text
YYYY-MM-DD - Post Title - PostID - NN - OriginalFilename.ext
```

Downloads are written to `.part` files first, then renamed when complete. If a complete file already exists, MSD skips it.

## Configuration

MSD loads configuration from:

| Platform | Path |
|---|---|
| Linux | `~/.config/msd/config.yaml` |
| macOS | `~/Library/Application Support/msd/config.yaml` |
| Windows | `%APPDATA%\msd\config.yaml` |

See `config.example.yaml` for a commented template.

Precedence, highest first:

1. CLI flags
2. Environment variables
3. Config file
4. Built-in defaults

Supported environment variables:

| Variable | Purpose |
|---|---|
| `MSD_DOWNLOAD_DIR` | Default download directory. |
| `MSD_CONCURRENCY` | Default concurrent download count. |
| `MSD_GOFILE_TOKEN` | Gofile account token. |
| `GOFILE_TOKEN` | Alternate Gofile account token name. |

Site credentials can also be set in `config.yaml`:

```yaml
sites:
  gofile:
    account_token: your-token
```

## Tests

Unit tests:

```bash
make test
```

Live integration smoke tests:

```bash
make smoke
```

Run one site smoke test:

```bash
make smoke-one SITE=pixeldrain
```

Gofile live smoke tests require `MSD_GOFILE_TOKEN` or `GOFILE_TOKEN`.

Gofile may return `rate limited` or `authentication required` for anonymous requests from some networks. That usually means Gofile is blocking or throttling guest account creation for the current IP, not that URL parsing is broken.

## Notes

Sites change APIs, anti-bot rules, and rate limits. If a handler starts returning `site structure changed`, `rate limited`, or authentication errors, verify the URL in a browser and run with `-d` for request-level diagnostics.
