package diagnostic

import (
	"fmt"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/persist"
	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
	"github.com/gdcs-dev/vcpe/controlplane/internal/planner"
	"gopkg.in/yaml.v3"
)

// ResolveRequest selects one diagnostic source and target.
type ResolveRequest struct {
	Deployment    string
	Source        string
	Target        string
	Replica       *int
	ClientService string
}

// Selection is the fully resolved, runtime-independent diagnostic context.
type Selection struct {
	Provider   Provider
	Deployment plan.Deployment
	Source     plan.Service
	Instance   plan.Instance
	Target     plan.Service
	Endpoint   persist.HealthEndpoint
}

// Resolve loads the persisted desired deployment, rebuilds its deterministic
// plan, resolves one supported source replica and WebPA target, and finds the
// source's already-persisted loopback endpoint.
func Resolve(store *persist.Store, registry *Registry, request ResolveRequest) (Selection, error) {
	if request.Deployment == "" || request.Source == "" || request.Target == "" {
		return Selection{}, fmt.Errorf("diagnose requires deployment, source, and target")
	}
	if request.Target != "webpa" {
		return Selection{}, fmt.Errorf("unsupported diagnostic target %q: expected webpa", request.Target)
	}
	raw, ok, err := store.LatestDesiredSnapshot(request.Deployment)
	if err != nil {
		return Selection{}, err
	}
	if !ok {
		return Selection{}, fmt.Errorf("unknown deployment %q", request.Deployment)
	}
	var document manifest.Document
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return Selection{}, fmt.Errorf("decode persisted deployment %q: %w", request.Deployment, err)
	}
	resolved, err := planner.Build(document, nil)
	if err != nil {
		return Selection{}, fmt.Errorf("resolve persisted deployment %q: %w", request.Deployment, err)
	}
	var source *plan.Service
	var targets []plan.Service
	for index := range resolved.Services {
		service := &resolved.Services[index]
		if service.Name == request.Source {
			source = service
		}
		if service.Type == "webpa" {
			targets = append(targets, *service)
		}
	}
	if source == nil {
		return Selection{}, fmt.Errorf("deployment %q has no source service %q", request.Deployment, request.Source)
	}
	if len(targets) == 0 {
		return Selection{}, fmt.Errorf("deployment %q has no service of type webpa", request.Deployment)
	}
	if len(targets) > 1 {
		names := make([]string, len(targets))
		for index := range targets {
			names[index] = targets[index].Name
		}
		return Selection{}, fmt.Errorf("deployment %q has ambiguous webpa targets %v", request.Deployment, names)
	}
	provider, ok := registry.Lookup(JourneyCPEWebPA, source.Type, targets[0].Type)
	if !ok {
		return Selection{}, fmt.Errorf("source service %q has unsupported type %q for %s", source.Name, source.Type, JourneyCPEWebPA)
	}
	if len(source.Instances) == 0 {
		return Selection{}, fmt.Errorf("source service %q has no active replicas", source.Name)
	}
	replica := 0
	if request.Replica != nil {
		replica = *request.Replica
	} else if len(source.Instances) > 1 {
		return Selection{}, fmt.Errorf("source service %q has replicas 0..%d: --replica is required", source.Name, len(source.Instances)-1)
	}
	if replica < 0 || replica >= len(source.Instances) {
		return Selection{}, fmt.Errorf("source service %q replica %d is out of range 0..%d", source.Name, replica, len(source.Instances)-1)
	}
	endpoints, err := store.ListHealthEndpoints(request.Deployment)
	if err != nil {
		return Selection{}, err
	}
	var endpoint *persist.HealthEndpoint
	for index := range endpoints {
		candidate := &endpoints[index]
		if candidate.Service == source.Name && candidate.Replica == replica {
			endpoint = candidate
			break
		}
	}
	if endpoint == nil {
		return Selection{}, fmt.Errorf("source service %q replica %d has no persisted loopback endpoint", source.Name, replica)
	}
	return Selection{Provider: provider, Deployment: resolved, Source: *source, Instance: source.Instances[replica], Target: targets[0], Endpoint: *endpoint}, nil
}
