# Contributing

## The one rule

MoanDrop must never claim more than it knows. Match confidence wording,
sync caveats and the AI badge come from `internal/core` and are shared
verbatim by the CLI and the window — a change that lets the two surfaces
phrase the same result differently, or that upgrades an unverified sync
to a promise, is wrong even when it looks nicer.

## Building

The GUI half is Fyne, which means cgo and, on Linux, GL/X11/Wayland
development headers (the exact package list is in
[`.github/workflows/ci.yml`](.github/workflows/ci.yml) as `apt-packages`).
The CLI works everywhere the build does; there is no headless-only build
tag — one binary carries both.

Nothing in this repository cross-compiles: Fyne's OpenGL bindings are
cgo-only, so per-OS binaries come from native runners (see release.yml's
`native-artifacts`). Don't send a PR that adds a GOOS/GOARCH matrix.

## Tests

`go test -race -count=1 ./...` must pass headless and with no outbound
network — CI enforces both (the fyne test driver needs no display, and
`offline-isolation` re-runs the suite with only loopback up). Tests that
exercise the ffmpeg cache must use the `setTempCache` helper; see
CLAUDE.md for the per-OS trap it papers over.

Changing the pinned ffmpeg builds re-opens the bit-exactness question:
the linux pin must re-pass mediahash's `TestBitExact` harness against a
real Stash library with zero mismatches before release.

## Cutting a release

Releases are tagged `vMAJOR.MINOR.PATCH`. Pushing the tag runs
[`.github/workflows/release.yml`](.github/workflows/release.yml): five
native binaries, .deb/.rpm packages, the AUR package, the Homebrew tap
formula, and an Announcements discussion.

```bash
git tag -a v0.2.0 -m "One-line summary"
git push origin v0.2.0
```
