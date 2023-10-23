package stream

import (
	"context"
	"fmt"
	"reflect"
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
type Aggregator any // func(target *corev1.Reference, reply [T proto.Message], err error)

type StreamDelegate[T any] interface {
	WithTarget(*corev1.Reference) T
	WithBroadcastSelector(*streamv1.TargetSelector, Aggregator) T
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

func (w *delegatingClient[T]) WithBroadcastSelector(selector *streamv1.TargetSelector, aggregator Aggregator) T {
	rf := reflect.TypeOf(aggregator)
	msg := "expected a function matching `[T proto.Message] func(target *corev1.Reference, reply T, err error)`"
	if rf.Kind() != reflect.Func {
		panic(fmt.Sprintf("aggregator must be a function, got %T", aggregator))
	}
	if rf.NumIn() != 3 || rf.NumOut() != 0 {
		panic(fmt.Sprintf("%s, got %s (wrong number of inputs or outputs)", msg, rf))
	}
	if rf.In(0) != reflect.TypeOf((**corev1.Reference)(nil)).Elem() {
		panic(fmt.Sprintf("%s, got %s (first input must be *corev1.Reference)", msg, rf))
	}
	if !rf.In(1).Implements(reflect.TypeOf((*proto.Message)(nil)).Elem()) {
		panic(fmt.Sprintf("%s, got %s (second input must implement proto.Message)", msg, rf))
	}
	if rf.In(2) != reflect.TypeOf((*error)(nil)).Elem() {
		panic(fmt.Sprintf("%s, got %s (third input must be error)", msg, rf))
	}

	return w.newClientFunc(&targetedDelegatingClient[T]{
		selector:       selector,
		delegateClient: streamv1.NewDelegateClient(w.streamClientIntf),
		aggregator:     aggregator,
	})
}

type targetedDelegatingClient[T any] struct {
	target         *corev1.Reference
	selector       *streamv1.TargetSelector
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
			return status.Errorf(codes.Internal, "delegate request failed: %v", err)
		}
		resp := respMsg.GetResponse()
		stat := respMsg.GetStatus()
		if stat.Code() != codes.OK {
			return stat.Err()
		}
		if err := proto.Unmarshal(resp, reply.(proto.Message)); err != nil {
			return status.Errorf(codes.Internal, "failed to unmarshal response: %s: %v", string(resp), err)
		}
	case w.selector != nil:
		respMsg, err := w.delegateClient.Broadcast(ctx, &streamv1.BroadcastMessage{
			Request:        rpc,
			TargetSelector: w.selector,
		}, opts...)
		if err != nil {
			return err
		}

		var nilError error
		nilErrorValue := reflect.ValueOf(&nilError).Elem()

		fn := reflect.ValueOf(w.aggregator)
		for _, r := range respMsg.GetItems() {
			err := status.ErrorProto(r.GetResponse().GetStatusProto())
			if err != nil {
				fn.Call([]reflect.Value{
					reflect.ValueOf(r.GetTarget()),
					reflect.ValueOf(reflect.New(fn.Type().In(1)).Elem()), // nil pointer of reply type
					reflect.ValueOf(err),
				})
			} else {
				respBytes := r.GetResponse().GetResponse()
				resp := reflect.New(fn.Type().In(1)).Elem().Interface().(proto.Message).ProtoReflect().New().Interface()
				if err := proto.Unmarshal(respBytes, resp); err != nil {
					panic("bug: failed to unmarshal response: " + err.Error())
				}
				fn.Call([]reflect.Value{
					reflect.ValueOf(r.GetTarget()),
					reflect.ValueOf(resp),
					nilErrorValue,
				})
			}
		}
	default:
		panic("bug: no target or selector given")
	}

	return nil
}

func (w *targetedDelegatingClient[T]) NewStream(_ context.Context, _ *grpc.StreamDesc, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
	panic("not implemented")
}
