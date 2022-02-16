package waitctx

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ttacon/chalk"
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

func Wait(ctx context.Context, notifyAfter ...time.Duration) {
	data := ctx.Value(waitCtxDataKey)
	if data == nil {
		panic("context is not a WaitContext")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		data.(*waitCtxData).wg.Wait()
	}()
	if len(notifyAfter) > 0 {
		go func(d time.Duration) {
			for {
				select {
				case <-done:
					return
				case <-time.After(d):
					fmt.Fprint(os.Stderr, chalk.Yellow.Color("\n=== WARNING: waiting longer than expected for context to cancel ===\n"))
				}
			}
		}(notifyAfter[0])
	}
}
