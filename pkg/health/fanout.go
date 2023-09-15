package health

type FanOutUpdater struct {
	healthC chan HealthUpdate
	statusC chan StatusUpdate
}

var _ HealthStatusUpdateReader = (*FanOutUpdater)(nil)

func NewFanOutUpdater(source HealthStatusUpdateReader, writers ...HealthStatusUpdateWriter) HealthStatusUpdateReader {
	healthBufferC := make(chan HealthUpdate, 256)
	statusBufferC := make(chan StatusUpdate, 256)
	healthWriters := make([]chan<- HealthUpdate, len(writers))
	statusWriters := make([]chan<- StatusUpdate, len(writers))
	for i, sink := range writers {
		healthWriters[i] = sink.HealthWriterC()
		statusWriters[i] = sink.StatusWriterC()
	}

	go func() {
		defer close(healthBufferC)
		for h := range source.HealthC() {
			healthBufferC <- h
			for _, w := range healthWriters {
				w <- h
			}
		}
		for _, w := range healthWriters {
			close(w)
		}
	}()
	go func() {
		defer close(statusBufferC)
		for s := range source.StatusC() {
			statusBufferC <- s
			for _, w := range statusWriters {
				w <- s
			}
		}
		for _, w := range statusWriters {
			close(w)
		}
	}()

	return &FanOutUpdater{
		healthC: healthBufferC,
		statusC: statusBufferC,
	}
}

func (f *FanOutUpdater) HealthC() <-chan HealthUpdate {
	return f.healthC
}

func (f *FanOutUpdater) StatusC() <-chan StatusUpdate {
	return f.statusC
}
