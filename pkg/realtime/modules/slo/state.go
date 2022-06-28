package slo

import (
	"bytes"
	"context"
	"io"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/slo/pkg/apis/slo"
)

type stateReadWriter struct {
	ctx       context.Context
	id        string
	sloClient slo.SLOClient
}

var _ io.ReadWriter = (*stateReadWriter)(nil)

func (s *stateReadWriter) Read(p []byte) (n int, err error) {
	data, err := s.sloClient.GetState(s.ctx, &corev1.Reference{
		Id: s.id,
	})
	if err != nil {
		return 0, err
	}
	return io.ReadFull(bytes.NewReader(data.Data), p)
}

func (s *stateReadWriter) Write(p []byte) (n int, err error) {
	state := &slo.State{
		Data: p,
	}
	_, err = s.sloClient.SetState(s.ctx, &slo.SetStateRequest{
		Slo: &corev1.Reference{
			Id: s.id,
		},
		State: state,
	})
	return len(p), nil
}
