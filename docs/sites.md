# Supported Sites

This page lists the URL patterns MSD currently recognizes and the important behavior for each site.

## Summary

| Site | Handler | URL patterns |
|---|---|---|
| Bunkr | `bunkr` | `/a/<id>`, `/f/<id>`, `/i/<id>`, `/v/<id>` on supported Bunkr hosts |
| CoomerFans | `coomerfans` | `/u/<service>/<id>/<name>`, `/p/<post>/<id>/<service>` |
| Cyberdrop | `cyberdrop` | `/f/<id>` |
| Filester | `filester` | `/f/<slug>` |
| Gofile | `gofile` | `/d/<id>` |
| Kemono/Pawchive | `kemono` | `/<service>/user/<id>` |
| Pixeldrain | `pixeldrain` | `/l/<id>`, `/u/<id>` |
| Turbo | `turbo` | `/a/<id>`, `/d/<id>`, `/v/<id>` |

## Bunkr

Examples:

```text
https://bunkr.cr/a/<id>
https://bunkr.cr/f/<id>
```

Notes:

- Resolves public albums and single files.
- Uses Bunkr signing/bridge APIs for short-lived media URLs.
- Default concurrency and request delays are conservative because generated links and CDN behavior can change quickly.

## CoomerFans

Examples:

```text
https://coomerfans.com/u/onlyfans/<id>/<name>
https://coomerfans.com/u/fansly/<id>/<name>
https://coomerfans.com/p/<post>/<creator>/<service>
```

Notes:

- Creator URLs are paginated. MSD follows `Next` links and fetches each post page.
- Post URLs resolve the media from that single post.
- Media links are taken from CoomerFans storage hosts.
- `post-links.txt` is written for traceability.

## Cyberdrop

Example:

```text
https://cyberdrop.cr/f/<id>
```

Notes:

- Resolves public single-file pages.
- Uses Cyberdrop's file info API for filename and size.
- Generates the final signed CDN URL right before download.

## Filester

Example:

```text
https://filester.me/f/<slug>
```

Notes:

- Resolves public folder pages and follows pagination.
- Generates final download links through Filester's public view API.

## Gofile

Example:

```text
https://gofile.io/d/<id>
```

Notes:

- Tries to work without a provided key.
- Uses a configured token if one is provided.
- Supports `--password` for password-protected content.
- Guest access can be blocked by IP, rate limits, account requirements, or premium-only content.

Credential order:

1. `MSD_GOFILE_TOKEN`
2. `GOFILE_TOKEN`
3. `sites.gofile.account_token`
4. cached token
5. automatic account creation

## Kemono / Pawchive

Examples:

```text
https://kemono.cr/patreon/user/<id>
https://pawchive.st/patreon/user/<id>
```

Notes:

- Resolves creator archives from API endpoints.
- Writes `post-links.txt`.
- Default downloads use original attachment files.
- `--kemono-thumbnails` downloads thumbnail URLs instead.

Filename pattern:

```text
YYYY-MM-DD - Post Title - PostID - NN - OriginalFilename.ext
```

## Pixeldrain

Examples:

```text
https://pixeldrain.com/l/<id>
https://pixeldrain.com/u/<id>
```

Notes:

- Resolves public lists and public single files.
- No credentials are currently supported.

## Turbo

Examples:

```text
https://turbo.cr/a/<id>
https://turbo.cr/d/<id>
```

Notes:

- Resolves public albums and single files.
- Uses Turbo signing APIs for final CDN URLs.

## Common Failure Modes

| Symptom | Likely cause |
|---|---|
| No handler matches | URL shape is not supported. |
| Not found | URL is private, removed, malformed, or parser found no downloadable files. |
| Authentication required | Password, account token, or premium access is needed. |
| Rate limited | Site or CDN throttled the current IP/session. |
| Site structure changed | HTML/API changed and the handler needs maintenance. |
