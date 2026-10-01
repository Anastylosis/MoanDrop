package core

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// videoExts matches the extensions the Windows context-menu entry covers,
// plus the two other containers common in libraries.
var videoExts = []string{".mp4", ".m4v", ".mkv", ".avi", ".wmv", ".flv", ".mov", ".mpg", ".mpeg", ".webm", ".m2ts"}

// IsVideoFile reports whether path has a video extension.
func IsVideoFile(path string) bool {
	ext := filepath.Ext(path)
	for _, e := range videoExts {
		if strings.EqualFold(ext, e) {
			return true
		}
	}
	return false
}

// CollectVideos expands paths into a sorted, de-duplicated list of video
// files: a file is taken as named (whatever its extension), a directory is
// walked recursively for video extensions. Symlinked directories inside a
// walk are not followed; a symlink to a regular file counts as a file.
func CollectVideos(paths []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = filepath.Clean(p)
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, root := range paths {
		info, err := os.Stat(root)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			add(root)
			continue
		}
		err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !IsVideoFile(p) {
				return nil
			}
			if fi, serr := os.Stat(p); serr != nil || !fi.Mode().IsRegular() {
				return nil
			}
			add(p)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scanning %s: %w", root, err)
		}
	}
	sort.Strings(out)
	return out, nil
}

// HasSidecarFor reports whether videoPath already has a subtitle sidecar
// for every language in langs (matched by base subtag). False when langs
// is empty: without a requested language there is nothing to be "had".
func HasSidecarFor(videoPath string, langs []string) bool {
	if len(langs) == 0 {
		return false
	}
	found, err := FindSidecars(videoPath)
	if err != nil {
		return false
	}
	have := map[string]bool{}
	for _, s := range found {
		if s.Lang == "" {
			continue
		}
		if b, err := baseSubtag(s.Lang); err == nil {
			have[b] = true
		}
	}
	for _, l := range langs {
		b, err := baseSubtag(l)
		if err != nil || !have[b] {
			return false
		}
	}
	return true
}
