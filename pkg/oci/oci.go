package oci

import (
	"context"
	"fmt"
	"sync"

	"github.com/opencontainers/go-digest"
)

type ImageType string

const (
	ImageTypeOpni    ImageType = "opni"
	ImageTypeMinimal ImageType = "minimal"
	ImageTypePlugins ImageType = "plugins"
)

var (
	ErrInvalidImageFormat     = fmt.Errorf("image string is either invalid or empty")
	ErrInvalidReferenceFormat = fmt.Errorf("reference string is either invalid or empty")
)

type Fetcher interface {
	GetImage(context.Context, ImageType) (*Image, error)
}

type Image struct {
	Registry   string
	Repository string
	Tag        string
	Digest     digest.Digest
}

func Parse(s string) (*Image, error) {
	matches := ReferenceRegexp.FindStringSubmatch(s)
	if matches == nil {
		return nil, ErrInvalidImageFormat
	}

	image := &Image{}
	nameMatch := anchoredNameRegexp.FindStringSubmatch(matches[1])
	switch len(nameMatch) {
	case 3:
		image.Registry = nameMatch[1]
		image.Repository = nameMatch[2]
	case 2:
		image.Repository = nameMatch[1]
	}

	image.Tag = matches[2]

	if matches[3] != "" {
		var err error
		image.Digest, err = digest.Parse(matches[3])
		if err != nil {
			return nil, err
		}
	}

	return image, nil
}

func (i *Image) String() string {
	var prefix, suffix string
	if i.Registry != "" {
		prefix = fmt.Sprintf("%s/", i.Registry)
	}

	if i.Tag != "" {
		suffix += fmt.Sprintf(":%s", i.Tag)
	}
	if i.Digest != "" {
		suffix += fmt.Sprintf("@%s", i.Digest.String())
	}

	return fmt.Sprintf("%s%s%s", prefix, i.Repository, suffix)
}

func (i *Image) Path() string {
	var prefix string
	if i.Registry != "" {
		prefix = fmt.Sprintf("%s/", i.Registry)
	}
	return fmt.Sprintf("%s%s", prefix, i.Repository)
}

func (i *Image) Empty() bool {
	return i.Repository == ""
}

func (i *Image) UpdateDigestOrTag(ref string) error {
	match := anchoredTagRegexp.FindString(ref)
	if match != "" {
		i.Tag = match
		i.Digest = ""
		return nil
	}
	match = anchoredDigestRegexp.FindString(ref)
	if match != "" {
		i.Tag = ""
		d, err := digest.Parse(match)
		if err != nil {
			return err
		}
		i.Digest = d
		return nil
	}

	return fmt.Errorf("%w: %q", ErrInvalidReferenceFormat, ref)
}

func (i *Image) DigestOrTag() string {
	if i.Digest != "" {
		return i.Digest.String()
	}
	if i.Tag != "" {
		return i.Tag
	}
	return ""
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
