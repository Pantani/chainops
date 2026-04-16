package renderer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Pantani/gorchestrator/internal/domain"
)

func WriteArtifacts(baseDir string, artifacts []domain.Artifact) error {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	for _, a := range artifacts {
		rel, err := safeRelPath(a.Path)
		if err != nil {
			return fmt.Errorf("invalid artifact path %q: %w", a.Path, err)
		}
		abs := filepath.Join(baseDir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return fmt.Errorf("create artifact dir: %w", err)
		}
		if err := os.WriteFile(abs, []byte(a.Content), 0o644); err != nil {
			return fmt.Errorf("write artifact %s: %w", abs, err)
		}
	}
	return nil
}

func safeRelPath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == "" {
		return "", fmt.Errorf("empty path")
	}
	if strings.HasPrefix(clean, "..") || strings.Contains(clean, "../") {
		return "", fmt.Errorf("path escapes output dir")
	}
	return clean, nil
}
