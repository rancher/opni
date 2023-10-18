package services

import (
	"context"
	"errors"
	"net/url"

	"github.com/google/uuid"
	amcfg "github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promlog"
	"github.com/rancher/opni/pkg/alerting/configutil"
	"github.com/rancher/opni/pkg/alerting/notifiers"
	alertingv2 "github.com/rancher/opni/pkg/apis/alerting/v2"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	yamlv2 "gopkg.in/yaml.v2"
)

type ReceiverStorageService interface {
	storage.KeyValueStoreT[*alertingv2.OpniReceiver]
	alertingv2.ReceiverServerServer
	Impl() grpc.ServiceDesc
}

type receiverServer struct {
	storage.KeyValueStoreT[*alertingv2.OpniReceiver]
	alertingv2.UnsafeReceiverServerServer
}

func (r *receiverServer) Impl() grpc.ServiceDesc {
	return alertingv2.ReceiverServer_ServiceDesc
}

func (r *receiverServer) getWithRev(ctx context.Context, ref *corev1.Reference) (*alertingv2.OpniReceiver, error) {
	var revision int64
	recv, err := r.KeyValueStoreT.Get(ctx, ref.GetId(), storage.WithRevisionOut(&revision))
	if err != nil {
		return nil, err
	}
	recv.Revision = revision
	return recv, nil
}

func (r *receiverServer) GetReceiver(ctx context.Context, ref *corev1.Reference) (*alertingv2.OpniReceiver, error) {
	return r.getWithRev(ctx, ref)
}

func (r *receiverServer) ListReceivers(ctx context.Context, _ *emptypb.Empty) (*alertingv2.OpniReceiverList, error) {
	keys, err := r.KeyValueStoreT.ListKeys(ctx, "")
	if err != nil {
		return nil, err
	}

	var eg util.MultiErrGroup
	yieldChan := make(chan *alertingv2.OpniReceiver, len(keys))
	go func() {
		defer close(yieldChan)
		for _, key := range keys {
			key := key
			eg.Go(func() error {
				recv, err := r.getWithRev(ctx, &corev1.Reference{Id: key})
				if err != nil {
					return err
				}
				yieldChan <- recv
				return nil
			})
		}
		eg.Wait()
	}()
	res := &alertingv2.OpniReceiverList{
		Items: []*alertingv2.OpniReceiver{},
	}
	for recv := range yieldChan {
		res.Items = append(res.Items, recv)
	}
	return res, eg.Error()
}

func toReceiverStruct(recv *alertingv2.OpniReceiver) (*amcfg.Receiver, error) {
	c := &amcfg.Receiver{}
	if err := configutil.LoadFromAPI[*amcfg.Receiver](c, recv.Receiver); err != nil {
		return nil, err
	}
	_, err := yamlv2.Marshal(c)
	if err != nil {
		return nil, validation.Error(err.Error())
	}
	return c, nil
}

func (r *receiverServer) PutReceiver(ctx context.Context, req *alertingv2.OpniReceiver) (*corev1.Reference, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if _, err := toReceiverStruct(req); err != nil {
		return nil, err
	}

	ref := req.GetReference()
	if ref == nil || ref.GetId() == "" {
		ref = &corev1.Reference{
			Id: uuid.New().String(),
		}
	}
	if err := r.KeyValueStoreT.Put(ctx, ref.Id, req, storage.WithRevision(req.Revision)); err != nil {
		return nil, err
	}
	return ref, nil
}

func (r *receiverServer) DeleteReceiver(ctx context.Context, ref *alertingv2.DeleteReceiverRequest) (*emptypb.Empty, error) {
	if err := r.KeyValueStoreT.Delete(ctx, ref.GetReference().GetId(), storage.WithRevision(
		ref.GetRevision(),
	)); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (r *receiverServer) TestReceiver(ctx context.Context, req *alertingv2.OpniReceiver) (*emptypb.Empty, error) {
	ncOrig, err := toReceiverStruct(req)
	if err != nil {
		return nil, err
	}
	// == this section is kind of a hack, we let alertmanager's unmarshalling handle the
	// correct replacement of default configs

	mockConfig := &amcfg.Config{
		Receivers: []amcfg.Receiver{*ncOrig},
		Route: &amcfg.Route{
			Receiver: "test",
		},
	}

	bytes, err := configutil.MarshalConfig(mockConfig)
	if err != nil {
		return nil, err
	}

	mockConfig, err = amcfg.Load(string(bytes))
	if err != nil {
		return nil, err
	}
	templates, err := template.FromGlobs(mockConfig.Templates)
	if err != nil {
		return nil, err
	}
	templates.ExternalURL = &url.URL{
		Scheme: "http",
		Host:   "localhost:9093",
	}
	if len(mockConfig.Receivers) == 0 {
		return nil, status.Error(codes.Internal, "constructed config had no receivers")
	}
	nc := &mockConfig.Receivers[0]

	// == end of hack
	notifiers, err := notifiers.BuildReceiverIntegrations(*nc, templates, promlog.New(&promlog.Config{}))
	if err != nil {
		return nil, err
	}
	errs := []error{}
	for _, notifier := range notifiers {
		_, err := notifier.Notify(ctx, &types.Alert{
			Alert: model.Alert{
				Labels: model.LabelSet{
					"alertname": "TestAlert",
				},
			},
		})
		errs = append(errs, err)
	}
	return &emptypb.Empty{}, errors.Join(errs...)
}

var _ ReceiverStorageService = (*receiverServer)(nil)

func NewReceiverServer(
	store storage.KeyValueStoreT[*alertingv2.OpniReceiver],
) ReceiverStorageService {
	return &receiverServer{
		KeyValueStoreT: store,
	}
}
