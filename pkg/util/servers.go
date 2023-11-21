package util

import (
	"context"
	"net"
	"net/http"
	"reflect"

	"github.com/samber/lo"
)

// ServeHandler serves a http.Handler with the given listener. If the context
// is canceled, the server will be closed.
func ServeHandler(ctx context.Context, handler http.Handler, listener net.Listener) error {
	server := &http.Server{
		Handler: handler,
	}
	errC := lo.Async(func() error {
		return server.Serve(listener)
	})
	select {
	case <-ctx.Done():
		server.Close()
		return ctx.Err()
	case err := <-errC:
		return err
	}
}

// WaitAll waits for all the given channels to be closed, under the
// following rules:
// 1. The lifetime of the task represented by each channel is directly tied to
// the provided context.
// 2. If a task exits with an error before the context is canceled, the
// context should be canceled.
// 3. If a task exits successfully, the context should not be canceled and
// other tasks should continue to run.
//
// Once WaitAll returns, the error can be retrieved using context.Cause(ctx).
func WaitAll(ctx context.Context, ca context.CancelCauseFunc, channels ...<-chan error) {
	cases := []reflect.SelectCase{
		{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(ctx.Done()),
		},
	}
	for _, ch := range channels {
		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(ch),
		})
	}
	i, value, _ := reflect.Select(cases)
	if i == 0 {
		ca(ctx.Err())
		for _, c := range channels {
			<-c
		}
		return
	}
	channelIdx := i - 1
	var err error
	if i := value.Interface(); i != nil {
		err = i.(error)
	}
	if err == nil {
		// run again, but skip the channel which exited successfully
		WaitAll(ctx, ca, append(channels[:channelIdx], channels[channelIdx+1:]...)...)
		return
	}
	ca(err)
	for i, c := range channels {
		if i == channelIdx {
			continue
		}
		<-c
	}
}
