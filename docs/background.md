# Background Runs

MSD is a command-line downloader, not a resident daemon. To keep an album or
Instagram profile up to date, schedule MSD to run periodically with the same URL
list. Existing complete files are skipped, partial files are resumed, and newly
discovered files are downloaded.

This works well for public album, creator, and Instagram profile URLs. For
Instagram profile URLs, MSD downloads public media returned by the profile feed,
including photos, post videos, carousels, and reels that appear in that feed.

## URL List

Create a plain text file with one URL per line:

```text
https://www.instagram.com/salmahayek/
https://www.instagram.com/reel/<shortcode>/
https://pixeldrain.com/l/<id>
```

Avoid comments or blank lines if you use the simple scripts below.

## Linux systemd User Timer

Create the URL list:

```bash
mkdir -p ~/.config/msd
nano ~/.config/msd/urls.txt
```

Create a wrapper script:

```bash
mkdir -p ~/.local/bin
nano ~/.local/bin/msd-sync.sh
chmod +x ~/.local/bin/msd-sync.sh
```

Script contents:

```bash
#!/usr/bin/env bash
set -euo pipefail

URLS="$HOME/.config/msd/urls.txt"
OUT="${MSD_DOWNLOAD_DIR:-$HOME/Downloads/msd}"
LOCK="/tmp/msd-sync.lock"

exec 9>"$LOCK"
flock -n 9 || exit 0

xargs msd --output "$OUT" --concurrency 1 --request-delay 2s < "$URLS"
```

Create `~/.config/systemd/user/msd-sync.service`:

```ini
[Unit]
Description=MSD scheduled download run

[Service]
Type=oneshot
ExecStart=%h/.local/bin/msd-sync.sh
```

Create `~/.config/systemd/user/msd-sync.timer`:

```ini
[Unit]
Description=Run MSD every 30 minutes

[Timer]
OnBootSec=5min
OnUnitActiveSec=30min
Persistent=true

[Install]
WantedBy=timers.target
```

Enable it:

```bash
systemctl --user daemon-reload
systemctl --user enable --now msd-sync.timer
systemctl --user list-timers msd-sync.timer
```

View logs:

```bash
journalctl --user -u msd-sync.service -f
```

## macOS launchd

Create the URL list:

```bash
mkdir -p "$HOME/Library/Application Support/msd"
nano "$HOME/Library/Application Support/msd/urls.txt"
```

Create a wrapper script:

```bash
mkdir -p "$HOME/Library/Scripts"
nano "$HOME/Library/Scripts/msd-sync.sh"
chmod +x "$HOME/Library/Scripts/msd-sync.sh"
```

Script contents:

```bash
#!/bin/sh
set -eu

URLS="$HOME/Library/Application Support/msd/urls.txt"
OUT="${MSD_DOWNLOAD_DIR:-$HOME/Downloads/msd}"
LOCKDIR="${TMPDIR:-/tmp}/msd-sync.lock"

if ! mkdir "$LOCKDIR" 2>/dev/null; then
  exit 0
fi
trap 'rmdir "$LOCKDIR"' EXIT

xargs msd --output "$OUT" --concurrency 1 --request-delay 2s < "$URLS"
```

Create `~/Library/LaunchAgents/com.local.msd-sync.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.local.msd-sync</string>

  <key>ProgramArguments</key>
  <array>
    <string>/bin/sh</string>
    <string>-lc</string>
    <string>$HOME/Library/Scripts/msd-sync.sh</string>
  </array>

  <key>StartInterval</key>
  <integer>1800</integer>

  <key>RunAtLoad</key>
  <true/>

  <key>StandardOutPath</key>
  <string>/tmp/msd-sync.out.log</string>

  <key>StandardErrorPath</key>
  <string>/tmp/msd-sync.err.log</string>
</dict>
</plist>
```

Enable it:

```bash
launchctl unload "$HOME/Library/LaunchAgents/com.local.msd-sync.plist" 2>/dev/null || true
launchctl load "$HOME/Library/LaunchAgents/com.local.msd-sync.plist"
launchctl start com.local.msd-sync
```

View logs:

```bash
tail -f /tmp/msd-sync.out.log /tmp/msd-sync.err.log
```

## Windows Task Scheduler

Create the URL list at:

```text
%APPDATA%\msd\urls.txt
```

Example PowerShell setup:

```powershell
New-Item -ItemType Directory -Force -Path "$env:APPDATA\msd" | Out-Null
notepad "$env:APPDATA\msd\urls.txt"
```

Create `%APPDATA%\msd\msd-sync.ps1`:

```powershell
$ErrorActionPreference = "Stop"

$Urls = Get-Content "$env:APPDATA\msd\urls.txt" | Where-Object { $_.Trim() -ne "" }
$Out = if ($env:MSD_DOWNLOAD_DIR) { $env:MSD_DOWNLOAD_DIR } else { "$env:USERPROFILE\Downloads\msd" }
$Lock = "$env:TEMP\msd-sync.lock"

if (Test-Path $Lock) {
    exit 0
}

New-Item -ItemType File -Path $Lock -Force | Out-Null
try {
    msd --output $Out --concurrency 1 --request-delay 2s @Urls
}
finally {
    Remove-Item $Lock -Force -ErrorAction SilentlyContinue
}
```

Register a task that runs every 30 minutes:

```powershell
$Action = New-ScheduledTaskAction `
  -Execute "powershell.exe" `
  -Argument "-NoProfile -ExecutionPolicy Bypass -File `"$env:APPDATA\msd\msd-sync.ps1`""

$Trigger = New-ScheduledTaskTrigger -Once -At (Get-Date).AddMinutes(1) `
  -RepetitionInterval (New-TimeSpan -Minutes 30)

Register-ScheduledTask `
  -TaskName "MSD Sync" `
  -Action $Action `
  -Trigger $Trigger `
  -Description "Periodically download new MSD files" `
  -Force
```

Run it once immediately:

```powershell
Start-ScheduledTask -TaskName "MSD Sync"
```

View task status:

```powershell
Get-ScheduledTaskInfo -TaskName "MSD Sync"
```

## Tuning

- Use `--concurrency 1` and `--request-delay 2s` or higher for Instagram and
  other sites that rate limit guest traffic.
- Increase the interval, for example to 1 to 6 hours, if a site starts returning
  rate-limit or authentication errors.
- Keep using the same output directory. This lets MSD skip files it already
  downloaded.
- Do not use `--dry-run` in scheduled jobs unless you only want logs.
- If the scheduler cannot find `msd`, replace `msd` in the wrapper script with
  the absolute path from `which msd` on Linux/macOS or `Get-Command msd` on
  Windows.
- If a machine is forcibly powered off during a run, remove the lock file if the
  next scheduled run keeps exiting immediately.
