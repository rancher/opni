package ginkgo

import "strings"

#Run: {
	name?:    string
	packages: string
	skip?: [...string]
	labelFilter?: string
	cover: {
		enabled:   bool | *false
		covermode: string | *"atomic"
		coverpkg?: [...string]
		coverprofile: string | *"cover.out"
		if name != _|_ {
			coverprofile: "cover-\(name).out"
		}
	}
	race:            bool | *true
	randomizeSuites: bool | *true
	keepGoing:       bool | *true
	trace:           bool | *true
	timeout:         string | *"10m"

	command: {
		name: "go"
		args: [
			"run",
			"github.com/onsi/ginkgo/v2/ginkgo",
			if randomizeSuites {"--randomize-suites"},
			if keepGoing {"--keep-going"},
			if race {"--race"},
			if trace {"--trace"},
			if skip != _|_ {"--skip=\(skip)"},
			if labelFilter != _|_ {"--label-filter=\(labelFilter)"},
			"--timeout=\(timeout)",
			if cover.enabled {"--cover"},
			if cover.enabled {"--covermode=\(cover.covermode)"},
			if cover.enabled {"--coverprofile=\(cover.coverprofile)"},
			if cover.enabled && cover.coverpkg != _|_ {"--coverpkg=\(strings.Join(cover.coverpkg, ","))"},
			packages,
		]
	}
}
