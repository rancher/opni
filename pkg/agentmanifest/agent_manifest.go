package agentmanifest

import (
	"context"
	"fmt"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ResolveAgentManfestDriver interface {
	GetDigest(ctx context.Context, packageName string) string
	GetPath(ctx context.Context, packageName string) string
	GetScheme() string
}

type AgentManifestResolver struct {
	AgentManifestResolverOptions
	ResolveAgentManfestDriver
}

type AgentManifestResolverOptions struct {
	packages []string
}

type AgentManifestResolverOption func(*AgentManifestResolverOptions)

func (o *AgentManifestResolverOptions) apply(opts ...AgentManifestResolverOption) {
	for _, opt := range opts {
		opt(o)
	}
}

func WithPackages(packages ...string) AgentManifestResolverOption {
	return func(o *AgentManifestResolverOptions) {
		o.packages = packages
	}
}

func NewAgentManifestResolver(driver ResolveAgentManfestDriver, opts ...AgentManifestResolverOption) *AgentManifestResolver {
	options := AgentManifestResolverOptions{}
	options.apply(opts...)
	return &AgentManifestResolver{
		AgentManifestResolverOptions: options,
		ResolveAgentManfestDriver:    driver,
	}
}

func (r *AgentManifestResolver) GetAgentManifest(ctx context.Context, _ *emptypb.Empty) (*controlv1.UpdateManifest, error) {
	items := []*controlv1.UpdateManifestEntry{}
	for _, pkg := range r.packages {
		if digest := r.GetDigest(ctx, pkg); digest != "" {
			items = append(items, &controlv1.UpdateManifestEntry{
				Package: pkg,
				Digest:  digest,
				Path:    fmt.Sprintf("%s://%s", r.GetScheme(), r.GetPath(ctx, pkg)),
			})
		}
	}
	return &controlv1.UpdateManifest{
		Items: items,
	}, nil
}

var (
	agentManifestBuilderCache = map[string]func(...any) (ResolveAgentManfestDriver, error){}
)

func RegisterAgentManifestBuilder[T ~string](name T, builder func(...any) (ResolveAgentManfestDriver, error)) {
	agentManifestBuilderCache[string(name)] = builder
}

func GetAgentManifestBuilder[T ~string](name T) func(...any) (ResolveAgentManfestDriver, error) {
	return agentManifestBuilderCache[string(name)]
}
