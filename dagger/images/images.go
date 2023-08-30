package images

import (
	"runtime"
	"strings"

	"dagger.io/dagger"
)

func Base(client *dagger.Client) *dagger.Container {
	return client.
		Container().
		Pipeline("Go Base Image").
		From("cimg/go:"+strings.TrimPrefix(runtime.Version(), "go")+"-node").
		WithUser("root").
		WithEnvVariable("GOPATH", "/go").
		WithEnvVariable("PATH", "/go/bin:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin").
		WithExec([]string{"apt-get", "update"}).
		WithExec([]string{"apt-get", "install", "-y", "libnspr4", "libnss3", "libexpat1", "libfontconfig1", "libuuid1"}).
		WithDirectory("/go", client.Directory(), dagger.ContainerWithDirectoryOpts{Owner: "1777"}).
		WithDirectory("/go/src", client.Directory(), dagger.ContainerWithDirectoryOpts{Owner: "1777"}).
		WithDirectory("/go/bin", client.Directory(), dagger.ContainerWithDirectoryOpts{Owner: "1777"})
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
