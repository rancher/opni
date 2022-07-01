package bootstrap

import (
	"context"
	"fmt"
	"time"

	v1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/keyring"
	"github.com/rancher/opni/pkg/pkp"
	"github.com/rancher/opni/pkg/tokens"
	"github.com/rancher/opni/pkg/trust"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

// InClusterBootstrapper is a Bootstrapper that can bootstrap itself inside the
// main cluster with direct access to the management api.
type InClusterBootstrapper struct {
	Capability         string
	GatewayEndpoint    string
	ManagementEndpoint string

	cc       ClientConfig
	finalize func(context.Context) error
}

func (b *InClusterBootstrapper) Bootstrap(ctx context.Context, ident ident.Provider) (keyring.Keyring, error) {
	client, err := clients.NewManagementClient(ctx, clients.WithAddress(b.ManagementEndpoint))
	if err != nil {
		return nil, err
	}
	certInfo, err := client.CertsInfo(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	if len(certInfo.Chain) == 0 {
		return nil, fmt.Errorf("server certificate chain is empty")
	}
	token, err := client.CreateBootstrapToken(ctx, &v1.CreateBootstrapTokenRequest{
		Ttl: durationpb.New(5 * time.Minute),
	})
	if err != nil {
		return nil, err
	}
	b.cc.Capability = b.Capability
	b.cc.Token, err = tokens.FromBootstrapToken(token)
	if err != nil {
		return nil, err
	}
	b.cc.Endpoint = b.GatewayEndpoint

	pin, err := pkp.DecodePin(certInfo.Chain[len(certInfo.Chain)-1].Fingerprint)
	if err != nil {
		return nil, err
	}
	stratConf := trust.StrategyConfig{
		PKP: &trust.PKPConfig{
			Pins: trust.NewPinSource([]*pkp.PublicKeyPin{pin}),
		},
	}
	b.cc.TrustStrategy, err = stratConf.Build()
	if err != nil {
		return nil, err
	}

	b.finalize = func(ctx context.Context) error {
		_, err := client.RevokeBootstrapToken(ctx, token.Reference())
		return err
	}
	return b.cc.Bootstrap(ctx, ident)
}

func (b *InClusterBootstrapper) Finalize(ctx context.Context) error {
	if b.finalize == nil {
		// this can happen when finalization is skipped or encounters an error,
		// and attempts to run the finalization steps again later. In this case,
		// we can safely do nothing and just let the token expire.
		return nil
	}
	return b.finalize(ctx)
}
