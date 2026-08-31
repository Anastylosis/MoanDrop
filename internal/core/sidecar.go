package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MaxTrackBytes mirrors the moansubs server's own upload cap
// (internal/subtitle.MaxBytes, 2 MiB). A track body over it here is lying
// about its size, not a legitimate subtitle to write next to a video.
const MaxTrackBytes = 2 * 1024 * 1024

// SidecarPath computes the caption path for a video file: same directory,
// same stem, `.<base>.srt` suffix — the convention Plex, Jellyfin, Kodi and
// VLC all pick up without a scan step.
func SidecarPath(videoPath string, lang CaptionLang) string {
	ext := filepath.Ext(videoPath)
	stem := strings.TrimSuffix(videoPath, ext)
	return fmt.Sprintf("%s.%s.srt", stem, lang.Base)
}

// WriteSidecar writes body next to the video file. It refuses to overwrite
// an existing caption unless overwrite is set — the existing file may be a
// hand-made subtitle, which must never be destroyed by a tool.
//
// Returns the written path and whether the file is genuinely new (false
// when an existing caption was overwritten in place).
func WriteSidecar(videoPath string, lang CaptionLang, body string, overwrite bool) (path string, created bool, err error) {
	path = SidecarPath(videoPath, lang)
	_, statErr := os.Stat(path)
	exists := statErr == nil

	if exists && !overwrite {
		return "", false, fmt.Errorf("caption %s already exists; pass --overwrite to replace it", path)
	}

	if len(body) > MaxTrackBytes {
		return "", false, fmt.Errorf("track body is %d bytes, over the %d byte cap; refusing to write it", len(body), MaxTrackBytes)
	}

	if err := writeFileAtomic(path, []byte(body), 0o644); err != nil {
		return "", false, fmt.Errorf("writing sidecar: %w", err)
	}
	return path, !exists, nil
}

// writeFileAtomic writes data to a randomly-named temp file in target's
// directory, fsyncs and closes it, then renames it into place. A write that
// fails partway — disk full, permission denied — must never leave a
// truncated file at target: WriteSidecar's never-overwrite guard only ever
// checks the final name, so a half-written file there would be "protected"
// as if it were a real caption forever after. Any failure removes the temp
// file rather than leaving it behind.
func writeFileAtomic(target string, data []byte, perm os.FileMode) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(target), filepath.Base(target)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, werr := tmp.Write(data); werr != nil {
		_ = tmp.Close()
		err = werr
		return err
	}
	if serr := tmp.Sync(); serr != nil {
		_ = tmp.Close()
		err = serr
		return err
	}
	if cerr := tmp.Close(); cerr != nil {
		err = cerr
		return err
	}
	if cherr := os.Chmod(tmpPath, perm); cherr != nil {
		err = cherr
		return err
	}
	if rerr := os.Rename(tmpPath, target); rerr != nil {
		err = rerr
		return err
	}
	return nil
}
