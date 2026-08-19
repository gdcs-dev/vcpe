package diagnostic

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gdcs-dev/vcpe/controlplane/internal/plan"
)

const (
	ReasonApplicationEvidenceUnavailable = "application-evidence-unavailable"
	ReasonSubscriberIntentUnavailable    = "subscriber-intent-unavailable"
	ReasonParticipantResultIncomplete    = "participant-result-incomplete"
	ReasonArgusUnreachable               = "argus-unreachable"
	ReasonArgusAuthenticationFailed      = "argus-authentication-failed"
	ReasonArgusInventoryUnavailable      = "argus-inventory-unavailable"
	ReasonRegistrationMissing            = "registration-missing"
	ReasonRegistrationAmbiguous          = "registration-ambiguous"
	ReasonRegistrationExpired            = "registration-expired"
	ReasonRegistrationStale              = "registration-stale"
	ReasonRegistrationMismatch           = "registration-mismatch"
	ReasonActiveCallbackNotRequested     = "active-callback-not-requested"
	ReasonCallbackDNSFailed              = "callback-dns-failed"
	ReasonCallbackTransportFailed        = "callback-transport-failed"
	ReasonCallbackRejected               = "callback-rejected"
	ReasonCaduceusIngestionRejected      = "caduceus-ingestion-rejected"
	ReasonCaduceusReceiptMissing         = "caduceus-receipt-missing"
	ReasonActiveEventRejected            = "active-event-rejected"
	ReasonActiveEventUnsupported         = "active-event-unsupported"
	ReasonActiveEventNotExecuted         = "active-event-not-executed"
	ReasonRoutingObservationUnavailable  = "routing-observation-unavailable"
	ReasonReceiptRestarted               = "receipt-restarted"

	RemediationExposeSubscriberIntent  = "expose-subscriber-intent"
	RemediationCheckParticipant        = "check-participant-diagnostic"
	RemediationCheckArgusReachability  = "check-argus-reachability"
	RemediationCheckArgusCredentials   = "check-argus-credentials"
	RemediationCheckArgusInventory     = "check-argus-inventory"
	RemediationRegisterWebhook         = "register-webhook"
	RemediationRemoveDuplicateHooks    = "remove-duplicate-webhooks"
	RemediationRefreshWebhook          = "refresh-webhook-registration"
	RemediationAlignWebhookConfig      = "align-webhook-configuration"
	RemediationAllowActiveCallback     = "allow-active-callback"
	RemediationCheckCallbackDNS        = "check-callback-dns"
	RemediationCheckCallbackTransport  = "check-callback-transport"
	RemediationCheckCallbackSignature  = "check-callback-signature"
	RemediationCheckCaduceusIngestion  = "check-caduceus-ingestion"
	RemediationCheckCaduceusDelivery   = "check-caduceus-delivery"
	RemediationCheckActiveEvent        = "check-active-event"
	RemediationUseSupportedCPESource   = "use-supported-cpe-source"
	RemediationCheckRoutingObservation = "check-routing-observation"
	RemediationRestartSubscriber       = "restart-subscriber"
)

