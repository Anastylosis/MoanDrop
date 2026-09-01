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

## GUI (desktop window)

Run `moandrop` with no arguments to open the window — a drop target and a
candidate list, not an application. Run `moandrop "Some Scene.mp4"` to open
it preloaded with that file and start matching immediately (this is the
form a file manager's "Open with" runs, `moandrop "%f"`).

- **Find a video**: drag it onto the window, or use File → Open Video….
- **First run**: a one-time 18+ confirmation, matching the server's own
  age gate on every human-facing page. Declining exits the app; accepting
  is remembered.
- **Privacy**: the window always shows the same claim the CLI prints
  before fingerprinting — "the video never leaves this machine" — fingerprints
  only ever leave the machine, never the file.
- **Settings**: File → Settings… holds the moansubs node to query (defaults
  to `https://moansubs.org`, same as the CLI), the account token (needed
  only for sharing and voting — finding and downloading stay anonymous),
  and what closing the window does — hide to the system tray (the
  default) or quit outright, for desktops where the tray icon doesn't
  show (see Tray, below). All three save for next launch, and the
  close-behavior choice takes effect immediately.
- **Results**: each card is titled by the database's own display title
  when it has one (curated, or derived from a cleaned upload filename),
  else by resolution/runtime/codec. Each release shows the same evidence wording as `match`
  (byte-identical, a verified/estimated/unknown-sync sibling cut, and so
  on), and every track lists its language. A generated track carries an
  "AI" badge; click it for the same explainer the CLI prints under a
  result list. Human-made tracks sort first.
- **Download**: click a track to write its sidecar beside the video. An
  existing sidecar is never replaced silently — a confirmation dialog asks
  first, same as `--overwrite` gates it on the CLI.
- **Vote**: every track has +1/-1 buttons — voting on the best cut of a
  scene helps it rise for the next person who looks it up. Voting needs an
  account token, prompted for on the spot if Settings hasn't set one yet;
  a down-vote asks for a one-line reason first, since the server requires
  one so down-votes stay accountable. A track you've voted on this session
  shows a "remove vote" button instead — retracting is intentionally
  session-local, since lookups are anonymous and the window has no way to
  know about a vote cast in an earlier session.
- **Share**: below the results, a small "Share what you have" section lists
  any subtitle files already sitting beside the video (the same
  `<stem>.<lang>.srt` sidecar convention the window writes, read in
  reverse) with a Share button per file; a "Share a subtitle..." button is
  always there too, opening a file picker for anything discovery didn't
  catch. Dropping a video together with one or more `.srt`/`.vtt` files
  pairs them explicitly for sharing, even with unconventional filenames.
  Sharing needs an account token, the same as voting does — Settings saves
  one in Preferences; absent that, `MOANDROP_TOKEN` works too, so a CLI
  user's env just works in the window (the preference wins when both are
  set). A push that matches a subtitle already on the node reports that
  calmly ("already on the node") rather than as an error — the server
  never stores identical bytes twice.
- **Did it fit?**: after downloading a subtitle authored for another cut,
  the window asks whether it lined up. The verdict (never a timing value)
  is reported with your token; enough independent "fits" mark the pairing
  **sync confirmed by users** in everyone's results, and misfit reports
  reach the moderators. Only offered when the server supports it.
- **No ffmpeg**: a dialog gives the same guidance `match` prints, with a
  button to fall back to exact-file (oshash-only) matching instead of
  installing ffmpeg — the GUI equivalent of `--no-phash`.
- **Tray**: by default, closing the window hides it to the system tray,
  keeping the drop target a click away; quit from the File menu or the
  tray entry. On GNOME the tray icon needs the AppIndicator extension —
  without it the hidden window is only reachable by running `moandrop`
  again, or by switching Settings' close behavior to quit outright.

### Build notes

The GUI needs CGO (Fyne's OpenGL/window-system bindings) and, on Linux,
X11/Wayland/GL development headers at build time — the headless CLI has
none of these requirements. On Windows, build with
`-ldflags -H=windowsgui` to suppress the console window the GUI would
otherwise open behind it; the release builds do (release.yml's
`windows-gui: true`).

### What the results mean

- **exact** — byte-identical file. The subtitle fits.
- **high** — same video, different encode (perceptual match, runtime within
  a second). Sync is almost always fine.
- **offer** — worth trying, not promised: a looser perceptual match, or a
  subtitle authored for *another cut* of the same video. When a timing
  shift for a cut is known, the server applies it on download; when it
  says the sync is unverified, believe it.
- **sync confirmed by users** — enough people reported this exact pairing
  plays in sync as served; a separate signal from a measured shift.
- **AI** — machine-transcribed, unreviewed. Human-made tracks always sort
  first. AI tracks are usually accurate but may mishear names and slang;
  most of the database is AI-transcribed, so expect the badge often.

## Shell integration

Right-click a video → find subtitles, without opening a terminal. Files
in `contrib/`.

**Linux**

- App launcher (`Exec=moandrop %f` — opens the GUI preloaded with that
  file):
  ```sh
  cp contrib/linux/moandrop.desktop ~/.local/share/applications/
  update-desktop-database ~/.local/share/applications/
  ```
- Nautilus script (headless: fingerprints, writes the sidecar, and
  reports the result with `notify-send`; works from Nemo and Caja too):
  ```sh
  mkdir -p ~/.local/share/nautilus/scripts
  cp contrib/linux/moandrop-match.sh ~/.local/share/nautilus/scripts/
  chmod +x ~/.local/share/nautilus/scripts/moandrop-match.sh
  ```
  The desktop entry names `Icon=moandrop` — the deb/rpm/AUR packages
  install it; for a manual install copy `internal/ui/icon.png` to
  `~/.local/share/icons/hicolor/256x256/apps/moandrop.png`. Right-click
  one or more videos → Scripts → moandrop-match.sh. Set
  `MOANDROP_LANG` (e.g. in the script itself, or your shell profile
  before launching the file manager) to change the language from the
  `en` default.

**Windows**

- Run `contrib\windows\install-context-menu.ps1` (per-user, no admin
  needed) — it finds `moandrop.exe` next to itself, or pass
  `-ExePath <path>`. `uninstall-context-menu.ps1` removes it.
- Prefer not to run scripts? Edit the path placeholder in
  `contrib\windows\install-context-menu.reg` and double-click it;
  `uninstall-context-menu.reg` removes it.
- Adds "Find subtitles (MoanDrop)" for `.mp4 .m4v .mkv .avi .wmv .flv
  .mov .mpg .mpeg`, running `moandrop.exe "%1"` — the GUI, preloaded
  with that file. On Windows 11 the entry lands under **Show more
  options** — the modern top-level menu needs an MSIX-packaged app, out
  of scope here.

**macOS**

Best-effort, no installer: see `contrib/macos/README.md` for an
Automator Quick Action that wraps the CLI.

## Status

The headless CLI and the desktop window (see above) both wrap the same
`internal/core` engine, so they cannot drift apart; ffmpeg auto-download
and file-manager integration (`contrib/`) are in place for both.

## Privacy & scope

- Lookups are anonymous; no account, no cookies, no telemetry.
- The default lookup never sends full fingerprints (`--exact` opts into a
  wider server-side search that does).
- MoanDrop is a client for moansubs.org (or your own node via `--server`).
  The database is community-fed: `push` what you have.
