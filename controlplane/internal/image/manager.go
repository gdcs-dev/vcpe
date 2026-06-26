package image

import (
	"context"
	"fmt"
	"sort"

	"github.com/gdcs-dev/vcpe/controlplane/internal/manifest"
	"github.com/gdcs-dev/vcpe/controlplane/internal/typeregistry"
)

const (
	PolicyBuildIfMissing = "build-if-missing"
	PolicyMissing        = "missing"
	PolicyAlwaysPull     = "always-pull"
	PolicyNeverBuild     = "never-build"
)

type Backend interface {
	ImageExists(ctx context.Context, reference string) (bool, error)
	BuildImage(ctx context.Context, req BuildRequest) error
	PullImage(ctx context.Context, req PullRequest) error
	PushImage(ctx context.Context, req PushRequest) error
	TagImage(ctx context.Context, req TagRequest) error
}

type BuildRequest struct {
	Tag     string
	Context string
	File    string
	NoCache bool
}

type BuildOptions struct {
	NoCache bool
}

type PullRequest struct {
	Reference string
}

type PushRequest struct {
	Reference string
}

type TagRequest struct {
	Source string
	Target string
}

type Action struct {
	Service string `json:"service"`
	Type    string `json:"type"`
	Image   string `json:"image"`
	Policy  string `json:"policy"`
	Action  string `json:"action"`
}

type Summary struct {
	Actions []Action `json:"actions"`
}

type LifecycleError struct {
	Service string
	Image   string
	Action  string
	Reason  string
	Err     error
}

func (e *LifecycleError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return fmt.Sprintf("image lifecycle %s failed for service %s (%s): %s: %v", e.Action, e.Service, e.Image, e.Reason, e.Err)
	}
	return fmt.Sprintf("image lifecycle %s failed for service %s (%s): %s", e.Action, e.Service, e.Image, e.Reason)
}

func (e *LifecycleError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type Manager struct {
	backend Backend
}

func New(backend Backend) *Manager {
	return &Manager{backend: backend}
}

func (m *Manager) Build(ctx context.Context, doc manifest.Document) (Summary, error) {
	return m.BuildWithOptions(ctx, doc, BuildOptions{})
}

func (m *Manager) BuildWithOptions(ctx context.Context, doc manifest.Document, opts BuildOptions) (Summary, error) {
	services := selectedServices(doc)
	summary := Summary{Actions: make([]Action, 0, len(services))}
	for _, service := range services {
		imageRef := imageReference(service.Image)
		policy := resolvePolicy(service)
		action := Action{Service: service.Name, Type: service.Type, Image: imageRef, Policy: policy, Action: "noop"}
		switch policy {
		case PolicyAlwaysPull:
			action.Action = "pull"
			if err := m.backend.PullImage(ctx, PullRequest{Reference: imageRef}); err != nil {
				return summary, &LifecycleError{Service: action.Service, Image: action.Image, Action: action.Action, Reason: "build command pull failed", Err: err}
			}
		case PolicyNeverBuild:
			exists, err := m.backend.ImageExists(ctx, imageRef)
			if err != nil {
				return summary, &LifecycleError{Service: action.Service, Image: action.Image, Action: "verify", Reason: "image existence check failed", Err: err}
			}
			if !exists {
				return summary, &LifecycleError{Service: action.Service, Image: action.Image, Action: "verify", Reason: "policy forbids building missing images"}
			}
		default:
			action.Action = "build"
			if err := m.backend.BuildImage(ctx, BuildRequest{Tag: imageRef, Context: service.Image.BuildContext, File: service.Image.Containerfile, NoCache: opts.NoCache}); err != nil {
				return summary, &LifecycleError{Service: action.Service, Image: action.Image, Action: action.Action, Reason: "build command failed", Err: err}
			}
		}
		summary.Actions = append(summary.Actions, action)
	}
	return summary, nil
}

func (m *Manager) EnsureForApply(ctx context.Context, doc manifest.Document) (Summary, error) {
	services := selectedServices(doc)
	summary := Summary{Actions: make([]Action, 0, len(services))}
	for _, service := range services {
		policy := resolvePolicy(service)
		imageRef := imageReference(service.Image)
		exists, err := m.backend.ImageExists(ctx, imageRef)
		if err != nil {
			return summary, &LifecycleError{Service: service.Name, Image: imageRef, Action: "exists", Reason: "image existence check failed", Err: err}
		}
		actionName := "noop"
		switch policy {
		case PolicyAlwaysPull:
			actionName = "pull"
			if err := m.backend.PullImage(ctx, PullRequest{Reference: imageRef}); err != nil {
				return summary, &LifecycleError{Service: service.Name, Image: imageRef, Action: actionName, Reason: "pull policy requires remote refresh", Err: err}
			}
		case PolicyNeverBuild:
			if !exists {
				return summary, &LifecycleError{Service: service.Name, Image: imageRef, Action: "verify", Reason: "policy forbids building missing images"}
			}
		default:
			if !exists {
				actionName = "build"
				if err := m.backend.BuildImage(ctx, BuildRequest{Tag: imageRef, Context: service.Image.BuildContext, File: service.Image.Containerfile}); err != nil {
					return summary, &LifecycleError{Service: service.Name, Image: imageRef, Action: actionName, Reason: "build-if-missing policy selected", Err: err}
				}
			}
		}
		summary.Actions = append(summary.Actions, Action{
			Service: service.Name,
			Type:    service.Type,
			Image:   imageRef,
			Policy:  policy,
			Action:  actionName,
		})
	}
	return summary, nil
}

func selectedServices(doc manifest.Document) []manifest.Service {
	services := make([]manifest.Service, len(doc.Spec.Services))
	copy(services, doc.Spec.Services)
	sort.Slice(services, func(left, right int) bool {
		return services[left].Name < services[right].Name
	})
	return services
}

func imageReference(image manifest.Image) string {
	if image.Repository == "" {
		return ""
	}
	tag := image.Tag
	if tag == "" {
		tag = "latest"
	}
	return fmt.Sprintf("%s:%s", image.Repository, tag)
}

func resolvePolicy(service manifest.Service) string {
	if service.Image.PullPolicy != "" {
		return normalizedPolicy(service.Image.PullPolicy)
	}
	if st, ok := typeregistry.Lookup(service.Type); ok {
		return normalizedPolicy(st.DefaultImagePolicy())
	}
	return PolicyBuildIfMissing
}

func normalizedPolicy(policy string) string {
	switch policy {
	case PolicyAlwaysPull, PolicyNeverBuild, PolicyBuildIfMissing:
		return policy
	case PolicyMissing, "build":
		return PolicyBuildIfMissing
	case "pull":
		return PolicyAlwaysPull
	default:
		return PolicyBuildIfMissing
	}
}
