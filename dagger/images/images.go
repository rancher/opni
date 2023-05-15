package images

import (
	"runtime"
	"strings"

	"dagger.io/dagger"
)

func NodeBase(client *dagger.Client) *dagger.Container {
	return client.
		Container().
		Pipeline("Node Base Image").
		From("node:14")
}

func GoBase(client *dagger.Client) *dagger.Container {
	return client.
		Container().
		Pipeline("Go Base Image").
		From("golang:" + strings.TrimPrefix(runtime.Version(), "go"))
}

type AlpineOptions struct {
	packages []string
}

type AlpineOption func(*AlpineOptions)

func (o *AlpineOptions) apply(opts ...AlpineOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithPackages(packages ...string) AlpineOption {
	return func(o *AlpineOptions) {
		o.packages = append(o.packages, packages...)
	}
}

func AlpineBase(client *dagger.Client, opts ...AlpineOption) *dagger.Container {
	options := AlpineOptions{
		packages: []string{"ca-certificates"},
	}
	options.apply(opts...)

	return client.
		Container().
		Pipeline("Alpine Base Image").
		From("alpine:3.18").
		WithExec(append([]string{"apk", "add", "--no-cache"}, options.packages...))
}
