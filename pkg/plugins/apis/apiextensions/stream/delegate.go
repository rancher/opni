package stream

import (
	"context"
	"fmt"
	"strings"

	"github.com/kralicky/totem"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type StreamDelegate[T any] interface {
	WithTarget(*corev1.Reference) T
	WithBroadcastSelector(*corev1.ClusterSelector) T
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

func (w *delegatingClient[T]) WithBroadcastSelector(selector *corev1.ClusterSelector) T {
	return w.newClientFunc(&targetedDelegatingClient[T]{
		selector:       selector,
		delegateClient: streamv1.NewDelegateClient(w.streamClientIntf),
	})
}

type targetedDelegatingClient[T any] struct {
	target         *corev1.Reference
	selector       *corev1.ClusterSelector
	delegateClient streamv1.DelegateClient
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
	if w.target != nil {
		respMsg, err := w.delegateClient.Request(ctx, &streamv1.DelegatedMessage{
			Request: rpc,
			Target:  w.target,
		})
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
	} else if w.selector != nil {
		_, err := w.delegateClient.Broadcast(ctx, &streamv1.BroadcastMessage{
			Request:        rpc,
			TargetSelector: w.selector,
		})
		if err != nil {
			return err
		}
	}
	panic("bug: no target or selector given")
}

func (w *targetedDelegatingClient[T]) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	panic("not implemented")
}
