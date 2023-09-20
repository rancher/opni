package stream

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kralicky/totem"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// Aggregator allows the user to aggregate the responses from a broadcast request, and store the result in reply.
type Aggregator func(reply any, msg *streamv1.BroadcastReplyList) error

func DiscardReplies(any, *streamv1.BroadcastReplyList) error {
	return nil
}

type StreamDelegate[T any] interface {
	WithTarget(*corev1.Reference) T
	WithBroadcastSelector(*corev1.ClusterSelector, Aggregator) T
}

func NewDelegate[T any](cc grpc.ClientConnInterface, newClientFunc func(grpc.ClientConnInterface) T) StreamDelegate[T] {
	return &delegatingClient[T]{
		streamClientIntf: cc,
		newClientFunc:    newClientFunc,
	}
}

type delegatingClient[T any] struct {
	streamClientIntf grpc.ClientConnInterface
	newClientFunc    func(grpc.ClientConnInterface) T
}

func (w *delegatingClient[T]) WithTarget(target *corev1.Reference) T {
	return w.newClientFunc(&targetedDelegatingClient[T]{
		target:         target,
		delegateClient: streamv1.NewDelegateClient(w.streamClientIntf),
	})
}

func (w *delegatingClient[T]) WithBroadcastSelector(selector *corev1.ClusterSelector, aggregator Aggregator) T {
	return w.newClientFunc(&targetedDelegatingClient[T]{
		selector:       selector,
		delegateClient: streamv1.NewDelegateClient(w.streamClientIntf),
		aggregator:     aggregator,
	})
}

type targetedDelegatingClient[T any] struct {
	target         *corev1.Reference
	selector       *corev1.ClusterSelector
	delegateClient streamv1.DelegateClient
	aggregator     Aggregator
}

// parses a method name of the form "/service/method"
func parseQualifiedMethod(method string) (string, string, error) {
	parts := strings.Split(method, "/")
	if len(parts) != 3 {
		return "", "", fmt.Errorf("invalid method name: %s", method)
	}
	return parts[1], parts[2], nil
}

func (w *targetedDelegatingClient[T]) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	svcName, methodName, err := parseQualifiedMethod(method)
	if err != nil {
		return err
	}
	requestBytes, err := proto.Marshal(args.(proto.Message))
	if err != nil {
		return err
	}
	rpc := &totem.RPC{
		ServiceName: svcName,
		MethodName:  methodName,
		Content: &totem.RPC_Request{
			Request: requestBytes,
		},
	}
	switch {
	case w.target != nil && w.target.Id != "":
		respMsg, err := w.delegateClient.Request(ctx, &streamv1.DelegatedMessage{
			Request: rpc,
			Target:  w.target,
		}, opts...)
		if err != nil {
			return err
		}
		resp := respMsg.GetResponse()
		stat := resp.GetStatus()
		if stat.Code() != codes.OK {
			return stat.Err()
		}
		if err := proto.Unmarshal(resp.Response, reply.(proto.Message)); err != nil {
			return status.Errorf(codes.Internal, "failed to unmarshal response: %v", err)
		}
	case w.selector != nil:
		respMsg, err := w.delegateClient.Broadcast(ctx, &streamv1.BroadcastMessage{
			Request:        rpc,
			TargetSelector: w.selector,
		}, opts...)
		if err != nil {
			return err
		}

		if err := w.aggregator(reply, respMsg); err != nil {
			return fmt.Errorf("broadcast aggregator failed: %w", err)
		}
	default:
		panic("bug: no target or selector given")
	}

	return nil
}

func (w *targetedDelegatingClient[T]) NewStream(_ context.Context, _ *grpc.StreamDesc, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
	panic("not implemented")
}
