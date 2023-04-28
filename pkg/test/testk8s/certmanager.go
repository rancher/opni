package testk8s

import (
	"strings"

	"github.com/rancher/opni/pkg/test/testdata"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

func GetCertManagerCRDs(scheme *runtime.Scheme) []*apiextensionsv1.CustomResourceDefinition {
	yamlData := testdata.TestData("crds/cert-manager.crds.yaml")
	documents := strings.Split(string(yamlData), "\n---\n")
	var crds []*apiextensionsv1.CustomResourceDefinition
	uds := serializer.NewCodecFactory(scheme).UniversalDeserializer()
	for _, document := range documents {
		crd := &apiextensionsv1.CustomResourceDefinition{}
		_, _, err := uds.Decode(append([]byte("---\n"), document...), nil, crd)
		if err != nil {
			panic(err)
		}
		if crd.TypeMeta == (metav1.TypeMeta{}) {
			continue
		}
		crds = append(crds, crd)
	}
	return crds
}
