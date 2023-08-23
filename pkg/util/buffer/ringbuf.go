package buffer

import (
	"github.com/rancher/opni/pkg/logger"
	"go.uber.org/zap"
)

type RingBuffer[T any] struct {
	inputChannel  <-chan T
	outputChannel chan T
	lg            *zap.SugaredLogger
}

func NewRingBuffer[T any](inputChannel <-chan T, outputChannel chan T) *RingBuffer[T] {
	return &RingBuffer[T]{inputChannel: inputChannel, outputChannel: outputChannel, lg: logger.NewPluginLogger().Named("ring-buffer")}
}

func (r *RingBuffer[T]) Run() {
	for v := range r.inputChannel {
		select {
		case r.outputChannel <- v:
		default:
			r.lg.Debugf("dropping item from ring buffer")
			<-r.outputChannel
			r.outputChannel <- v
		}
	}
	close(r.outputChannel)
}
