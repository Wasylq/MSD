# Usage Manual

Technical reference for running `msd`.

- [CLI flags](#cli-flags)
- [Examples](#examples)
- [Output behavior](#output-behavior)
- [Resume behavior](#resume-behavior)
- [Errors](#errors)
- [Development commands](#development-commands)

## CLI Flags

`msd <url>... [flags]`

| Flag | Type | Default | Description |
|---|---|---|---|
| `--output`, `-o` | string | user Downloads directory | Directory where downloads are written. Supports `~`, `$HOME`, environment variables, and `${XDG_DOWNLOAD_DIR}`. |
| `--concurrency`, `-c` | int | site default | Max concurrent downloads. |
| `--request-delay` | duration | site default | Delay between download requests, for example `500ms`, `2s`, `1m`. |
| `--password` | string | empty | Password for sites that support password-protected folders. |
| `--no-resume` | bool | false | Ignore existing `.part` files and restart downloads. |
| `--dry-run` | bool | false | Resolve and print files without downloading. |
| `--kemono-thumbnails` | bool | false | Download Kemono/Pawchive thumbnails instead of original attachment files. |
| `--debug`, `-d` | count | 0 | Enable debug logs on stderr. Stackable. |
| `--help`, `-h` | bool | false | Show command help. |
| `--version` | bool | false | Print version, commit, and build date. |

## Examples

Preview a URL:

```bash
msd --dry-run 'https://pixeldrain.com/l/<id>'
```

Download one URL:

```bash
msd 'https://bunkr.cr/a/<id>'
```

Download into a specific directory:

```bash
msd -o /srv/downloads 'https://turbo.cr/a/<id>'
```

Use the user's platform Downloads directory explicitly:

```bash
msd -o '${XDG_DOWNLOAD_DIR}/msd' 'https://turbo.cr/a/<id>'
```

Download several URLs:

```bash
msd -o downloads \
  'https://pixeldrain.com/l/<id>' \
  'https://filester.me/f/<slug>' \
  'https://coomerfans.com/u/onlyfans/<id>/<name>'
```

Use a folder password:

```bash
msd --password 'folder-password' 'https://gofile.io/d/<id>'
```

Use a Gofile account token through the environment:

```bash
MSD_GOFILE_TOKEN='token' msd 'https://gofile.io/d/<id>'
```

Use config-based credentials:

```yaml
sites:
  gofile:
    account_token: token
```

Download Kemono/Pawchive thumbnails:

```bash
msd --kemono-thumbnails 'https://pawchive.st/patreon/user/<id>'
```

Reduce pressure on a site:

```bash
msd --concurrency 1 --request-delay 5s 'https://gofile.io/d/<id>'
```

## Output Behavior

MSD creates an album directory when the resolver returns an album/archive name:

```text
downloads/
  gofile-folder-name/
    file1.mp4
    file2.jpg
```

When no album name is available, files are written directly under `--output`.
If neither `--output` nor `download_dir` is set, MSD uses the user's Downloads
directory, including localized XDG user-dirs on Linux.

Filenames are sanitized for common filesystem restrictions:

- `/` and `\` become `_`
- Windows-reserved characters are replaced
- reserved device names are prefixed
- names are truncated to fit common filesystem limits

For creator/archive sites that expose source posts, MSD writes:

```text
post-links.txt
```

This file is informational and contains one source post URL per line.

## Resume Behavior

Downloads are written as:

```text
filename.ext.part
```

When the download completes, the `.part` file is renamed to the final filename.

Default behavior:

- If the final file exists and size matches, MSD skips it.
- If a `.part` file exists, MSD tries an HTTP range request and resumes.
- If the server rejects the range, MSD restarts that file.

Use `--no-resume` to force fresh downloads.

## Errors

MSD maps site handler errors into user-facing messages:

| Error | Meaning | What to try |
|---|---|---|
| `album or file not found` | URL does not exist or parser found no files. | Check the URL in a browser. |
| `authentication required` | Password, token, account, or premium access is needed. | Use `--password` or configure site credentials. |
| `rate limited by site` | Site throttled or blocked the current IP/session. | Wait, lower concurrency, increase delay, or use credentials. |
| `site structure changed` | The handler could not parse the current HTML/API. | Run with `-d` and report the URL shape. |

## Development Commands

Build:

```bash
make build
```

Run unit tests:

```bash
make test
```

Run vet and lint:

```bash
make lint
```

Run live integration smoke tests:

```bash
make smoke
make smoke-one SITE=gofile
```

Smoke tests use real network requests and are intentionally not part of normal CI.
