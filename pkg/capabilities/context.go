package capabilities

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	"go.uber.org/atomic"
)

var (
	ErrObjectDeleted      = errors.New("object deleted")
	ErrCapabilityNotFound = errors.New("capability not found")
	ErrEventChannelClosed = errors.New("event channel closed")
)

type capabilityContext[T corev1.Capability[T], U corev1.MetadataAccessor[T]] struct {
	base context.Context

	eventC       <-chan storage.WatchEvent[U]
	done         chan struct{}
	capabilities []T
	err          *atomic.Error
}

// Returns a context that listens on a watch event channel and closes its
// Done channel when the watched object loses one of the specified capabilities,
// or if the object is deleted. This context should have exclusive read access
// to the event channel to avoid missing events.
// If the list of capabilities is empty, the context will close its Done
// channel if any capabilities are deleted.
// Specific capabilities listed will be ignored if the context does not have
// them. The context must have previously had one of the capabilities and lose
// it to cause the context to close its Done channel.
func NewContext[T corev1.Capability[T], U corev1.MetadataAccessor[T]](
	base context.Context,
	eventC <-chan storage.WatchEvent[U],
	capabilities ...T,
) (context.Context, context.CancelFunc) {
	base, ca := context.WithCancel(base)
	cc := &capabilityContext[T, U]{
		base:         base,
		eventC:       eventC,
		done:         make(chan struct{}),
		capabilities: capabilities,
		err:          atomic.NewError(nil),
	}
	go cc.watch()
	return cc, ca
}

func (cc *capabilityContext[T, U]) watch() {
	defer close(cc.done)
	for {
		select {
		case <-cc.base.Done():
			cc.err.Store(cc.base.Err())
			return
		case event, ok := <-cc.eventC:
			if !ok {
				cc.err.Store(ErrEventChannelClosed)
				return
			}
			switch event.EventType {
			case storage.WatchEventDelete:
				cc.err.Store(ErrObjectDeleted)
				return
			case storage.WatchEventUpdate:
				if len(cc.capabilities) == 0 {
					// check if any capabilities were lost
					for _, c := range event.Previous.GetCapabilities() {
						if !Has(event.Current, c) {
							cc.err.Store(fmt.Errorf("%w: %s", ErrCapabilityNotFound, c))
							return
						}
					}
				} else {
					// check if specific capabilities were lost
					for _, c := range cc.capabilities {
						if Has(event.Previous, c) && !Has(event.Current, c) {
							cc.err.Store(fmt.Errorf("%w: %s", ErrCapabilityNotFound, c))
							return
						}
					}
				}
			}
		}
	}
}

func (cc *capabilityContext[T, U]) Deadline() (time.Time, bool) {
	return cc.base.Deadline()
}

func (cc *capabilityContext[T, U]) Done() <-chan struct{} {
	return cc.done
}

func (cc *capabilityContext[T, U]) Err() error {
	return cc.err.Load()
}

func (cc *capabilityContext[T, U]) Value(key any) any {
	return cc.base.Value(key)
}
