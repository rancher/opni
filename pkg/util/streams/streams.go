package streams

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func NewServerStreamWithContext(stream grpc.ServerStream) *ServerStreamWithContext {
	if s, ok := stream.(*ServerStreamWithContext); ok {
		return s
	}
	return &ServerStreamWithContext{
		Stream: stream,
		Ctx:    stream.Context(),
	}
}

func NewClientStreamWithContext(stream grpc.ClientStream) *ClientStreamWithContext {
	if s, ok := stream.(*ClientStreamWithContext); ok {
		return s
	}
	cs := &ClientStreamWithContext{
		Stream: stream,
		Ctx:    stream.Context(),
	}
	return cs
}

type ClientStreamWithContext struct {
	Stream grpc.ClientStream
	Ctx    context.Context
}

var _ grpc.ClientStream = (*ClientStreamWithContext)(nil)

func (s *ClientStreamWithContext) Header() (metadata.MD, error) {
	return s.Stream.Header()
}

func (s *ClientStreamWithContext) Trailer() metadata.MD {
	return s.Stream.Trailer()
}

func (s *ClientStreamWithContext) CloseSend() error {
	return s.Stream.CloseSend()
}

func (s *ClientStreamWithContext) Context() context.Context {
	return s.Ctx
}

func (s *ClientStreamWithContext) SendMsg(m interface{}) error {
	return s.Stream.SendMsg(m)
}

func (s *ClientStreamWithContext) RecvMsg(m interface{}) error {
	return s.Stream.RecvMsg(m)
}

func (s *ClientStreamWithContext) SetContext(ctx context.Context) {
	s.Ctx = ctx
}

type Stream interface {
	Context() context.Context
	SendMsg(m any) error
	RecvMsg(m any) error
}

type ContextSetter interface {
	SetContext(ctx context.Context)
}

type StreamWrapper interface {
	Stream
	ContextSetter
}

type ContextStream struct {
	stream Stream
	ctx    context.Context
}

func (s *ContextStream) Context() context.Context {
	return s.ctx
}

func (s *ContextStream) SetContext(ctx context.Context) {
	s.ctx = ctx
}

func (s *ContextStream) RecvMsg(m any) error {
	return s.stream.RecvMsg(m)
}

func (s *ContextStream) SendMsg(m any) error {
	return s.stream.SendMsg(m)
}

func NewStreamWithContext(ctx context.Context, s Stream) StreamWrapper {
	if ws, ok := s.(StreamWrapper); ok {
		ws.SetContext(ctx)
		return ws
	}
	return &ContextStream{
		stream: s,
		ctx:    ctx,
	}
}

type ServerStreamWithContext struct {
	Stream grpc.ServerStream
	Ctx    context.Context
}

var _ grpc.ServerStream = (*ServerStreamWithContext)(nil)

func (s *ServerStreamWithContext) SetHeader(md metadata.MD) error {
	return s.Stream.SetHeader(md)
}

func (s *ServerStreamWithContext) SendHeader(md metadata.MD) error {
	return s.Stream.SendHeader(md)
}

func (s *ServerStreamWithContext) SetTrailer(md metadata.MD) {
	s.Stream.SetTrailer(md)
}

func (s *ServerStreamWithContext) Context() context.Context {
	return s.Ctx
}

func (s *ServerStreamWithContext) SendMsg(m interface{}) error {
	return s.Stream.SendMsg(m)
}

func (s *ServerStreamWithContext) RecvMsg(m interface{}) error {
	return s.Stream.RecvMsg(m)
}

func (s *ServerStreamWithContext) SetContext(ctx context.Context) {
	s.Ctx = ctx
}
