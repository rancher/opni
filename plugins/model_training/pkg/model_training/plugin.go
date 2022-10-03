package model_training

import (
	"context"
	"log"
	"time"

	backoffv2 "github.com/lestrrat-go/backoff/v2"
	"github.com/nats-io/nats.go"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ModelTrainingPlugin struct {
	system.UnimplementedSystemPluginClient
	ctx            context.Context
	Logger         *zap.SugaredLogger
	k8sClient      client.Client
	natsConnection *nats.Conn
	kv             nats.KeyValue
}

func (s *ModelTrainingPlugin) UseManagementAPI(api managementv1.ManagementClient) {
	lg := s.Logger
	lg.Info("querying management API...")
	var list *managementv1.APIExtensionInfoList
	for {
		var err error
		list, err = api.APIExtensions(context.Background(), &emptypb.Empty{})
		if err != nil {
			log.Fatal(err)
		}
		if len(list.Items) == 0 {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		break
	}
	for _, ext := range list.Items {
		lg.Info("found API extension service", "name", ext.ServiceDesc.GetName())
	}
}

func (s *ModelTrainingPlugin) get_kv() {
	retrier := backoffv2.Exponential(
		backoffv2.WithMaxRetries(0),
		backoffv2.WithMinInterval(5*time.Second),
		backoffv2.WithMaxInterval(1*time.Minute),
		backoffv2.WithMultiplier(1.1),
	)
	b := retrier.Start(s.ctx)
	for backoffv2.Continue(b) {
		nc, err := s.newNatsConnection()
		if err == nil {
			break
		}
		s.Logger.Error("failed to connect to nats, retrying")
	}
}

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme()
	p := &ModelTrainingPlugin{
		Logger: logger.NewPluginLogger().Named("model_training"),
	}
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	return scheme
}
