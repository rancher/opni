package e2e

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	demoCrName      = "test-opnidemo"
	demoCrNamespace = "opnidemo-test"
)

var _ = Describe("OpniDemo E2E", func() {
	When("creating an opnidemo", func() {
		It("should succeed", func() {
			demo := v1alpha1.OpniDemo{
				ObjectMeta: metav1.ObjectMeta{
					Name:      demoCrName,
					Namespace: demoCrNamespace,
				},
				Spec: v1alpha1.OpniDemoSpec{
					Components: v1alpha1.ComponentsSpec{
						Infra: v1alpha1.InfraStack{
							HelmController:       false,
							LocalPathProvisioner: false,
						},
						Opni: v1alpha1.OpniStack{
							Minio:          true,
							Nats:           true,
							Elastic:        true,
							RancherLogging: true,
							Traefik:        false,
						},
					},
					MinioAccessKey:         "testAccessKey",
					MinioSecretKey:         "testSecretKey",
					MinioVersion:           "8.0.10",
					NatsVersion:            "2.2.1",
					NatsPassword:           "password",
					NatsReplicas:           1,
					NatsMaxPayload:         10485760,
					NvidiaVersion:          "1.0.0-beta6",
					ElasticsearchUser:      "admin",
					ElasticsearchPassword:  "admin",
					TraefikVersion:         "v9.18.3",
					NulogServiceCpuRequest: "1",
					Quickstart:             true,
				},
			}
			Expect(k8sClient.Create(context.Background(), &demo)).To(Succeed())
		})
		It("should become ready", func() {
			demo := v1alpha1.OpniDemo{}
			i := 0
			Eventually(func() error {
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      demoCrName,
					Namespace: demoCrNamespace,
				}, &demo)
				if err != nil {
					return err
				}
				if demo.Status.State != "Ready" {
					conditions := strings.Join(demo.Status.Conditions, "; ")
					i++
					if i%4 == 0 {
						fmt.Println(conditions)
					}
					return errors.New(conditions)
				}
				return nil
			}, 10*time.Minute, 500*time.Millisecond).Should(BeNil())
		})
	})
	Context("verifying logs are being shipped to elasticsearch", func() {

	})
})
