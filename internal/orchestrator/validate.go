package orchestrator

import (
	"fmt"

	"github.com/distributed_task_queue/distributed_task_queue/internal/db"
)

func validateJobSpecs(specs []db.TaskSpec) error {
	if len(specs) == 0 {
		return fmt.Errorf("at least one task is required")
	}
	seen := make(map[string]struct{}, len(specs))
	for _, s := range specs {
		if s.Name == "" {
			return fmt.Errorf("task name is required")
		}
		if _, dup := seen[s.Name]; dup {
			return fmt.Errorf("duplicate task name %q", s.Name)
		}
		seen[s.Name] = struct{}{}
		if s.Kind == "" {
			return fmt.Errorf("kind is required for task %q", s.Name)
		}
		for _, d := range s.DependsOn {
			if d == s.Name {
				return fmt.Errorf("task %q cannot depend on itself", s.Name)
			}
		}
	}
	for _, s := range specs {
		for _, d := range s.DependsOn {
			if _, ok := seen[d]; !ok {
				return fmt.Errorf("unknown dependency %q for task %q", d, s.Name)
			}
		}
	}
	if hasCycle(specs) {
		return fmt.Errorf("cyclic task dependencies")
	}
	return nil
}

// Kahn topological order: edges go from dependency → dependent task name.
func hasCycle(specs []db.TaskSpec) bool {
	indeg := make(map[string]int, len(specs))
	adj := make(map[string][]string)
	for _, s := range specs {
		indeg[s.Name] = len(s.DependsOn)
	}
	for _, s := range specs {
		for _, d := range s.DependsOn {
			adj[d] = append(adj[d], s.Name)
		}
	}
	var q []string
	for name, d := range indeg {
		if d == 0 {
			q = append(q, name)
		}
	}
	done := 0
	for len(q) > 0 {
		u := q[0]
		q = q[1:]
		done++
		for _, v := range adj[u] {
			indeg[v]--
			if indeg[v] == 0 {
				q = append(q, v)
			}
		}
	}
	return done != len(specs)
}
