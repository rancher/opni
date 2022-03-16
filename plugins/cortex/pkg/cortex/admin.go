package cortex

import (
	"context"
	"crypto/tls"
	"os"
	"time"

	"github.com/cortexproject/cortex/pkg/cortexpb"
	"github.com/cortexproject/cortex/pkg/ingester/client"
	"github.com/rancher/opni-monitoring/plugins/cortex/pkg/apis/cortexadmin"
	"github.com/samber/lo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) AllUserStats(context.Context, *emptypb.Empty) (*cortexadmin.UserStatsList, error) {
	return p.UnimplementedCortexAdminServer.AllUserStats(context.Background(), &emptypb.Empty{})
}

func mapLabels(l *cortexadmin.Label, i int) cortexpb.LabelAdapter {
	return cortexpb.LabelAdapter{
		Name:  l.Name,
		Value: l.Value,
	}
}

func mapSamples(s *cortexadmin.Sample, i int) cortexpb.Sample {
	return cortexpb.Sample{
		TimestampMs: s.TimestampMs,
		Value:       s.Value,
	}
}

func mapExemplars(e *cortexadmin.Exemplar, i int) cortexpb.Exemplar {
	return cortexpb.Exemplar{
		Value:       e.Value,
		TimestampMs: e.TimestampMs,
		Labels:      lo.Map(e.Labels, mapLabels),
	}
}
func mapMetadata(m *cortexadmin.MetricMetadata, i int) *cortexpb.MetricMetadata {
	return &cortexpb.MetricMetadata{
		Type:             cortexpb.MetricMetadata_MetricType(m.Type),
		MetricFamilyName: m.MetricFamilyName,
		Help:             m.Help,
		Unit:             m.Unit,
	}
}

func mapTimeSeries(t *cortexadmin.TimeSeries, i int) cortexpb.PreallocTimeseries {
	return cortexpb.PreallocTimeseries{
		TimeSeries: &cortexpb.TimeSeries{
			Labels:    lo.Map(t.Labels, mapLabels),
			Samples:   lo.Map(t.Samples, mapSamples),
			Exemplars: lo.Map(t.Exemplars, mapExemplars),
		},
	}
}

func (p *Plugin) WriteMetrics(ctx context.Context, in *cortexadmin.WriteRequest) (*cortexadmin.WriteResponse, error) {
	cortexReq := &cortexpb.WriteRequest{
		Timeseries: lo.Map(in.Timeseries, mapTimeSeries),
		Source:     cortexpb.API,
		Metadata:   lo.Map(in.Metadata, mapMetadata),
	}
	_, err := p.distributorClient.Get().Push(outgoingContext(ctx, in), cortexReq)
	if err != nil {
		p.logger.With(
			"err", err,
		).Error("failed to write metrics")
		return nil, err
	}
	return &cortexadmin.WriteResponse{}, nil
}

type clusterIDGetter interface {
	GetClusterID() string
}

func outgoingContext(ctx context.Context, in clusterIDGetter) context.Context {
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-scope-orgid", in.GetClusterID(),
	))
}

func (p *Plugin) configureDistributorClient(tlsConfig *tls.Config) {
	cfg := p.config.Get()
	cc, err := grpc.DialContext(p.ctx, cfg.Spec.Cortex.Distributor.GRPCAddress,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                time.Second * 20,
			Timeout:             time.Second * 10,
			PermitWithoutStream: true,
		},
		))
	if err != nil {
		p.logger.With(
			"err", err,
		).Error("Failed to dial distributor")
		os.Exit(1)
	}
	p.distributorClient.Set(client.NewIngesterClient(cc))
}
