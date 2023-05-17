package oci

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

type ImageType string

const (
	ImageTypeOpni    ImageType = "opni"
	ImageTypeMinimal ImageType = "minimal"
	ImageTypePlugins ImageType = "plugins"
)

type Fetcher interface {
	GetImage(context.Context, ImageType) (Image, error)
}

type Image struct {
	Registry   string
	Repository string
	Digest     string
}

func Parse(image string) Image {
	var registry, repo, digest string
	splitImage := strings.Split(image, "/")
	if len(splitImage) > 1 {
		if strings.Contains(splitImage[0], ".") {
			registry = splitImage[0]
			image = strings.TrimPrefix(image, fmt.Sprintf("%s/", registry))
		}
	}
	if strings.Contains(image, "@") {
		splitImage := strings.Split(image, "@")
		repo = splitImage[0]
		digest = splitImage[1]
	} else {
		splitImage := strings.Split(image, ":")
		repo = splitImage[0]
		if len(splitImage) > 1 {
			digest = splitImage[1]
		}
	}

	return Image{
		Registry:   registry,
		Repository: repo,
		Digest:     digest,
	}
}

func (i Image) String() string {
	var prefix, suffix string
	if i.Registry != "" {
		prefix = fmt.Sprintf("%s/", i.Registry)
	}
	switch {
	case strings.HasPrefix(i.Digest, "sha256:"):
		suffix = fmt.Sprintf("@%s", i.Digest)
	case i.Digest != "":
		suffix = fmt.Sprintf(":%s", i.Digest)
	}

	return fmt.Sprintf("%s%s%s", prefix, i.Repository, suffix)
}

func (i Image) Path() string {
	i.Digest = ""
	return i.String()
}

var (
	fetcherBuilderCache = map[string]func(...any) (Fetcher, error){}
	fetcherCacheMutex   = sync.Mutex{}
)

func RegisterFetcherBuilder[T ~string](name T, builder func(...any) (Fetcher, error)) {
	fetcherCacheMutex.Lock()
	defer fetcherCacheMutex.Unlock()
	fetcherBuilderCache[string(name)] = builder
}

func GetFetcherBuilder[T ~string](name T) func(...any) (Fetcher, error) {
	fetcherCacheMutex.Lock()
	defer fetcherCacheMutex.Unlock()
	return fetcherBuilderCache[string(name)]
}
