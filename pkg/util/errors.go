package util

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// IErrGroup
// Best effort to run all tasks, then combines all errors
type IErrGroup struct {
	errMu sync.Mutex
	errs  []error
	sync.WaitGroup
}

func (i *IErrGroup) Add(tasks int) {
	i.WaitGroup.Add(tasks)
}

func (i *IErrGroup) Done() {
	i.WaitGroup.Done()
}

func (i *IErrGroup) Wait() {
	i.WaitGroup.Wait()
}

func (i *IErrGroup) AddError(err error) {
	i.errMu.Lock()
	defer i.errMu.Unlock()
	i.errs = append(i.errs, err)
}

func (i *IErrGroup) Error() error {
	if len(i.errs) == 0 {
		return nil
	}
	duped := map[string]struct{}{}
	resErr := []string{}
	for _, err := range i.errs {
		if _, ok := duped[err.Error()]; !ok {
			duped[err.Error()] = struct{}{}
			resErr = append(resErr, err.Error())
		}
	}
	sort.Strings(resErr)
	return fmt.Errorf(strings.Join(resErr, ","))
}
