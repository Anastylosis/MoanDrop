package main

import "testing"

func TestIsBatchArgs(t *testing.T) {
	dir := t.TempDir()
	if !isBatchArgs([]string{dir}) || !isBatchArgs([]string{"a.mp4", "b.mp4"}) {
		t.Error("a folder or several paths must be a batch")
	}
	if isBatchArgs([]string{"missing.mp4"}) {
		t.Error("a single non-directory must stay on the single-file path")
	}
}
