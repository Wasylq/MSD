# Docker

MSD ships as a container image on GitHub Container Registry.

## Image

| Registry | Path |
|---|---|
| GHCR | `ghcr.io/wasylq/msd` |

Architectures:

| Platform | Status |
|---|---|
| `linux/amd64` | built |
| `linux/arm64` | built |

## Tags

| Tag | Meaning |
|---|---|
| `latest` | Latest released version. |
| `vX.Y.Z`, `vX.Y`, `vX` | Release tracks. |
| `master` or `main` | Development image from the default branch. |
| `sha-<short>` | Development image for a specific commit. |
| `pr-<n>` | Pull request build, when CI runs on a PR. |

Pin to a version tag for repeatable systems.

## Quick Start

```bash
docker run --rm ghcr.io/wasylq/msd:latest --help
```

The image entrypoint is `msd`, so pass normal CLI args directly:

```bash
docker run --rm ghcr.io/wasylq/msd:latest --dry-run 'https://pixeldrain.com/l/<id>'
```

## Downloads Volume

Use `/data` as the working directory and mount it to the host:

```bash
mkdir -p downloads

docker run --rm \
  --user "$(id -u):$(id -g)" \
  -v "$PWD/downloads:/data" \
  -w /data \
  ghcr.io/wasylq/msd:latest \
  'https://pixeldrain.com/l/<id>'
```

The `--user` flag avoids root-owned files on Linux bind mounts.

## Config Volume

The image does not set a custom `XDG_CONFIG_HOME`, so either mount the expected home config path or set `XDG_CONFIG_HOME` yourself.

Recommended pattern:

```bash
mkdir -p config/msd downloads
cp config.example.yaml config/msd/config.yaml

docker run --rm \
  --user "$(id -u):$(id -g)" \
  -e XDG_CONFIG_HOME=/config \
  -v "$PWD/config:/config" \
  -v "$PWD/downloads:/data" \
  -w /data \
  ghcr.io/wasylq/msd:latest \
  'https://gofile.io/d/<id>'
```

MSD will read:

```text
/config/msd/config.yaml
```

## Credentials

Prefer passing tokens through environment variables:

```bash
docker run --rm \
  --user "$(id -u):$(id -g)" \
  -e MSD_GOFILE_TOKEN \
  -v "$PWD/downloads:/data" \
  -w /data \
  ghcr.io/wasylq/msd:latest \
  'https://gofile.io/d/<id>'
```

Or store them in the mounted config file:

```yaml
sites:
  gofile:
    account_token: your-token
```

Environment variables override config values.

## Building Locally

```bash
docker build \
  --build-arg GO_VERSION=$(./scripts/go-version.sh) \
  --build-arg VERSION=dev \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -t msd:dev .
```

Or:

```bash
make docker
```

## Troubleshooting

**Files are owned by root**: add `--user "$(id -u):$(id -g)"`.

**Config is not found**: set `XDG_CONFIG_HOME=/config` and mount a directory containing `/config/msd/config.yaml`.

**Downloads disappear after the container exits**: mount an output directory with `-v "$PWD/downloads:/data"` and run with `-w /data`.

**Network/auth errors**: reproduce outside Docker with the same URL. If only Docker fails, check DNS, proxy settings, and whether the token environment variable is passed into the container.
