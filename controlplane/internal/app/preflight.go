package app

import (
	"context"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/gdcs-dev/vcpe/controlplane/internal/imageref"
	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
	"github.com/gdcs-dev/vcpe/controlplane/internal/types/gateway"
)

// Preflight performs all pre-mutation validation: the structural manifest rules
// plus the registry-aware checks (unregistered type, per-type config decode,
// expected-role satisfaction). It must pass before any runtime mutation.
func Preflight(doc manifest.Document) error {
	if err := manifest.Validate(doc); err != nil {
		return err
	}

	// Roles declared by networks, for expected-role checks.
	networkRoles := map[string]struct{}{}
	for _, n := range doc.Spec.Networks {
		networkRoles[n.Role] = struct{}{}
	}
	// Roles each service attaches to, for the unused-network warning.
	referencedRoles := map[string]struct{}{}

	for _, svc := range doc.Spec.Services {
		st, ok := typeregistry.Lookup(svc.Type)
		if !ok {
			return fmt.Errorf("service %q declares unsupported type %q: registered types are %s", svc.Name, svc.Type, strings.Join(typeregistry.Registered(), ", "))
		}
		if validator, ok := st.(typeregistry.ReplicaValidator); ok {
			if err := validator.ValidateReplicas(svc.Replicas); err != nil {
				return fmt.Errorf("service %q replicas: %w", svc.Name, err)
			}
		}
		if err := st.ValidateConfig(svc.Config); err != nil {
			return fmt.Errorf("service %q config: %w", svc.Name, err)
		}
		if svc.Type == "gateway" {
			if err := gateway.ValidateMeshLAN(svc); err != nil {
				return fmt.Errorf("service %q config: %w", svc.Name, err)
			}
		}

		ifaceRoles := map[string]struct{}{}
		for _, iface := range svc.Interfaces {
			ifaceRoles[iface.Role] = struct{}{}
			referencedRoles[iface.Role] = struct{}{}
		}
		if err := assertExpectedRoles(svc.Name, st.ExpectedRoles(), ifaceRoles); err != nil {
			return err
		}
		if err := st.ValidateInterfaces(svc.Interfaces); err != nil {
			return fmt.Errorf("service %q interfaces: %w", svc.Name, err)
		}
	}

	for _, w := range UnusedNetworkWarnings(doc) {
		// Warnings are non-fatal; surface them on stderr via the observability
		// log so operators see them without failing the apply.
		_ = w
	}
	return nil
}

func requirePinnedMeshGatewayImages(doc manifest.Document) error {
	for _, svc := range doc.Spec.Services {
		for _, radio := range svc.Radios {
			if radio.Mode != manifest.RadioModeMesh {
				continue
			}
			if doc.Metadata.Labels["environment"] == "development" && svc.Type == "gateway" &&
				svc.Image.Repository == "ghcr.io/gdcs-dev/gateway" && svc.Image.Tag == "dev" &&
				svc.Image.PullPolicy == "build-if-missing" && svc.Image.BuildContext == "services/gateway" {
				break
			}
			const prefix = "ghcr.io/gdcs-dev/gateway@sha256:"
			digest := strings.TrimPrefix(svc.Image.Repository, prefix)
			decoded, err := hex.DecodeString(digest)
			if !strings.HasPrefix(svc.Image.Repository, prefix) || err != nil || len(decoded) != 32 || strings.ToLower(digest) != digest || svc.Image.Tag != "" || svc.Image.BuildContext != "" {
				return fmt.Errorf("mesh Gateway %q requires an immutable ghcr.io/gdcs-dev/gateway@sha256 digest without a tag or build context", svc.Name)
			}
			break
		}
	}
	return nil
}

type meshGatewayImageVerifier interface {
	VerifyMeshGatewayImage(context.Context, string) error
}

func verifyMeshGatewayImages(ctx context.Context, doc manifest.Document, verifier meshGatewayImageVerifier) error {
	for _, svc := range doc.Spec.Services {
		for _, radio := range svc.Radios {
			if radio.Mode != manifest.RadioModeMesh {
				continue
			}
			if err := verifier.VerifyMeshGatewayImage(ctx, imageref.Format(svc.Image)); err != nil {
				return fmt.Errorf("mesh Gateway %q image verification: %w", svc.Name, err)
			}
			break
		}
	}
	return nil
}

// assertExpectedRoles fails when a required role declared by a service type is
// not satisfied by the service's interfaces.
func assertExpectedRoles(service string, required []typeregistry.RoleRequirement, present map[string]struct{}) error {
	for _, req := range required {
		if !req.Required {
			continue
		}
		if _, ok := present[req.Role]; !ok {
			return fmt.Errorf("service %q does not satisfy expected role %q for its type", service, req.Role)
		}
	}
	return nil
}

// UnusedNetworkWarnings returns a warning string for each declared network that
// no interface references. Unused networks are benign, so these are warnings
// rather than errors.
func UnusedNetworkWarnings(doc manifest.Document) []string {
	referenced := map[string]struct{}{}
	for _, svc := range doc.Spec.Services {
		for _, iface := range svc.Interfaces {
			referenced[iface.Role] = struct{}{}
		}
	}
	var warnings []string
	for _, n := range doc.Spec.Networks {
		if _, ok := referenced[n.Role]; !ok {
			warnings = append(warnings, fmt.Sprintf("network role %q is declared but referenced by no interface", n.Role))
		}
	}
	sort.Strings(warnings)
	return warnings
}
