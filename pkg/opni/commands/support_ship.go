package commands

import (
	"context"
	"os"

	"github.com/rancher/opni/pkg/supportagent/dateparser"
	"github.com/rancher/opni/pkg/supportagent/filereader"
	"github.com/rancher/opni/pkg/supportagent/shipper"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

func shipRKELogs(ctx context.Context, cc grpc.ClientConnInterface, lg *zap.SugaredLogger) error {
	if err := shipRKEEtcd(ctx, cc, lg); err != nil {
		return err
	}
	if err := shipRKEKubeApi(ctx, cc, lg); err != nil {
		return err
	}
	return nil
}

func shipRKEEtcd(ctx context.Context, cc grpc.ClientConnInterface, lg *zap.SugaredLogger) error {
	reader, err := filereader.NewFileReader("k8s/containerlogs/etcd")
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	defer reader.Close()
	p := &dateparser.DefaultParser{
		TimestampRegex: dateparser.EtcdRegex,
	}
	s := shipper.NewOTLPShipper(cc, p, lg,
		shipper.WithComponent("etcd"),
		shipper.WithLogType("controlplane"),
	)
	return s.Publish(ctx, reader.Scan())
}

func shipRKEKubeApi(ctx context.Context, cc grpc.ClientConnInterface, lg *zap.SugaredLogger) error {
	reader, err := filereader.NewFileReader("k8s/containerlogs/kube-apiserver")
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	defer reader.Close()
	p := &dateparser.DefaultParser{
		TimestampRegex: dateparser.KlogRegex,
	}
	s := shipper.NewOTLPShipper(cc, p, lg,
		shipper.WithComponent("kube-apiserver"),
		shipper.WithLogType("controlplane"),
	)
	return s.Publish(ctx, reader.Scan())
}
