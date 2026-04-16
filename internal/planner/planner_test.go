package planner

import (
	"testing"

	"github.com/Pantani/gorchestrator/internal/domain"
	"github.com/Pantani/gorchestrator/internal/state"
)

func TestBuildPlanInitialCreate(t *testing.T) {
	desired := domain.DesiredState{
		ClusterName: "test",
		Backend:     "docker-compose",
		Services: []domain.Service{
			{Name: "svc-a", Image: "nginx"},
		},
		Artifacts: []domain.Artifact{{Path: "compose.yaml", Content: "services: {}\n"}},
	}

	plan := Build(desired, nil)
	if len(plan.Changes) != 2 {
		t.Fatalf("expected 2 create changes, got %d", len(plan.Changes))
	}
	for _, c := range plan.Changes {
		if c.Type != domain.ChangeCreate {
			t.Fatalf("expected create change, got %s", c.Type)
		}
	}
}

func TestBuildPlanUpdateAndDelete(t *testing.T) {
	desired := domain.DesiredState{
		ClusterName: "test",
		Backend:     "docker-compose",
		Services: []domain.Service{
			{Name: "svc-a", Image: "nginx:latest"},
		},
		Artifacts: []domain.Artifact{{Path: "compose.yaml", Content: "services:\n  svc-a: {}\n"}},
	}
	current := &state.Snapshot{
		Version:     state.SnapshotVersion,
		ClusterName: "test",
		Backend:     "docker-compose",
		Services: map[string]string{
			"svc-a": "outdated",
			"svc-b": "old",
		},
		Artifacts: map[string]string{
			"compose.yaml": "old",
			"extra.txt":    "old",
		},
	}

	plan := Build(desired, current)
	var hasSvcUpdate, hasSvcDelete, hasArtifactUpdate, hasArtifactDelete bool
	for _, c := range plan.Changes {
		switch {
		case c.ResourceType == "service" && c.Name == "svc-a" && c.Type == domain.ChangeUpdate:
			hasSvcUpdate = true
		case c.ResourceType == "service" && c.Name == "svc-b" && c.Type == domain.ChangeDelete:
			hasSvcDelete = true
		case c.ResourceType == "artifact" && c.Name == "compose.yaml" && c.Type == domain.ChangeUpdate:
			hasArtifactUpdate = true
		case c.ResourceType == "artifact" && c.Name == "extra.txt" && c.Type == domain.ChangeDelete:
			hasArtifactDelete = true
		}
	}
	if !(hasSvcUpdate && hasSvcDelete && hasArtifactUpdate && hasArtifactDelete) {
		t.Fatalf("unexpected plan changes: %+v", plan.Changes)
	}
}
