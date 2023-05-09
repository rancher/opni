package util

import (
	"errors"
	"sort"
	"sync"
)

// MultiErrGroup
// Best effort to run all tasks, then combines all errors
type MultiErrGroup struct {
	errMu sync.Mutex
	errs  []error
	sync.WaitGroup
}

func (i *MultiErrGroup) Add(tasks int) {
	i.WaitGroup.Add(tasks)
}

func (i *MultiErrGroup) Done() {
	i.WaitGroup.Done()
}

func (i *MultiErrGroup) Wait() {
	i.WaitGroup.Wait()
}

func (i *MultiErrGroup) AddError(err error) {
	i.errMu.Lock()
	defer i.errMu.Unlock()
	i.errs = append(i.errs, err)
}

func (i *MultiErrGroup) Error() error {
	if len(i.errs) == 0 {
		return nil
	}
	duped := map[string]struct{}{}
	resErr := []error{}
	for _, err := range i.errs {
		if _, ok := duped[err.Error()]; !ok {
			duped[err.Error()] = struct{}{}
			resErr = append(resErr, err)
		}
	}
	sort.Slice(resErr, func(i, j int) bool {
		return resErr[i].Error() < resErr[j].Error()
	})
	return errors.Join(resErr...)
}
