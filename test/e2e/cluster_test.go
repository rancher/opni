//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/apis/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
)

const (
	clusterCrName      = "test-opnicluster"
	clusterCrNamespace = "opnicluster-test"
)

var _ = Describe("OpniCluster E2E Test", func() {
	When("creating a pretrained model", func() {
		It("should succeed", func() {
			pretrained := v1beta1.PretrainedModel{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterCrName,
					Namespace: clusterCrNamespace,
				},
				Spec: v1beta1.PretrainedModelSpec{
					ModelSource: v1beta1.ModelSource{
						HTTP: &v1beta1.HTTPSource{
							URL: "https://opni-public.s3.us-east-2.amazonaws.com/pretrain-models/control-plane-model-v0.1.2.zip",
						},
					},
					Hyperparameters: map[string]intstr.IntOrString{
						"modelThreshold": intstr.FromString("0.6"),
						"minLogTokens":   intstr.FromInt(4),
						"isControlPlane": intstr.FromString("true"),
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), &pretrained)).To(Succeed())
		})
	})
	When("creating a logadapter", func() {
		It("should succeed", func() {
			logadapter := v1beta1.LogAdapter{
				ObjectMeta: metav1.ObjectMeta{
					Name: clusterCrName,
				},
				Spec: v1beta1.LogAdapterSpec{
					Provider: v1beta1.LogProviderK3S,
					K3S: &v1beta1.K3SSpec{
						ContainerEngine: v1beta1.ContainerEngineOpenRC,
					},
					OpniCluster: v1beta1.OpniClusterNameSpec{
						Name:      clusterCrName,
						Namespace: clusterCrNamespace,
					},
				},
			}
			logadapter.Default()
			Expect(k8sClient.Create(context.Background(), &logadapter)).To(Succeed())
		})
	})
	When("creating an opnicluster", func() {
		It("should succeed", func() {
			opnicluster := v1beta1.OpniCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterCrName,
					Namespace: clusterCrNamespace,
				},
				Spec: v1beta1.OpniClusterSpec{
					Version:            "v0.1.3",
					DeployLogCollector: pointer.BoolPtr(true),
					Services: v1beta1.ServicesSpec{
						GPUController: v1beta1.GPUControllerServiceSpec{
							Enabled: pointer.BoolPtr(false),
						},
					},
					Elastic: v1beta1.ElasticSpec{
						Version: "1.13.2",
					},
					S3: v1beta1.S3Spec{
						Internal: &v1beta1.InternalSpec{},
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), &opnicluster)).To(Succeed())
		})
		It("should become ready", func() {
			opnicluster := v1beta1.OpniCluster{}
			i := 0
			Eventually(func() error {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      clusterCrName,
					Namespace: clusterCrNamespace,
				}, &opnicluster)
				if err != nil {
					return err
				}
				if opnicluster.Status.State == "" {
					return errors.New("State not populated yet")
				}
				if opnicluster.Status.State != "Ready" {
					conditions := strings.Join(opnicluster.Status.Conditions, "; ")
					i++
					if i%4 == 0 {
						fmt.Println(conditions)
					}
					return errors.New(conditions)
				}
				if opnicluster.Status.IndexState != "Ready" {
					conditions := strings.Join(opnicluster.Status.Conditions, "; ")
					i++
					if i%4 == 0 {
						fmt.Println(conditions)
					}
					return errors.New(conditions)
				}
				if opnicluster.Status.LogCollectorState != "Ready" {
					conditions := strings.Join(opnicluster.Status.Conditions, "; ")
					i++
					if i%4 == 0 {
						fmt.Println(conditions)
					}
					return errors.New(conditions)
				}
				Expect(opnicluster.Status.Conditions).To(BeEmpty(),
					"Expected no conditions if state is Ready")
				return nil
			}, 10*time.Minute, 500*time.Millisecond).Should(BeNil())
		})
	})
})
