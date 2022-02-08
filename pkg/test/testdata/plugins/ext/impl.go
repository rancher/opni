package ext

import (
	"context"
	"strings"
)

type ExtServerImpl struct {
	UnimplementedExtServer
}

func (s *ExtServerImpl) Foo(_ context.Context, req *FooRequest) (*FooResponse, error) {
	return &FooResponse{
		Baz: strings.ToUpper(req.Bar),
	}, nil
}
