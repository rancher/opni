package test

import (
	"fmt"
	"strings"

	"google.golang.org/grpc/resolver"
)

type environmentResolverBuilder struct {
	envs []*Environment
}

func NewEnvironmentResolver(envs ...*Environment) resolver.Builder {
	return &environmentResolverBuilder{
		envs: envs,
	}
}

const scheme = "testenv"

// Build implements resolver.Builder.
func (b *environmentResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, _ resolver.BuildOptions) (resolver.Resolver, error) {
	switch target.Endpoint() {
	case "management", "gateway":
	default:
		return nil, fmt.Errorf("unknown endpoint %q (valid endpoints are 'management' and 'gateway')", target.Endpoint())
	}
	r := &environmentResolver{
		envs:   b.envs,
		target: target,
		cc:     cc,
	}
	r.ResolveNow(resolver.ResolveNowOptions{})
	return r, nil
}

// Scheme implements resolver.Builder.
func (*environmentResolverBuilder) Scheme() string {
	return scheme
}

var _ resolver.Builder = (*environmentResolverBuilder)(nil)

type environmentResolver struct {
	envs   []*Environment
	target resolver.Target
	cc     resolver.ClientConn
}

func (r *environmentResolver) ResolveNow(resolver.ResolveNowOptions) {
	addrs := []resolver.Address{}
	for _, env := range r.envs {
		switch r.target.Endpoint() {
		case "management":
			addrs = append(addrs, resolver.Address{
				Addr: strings.TrimPrefix(env.GatewayConfig().Spec.Management.GRPCListenAddress, "tcp://"),
			})
		case "gateway":
			addrs = append(addrs, resolver.Address{
				Addr: strings.TrimPrefix(env.GatewayConfig().Spec.GRPCListenAddress, "tcp://"),
			})
		}
	}
	r.cc.UpdateState(resolver.State{Addresses: addrs})
}

func (*environmentResolver) Close() {}
