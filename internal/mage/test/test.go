package test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sync"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"github.com/jaypipes/ghw"
	"github.com/kralicky/spellbook/build"
	"github.com/kralicky/spellbook/testbin"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util"
)

func SysInfo() {
	fmt.Println("System Info:")
	for _, proc := range util.Must(ghw.CPU()).Processors {
		fmt.Printf(" %v (%d cores, %d threads)\n", proc.Model, proc.NumCores, proc.NumThreads)
	}
	fmt.Printf(" %v\n", util.Must(ghw.Topology()))
	fmt.Printf(" %v\n", util.Must(ghw.Memory()))
	fmt.Printf("CI Environment: %s\n", testutil.IfCI("yes").Else("no"))
}

func Test() error {
	mg.Deps(testbin.Testbin, build.Build, SysInfo)

	instances := load.Instances([]string{}, &load.Config{
		Tags: []string{"test"},
	})
	cc := cuecontext.New()
	if len(instances) != 1 {
		return errors.New("expected 1 main package")
	}
	value := cc.BuildInstance(instances[0])
	if err := value.Err(); err != nil {
		return fmt.Errorf("cue compiler error: %w", err)
	}
	tests := value.LookupPath(cue.ParsePath("tests"))
	if err := tests.Err(); err != nil {
		return err
	}
	if err := tests.Validate(cue.Concrete(true)); err != nil {
		return fmt.Errorf("cue validation error: %w", err)
	}
	actions := tests.LookupPath(cue.ParsePath("actions"))
	if err := actions.Err(); err != nil {
		return err
	}
	parallel, err := tests.LookupPath(cue.ParsePath("parallel")).Bool()
	if err != nil {
		return err
	}
	iter, err := actions.Fields()
	if err != nil {
		return err
	}
	coverProfiles := []string{}
	wg := sync.WaitGroup{}
	var testErr error
	for iter.Next() {
		action := iter.Value()
		command := action.LookupPath(cue.ParsePath("command"))
		if err := command.Err(); err != nil {
			return err
		}
		cover := action.LookupPath(cue.ParsePath("cover"))
		if err := cover.Err(); err == nil {
			if enabled, err := cover.LookupPath(cue.ParsePath("enabled")).Bool(); err == nil && enabled {
				profile, err := cover.LookupPath(cue.ParsePath("coverprofile")).String()
				if err != nil {
					return err
				}
				coverProfiles = append(coverProfiles, profile)
			}
		}
		cmd := struct {
			Name string
			Args []string
		}{}
		if err := command.Decode(&cmd); err != nil {
			return err
		}
		if parallel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				testErr = sh.RunV(cmd.Name, cmd.Args...)
			}()
		} else {
			testErr = sh.RunV(cmd.Name, cmd.Args...)
		}
	}
	wg.Wait()
	if testErr != nil {
		return testErr
	}
	if len(coverProfiles) > 0 {
		enabled, err := tests.LookupPath(cue.ParsePath("coverage.mergeReports")).Bool()
		if err != nil {
			return err
		}
		wroteHeader := false
		if enabled {
			mergedCoverProfile, err := tests.LookupPath(cue.ParsePath("coverage.mergedCoverProfile")).String()
			if err != nil {
				return err
			}
			merged := new(bytes.Buffer)
			for _, profile := range coverProfiles {
				contents, err := os.ReadFile(profile)
				if err != nil {
					return err
				}
				header, rest, ok := bytes.Cut(contents, []byte("\n"))
				if !ok {
					return errors.New("failed to find header in coverprofile")
				}
				if !wroteHeader {
					merged.Write(append(header, '\n'))
					wroteHeader = true
				}
				merged.Write(rest)
				os.Remove(profile)
			}
			os.WriteFile(mergedCoverProfile, merged.Bytes(), 0644)
		}
	}
	return nil
}
