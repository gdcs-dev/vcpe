package rfmedium

import (
	"strings"
	"testing"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
)

func TestBuildBaselineUsesPlannedVAPsAndIsolatesDeployments(t *testing.T) {
	first := plan.Deployment{Name: "edge-a", Services: []plan.Service{{Instances: []plan.Instance{{Radios: []plan.Radio{
		{MAC: "02:00:00:00:00:01", VAPs: []plan.VAP{{Slot: 0, MAC: "02:00:00:00:00:01"}, {Slot: 1, MAC: "02:00:00:00:00:02"}}},
		{MAC: "02:00:00:00:00:03"},
	}}}}}}
	second := plan.Deployment{Name: "edge-b", Services: []plan.Service{{Instances: []plan.Instance{{Radios: []plan.Radio{
		{MAC: "02:00:00:00:00:04"}, {MAC: "02:00:00:00:00:05"},
	}}}}}}
	config, err := BuildBaseline([]plan.Deployment{second, first})
	if err != nil {
		t.Fatal(err)
	}
	reordered, err := BuildBaseline([]plan.Deployment{first, second})
	if err != nil || config != reordered {
		t.Fatalf("baseline depends on deployment order: %v", err)
	}
	for _, expected := range []string{`"02:00:00:00:00:02"`, "(0,1,35)", "(3,4,35)", "(0,3,-100)", "(2,4,-100)"} {
		if !strings.Contains(config, expected) {
			t.Errorf("baseline missing %q: %s", expected, config)
		}
	}
	if strings.Count(config, `"02:00:00:00:00:01"`) != 1 {
		t.Fatalf("primary VAP MAC should appear once: %s", config)
	}
}

func TestBuildBaselineRejectsDuplicateOrInvalidOwnership(t *testing.T) {
	for _, test := range []struct {
		name  string
		plans []plan.Deployment
	}{
		{"duplicate deployment", []plan.Deployment{{Name: "edge"}, {Name: "edge"}}},
		{"duplicate MAC", []plan.Deployment{{Name: "edge", Services: []plan.Service{{Instances: []plan.Instance{{Radios: []plan.Radio{{MAC: "02:00:00:00:00:01"}, {MAC: "02:00:00:00:00:01"}}}}}}}}},
		{"invalid MAC", []plan.Deployment{{Name: "edge", Services: []plan.Service{{Instances: []plan.Instance{{Radios: []plan.Radio{{MAC: `02:00:00:00:00:01"; links = ()`}}}}}}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := BuildBaseline(test.plans); err == nil {
				t.Fatal("expected invalid RF baseline to fail")
			}
		})
	}
}