// ExpectedInput contains resolved, runtime-independent topology for one
// diagnostic journey.
type ExpectedInput struct {
	Deployment plan.Deployment
	Source     plan.Service
	Instance   plan.Instance
	Target     plan.Service
	Subscriber plan.Service
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

type parodusClientsProvider struct{}

// NewParodusClientsProvider creates the Gateway-only source-local client-list journey.
func NewParodusClientsProvider() Provider { return parodusClientsProvider{} }

func (parodusClientsProvider) Journey() string    { return JourneyParodusClients }
func (parodusClientsProvider) SourceType() string { return "gateway" }
func (parodusClientsProvider) TargetType() string { return "parodus" }

func (provider parodusClientsProvider) Expected(input ExpectedInput) (ExpectedGraph, error) {
	if input.Source.Type != provider.SourceType() || input.Target.Type != provider.TargetType() {
		return ExpectedGraph{}, fmt.Errorf("provider %s does not support %s to %s", provider.SourceType(), input.Source.Type, input.Target.Type)
	}
	return ExpectedGraph{
		Journey: JourneyParodusClients,
		Source:  EndpointIdentity{Deployment: input.Deployment.Name, Service: input.Source.Name, Type: input.Source.Type, Replica: input.Instance.Index},
		Target:  EndpointIdentity{Deployment: input.Deployment.Name, Service: input.Target.Name, Type: input.Target.Type, Replica: 0},
		Metadata: []Evidence{
			{Key: "parodus-endpoint", Value: "tcp://127.0.0.1:6666"},
		},
		Nodes: []Node{
			{ID: "gateway", Label: input.Source.Name, Kind: "service"},
			{ID: "parodus", Label: "Parodus", Kind: "service"},
		},
		Edges: []Edge{
			{ID: "parodus-client-list", From: "gateway", To: "parodus", Label: "list registered clients", BlocksFollowing: true},
		},
	}, nil
}

type argusWebhooksProvider struct{}

// NewArgusWebhooksProvider creates the WebPA-local authoritative inventory journey.
func NewArgusWebhooksProvider() Provider { return argusWebhooksProvider{} }

func (argusWebhooksProvider) Journey() string    { return JourneyArgusWebhooks }
func (argusWebhooksProvider) SourceType() string { return "webpa" }
func (argusWebhooksProvider) TargetType() string { return "argus" }

func (provider argusWebhooksProvider) Expected(input ExpectedInput) (ExpectedGraph, error) {
	if input.Source.Type != provider.SourceType() || input.Target.Type != provider.TargetType() {
		return ExpectedGraph{}, fmt.Errorf("provider %s does not support %s to %s", provider.SourceType(), input.Source.Type, input.Target.Type)
	}
	return ExpectedGraph{
		Journey: JourneyArgusWebhooks,
		Source:  EndpointIdentity{Deployment: input.Deployment.Name, Service: input.Source.Name, Type: input.Source.Type, Replica: input.Instance.Index},
		Target:  EndpointIdentity{Deployment: input.Deployment.Name, Service: input.Target.Name, Type: input.Target.Type, Replica: 0},
		Nodes: []Node{
			{ID: "webpa", Label: input.Source.Name, Kind: "service"},
			{ID: "argus", Label: "Argus", Kind: "registry"},
		},
		Edges: []Edge{
			{ID: "argus-reachability", From: "webpa", To: "argus", Label: "reach Argus", BlocksFollowing: true},
			{ID: "argus-inventory", From: "argus", To: "argus", Label: "list registered webhooks", BlocksFollowing: true},
		},
	}, nil
}

type cpeWebPACallbackProvider struct{}

// NewCPEWebPACallbackProvider creates the bounded Gateway-to-event-sink
// callback journey. XB10 is intentionally excluded because it has no
// repository-owned Parodus event source.
func NewCPEWebPACallbackProvider() Provider { return cpeWebPACallbackProvider{} }

func (cpeWebPACallbackProvider) Journey() string    { return JourneyCPEWebPACallback }
func (cpeWebPACallbackProvider) SourceType() string { return "gateway" }
func (cpeWebPACallbackProvider) TargetType() string { return "webpa" }

func (provider cpeWebPACallbackProvider) Expected(input ExpectedInput) (ExpectedGraph, error) {
	if input.Source.Type != provider.SourceType() || input.Target.Type != provider.TargetType() {
		return ExpectedGraph{}, fmt.Errorf("provider %s does not support %s to %s", provider.SourceType(), input.Source.Type, input.Target.Type)
	}
	if input.Subscriber.Type != "event-sink" {
		return ExpectedGraph{}, fmt.Errorf("callback journey requires an event-sink subscriber")
	}
	return ExpectedGraph{
		Journey: JourneyCPEWebPACallback,
		Source:  EndpointIdentity{Deployment: input.Deployment.Name, Service: input.Source.Name, Type: input.Source.Type, Replica: input.Instance.Index},
		Target:  EndpointIdentity{Deployment: input.Deployment.Name, Service: input.Target.Name, Type: input.Target.Type, Replica: 0},
		Nodes: []Node{
			{ID: "application", Label: input.Source.Name + " application", Kind: "application"},
			{ID: "parodus", Label: "Parodus", Kind: "service"},
			{ID: "talaria", Label: "Talaria", Kind: "service"},
			{ID: "device", Label: "Device registration", Kind: "registry"},
			{ID: "subscriber", Label: input.Subscriber.Name + " subscriber", Kind: "subscriber"},
			{ID: "intent", Label: "Registration intent", Kind: "configuration"},
			{ID: "argus", Label: "Argus", Kind: "registry"},
			{ID: "registration", Label: "Webhook registration", Kind: "registration"},
			{ID: "caduceus", Label: "Caduceus", Kind: "delivery"},
			{ID: "callback", Label: "Callback receipt", Kind: "endpoint"},
		},
		Edges: []Edge{
			{ID: "application-parodus", From: "application", To: "parodus", Label: "verify selected Parodus client", BlocksFollowing: true},
			{ID: "talaria-dns", From: "parodus", To: "talaria", Label: "resolve Talaria", BlocksFollowing: true},
			{ID: "talaria-transport", From: "talaria", To: "talaria", Label: "connect to Talaria", BlocksFollowing: true},
			{ID: "talaria-authentication", From: "talaria", To: "talaria", Label: "authenticate to Talaria", BlocksFollowing: true},
			{ID: "device-registration", From: "talaria", To: "device", Label: "find device registration", BlocksFollowing: true},
			{ID: "subscriber-intent", From: "subscriber", To: "intent", Label: "read subscriber intent", BlocksFollowing: true},
			{ID: "argus-reachability", From: "intent", To: "argus", Label: "reach Argus", BlocksFollowing: true},
			{ID: "argus-authentication", From: "argus", To: "argus", Label: "authenticate to Argus", BlocksFollowing: true},
			{ID: "registration-present", From: "argus", To: "registration", Label: "find one registration", BlocksFollowing: true},
			{ID: "registration-fresh", From: "registration", To: "registration", Label: "verify registration freshness", BlocksFollowing: true},
			{ID: "registration-conformant", From: "registration", To: "callback", Label: "validate event and device matcher", BlocksFollowing: true},
			{ID: "active-event-acceptance", From: "parodus", To: "caduceus", Label: "accept one marked event", BlocksFollowing: true},
			{ID: "routing-observation", From: "caduceus", To: "caduceus", Label: "observe routing selection", BlocksFollowing: true},
			{ID: "callback-receipt", From: "caduceus", To: "callback", Label: "record matching callback receipt", BlocksFollowing: true},
		},
	}, nil
}

type webhookProvider struct{}

// NewWebhookProvider creates the event-sink to WebPA webhook diagnostic provider.
func NewWebhookProvider() Provider { return webhookProvider{} }

func (webhookProvider) Journey() string    { return JourneyWebhook }
func (webhookProvider) SourceType() string { return "event-sink" }
func (webhookProvider) TargetType() string { return "webpa" }

func (provider webhookProvider) Expected(input ExpectedInput) (ExpectedGraph, error) {
	if input.Source.Type != provider.SourceType() || input.Target.Type != provider.TargetType() {
		return ExpectedGraph{}, fmt.Errorf("provider %s does not support %s to %s", provider.SourceType(), input.Source.Type, input.Target.Type)
	}
	return ExpectedGraph{
		Journey: JourneyWebhook,
		Source:  EndpointIdentity{Deployment: input.Deployment.Name, Service: input.Source.Name, Type: input.Source.Type, Replica: input.Instance.Index},
		Target:  EndpointIdentity{Deployment: input.Deployment.Name, Service: input.Target.Name, Type: input.Target.Type, Replica: 0},
		Nodes: []Node{
			{ID: "subscriber", Label: input.Source.Name + " subscriber", Kind: "subscriber"},
			{ID: "intent", Label: "Registration intent", Kind: "configuration"},
			{ID: "argus", Label: "Argus", Kind: "registry"},
			{ID: "registration", Label: "Webhook registration", Kind: "registration"},
			{ID: "callback", Label: "Callback endpoint", Kind: "endpoint"},
			{ID: "caduceus", Label: "Caduceus", Kind: "delivery"},
		},
		Edges: []Edge{
			{ID: "subscriber-intent", From: "subscriber", To: "intent", Label: "read registration intent", BlocksFollowing: true},
			{ID: "argus-reachability", From: "intent", To: "argus", Label: "reach Argus", BlocksFollowing: true},
			{ID: "argus-authentication", From: "argus", To: "argus", Label: "authenticate to Argus", BlocksFollowing: true},
			{ID: "registration-present", From: "argus", To: "registration", Label: "find one registration", BlocksFollowing: true},
			{ID: "registration-fresh", From: "registration", To: "registration", Label: "verify registration freshness", BlocksFollowing: true},
			{ID: "registration-conformant", From: "registration", To: "callback", Label: "compare registration intent", BlocksFollowing: true},
			{ID: "callback-dns", From: "callback", To: "callback", Label: "resolve callback endpoint", BlocksFollowing: true},
			{ID: "callback-transport", From: "callback", To: "callback", Label: "connect to callback endpoint", BlocksFollowing: true},
			{ID: "callback-acceptance", From: "callback", To: "callback", Label: "validate callback signature and acceptance", BlocksFollowing: true},
			{ID: "caduceus-ingestion", From: "caduceus", To: "caduceus", Label: "accept synthetic event", BlocksFollowing: true},
			{ID: "caduceus-receipt", From: "caduceus", To: "subscriber", Label: "receive Caduceus callback", BlocksFollowing: true},
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
