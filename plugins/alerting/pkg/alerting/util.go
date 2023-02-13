package alerting

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// as opposed to an errGroup which stops on the first error,
// this is a best effort to run all tasks and return all errors
type independentErrGroup struct {
	errMu sync.Mutex
	errs  []error
	sync.WaitGroup
}

func (i *independentErrGroup) Add(tasks int) {
	i.WaitGroup.Add(tasks)
}

func (i *independentErrGroup) Done() {
	i.WaitGroup.Done()
}

func (i *independentErrGroup) Wait() {
	i.WaitGroup.Wait()
}

func (i *independentErrGroup) AddError(err error) {
	i.errMu.Lock()
	defer i.errMu.Unlock()
	i.errs = append(i.errs, err)
}

func (i *independentErrGroup) Error() error {
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
