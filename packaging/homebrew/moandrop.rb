# Template for the Homebrew formula. The release workflow renders this into
# Anastylosis/homebrew-tap as Formula/moandrop.rb, substituting VERSION and
# the four __SHA256_*__ placeholders with checksums from the release's
# SHA256SUMS.
#
# Binary, not source: a source formula would demand a Go toolchain plus cgo
# and GL headers from every user; the release already publishes per-OS
# binaries built natively.
#
# Keep this file and the workflow's placeholder list in step — the render
# step fails loudly on a leftover placeholder rather than shipping a formula
# that cannot compute a checksum.
class Moandrop < Formula
  desc "Find and share subtitles for your videos by fingerprint, not filename"
  homepage "https://github.com/Anastylosis/MoanDrop"
  # Explicit on purpose: Homebrew's URL scan misreads the arch suffix as the
  # version on macOS (see the fss formula for the full story).
  version "__VERSION__"
  license "GPL-3.0-only"

  on_macos do
    on_arm do
      url "https://github.com/Anastylosis/MoanDrop/releases/download/v__VERSION__/moandrop-v__VERSION__-darwin-arm64.tar.gz"
      sha256 "__SHA256_DARWIN_ARM64__"
    end
    on_intel do
      url "https://github.com/Anastylosis/MoanDrop/releases/download/v__VERSION__/moandrop-v__VERSION__-darwin-amd64.tar.gz"
      sha256 "__SHA256_DARWIN_AMD64__"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/Anastylosis/MoanDrop/releases/download/v__VERSION__/moandrop-v__VERSION__-linux-arm64.tar.gz"
      sha256 "__SHA256_LINUX_ARM64__"
    end
    on_intel do
      url "https://github.com/Anastylosis/MoanDrop/releases/download/v__VERSION__/moandrop-v__VERSION__-linux-amd64.tar.gz"
      sha256 "__SHA256_LINUX_AMD64__"
    end
  end

  def install
    bin.install "moandrop"
    generate_completions_from_executable(bin/"moandrop", "completion")
  end

  test do
    # --help is offline; `moandrop` bare would try to open a window, which
    # Homebrew's sandboxed CI has no display for.
    assert_match "fingerprint", shell_output("#{bin}/moandrop --help")
    assert_match version.to_s, shell_output("#{bin}/moandrop --version")
  end
end
