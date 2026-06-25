# Configuration

MSD can run without a config file. Config is useful when you want persistent defaults, an output directory, slower request pacing, or optional site credentials.

## Location

| Platform | Path |
|---|---|
| Linux | `~/.config/msd/config.yaml` |
| macOS | `~/Library/Application Support/msd/config.yaml` |
| Windows | `%APPDATA%\msd\config.yaml` |

Create one from the template:

```bash
mkdir -p ~/.config/msd
cp config.example.yaml ~/.config/msd/config.yaml
chmod 600 ~/.config/msd/config.yaml
```

## Precedence

Highest priority wins:

1. CLI flags
2. Environment variables
3. Config file
4. Built-in defaults

Site credentials follow the same rule. A configured key is optional; MSD still tries the no-key/guest path when no key is present.

## Full Example

```yaml
download_dir: /srv/downloads/msd
concurrency: 3
request_delay: 1s
no_resume: false

sites:
  gofile:
    account_token: ""
```

## Keys

| Key | Type | Default | Description |
|---|---|---|---|
| `download_dir` | string | `.` | Default output directory. CLI: `--output`. Env: `MSD_DOWNLOAD_DIR`. |
| `concurrency` | int | site default | Max concurrent downloads. CLI: `--concurrency`. Env: `MSD_CONCURRENCY`. |
| `request_delay` | duration | site default | Delay between download requests. CLI: `--request-delay`. |
| `no_resume` | bool | false | Disable `.part` resume behavior. CLI: `--no-resume`. |
| `sites.gofile.account_token` | string | empty | Optional Gofile account token. Env: `MSD_GOFILE_TOKEN`, `GOFILE_TOKEN`. |

Go duration strings are accepted for `request_delay`, for example:

```yaml
request_delay: 500ms
request_delay: 2s
request_delay: 1m
```

## Environment Variables

| Variable | Purpose |
|---|---|
| `MSD_DOWNLOAD_DIR` | Default download directory. |
| `MSD_CONCURRENCY` | Default concurrent download count. |
| `MSD_GOFILE_TOKEN` | Preferred Gofile account token variable. |
| `GOFILE_TOKEN` | Alternate Gofile account token variable. |

## Gofile Token Behavior

Gofile is designed to work without a provided token where possible.

Resolution order:

1. `MSD_GOFILE_TOKEN`
2. `GOFILE_TOKEN`
3. `sites.gofile.account_token`
4. cached guest/account token at `~/.config/msd/gofile_token.json`
5. automatic account creation

If all no-key paths fail because Gofile blocks guest access from the current network or content requires a premium account, MSD returns an authentication or rate-limit error.

## Credential Safety

- Prefer env vars for short-lived or shared systems.
- If storing credentials in YAML, set file permissions to owner-only where possible.
- Do not commit your real `config.yaml`.
- Debug logs should not print configured token values.
