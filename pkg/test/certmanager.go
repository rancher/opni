package test

import (
	"io"
	"net/http"
	"strings"

	"github.com/rancher/opni/pkg/util"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

var downloadAddr = "https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.crds.yaml"

func downloadCertManagerCRDs(scheme *runtime.Scheme) []*apiextensionsv1.CustomResourceDefinition {
	resp, err := http.Get(downloadAddr)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	yamlData := util.Must(io.ReadAll(resp.Body))
	documents := strings.Split(string(yamlData), "\n---\n")
	var crds []*apiextensionsv1.CustomResourceDefinition
	uds := serializer.NewCodecFactory(scheme).UniversalDeserializer()
	for _, document := range documents {
		crd := &apiextensionsv1.CustomResourceDefinition{}
		_, _, err := uds.Decode([]byte(document), nil, crd)
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
