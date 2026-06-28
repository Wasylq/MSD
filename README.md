# MSD - Multi Site Downloader

[![CI](https://github.com/Wasylq/MSD/actions/workflows/ci.yml/badge.svg)](https://github.com/Wasylq/MSD/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Wasylq/MSD)](https://goreportcard.com/report/github.com/Wasylq/MSD)
[![codecov](https://codecov.io/gh/Wasylq/MSD/branch/master/graph/badge.svg?token=JZM3FGDXL0)](https://codecov.io/gh/Wasylq/MSD)

MSD resolves supported album, folder, creator, and post URLs into downloadable files, then downloads them concurrently with resume support. It is meant for public links first, while still allowing optional site credentials when a site blocks guest access or requires an account token.

## Supported Sites

| Site | URL shape | Notes |
|---|---|---|
| Bunkr | `https://bunkr.cr/a/<id>`, `https://bunkr.cr/f/<id>` | Public albums and single files with signed CDN links. |
| CoomerFans | `https://coomerfans.com/u/<service>/<id>/<name>`, `https://coomerfans.com/p/<post>/<id>/<service>` | Creator pages and single posts. Writes `post-links.txt`. |
| Cyberdrop | `https://cyberdrop.cr/f/<id>` | Public single files with signed CDN links. |
| Filester | `https://filester.me/f/<slug>` | Public folders with HTML pagination plus generated CDN links. |
| Gofile | `https://gofile.io/d/<id>` | Guest mode by default. Uses a configured token if provided. |
| Instagram | `https://www.instagram.com/<username>/` | Public profile media. Writes `post-links.txt`; filenames use `YYMMDD_N.ext`. |
| Kemono/Pawchive | `https://kemono.cr/<service>/user/<id>`, `https://pawchive.st/<service>/user/<id>` | Creator archives. Writes `post-links.txt`. |
| Pixeldrain | `https://pixeldrain.com/l/<id>`, `https://pixeldrain.com/u/<id>` | Public lists and single files. |
| Turbo | `https://turbo.cr/a/<id>`, `https://turbo.cr/d/<id>` | Public albums and single files with signed CDN links. |

See [docs/sites.md](docs/sites.md) for per-site behavior, limits, and authentication notes.

## Install

Pick one install route. Pre-built release binaries are simplest, packages are best for system installs, Docker is useful for isolated runs, and source builds are best for development.

### Option 1: pre-built binary

Download the archive for your platform from the [latest release](https://github.com/Wasylq/MSD/releases/latest), extract it, and put `msd` on your `PATH`.

Asset names follow this pattern:

```text
msd-<version>-<os>-<arch>.tar.gz
msd-<version>-windows-amd64.zip
```

Release builds currently target:

| OS | Architectures |
|---|---|
| Linux | `amd64`, `arm64` |
| macOS | `amd64`, `arm64` |
| Windows | `amd64` |

Linux example:

```bash
VERSION=$(curl -sIL -o /dev/null -w '%{url_effective}' https://github.com/Wasylq/MSD/releases/latest | sed 's|.*/||')
ARCH=amd64

curl -LO "https://github.com/Wasylq/MSD/releases/download/${VERSION}/msd-${VERSION}-linux-${ARCH}.tar.gz"
tar xzf "msd-${VERSION}-linux-${ARCH}.tar.gz"
sudo install -m 0755 msd /usr/local/bin/msd
msd --version
```

macOS example:

```bash
VERSION=$(curl -sIL -o /dev/null -w '%{url_effective}' https://github.com/Wasylq/MSD/releases/latest | sed 's|.*/||')
ARCH=arm64    # use amd64 for Intel Macs

curl -LO "https://github.com/Wasylq/MSD/releases/download/${VERSION}/msd-${VERSION}-darwin-${ARCH}.tar.gz"
tar xzf "msd-${VERSION}-darwin-${ARCH}.tar.gz"
sudo install -m 0755 msd /usr/local/bin/msd
msd --version
```

Windows PowerShell example:

```powershell
$Version = (Invoke-RestMethod -Uri "https://api.github.com/repos/Wasylq/MSD/releases/latest").tag_name

Invoke-WebRequest -Uri "https://github.com/Wasylq/MSD/releases/download/$Version/msd-$Version-windows-amd64.zip" -OutFile msd.zip
Expand-Archive -Path msd.zip -DestinationPath .

New-Item -ItemType Directory -Force -Path "$env:USERPROFILE\bin" | Out-Null
Move-Item -Force msd.exe "$env:USERPROFILE\bin\msd.exe"
[Environment]::SetEnvironmentVariable("Path", "$env:Path;$env:USERPROFILE\bin", "User")
```

Restart the shell after changing `PATH`, then run:

```powershell
msd --version
```

### Option 2: Linux packages

Each tagged release publishes `.deb` and `.rpm` packages.

```bash
TAG=$(curl -sIL -o /dev/null -w '%{url_effective}' https://github.com/Wasylq/MSD/releases/latest | sed 's|.*/||')
VERSION=${TAG#v}
ARCH=amd64

# Debian / Ubuntu
curl -LO "https://github.com/Wasylq/MSD/releases/download/${TAG}/msd_${VERSION}_${ARCH}.deb"
sudo dpkg -i "msd_${VERSION}_${ARCH}.deb"

# Fedora / RHEL
curl -LO "https://github.com/Wasylq/MSD/releases/download/${TAG}/msd-${VERSION}-1.x86_64.rpm"
sudo rpm -i "msd-${VERSION}-1.x86_64.rpm"
```

Arch Linux:

```bash
yay -S msd
```

### Option 3: Docker

```bash
docker pull ghcr.io/wasylq/msd:latest
docker run --rm ghcr.io/wasylq/msd:latest --help
```

For real downloads, mount an output directory:

```bash
docker run --rm \
  --user "$(id -u):$(id -g)" \
  -v "$PWD/downloads:/data" \
  -w /data \
  ghcr.io/wasylq/msd:latest \
  'https://pixeldrain.com/l/<id>'
```

See [docs/docker.md](docs/docker.md) for config mounts, credentials, image tags, and troubleshooting.

### Option 4: build from source

Requires Go 1.26.3 or newer, matching the `go` directive in [go.mod](go.mod).

```bash
git clone https://github.com/Wasylq/MSD
cd MSD
make build
./msd --version
```

Or install directly:

```bash
go install github.com/Wasylq/MSD/cmd/msd@latest
```

## Quick Start

Preview what would be downloaded:

```bash
msd --dry-run 'https://pixeldrain.com/l/<id>'
```

Download into the current directory:

```bash
msd 'https://pixeldrain.com/l/<id>'
```

Choose an output directory:

```bash
msd -o downloads 'https://pawchive.st/patreon/user/11111111'
```

Download multiple URLs in one run:

```bash
msd -o downloads \
  'https://pixeldrain.com/l/<id>' \
  'https://filester.me/f/<slug>'
```

Use a password for protected albums:

```bash
msd --password 'album-password' 'https://gofile.io/d/<id>'
```

Use a Gofile token when available:

```bash
MSD_GOFILE_TOKEN='your-token' msd 'https://gofile.io/d/<id>'
```

Or place it in `config.yaml`:

```yaml
sites:
  gofile:
    account_token: your-token
```

Keys are optional. MSD tries guest/no-key access first unless a token is provided. If a site rejects anonymous access, MSD reports an authentication or rate-limit error instead of requiring credentials up front.

## Output Layout

Downloads go under the configured output directory. By default, MSD uses the user's Downloads directory; `--output`, `MSD_DOWNLOAD_DIR`, or `download_dir` can override it. If the site provides an album/archive name, MSD creates a sanitized subdirectory:

```text
downloads/
  kemono-patreon-creator/
    2026-03-18 - Post Title - 33333333 - 01 - file.jpg
    post-links.txt
```

Files are written to `.part` paths first, then renamed after completion. Existing complete files are skipped. Existing partial files are resumed unless `--no-resume` is set.

For creator/archive sites that expose source posts, MSD writes `post-links.txt` beside the downloaded files.

## Config File

Optional YAML config lives at:

| Platform | Path |
|---|---|
| Linux | `~/.config/msd/config.yaml` |
| macOS | `~/Library/Application Support/msd/config.yaml` |
| Windows | `%APPDATA%\msd\config.yaml` |

Start from [config.example.yaml](config.example.yaml):

```bash
mkdir -p ~/.config/msd
cp config.example.yaml ~/.config/msd/config.yaml
chmod 600 ~/.config/msd/config.yaml
```

`download_dir` supports `~`, `$HOME`, environment variables, and `${XDG_DOWNLOAD_DIR}` for the platform Downloads folder.

Precedence, highest first:

1. CLI flags
2. Environment variables
3. Config file
4. Built-in defaults

See [docs/configuration.md](docs/configuration.md) for every config key and credential source.

## Documentation

| Document | Contents |
|---|---|
| [docs/usage.md](docs/usage.md) | CLI flags, examples, output behavior, and troubleshooting |
| [docs/configuration.md](docs/configuration.md) | Config file reference, environment variables, credential precedence |
| [docs/sites.md](docs/sites.md) | Supported sites, URL patterns, site-specific notes |
| [docs/docker.md](docs/docker.md) | Docker image tags, volumes, config, credentials |
| [docs/architecture.md](docs/architecture.md) | Internal design for contributors |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Development workflow and adding site handlers |

## Development

```bash
make test
make lint
```

Live integration tests use real target sites and are intentionally manual:

```bash
make smoke
make smoke-one SITE=pixeldrain
```

Gofile live smoke tests may require `MSD_GOFILE_TOKEN`, `GOFILE_TOKEN`, or `sites.gofile.account_token` depending on the network and content.

## Troubleshooting

**No site handler matches URL**: check [docs/sites.md](docs/sites.md) for supported URL shapes.

**Authentication required**: the URL may need a password, account token, premium account, or the site may be blocking guest access.

**Rate limited**: wait and retry with lower concurrency or a longer request delay.

**Site structure changed**: the target site changed its HTML/API. Run with `-d` and open an issue with the URL shape and error.
