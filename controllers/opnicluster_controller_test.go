package controllers

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rancher/opni/apis/v1beta1"
)

var _ = Describe("OpniCluster Controller", func() {
	When("creating an opnicluster ", func() {
		cluster := v1beta1.OpniCluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1beta1.GroupVersion.String(),
				Kind:       "OpniCluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      crName,
				Namespace: crNamespace,
			},
			Spec: v1beta1.OpniClusterSpec{},
		}
	})
})
