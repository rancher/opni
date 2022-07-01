package test

import (
	"errors"
	"fmt"
	"os"
	"regexp"
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
	"golang.org/x/exp/slices"
)

const (
	ginkgoPkg = "github.com/onsi/ginkgo/v2/ginkgo"
)

func SysInfo() {
	cpuInfo, err := ghw.CPU()
	if err != nil {
		return
	}
	topologyInfo, err := ghw.Topology()
	if err != nil {
		return
	}
	memInfo, err := ghw.Memory()
	if err != nil {
		return
	}
	fmt.Println("System Info:")
	for _, proc := range cpuInfo.Processors {
		fmt.Printf(" %v (%d cores, %d threads)\n", proc.Model, proc.NumCores, proc.NumThreads)
	}
	fmt.Printf(" %v\n", topologyInfo)
	fmt.Printf(" %v\n", memInfo)
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
	Explicit bool

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
	testPackages []testPkg
}

type testPkg struct {
	dir    string
	labels []string
}

func NewTestPlanRuntime() (*TestPlanRuntime, error) {
	pkgs, err := findTestPackages()
	if err != nil {
		return nil, err
	}

	allPkgs := make([]string, len(pkgs))
	for i, pkg := range pkgs {
		allPkgs[i] = pkg.dir
	}
	pkgsByLabel := map[string][]string{}
	for _, pkg := range pkgs {
		for _, label := range pkg.labels {
			pkgsByLabel[label] = append(pkgsByLabel[label], pkg.dir)
		}
	}

	tags := []string{
		"test",
		"packages=" + strings.Join(allPkgs, ","),
	}
	for label, pkgNames := range pkgsByLabel {
		tags = append(tags, fmt.Sprintf("packages_%s=%s", label, strings.Join(pkgNames, ",")))
	}
	instances := load.Instances([]string{}, &load.Config{
		Tags: tags,
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

	completedActions := []RunSpec{}

	for _, run := range rt.spec.Actions {
		if len(actions) > 0 {
			if !slices.Contains(actions, run.Name) {
				continue
			}
		} else if run.Explicit {
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
				} else {
					completedActions = append(completedActions, run)
				}
			}()
		} else {
			err := sh.RunV(name, args...)
			if err != nil {
				once.Do(func() {
					testErr = err
				})
			} else {
				completedActions = append(completedActions, run)
			}
		}
	}
	wg.Wait()

	if testErr != nil {
		return
	}

	// merge coverage profiles
	coverProfiles := []string{}
	for _, run := range completedActions {
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

func findTestPackages() ([]testPkg, error) {
	pkgsOut, err := sh.Output(mg.GoCmd(), "list", "-f",
		`{{if (or .TestGoFiles .XTestGoFiles) }} {{.Dir}} {{end}}`,
		"./...",
	)
	if err != nil {
		return nil, err
	}
	labelsOut, err := sh.Output(mg.GoCmd(), append([]string{"run", ginkgoPkg, "labels"}, strings.Fields(pkgsOut)...)...)
	if err != nil {
		return nil, err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	dirs := strings.Fields(pkgsOut)
	pkgs := make([]testPkg, 0, len(dirs))
	for _, dir := range dirs {
		pkgs = append(pkgs, testPkg{
			dir: strings.TrimSpace(strings.Replace(dir, cwd, ".", 1)),
		})
	}
	labels := strings.Split(labelsOut, "\n")
	if len(labels) != len(pkgs) {
		fmt.Println("labels:", labels)
		fmt.Println("pkgs:", pkgs)
		panic(fmt.Sprintf("labels and pkgs have different lengths (%d != %d)", len(labels), len(pkgs)))
	}
	regex := regexp.MustCompile(`"(.*?)"`)
	for i, label := range labels {
		// Parse the quoted labels from the format `name: ["label1", "label2", ...]`
		// If no labels are found, the output is `name: No labels found`, which
		// contains no quoted strings.
		results := regex.FindAllStringSubmatch(label, -1)
		labels := []string{}
		for _, result := range results {
			labels = append(labels, result[1])
		}
		pkgs[i].labels = labels
	}
	return pkgs, nil
}
