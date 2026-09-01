# Security Policy

## Untrusted input

MoanDrop's inputs are video files the user names (treated as untrusted —
they typically arrive from downloads) and responses from the configured
moansubs server. Fingerprinting hands the video to ffmpeg with the path
passed as an argument vector, never a shell; the security boundary for
decoding hostile video is ffmpeg's, and the pinned builds are
auto-downloaded over HTTPS and verified against compiled-in sha256
checksums before a byte of them runs. Set `MOANDROP_NO_DOWNLOAD=1` to
forbid the download path entirely and use only a locally installed
ffmpeg.

Subtitle bodies from the server are size-capped and written atomically
next to the video as plain text; an existing sidecar is never replaced
without explicit confirmation. Lookups are anonymous and send hashes,
not the file.

## Reporting a vulnerability

If you find a security issue, please open a GitHub issue or email the
maintainer directly. There is no bug bounty program.
