package waitctx

import (
	"context"
	"sync"
)

type waitCtxDataKeyType struct{}

var waitCtxDataKey waitCtxDataKeyType

type waitCtxData struct {
	wg sync.WaitGroup
}

func FromContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, waitCtxDataKey, &waitCtxData{
		wg: sync.WaitGroup{},
	})
}

func AddOne(ctx context.Context) {
	data := ctx.Value(waitCtxDataKey)
	if data == nil {
		panic("context is not a WaitContext")
	}
	data.(*waitCtxData).wg.Add(1)
}

func Done(ctx context.Context) {
	data := ctx.Value(waitCtxDataKey)
	if data == nil {
		panic("context is not a WaitContext")
	}
	data.(*waitCtxData).wg.Done()
}

func Wait(ctx context.Context) {
	data := ctx.Value(waitCtxDataKey)
	if data == nil {
		panic("context is not a WaitContext")
	}
	data.(*waitCtxData).wg.Wait()
}
