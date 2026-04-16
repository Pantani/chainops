package planner

import (
	"sort"
	"time"

	"github.com/Pantani/gorchestrator/internal/domain"
	"github.com/Pantani/gorchestrator/internal/state"
)

func Build(desired domain.DesiredState, current *state.Snapshot) domain.Plan {
	plan := domain.Plan{GeneratedAt: time.Now().UTC(), Changes: make([]domain.PlanChange, 0)}
	next := state.FromDesired(desired)

	if current == nil {
		for _, svc := range desired.Services {
			plan.Changes = append(plan.Changes, domain.PlanChange{Type: domain.ChangeCreate, ResourceType: "service", Name: svc.Name, Reason: "service does not exist in state"})
		}
		for _, a := range desired.Artifacts {
			plan.Changes = append(plan.Changes, domain.PlanChange{Type: domain.ChangeCreate, ResourceType: "artifact", Name: a.Path, Reason: "artifact does not exist in state"})
		}
		sortChanges(plan.Changes)
		return plan
	}

	for name, nextHash := range next.Services {
		currentHash, ok := current.Services[name]
		switch {
		case !ok:
			plan.Changes = append(plan.Changes, domain.PlanChange{Type: domain.ChangeCreate, ResourceType: "service", Name: name, Reason: "new service"})
		case currentHash != nextHash:
			plan.Changes = append(plan.Changes, domain.PlanChange{Type: domain.ChangeUpdate, ResourceType: "service", Name: name, Reason: "service spec changed"})
		default:
			plan.Changes = append(plan.Changes, domain.PlanChange{Type: domain.ChangeNoop, ResourceType: "service", Name: name, Reason: "no changes"})
		}
	}
	for name := range current.Services {
		if _, ok := next.Services[name]; !ok {
			plan.Changes = append(plan.Changes, domain.PlanChange{Type: domain.ChangeDelete, ResourceType: "service", Name: name, Reason: "service removed from desired state"})
		}
	}

	for path, nextHash := range next.Artifacts {
		currentHash, ok := current.Artifacts[path]
		switch {
		case !ok:
			plan.Changes = append(plan.Changes, domain.PlanChange{Type: domain.ChangeCreate, ResourceType: "artifact", Name: path, Reason: "new artifact"})
		case currentHash != nextHash:
			plan.Changes = append(plan.Changes, domain.PlanChange{Type: domain.ChangeUpdate, ResourceType: "artifact", Name: path, Reason: "artifact content changed"})
		default:
			plan.Changes = append(plan.Changes, domain.PlanChange{Type: domain.ChangeNoop, ResourceType: "artifact", Name: path, Reason: "no changes"})
		}
	}
	for path := range current.Artifacts {
		if _, ok := next.Artifacts[path]; !ok {
			plan.Changes = append(plan.Changes, domain.PlanChange{Type: domain.ChangeDelete, ResourceType: "artifact", Name: path, Reason: "artifact removed from desired state"})
		}
	}

	sortChanges(plan.Changes)
	return plan
}

func sortChanges(changes []domain.PlanChange) {
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].ResourceType == changes[j].ResourceType {
			if changes[i].Name == changes[j].Name {
				return changes[i].Type < changes[j].Type
			}
			return changes[i].Name < changes[j].Name
		}
		return changes[i].ResourceType < changes[j].ResourceType
	})
}
