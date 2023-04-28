package ginkgo

import (
	"github.com/onsi/ginkgo/v2/types"
	"pkg.go.dev/time"
)

#Run: #RunConfig & {
	Name:     string
	Packages: string
	Explicit: bool | *false

	Suite: {
		RandomSeed:            int64 | *0
		RandomizeAllSpecs:     bool | *false
		LabelFilter:           string | *""
		FailOnPending:         bool | *false
		FailFast:              bool | *false
		FlakeAttempts:         int | *0
		DryRun:                bool | *false
		PollProgressAfter:     time.#Duration | *(time.#Second * 30)
		PollProgressInterval:  time.#Duration | *(time.#Second * 10)
		Timeout:               time.#Duration | *(time.#Minute * 30)
		EmitSpecProgress:      bool | *false
		OutputInterceptorMode: string | *""
		SourceRoots:           [...string] | *[]
		GracePeriod:           time.#Duration | *(time.#Second * 30)
		ParallelProcess:       int | *0
		ParallelTotal:         int | *0
		ParallelHost:          string | *""
	}
	Build: {
		Race:         bool | *true
		Cover:        bool | *true
		CoverMode:    string
		CoverPkg:     string
		CoverProfile: string
		*{
			Cover:        true
			CoverMode:    string | *"atomic"
			CoverPkg:     string | *""
			CoverProfile: string | *"cover-\(Name).out"
		} | {
			Cover:        false
			CoverMode:    ""
			CoverPkg:     ""
			CoverProfile: ""
		}
		Vet:                  string | *""
		BlockProfile:         string | *""
		BlockProfileRate:     int | *0
		CPUProfile:           string | *""
		MemProfile:           string | *""
		MemProfileRate:       int | *0
		MutexProfile:         string | *""
		MutexProfileFraction: int | *0
		Trace:                string | *""
		A:                    bool | *false
		ASMFlags:             string | *""
		BuildMode:            string | *""
		Compiler:             string | *""
		GCCGoFlags:           string | *""
		GCFlags:              string | *""
		InstallSuffix:        string | *""
		LDFlags:              string | *""
		LinkShared:           bool | *false
		Mod:                  string | *""
		N:                    bool | *false
		ModFile:              string | *""
		ModCacheRW:           bool | *false
		MSan:                 bool | *false
		PkgDir:               string | *""
		Tags:                 string | *""
		TrimPath:             bool | *false
		ToolExec:             string | *""
		Work:                 bool | *false
		X:                    bool | *false
	}
	Run: {
		Recurse:                   bool | *false
		SkipPackage:               string | *""
		RequireSuite:              bool | *true
		NumCompilers:              int | *0
		Procs:                     int | *0
		Parallel:                  bool | *false
		AfterRunHook:              string | *""
		OutputDir:                 string | *""
		KeepSeparateCoverprofiles: bool | *false
		KeepSeparateReports:       bool | *false
		KeepGoing:                 bool | *false
		UntilItFails:              bool | *false
		Repeat:                    int | *0
		RandomizeSuites:           bool | *true
		Depth:                     int | *0
		WatchRegExp:               string | *""
	}
	Reporter: {
		NoColor:        bool | *false
		Succinct:       bool | *false
		Verbose:        bool | *false
		VeryVerbose:    bool | *false
		FullTrace:      bool | *true
		ShowNodeEvents: bool | *false
		JSONReport:     string | *""
		JUnitReport:    string | *""
		TeamcityReport: string | *""
	}
}

#RunConfig: {
	Suite:    types.#SuiteConfig
	Build:    types.#GoFlagsConfig
	Run:      types.#CLIConfig
	Reporter: types.#ReporterConfig
	...
}
