package gateway

import (
	"context"
	"os"
	"time"

	"github.com/cenkalti/backoff/v4"
	backoffv2 "github.com/lestrrat-go/backoff/v2"
	"github.com/nats-io/nats.go"
	"github.com/rancher/opni/apis"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) UseManagementAPI(client managementv1.ManagementClient) {
	p.mgmtClient.Set(client)
	cfg, err := client.GetConfig(
		context.Background(),
		&emptypb.Empty{},
		grpc.WaitForReady(true),
	)
	if err != nil {
		p.logger.With("err", err).Error("failed to get config")
		os.Exit(1)
	}

	objectList, err := machinery.LoadDocuments(cfg.Documents)
	if err != nil {
		p.logger.With("err", err).Error("failed to load config")
		os.Exit(1)
	}

	objectList.Visit(func(config *v1beta1.GatewayConfig) {
		//TODO : get whatever is necessary from here
	})

	k8sclient, err := k8sutil.NewK8sClient(k8sutil.ClientOptions{
		Scheme: apis.NewScheme(),
	})
	if err != nil {
		p.logger.With("err", err).Error("failed to create k8s client")
		os.Exit(1)
	}
	p.k8sClient.Set(k8sclient)
	<-p.ctx.Done()
}

func (p *Plugin) UseKeyValueStore(client system.KeyValueStoreClient) {
	var (
		nc  *nats.Conn
		err error
	)
	retrier := backoffv2.Exponential(
		backoffv2.WithMaxRetries(0),
		backoffv2.WithMinInterval(5*time.Second),
		backoffv2.WithMaxInterval(1*time.Minute),
		backoffv2.WithMultiplier(1.1),
	)
	b := retrier.Start(p.ctx)
	for backoffv2.Continue(b) {
		nc, err = p.newNatsConnection()
		if err != nil {
			break
		}
		p.logger.Error("failed to connect to NATs, retrying")
	}
	p.nc.Set(nc)

	p.storage.Set(ConfigStorageAPIs{
		Placeholder: system.NewKVStoreClient[proto.Message](client),
	})
	<-p.ctx.Done()
}

func (p *Plugin) newNatsConnection() (*nats.Conn, error) {
	natsURL := os.Getenv("NATS_SERVER_URL")
	natsSeedPath := os.Getenv("NATS_SEED_FILENAME")

	opt, err := nats.NkeyOptionFromSeed(natsSeedPath)
	if err != nil {
		return nil, err
	}

	retryBackoff := backoff.NewExponentialBackOff()
	return nats.Connect(
		natsURL,
		opt,
		nats.MaxReconnects(-1),
		nats.CustomReconnectDelay(
			func(i int) time.Duration {
				if i == 1 {
					retryBackoff.Reset()
				}
				return retryBackoff.NextBackOff()
			},
		),
		nats.DisconnectErrHandler(
			func(nc *nats.Conn, err error) {
				p.logger.With("err", err).Warn("nats disconnected")
			},
		),
	)
}
