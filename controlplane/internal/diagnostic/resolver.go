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
	Deployment          string
	Source              string
	Target              string
	Replica             *int
	ClientService       string
	AllowActiveCallback bool
	AllowActiveEvent    bool
	Event               string
	DeviceID            string
	Subscriber          string
	SubscriberReplica   *int
}

// Selection is the fully resolved, runtime-independent diagnostic context.
type Selection struct {
	Provider           Provider
	Deployment         plan.Deployment
	Source             plan.Service
	Instance           plan.Instance
	Target             plan.Service
	Endpoint           persist.HealthEndpoint
	TargetEndpoint     persist.HealthEndpoint
	Subscriber         plan.Service
	SubscriberInstance plan.Instance
	SubscriberEndpoint persist.HealthEndpoint
}

// Resolve loads the persisted desired deployment, rebuilds its deterministic
// plan, resolves one supported source replica and WebPA target, and finds the
// source's already-persisted loopback endpoint.
func Resolve(store *persist.Store, registry *Registry, request ResolveRequest) (Selection, error) {
	if request.Deployment == "" || request.Source == "" || request.Target == "" {
		return Selection{}, fmt.Errorf("diagnose requires deployment, source, and target")
	}
	journey := JourneyCPEWebPA
	switch request.Target {
	case "webpa":
	case "webhook":
		journey = JourneyWebhook
	case "callback":
		journey = JourneyCPEWebPACallback
	case "parodus":
		journey = JourneyParodusClients
	default:
		return Selection{}, fmt.Errorf("unsupported diagnostic target %q: expected webpa, webhook, callback, or parodus", request.Target)
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
	target := plan.Service{Name: "parodus", Type: "parodus"}
	if journey != JourneyParodusClients {
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
		target = targets[0]
		if (journey == JourneyWebhook || journey == JourneyCPEWebPACallback) && len(target.Instances) != 1 {
			return Selection{}, fmt.Errorf("webpa service %q has %d replicas: exactly one WebPA participant is required", target.Name, len(target.Instances))
		}
	}
	provider, ok := registry.Lookup(journey, source.Type, target.Type)
	if !ok {
		return Selection{}, fmt.Errorf("source service %q has unsupported type %q for %s", source.Name, source.Type, journey)
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
	var subscriber *plan.Service
	subscriberReplica := 0
	if journey == JourneyCPEWebPACallback {
		if request.Subscriber == "" {
			return Selection{}, fmt.Errorf("callback diagnosis requires a subscriber service")
		}
		for index := range resolved.Services {
			service := &resolved.Services[index]
			if service.Name == request.Subscriber {
				subscriber = service
				break
			}
		}
		if subscriber == nil {
			return Selection{}, fmt.Errorf("deployment %q has no subscriber service %q", request.Deployment, request.Subscriber)
		}
		if subscriber.Type != "event-sink" {
			return Selection{}, fmt.Errorf("subscriber service %q has unsupported type %q for %s", subscriber.Name, subscriber.Type, journey)
		}
		if len(subscriber.Instances) == 0 {
			return Selection{}, fmt.Errorf("subscriber service %q has no active replicas", subscriber.Name)
		}
		if request.SubscriberReplica != nil {
			subscriberReplica = *request.SubscriberReplica
		} else if len(subscriber.Instances) > 1 {
			return Selection{}, fmt.Errorf("subscriber service %q has replicas 0..%d: --subscriber-replica is required", subscriber.Name, len(subscriber.Instances)-1)
		}
		if subscriberReplica < 0 || subscriberReplica >= len(subscriber.Instances) {
			return Selection{}, fmt.Errorf("subscriber service %q replica %d is out of range 0..%d", subscriber.Name, subscriberReplica, len(subscriber.Instances)-1)
		}
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
	var targetEndpoint persist.HealthEndpoint
	if journey == JourneyWebhook || journey == JourneyCPEWebPACallback {
		var found *persist.HealthEndpoint
		for index := range endpoints {
			candidate := &endpoints[index]
			if candidate.Service == target.Name && candidate.Replica == 0 {
				found = candidate
				break
			}
		}
		if found == nil {
			return Selection{}, fmt.Errorf("webpa service %q replica 0 has no persisted loopback endpoint", target.Name)
		}
		targetEndpoint = *found
	}
	var subscriberEndpoint persist.HealthEndpoint
	if subscriber != nil {
		var found *persist.HealthEndpoint
		for index := range endpoints {
			candidate := &endpoints[index]
			if candidate.Service == subscriber.Name && candidate.Replica == subscriberReplica {
				found = candidate
				break
			}
		}
		if found == nil {
			return Selection{}, fmt.Errorf("subscriber service %q replica %d has no persisted loopback endpoint", subscriber.Name, subscriberReplica)
		}
		subscriberEndpoint = *found
	}
	selection := Selection{Provider: provider, Deployment: resolved, Source: *source, Instance: source.Instances[replica], Target: target, Endpoint: *endpoint, TargetEndpoint: targetEndpoint}
	if subscriber != nil {
		selection.Subscriber = *subscriber
		selection.SubscriberInstance = subscriber.Instances[subscriberReplica]
		selection.SubscriberEndpoint = subscriberEndpoint
	}
	return selection, nil
}
