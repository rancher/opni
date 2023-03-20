package gateway

type InternalConditionWatcher interface {
	WatchEvents()
}

var _ InternalConditionWatcher = &SimpleInternalConditionWatcher{}

type SimpleInternalConditionWatcher struct {
	closures []func()
}

func NewSimpleInternalConditionWatcher(cl ...func()) *SimpleInternalConditionWatcher {
	return &SimpleInternalConditionWatcher{
		closures: cl,
	}
}

func (s *SimpleInternalConditionWatcher) WatchEvents() {
	for _, f := range s.closures {
		f := f
		go func() {
			f()
		}()
	}
}
