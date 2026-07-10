package manifest

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// APIVersion and Kind identify the only manifest schema this control plane
// understands. Manifests declaring any other apiVersion/kind are rejected.
const (
	APIVersion = "vcpe.dev/v1"
	Kind       = "Deployment"
)

// Document is the top-level desired-state manifest. The deployment identity is
// metadata.name; there is no first-class customer concept. A "customer" is at
// most an opaque label under metadata.labels.
type Document struct {
	APIVersion string   `json:"apiVersion" yaml:"apiVersion"`
	Kind       string   `json:"kind" yaml:"kind"`
	Metadata   Metadata `json:"metadata" yaml:"metadata"`
	Spec       Spec     `json:"spec" yaml:"spec"`
}

type Metadata struct {
	Name   string            `json:"name" yaml:"name"`
	Labels map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

type Spec struct {
	Networks              []Network   `json:"networks" yaml:"networks"`
	Services              []Service   `json:"services" yaml:"services"`
	Secrets               []SecretRef `json:"secrets,omitempty" yaml:"secrets,omitempty"`
	MaxReplicasPerService int         `json:"maxReplicasPerService,omitempty" yaml:"maxReplicasPerService,omitempty"`
	MaxActiveDeployments  int         `json:"maxActiveDeployments,omitempty" yaml:"maxActiveDeployments,omitempty"`
}

// Network declares a host-attached L2/L3 segment by role. Bridge defaults to
// "<metadata.name>-<role>" (subject to interface-name length limits). Address
// families are optional and dual-stack: a network may carry ipv4, ipv6, both,
// or neither (pure L2).
type Network struct {
	Role     string         `json:"role" yaml:"role"`
	Bridge   string         `json:"bridge,omitempty" yaml:"bridge,omitempty"`
	NAT      bool           `json:"nat,omitempty" yaml:"nat,omitempty"`
	Firewall bool           `json:"firewall,omitempty" yaml:"firewall,omitempty"`
	IPv4     *AddressFamily `json:"ipv4,omitempty" yaml:"ipv4,omitempty"`
	IPv6     *AddressFamily `json:"ipv6,omitempty" yaml:"ipv6,omitempty"`
}

type AddressFamily struct {
	CIDR    string `json:"cidr" yaml:"cidr"`
	Gateway string `json:"gateway,omitempty" yaml:"gateway,omitempty"`
	Pool    *Pool  `json:"pool,omitempty" yaml:"pool,omitempty"`
}

type Pool struct {
	Start string `json:"start" yaml:"start"`
	End   string `json:"end" yaml:"end"`
}

// Service is a workload to reconcile. Type selects the registered ServiceType
// that validates Config and provides the renderer. Config is an opaque YAML
// subtree decoded strictly by the per-type validator.
type Service struct {
	Name       string      `json:"name" yaml:"name"`
	Type       string      `json:"type" yaml:"type"`
	Replicas   int         `json:"replicas" yaml:"replicas"`
	Image      Image       `json:"image" yaml:"image"`
	DependsOn  []string    `json:"dependsOn,omitempty" yaml:"dependsOn,omitempty"`
	Interfaces []Interface `json:"interfaces,omitempty" yaml:"interfaces,omitempty"`
	Ports      []string    `json:"ports,omitempty" yaml:"ports,omitempty"`
	Volumes    []string    `json:"volumes,omitempty" yaml:"volumes,omitempty"`
	Config     yaml.Node   `json:"-" yaml:"config,omitempty"`
}

type Image struct {
	Repository    string `json:"repository" yaml:"repository"`
	Tag           string `json:"tag,omitempty" yaml:"tag,omitempty"`
	BuildContext  string `json:"buildContext,omitempty" yaml:"buildContext,omitempty"`
	Containerfile string `json:"containerfile,omitempty" yaml:"containerfile,omitempty"`
	PullPolicy    string `json:"pullPolicy,omitempty" yaml:"pullPolicy,omitempty"`
}

// Interface attaches a service to a network role. Device/MAC/addresses are
// optional: when omitted and replicas==1 the control plane derives stable,
// deterministic values; IPAM remains the sole authority for IP assignment.
type Interface struct {
	Role         string `json:"role" yaml:"role"`
	Device       string `json:"device,omitempty" yaml:"device,omitempty"`
	MAC          string `json:"mac,omitempty" yaml:"mac,omitempty"`
	IPv4         string `json:"ipv4,omitempty" yaml:"ipv4,omitempty"`
	IPv6         string `json:"ipv6,omitempty" yaml:"ipv6,omitempty"`
	DefaultRoute bool   `json:"defaultRoute,omitempty" yaml:"defaultRoute,omitempty"`
}

type SecretRef struct {
	Name     string `json:"name" yaml:"name"`
	Provider string `json:"provider" yaml:"provider"`
	Key      string `json:"key" yaml:"key"`
}

// Load reads and strictly decodes a manifest. YAML is a superset of JSON, so a
// single YAML decoder handles both .yaml and .json inputs. KnownFields(true)
// rejects unknown top-level keys; per-service Config is captured as an opaque
// node and validated later by the owning ServiceType.
func Load(path string) (Document, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Document{}, fmt.Errorf("read manifest: %w", err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)

	var doc Document
	if err := dec.Decode(&doc); err != nil {
		return Document{}, fmt.Errorf("parse manifest: %w", err)
	}

	return doc, nil
}
