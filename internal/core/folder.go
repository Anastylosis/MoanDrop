package core

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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

// CollectVideos lists videos under paths; symlinked directories are not followed.
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

// HasSidecarFor is false on empty langs: nothing requested, nothing to have.
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
