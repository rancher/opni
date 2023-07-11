package targets

import (
	"context"
	"os"
	"strings"
	"sync"

	"errors"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"github.com/magefile/mage/target"
	"gopkg.in/yaml.v3"
)

type Mocks struct {
	Mocks []Mock `yaml:"mocks"`
}

type Mock struct {
	Source string   `yaml:"source,omitempty"`
	Import string   `yaml:"import,omitempty"`
	Dest   string   `yaml:"dest"`
	Types  []string `yaml:"types"`
}

// Generates mocks using the config in pkg/test/mock/mockgen.yaml
func (Generate) Mocks(ctx context.Context) error {
	_, tr := Tracer.Start(ctx, "target.generate.mocks")
	defer tr.End()

	wg := sync.WaitGroup{}
	errs := []error{}
	mu := sync.Mutex{}

	var mocks Mocks
	yamlConfig, err := os.ReadFile("pkg/test/mock/mockgen.yaml")
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(yamlConfig, &mocks); err != nil {
		return err
	}
	for _, mock := range mocks.Mocks {
		if shouldGenerate, _ := target.Path(mock.Dest, mock.Source); !shouldGenerate {
			continue
		}
		wg.Add(1)
		go func(mock Mock) {
			defer wg.Done()
			args := []string{"run", "go.uber.org/mock/mockgen"}
			args = append(args, "-destination="+mock.Dest)
			if mock.Import != "" {
				args = append(args, mock.Import)
			} else if mock.Source != "" {
				args = append(args, "-source="+mock.Source)
			} else {
				panic("invalid mock config: either Source or Import must be set")
			}
			args = append(args, strings.Join(mock.Types, ","))
			err := sh.RunWithV(map[string]string{
				"CGO_ENABLED": "0",
			}, mg.GoCmd(), args...)
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(mock)
	}
	wg.Wait()
	return errors.Join(errs...)
}
