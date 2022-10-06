package model_training

import (
	"context"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/util/future"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ModelTrainingPlugin struct {
	system.UnimplementedSystemPluginClient
	ctx            context.Context
	Logger         *zap.SugaredLogger
	k8sClient      client.Client
	natsConnection future.Future[*nats.Conn]
	kv             future.Future[nats.KeyValue]
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
	nc, _ := newNatsConnection()
	mgr, _ := nc.JetStream()
	keyValue, _ := mgr.KeyValue("os-workload-aggregation")
	s.natsConnection.Set(nc)
	s.kv.Set(keyValue)

}

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme()
	p := &ModelTrainingPlugin{
		Logger:         logger.NewPluginLogger().Named("model_training"),
		natsConnection: future.New[*nats.Conn](),
		kv:             future.New[nats.KeyValue](),
	}
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	return scheme
}
