package renderer

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Pantani/gorchestrator/internal/domain"
)

// WriteArtifacts persists rendered artifacts under baseDir with path safety checks.
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

func safeRelPath(rawPath string) (string, error) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", fmt.Errorf("empty path")
	}

	normalized := strings.ReplaceAll(rawPath, "\\", "/")
	clean := path.Clean(normalized)
	if clean == "." || clean == "" {
		return "", fmt.Errorf("empty path")
	}
	if strings.HasPrefix(clean, "/") || filepath.IsAbs(rawPath) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path escapes output dir")
	}
	return filepath.FromSlash(clean), nil
}
