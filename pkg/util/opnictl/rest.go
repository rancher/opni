package opnictl

import (
	"bytes"
	"log"

	"github.com/rancher/opni/staging"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
)

func ForEachStagingResource(
	callback func(dynamic.ResourceInterface, *unstructured.Unstructured) error,
) (errors []string) {
	errors = []string{}

	clientConfig := LoadClientConfig()

	decodingSerializer := yaml.NewDecodingSerializer(
		unstructured.UnstructuredJSONScheme)
	decoder := yamlutil.NewYAMLOrJSONDecoder(
		bytes.NewReader([]byte(staging.StagingAutogenYaml)), 32)
	dynamicClient := dynamic.NewForConfigOrDie(clientConfig)

	dc, err := discovery.NewDiscoveryClientForConfig(clientConfig)
	if err != nil {
		log.Fatal(err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(
		memory.NewMemCacheClient(dc))

	for {
		var rawObj runtime.RawExtension
		if err = decoder.Decode(&rawObj); err != nil {
			break
		}

		obj := &unstructured.Unstructured{}
		_, gvk, err := decodingSerializer.Decode(rawObj.Raw, nil, obj)
		if err != nil {
			log.Fatal(err)
		}

		mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			errors = append(errors, err.Error())
			continue
		}

		var dr dynamic.ResourceInterface
		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			// namespaced resources should specify the namespace
			dr = dynamicClient.Resource(mapping.Resource).
				Namespace(obj.GetNamespace())
		} else {
			// for cluster-wide resources
			dr = dynamicClient.Resource(mapping.Resource)
		}

		if err := callback(dr, obj); err != nil {
			errors = append(errors, err.Error())
		}
	}
	return
}
