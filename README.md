# MoanDrop

[![CI](https://github.com/Anastylosis/MoanDrop/actions/workflows/ci.yml/badge.svg)](https://github.com/Anastylosis/MoanDrop/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/Anastylosis/MoanDrop/branch/master/graph/badge.svg)](https://codecov.io/gh/Anastylosis/MoanDrop)
[![Go Report Card](https://goreportcard.com/badge/github.com/Anastylosis/MoanDrop)](https://goreportcard.com/report/github.com/Anastylosis/MoanDrop)

Subtitles for your videos, found by what the file *is* instead of what it's
called. MoanDrop fingerprints a video the same way [Stash](https://stashapp.cc)
does, asks the [moansubs](https://moansubs.org) database, and writes the
subtitle beside the video as `<stem>.<lang>.srt` — the sidecar convention
Plex, Jellyfin, Kodi and VLC all pick up with no extra step.

You do **not** need Stash, an account, or an API key. Finding and
downloading subtitles is anonymous by design; only sharing your own
subtitles (`push`) needs a free account token.

**Your videos never leave your machine.** Fingerprinting is local; by
default the lookup sends bucketed hash prefixes, and which candidate
actually matched is decided on your side — the server can't tell.

## Install

Grab a binary from the releases page, or:

```sh
go install github.com/Anastylosis/MoanDrop@latest
```

`match` and `push` use `ffmpeg`/`ffprobe` for the perceptual hash. You do
not need to install anything: if neither binary is on your `PATH`,
MoanDrop downloads a pinned build (ffmpeg 6.1 on linux/macOS, the
gyan.dev essentials build on Windows, pinned at 8.1.2 — see the note
below) into
`$XDG_CACHE_HOME/moandrop/ffmpeg/<version>/` (or the OS equivalent of
`os.UserCacheDir()`) the first time it's needed, verifies its checksum,
and reuses it on every later run. Resolution order: `--ffmpeg`/`--ffprobe`
flag, then `MOANDROP_FFMPEG`/`MOANDROP_FFPROBE`, then `PATH`, then the
cache, then the download. Set `MOANDROP_NO_DOWNLOAD=1` to disable the
download step entirely; without ffmpeg reachable any other way,
`--no-phash` still matches byte-identical files by their file hash alone.

Windows' pin trails the others because gyan.dev only mirrors its most
recent release under a fixed, versioned URL — it doesn't keep an archive
of older versions the way the linux/macOS sources do.

## Use

```sh
# What does the database have for this file?
moandrop match "Some Scene (1080p).mp4"

# Write the best English subtitle beside the video
moandrop match --lang en --write "Some Scene (1080p).mp4"

# Share a subtitle you already have (needs a token — create an account
# on the server, then export MOANDROP_TOKEN or pass --token)
moandrop push "Some Scene (1080p).mp4" "Some Scene (1080p).en.srt"
```

Exit codes: `0` success, `1` error, `2` no match — so a file-manager script
can tell "nothing found" from "something broke".

### What the results mean

- **exact** — byte-identical file. The subtitle fits.
- **high** — same video, different encode (perceptual match, runtime within
  a second). Sync is almost always fine.
- **offer** — worth trying, not promised: a looser perceptual match, or a
  subtitle authored for *another cut* of the same video. When a timing
  shift for a cut is known, the server applies it on download; when it
  says the sync is unverified, believe it.
- **AI** — machine-transcribed, unreviewed. Human-made tracks always sort
  first. AI tracks are usually accurate but may mishear names and slang;
  most of the database is AI-transcribed, so expect the badge often.

## Status

Headless CLI (this binary) is the working core, with ffmpeg auto-download
already in place. Planned, in order: a drag-and-drop desktop window over
the same engine, then file-manager integration (right-click → find
subtitles). The engine lives in `internal/core` so the window and the CLI
cannot drift apart.

## Privacy & scope

- Lookups are anonymous; no account, no cookies, no telemetry.
- The default lookup never sends full fingerprints (`--exact` opts into a
  wider server-side search that does).
- MoanDrop is a client for moansubs.org (or your own node via `--server`).
  The database is community-fed: `push` what you have.
