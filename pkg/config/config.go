package config

import (
	"bytes"
	"errors"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/kralicky/opni-gateway/pkg/config/meta"
	"github.com/kralicky/opni-gateway/pkg/config/v1beta1"
	"sigs.k8s.io/yaml"
)

var (
	ErrConfigNotFound        = errors.New("config not found")
	ErrUnsupportedApiVersion = errors.New("unsupported api version")
)

type Unmarshaler interface {
	Unmarshal(into interface{}) error
}

type GatewayConfig = v1beta1.GatewayConfig

func LoadObjectsFromFile(path string) (meta.ObjectList, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	objects := []meta.Object{}
	documents := bytes.Split(data, []byte("\n---\n"))
	for _, document := range documents {
		if len(strings.TrimSpace(string(document))) == 0 {
			continue
		}
		typeMeta := meta.TypeMeta{}
		if err := yaml.Unmarshal(document, &typeMeta); err != nil {
			log.Println(err)
			continue
		}
		if typeMeta.APIVersion == "" || typeMeta.Kind == "" {
			continue
		}
		object, err := decodeObject(typeMeta, document)
		if err != nil {
			log.Println(err)
			continue
		}
		objects = append(objects, object)
	}
	return objects, nil
}

func decodeObject(typeMeta meta.TypeMeta, document []byte) (meta.Object, error) {
	switch typeMeta.APIVersion {
	case v1beta1.APIVersion:
		return v1beta1.DecodeObject(typeMeta.Kind, document)
	}
	return nil, ErrUnsupportedApiVersion
}

func FindConfig() (string, error) {
	pathsToSearch := []string{
		".",
		"/etc/opni-gateway",
	}
	filenamesToSearch := []string{
		"gateway.yaml",
		"gateway.yml",
		"gateway.json",
		"proxy.yaml",
		"proxy.yml",
		"proxy.json",
		"config.yaml",
		"config.yml",
		"config.json",
	}

	for _, path := range pathsToSearch {
		for _, filename := range filenamesToSearch {
			p, err := filepath.Abs(filepath.Join(path, filename))
			if err != nil {
				return "", err
			}
			if f, err := os.Open(p); err == nil {
				f.Close()
				return p, nil
			}
		}
	}

	return "", ErrConfigNotFound
}
