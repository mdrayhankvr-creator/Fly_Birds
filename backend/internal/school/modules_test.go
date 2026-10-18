package school

import (
	"errors"
	"slices"
	"testing"
)

func graph() map[string]moduleState {
	return map[string]moduleState{
		"authentication": {IsCore: true, Enabled: true},
		"academics":      {Enabled: false},
		"students":       {Enabled: false, DependsOn: []string{"academics"}},
		"staff":          {Enabled: false, DependsOn: []string{"academics"}},
		"attendance":     {Enabled: false, DependsOn: []string{"students", "staff"}},
		"fees":           {Enabled: false, DependsOn: []string{"students"}},
	}
}

func TestPlanModuleChangeEnables(t *testing.T) {
	var dep *DependencyError
	if _, err := PlanModuleChange(graph(), "attendance", true, false); !errors.As(err, &dep) || dep.Kind != "requires" || !slices.Equal(dep.Modules, []string{"academics", "students", "staff"}) {
		t.Fatalf("missing prerequisites: %v %+v", err, dep)
	}
	plan, err := PlanModuleChange(graph(), "attendance", true, true)
	if err != nil || !slices.Equal(plan, []string{"academics", "students", "staff", "attendance"}) {
		t.Fatalf("cascade enable %v %v", plan, err)
	}
	g := graph()
	g["academics"] = moduleState{Enabled: true}
	if plan, err := PlanModuleChange(g, "students", true, false); err != nil || !slices.Equal(plan, []string{"students"}) {
		t.Fatalf("satisfied enable %v %v", plan, err)
	}
	if plan, err := PlanModuleChange(g, "academics", true, false); err != nil || len(plan) != 0 {
		t.Fatalf("no-op enable %v %v", plan, err)
	}
	if _, err := PlanModuleChange(g, "nope", true, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown module %v", err)
	}
}

func TestPlanModuleChangeDisables(t *testing.T) {
	g := graph()
	for _, k := range []string{"academics", "students", "staff", "attendance", "fees"} {
		s := g[k]
		s.Enabled = true
		g[k] = s
	}
	if _, err := PlanModuleChange(g, "authentication", false, true); !errors.Is(err, ErrCoreModule) {
		t.Fatalf("core disable %v", err)
	}
	var dep *DependencyError
	if _, err := PlanModuleChange(g, "students", false, false); !errors.As(err, &dep) || dep.Kind != "required_by" || !slices.Equal(dep.Modules, []string{"attendance", "fees"}) {
		t.Fatalf("dependents %v %+v", err, dep)
	}
	plan, err := PlanModuleChange(g, "academics", false, true)
	if err != nil || plan[len(plan)-1] != "academics" || len(plan) != 5 {
		t.Fatalf("cascade disable %v %v", plan, err)
	}
	// Every module is switched off only after the modules that depend on it.
	position := map[string]int{}
	for i, k := range plan {
		position[k] = i
	}
	for k, s := range g {
		for _, d := range s.DependsOn {
			if _, both := position[k]; both && position[k] > position[d] {
				t.Fatalf("%s disabled after its prerequisite %s: %v", k, d, plan)
			}
		}
	}
	if plan, err := PlanModuleChange(g, "fees", false, false); err != nil || !slices.Equal(plan, []string{"fees"}) {
		t.Fatalf("leaf disable %v %v", plan, err)
	}
}
