package diagnostic

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
)

const ReasonApplicationEvidenceUnavailable = "application-evidence-unavailable"

// ExpectedInput contains resolved, runtime-independent topology for one
// diagnostic journey.
type ExpectedInput struct {
	Deployment plan.Deployment
	Source     plan.Service
	Instance   plan.Instance
	Target     plan.Service
}

// ExpectedGraph is the provider-owned graph before endpoint observations are
// merged by the orchestrator.
type ExpectedGraph struct {
	Journey  string
	Source   EndpointIdentity
	Target   EndpointIdentity
	Metadata []Evidence
	Nodes    []Node
	Edges    []Edge
}

// Provider contributes expected-path metadata for one source/target type pair.
type Provider interface {
	Journey() string
	SourceType() string
	TargetType() string
	Expected(ExpectedInput) (ExpectedGraph, error)
}

// Registry stores optional journey providers independently of ServiceType.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry creates an empty diagnostic provider registry.
func NewRegistry() *Registry { return &Registry{providers: map[string]Provider{}} }

// Register adds a provider and panics on duplicate registration because that
// indicates invalid static application wiring.
func (r *Registry) Register(provider Provider) {
	if provider == nil {
		panic("diagnostic: nil provider")
	}
	key := providerKey(provider.Journey(), provider.SourceType(), provider.TargetType())
	if key == "//" {
		panic("diagnostic: provider has empty identity")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, duplicate := r.providers[key]; duplicate {
		panic("diagnostic: duplicate provider " + key)
	}
	r.providers[key] = provider
}

// Lookup returns the provider for a journey and source/target type pair.
func (r *Registry) Lookup(journey, sourceType, targetType string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.providers[providerKey(journey, sourceType, targetType)]
	return provider, ok
}

// Keys returns deterministic provider identities for introspection and tests.
func (r *Registry) Keys() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := make([]string, 0, len(r.providers))
	for key := range r.providers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

var defaultRegistry = NewRegistry()

// Register installs a provider in the process-wide registry.
func Register(provider Provider) { defaultRegistry.Register(provider) }

// Lookup finds a provider in the process-wide registry.
func Lookup(journey, sourceType, targetType string) (Provider, bool) {
	return defaultRegistry.Lookup(journey, sourceType, targetType)
}

// DefaultRegistry returns the process-wide built-in provider registry.
func DefaultRegistry() *Registry { return defaultRegistry }

func providerKey(journey, sourceType, targetType string) string {
	return journey + "/" + sourceType + "/" + targetType
}

type cpeWebPAProvider struct{ sourceType string }

// NewCPEWebPAProvider creates the shared provider used by Gateway and XB10.
func NewCPEWebPAProvider(sourceType string) Provider {
	return cpeWebPAProvider{sourceType: sourceType}
}

func (provider cpeWebPAProvider) Journey() string    { return JourneyCPEWebPA }
func (provider cpeWebPAProvider) SourceType() string { return provider.sourceType }
func (cpeWebPAProvider) TargetType() string          { return "webpa" }

func (provider cpeWebPAProvider) Expected(input ExpectedInput) (ExpectedGraph, error) {
	if input.Source.Type != provider.sourceType || input.Target.Type != provider.TargetType() {
		return ExpectedGraph{}, fmt.Errorf("provider %s does not support %s to %s", provider.sourceType, input.Source.Type, input.Target.Type)
	}
	metadata := []Evidence{
		{Key: "parodus-endpoint", Value: "tcp://127.0.0.1:6666"},
		{Key: "talaria-endpoint", Value: "http://talaria:6200"},
	}
	for _, iface := range input.Instance.Interfaces {
		if iface.Device == "erouter0" || iface.Role == "wan" {
			if mac := strings.ReplaceAll(iface.MAC, ":", ""); mac != "" {
				metadata = append(metadata, Evidence{Key: "device-id", Value: "mac:" + mac})
			}
			break
		}
	}
	return ExpectedGraph{
		Journey:  JourneyCPEWebPA,
		Source:   EndpointIdentity{Deployment: input.Deployment.Name, Service: input.Source.Name, Type: input.Source.Type, Replica: input.Instance.Index},
		Target:   EndpointIdentity{Deployment: input.Deployment.Name, Service: input.Target.Name, Type: input.Target.Type, Replica: 0},
		Metadata: metadata,
		Nodes: []Node{
			{ID: "application", Label: input.Source.Name + " application", Kind: "application"},
			{ID: "parodus", Label: "Parodus", Kind: "service"},
			{ID: "dns", Label: "Talaria DNS", Kind: "boundary"},
			{ID: "transport", Label: "Talaria transport", Kind: "boundary"},
			{ID: "authentication", Label: "Talaria authentication", Kind: "boundary"},
			{ID: "registration", Label: "Device registration", Kind: "registry"},
		},
		Edges: []Edge{
			{ID: "application-parodus", From: "application", To: "parodus", Label: "local Parodus connection", BlocksFollowing: false},
			{ID: "talaria-dns", From: "parodus", To: "dns", Label: "resolve talaria", BlocksFollowing: true},
			{ID: "talaria-transport", From: "dns", To: "transport", Label: "connect to Talaria", BlocksFollowing: true},
			{ID: "talaria-authentication", From: "transport", To: "authentication", Label: "authenticate to Talaria", BlocksFollowing: true},
			{ID: "device-registration", From: "authentication", To: "registration", Label: "find device registration", BlocksFollowing: true},
		},
	}, nil
}

// ApplicationEvidenceUnavailable returns the conservative v1 observation used
// when only Parodus process/listener health is available.
func ApplicationEvidenceUnavailable(observedAt time.Time, serviceState string) Observation {
	return Observation{
		EdgeID:        "application-parodus",
		State:         StateUnknown,
		ReasonID:      ReasonApplicationEvidenceUnavailable,
		RemediationID: "expose-parodus-client-evidence",
		Message:       "Parodus is available, but no authoritative application client registry is exposed",
		Evidence:      []Evidence{{Key: "service-state", Value: serviceState}},
		ObservedAt:    observedAt,
	}
}
