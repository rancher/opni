package test

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"github.com/jaypipes/ghw"
	"github.com/kralicky/spellbook/build"
	"github.com/kralicky/spellbook/testbin"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"github.com/onsi/ginkgo/v2/types"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util"
	"golang.org/x/exp/slices"
)

const (
	ginkgoPkg = "github.com/onsi/ginkgo/v2/ginkgo"
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

var actions []string

func init() {
	idx := slices.Index(os.Args, "test")
	if idx != -1 {
		actions = os.Args[idx+1:]
		os.Args = os.Args[:idx+1]
	}
}

func Test() error {
	mg.Deps(testbin.Testbin, build.Build, SysInfo)

	rt, err := NewTestPlanRuntime()
	if err != nil {
		return err
	}

	return rt.Run(actions...)
}

type TestPlanSpec struct {
	Parallel bool
	Actions  map[string]RunSpec
	Coverage CoverageSpec
}

type RunSpec struct {
	Name     string
	Packages string

	Suite    *types.SuiteConfig
	Build    *types.GoFlagsConfig
	Run      *types.CLIConfig
	Reporter *types.ReporterConfig
}

func (rs *RunSpec) CommandLine() (string, []string, error) {
	flags := types.SuiteConfigFlags
	flags = flags.CopyAppend(types.ReporterConfigFlags...)
	flags = flags.CopyAppend(types.GinkgoCLISharedFlags...)
	flags = flags.CopyAppend(types.GinkgoCLIRunAndWatchFlags...)
	flags = flags.CopyAppend(types.GinkgoCLIRunFlags...)
	flags = flags.CopyAppend(types.GoBuildFlags...)
	flags = flags.CopyAppend(types.GoRunFlags...)

	args, err := types.GenerateFlagArgs(flags, map[string]interface{}{
		"S":  rs.Suite,
		"R":  rs.Reporter,
		"C":  rs.Run,
		"Go": rs.Build,
	})
	if err != nil {
		return "", nil, err
	}
	args = append(args, strings.Split(rs.Packages, ",")...)
	return mg.GoCmd(), append([]string{"run", ginkgoPkg}, args...), nil
}

type CoverageSpec struct {
	MergeProfiles     bool
	MergedProfileName string
	ExcludePatterns   []string
}

type TestPlanRuntime struct {
	ctx          *cue.Context
	spec         TestPlanSpec
	testPackages []string
}

func NewTestPlanRuntime() (*TestPlanRuntime, error) {
	pkgs, err := findTestPackages()
	if err != nil {
		return nil, err
	}

	instances := load.Instances([]string{}, &load.Config{
		Tags: []string{
			"test",
			"packages=" + strings.Join(pkgs, ","),
		},
	})
	if len(instances) != 1 {
		return nil, errors.New("expected 1 main package")
	}
	ctx := cuecontext.New()
	value := ctx.BuildInstance(instances[0])

	if err := value.Err(); err != nil {
		return nil, fmt.Errorf("cue compiler error: %w", err)
	}

	plan := value.LookupPath(cue.ParsePath("tests"))

	var spec TestPlanSpec
	if err := plan.Decode(&spec); err != nil {
		return nil, fmt.Errorf("error decoding test plan: %w", err)
	}
	return &TestPlanRuntime{
		ctx:          ctx,
		spec:         spec,
		testPackages: pkgs,
	}, nil
}

func (rt *TestPlanRuntime) Run(actions ...string) (testErr error) {
	var once sync.Once
	var wg sync.WaitGroup

	if len(actions) > 0 {
		fmt.Println("only running the following actions:", actions)
	}

	for _, run := range rt.spec.Actions {
		if len(actions) > 0 && !slices.Contains(actions, run.Name) {
			continue
		}
		name, args, err := run.CommandLine()
		if err != nil {
			return err
		}
		if rt.spec.Parallel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := sh.RunV(name, args...)
				if err != nil {
					once.Do(func() {
						testErr = err
					})
				}
			}()
		} else {
			err := sh.RunV(name, args...)
			if err != nil {
				once.Do(func() {
					testErr = err
				})
			}
		}
	}
	wg.Wait()

	// merge coverage profiles
	coverProfiles := []string{}
	for _, run := range rt.spec.Actions {
		if len(actions) > 0 && !slices.Contains(actions, run.Name) {
			continue
		}
		if run.Build.CoverProfile != "" {
			coverProfiles = append(coverProfiles, run.Build.CoverProfile)
		}
	}
	if len(coverProfiles) > 0 && rt.spec.Coverage.MergeProfiles {
		mergedProfileName := rt.spec.Coverage.MergedProfileName
		f, err := os.Create(mergedProfileName)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := testutil.MergeCoverProfiles(coverProfiles, f,
			testutil.WithExcludePatterns(rt.spec.Coverage.ExcludePatterns...),
		); err != nil {
			return err
		}
	}

	return
}

func findTestPackages() ([]string, error) {
	out, err := sh.Output(mg.GoCmd(), "list", "-f",
		`{{if (or .TestGoFiles .XTestGoFiles) }} {{.Dir}} {{end}}`,
		"./...",
	)
	if err != nil {
		return nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	dirs := strings.Split(out, "\n")
	for i, dir := range dirs {
		dirs[i] = strings.TrimSpace(strings.Replace(dir, cwd, ".", 1))
	}

	return dirs, nil
}
