package renderer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Pantani/gorchestrator/internal/domain"
)

func TestSafeRelPathRejectsEscapesAndAbsolute(t *testing.T) {
	t.Parallel()

	tests := []string{
		"",
		".",
		"..",
		"../escape.txt",
		`..\escape.txt`,
		"/tmp/escape.txt",
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc, func(t *testing.T) {
			t.Parallel()
			if _, err := safeRelPath(tc); err == nil {
				t.Fatalf("expected path %q to be rejected", tc)
			}
		})
	}
}

func TestWriteArtifactsWritesNormalizedRelativePaths(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	err := WriteArtifacts(baseDir, []domain.Artifact{
		{Path: "configs/node/../node/config.toml", Content: "ok"},
		{Path: `notes\info.txt`, Content: "details"},
	})
	if err != nil {
		t.Fatalf("write artifacts: %v", err)
	}

	paths := []string{
		filepath.Join(baseDir, "configs", "node", "config.toml"),
		filepath.Join(baseDir, "notes", "info.txt"),
	}
	for _, p := range paths {
		if _, statErr := os.Stat(p); statErr != nil {
			t.Fatalf("expected artifact %s to exist: %v", p, statErr)
		}
	}
}

func TestWriteArtifactsRejectsTraversal(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	err := WriteArtifacts(baseDir, []domain.Artifact{
		{Path: "../outside.txt", Content: "nope"},
	})
	if err == nil {
		t.Fatalf("expected traversal artifact path to fail")
	}
}
