package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Pantani/gorchestrator/internal/domain"
)

const SnapshotVersion = "v1alpha1"

type Snapshot struct {
	Version     string            `json:"version"`
	ClusterName string            `json:"clusterName"`
	Backend     string            `json:"backend"`
	Services    map[string]string `json:"services"`
	Artifacts   map[string]string `json:"artifacts"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

type Store struct {
	Dir string
}

func NewStore(dir string) *Store {
	return &Store{Dir: dir}
}

func (s *Store) Load(clusterName, backend string) (*Snapshot, error) {
	path := s.path(clusterName, backend)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read snapshot: %w", err)
	}
	var snap Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, fmt.Errorf("decode snapshot: %w", err)
	}
	return &snap, nil
}

func (s *Store) Save(snap *Snapshot) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	raw, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("encode snapshot: %w", err)
	}
	if err := os.WriteFile(s.path(snap.ClusterName, snap.Backend), raw, 0o644); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}
	return nil
}

func (s *Store) path(clusterName, backend string) string {
	file := fmt.Sprintf("%s--%s.json", normalizePart(clusterName), normalizePart(backend))
	return filepath.Join(s.Dir, file)
}

func normalizePart(in string) string {
	in = strings.ToLower(strings.TrimSpace(in))
	if in == "" {
		return "unknown"
	}
	repl := strings.NewReplacer("/", "-", " ", "-", "_", "-", ":", "-")
	in = repl.Replace(in)
	return in
}

func FromDesired(desired domain.DesiredState) *Snapshot {
	services := make(map[string]string, len(desired.Services))
	for _, s := range desired.Services {
		services[s.Name] = hashJSON(s)
	}
	artifacts := make(map[string]string, len(desired.Artifacts))
	for _, a := range desired.Artifacts {
		artifacts[a.Path] = hashString(a.Content)
	}
	return &Snapshot{
		Version:     SnapshotVersion,
		ClusterName: desired.ClusterName,
		Backend:     desired.Backend,
		Services:    services,
		Artifacts:   artifacts,
		UpdatedAt:   time.Now().UTC(),
	}
}

func hashJSON(v any) string {
	raw, _ := json.Marshal(v)
	return hashString(string(raw))
}

func hashString(v string) string {
	h := sha256.Sum256([]byte(v))
	return hex.EncodeToString(h[:])
}

func SortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
