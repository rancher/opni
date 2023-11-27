package controllers_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"slices"

	opsterv1 "github.com/Opster/opensearch-k8s-operator/opensearch-operator/api/v1"
	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/test/testk8s"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Logging OpniOpensearch Controller", Ordered, Label("controller"), func() {
	var (
		testNs string
		nats   *opnicorev1beta1.NatsCluster
		object *loggingv1beta1.OpniOpensearch
	)
	Specify("setup", func() {
		testNs = makeTestNamespace()
		certMgr.PopulateK8sObjects(context.Background(), k8sClient, testNs)
		nats = &opnicorev1beta1.NatsCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "natstest",
				Namespace: testNs,
			},
			Spec: opnicorev1beta1.NatsSpec{
				AuthMethod: opnicorev1beta1.NatsAuthNkey,
			},
		}
		Expect(k8sClient.Create(context.Background(), nats)).To(Succeed())
		Eventually(func() bool {
			err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(nats), nats)
			if err != nil {
				return false
			}
			return nats.Status.AuthSecretKeyRef != nil
		}).Should(BeTrue())

		object = &loggingv1beta1.OpniOpensearch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "opni-test",
				Namespace: testNs,
			},
			Spec: loggingv1beta1.OpniOpensearchSpec{
				ClusterConfigSpec: &loggingv1beta1.ClusterConfigSpec{
					IndexRetention: "7d",
				},
				ExternalURL:       "https://test.example.com",
				Version:           "1.0.0",
				OpensearchVersion: "1.0.0",
				ImageRepo:         "docker.io/rancher",
				OpensearchSettings: loggingv1beta1.OpensearchSettings{
					NodePools: []opsterv1.NodePool{
						{
							Component: "test",
							Replicas:  3,
							DiskSize:  "50Gi",
							Jvm:       fmt.Sprintf("-Xmx%s -Xms%s", "1G", "1G"),
							Roles: []string{
								"data",
								"master",
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
							},
							Persistence: &opsterv1.PersistenceConfig{
								PersistenceSource: opsterv1.PersistenceSource{
									PVC: &opsterv1.PVCSource{
										AccessModes: []corev1.PersistentVolumeAccessMode{
											corev1.ReadWriteOnce,
										},
									},
								},
							},
						},
					},
					Security: &opsterv1.Security{
						Tls: &opsterv1.TlsConfig{
							Transport: &opsterv1.TlsConfigTransport{
								Generate: true,
								PerNode:  true,
							},
							Http: &opsterv1.TlsConfigHttp{
								Generate: true,
							},
						},
					},
					Dashboards: opsterv1.DashboardsConfig{
						Replicas: 1,
						Enable:   true,
						Version:  "1.0.0",
						OpensearchCredentialsSecret: corev1.LocalObjectReference{
							Name: "opni-test-dashboards-auth",
						},
						Service: opsterv1.DashboardsServiceSpec{
							Type: corev1.ServiceTypeClusterIP,
						},
					},
				},
				NatsRef: &corev1.LocalObjectReference{
					Name: "natstest",
				},
			},
		}
	})

	When("creating an opni opensearch object", func() {
		It("should succeed", func() {
			Expect(k8sClient.Create(context.Background(), object)).To(Succeed())
		})
		It("should create the binding object", func() {
			Eventually(Object(&loggingv1beta1.MulticlusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      object.Name,
					Namespace: testNs,
				},
			})).Should(ExistAnd(
				HaveOwner(object),
			))
		})
		It("should create the opensearch object", func() {
			By("checking the object is created")
			Eventually(Object(&opsterv1.OpenSearchCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      object.Name,
					Namespace: testNs,
				},
			})).Should(ExistAnd(
				HaveOwner(object),
			))
			By("checking the config is correct")
			cluster := &opsterv1.OpenSearchCluster{}
			Expect(k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      object.Name,
				Namespace: testNs,
			}, cluster)).To(Succeed())
			Expect(cluster.Spec.General).To(Equal(
				opsterv1.GeneralConfig{
					ImageSpec: &opsterv1.ImageSpec{
						Image: lo.ToPtr(fmt.Sprintf(
							"%s/opensearch:v%s-%s",
							object.Spec.ImageRepo,
							object.Spec.Version,
							object.Spec.OpensearchVersion,
						)),
						ImagePullPolicy: lo.ToPtr(corev1.PullAlways),
					},
					Version:          object.Spec.Version,
					ServiceName:      fmt.Sprintf("%s-opensearch-svc", object.Name),
					HttpPort:         9200,
					SetVMMaxMapCount: true,
					AdditionalConfig: map[string]string{
						"plugins.security.ssl.http.clientauth_mode": "OPTIONAL",
					},
					AdditionalVolumes: []opsterv1.AdditionalVolume{
						{
							Name: "nkey",
							Path: "/etc/nkey",
							Secret: &corev1.SecretVolumeSource{
								SecretName: "natstest-nats-client",
							},
						},
						{
							Name: "pluginsettings",
							Path: "/usr/share/opensearch/config/preprocessing",
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: fmt.Sprintf("%s-nats-connection", object.Name),
								},
							},
						},
					},
				},
			))
			Expect(cluster.Spec.Dashboards).To(Equal(object.Spec.Dashboards))
			Expect(cluster.Spec.NodePools).To(Equal(object.Spec.NodePools))

			By("checking the auth configuration is added")
			security := object.Spec.OpensearchSettings.Security.DeepCopy()
			security.Config = &opsterv1.SecurityConfig{
				SecurityconfigSecret: corev1.LocalObjectReference{
					Name: fmt.Sprintf("%s-securityconfig", object.Name),
				},
				AdminCredentialsSecret: corev1.LocalObjectReference{
					Name: fmt.Sprintf("%s-internal-auth", object.Name),
				},
			}
			security.Tls.Transport.TlsCertificateConfig = opsterv1.TlsCertificateConfig{
				CaSecret: corev1.LocalObjectReference{
					Name: testk8s.MockCAName,
				},
			}
			security.Tls.Http.TlsCertificateConfig = opsterv1.TlsCertificateConfig{
				CaSecret: corev1.LocalObjectReference{
					Name: testk8s.MockCAName,
				},
			}
			Expect(cluster.Spec.Security).To(Equal(security))
		})
		It("should create a nats configmap", func() {
			Eventually(Object(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-nats-connection", object.Name),
					Namespace: testNs,
				},
			})).Should(ExistAnd(
				HaveOwner(object),
			))
		})
		It("should create a securityconfig secret", func() {
			Eventually(Object(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-securityconfig", object.Name),
					Namespace: testNs,
				},
			})).Should(ExistAnd(
				HaveOwner(object),
				HaveData("internal_users.yml", func(d string) bool {
					return strings.Contains(d, "internalopni:")
				}),
			))
		})
		It("should create an auth secret", func() {
			Eventually(Object(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-internal-auth", object.Name),
					Namespace: testNs,
				},
			})).Should(ExistAnd(
				HaveOwner(object),
				HaveData("username", "internalopni", "password", nil),
			))
		})
	})
	When("overriding the image", func() {
		It("should update successfully", func() {
			updateObject(object, func(c *loggingv1beta1.OpniOpensearch) {
				c.Spec.OpensearchSettings.ImageOverride = lo.ToPtr("example.com/override:latest")
			})
		})
		It("should update opensearch", func() {
			Eventually(func() bool {
				cluster := &opsterv1.OpenSearchCluster{}
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      object.Name,
					Namespace: testNs,
				}, cluster)
				if err != nil {
					return false
				}
				if cluster.Spec.General.Image == nil {
					return false
				}
				return *cluster.Spec.General.Image == "example.com/override:latest"
			}).Should(BeTrue())
		})
	})
	When("adding s3 settings", func() {
		It("should update successfully", func() {
			Expect(k8sClient.Create(context.Background(), &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: testNs,
				},
				StringData: map[string]string{
					"accessKey": "testaccesskey",
					"secretKey": "testsecretkey",
				},
			})).To(Succeed())
			updateObject(object, func(c *loggingv1beta1.OpniOpensearch) {
				c.Spec.OpensearchSettings.S3Settings = &loggingv1beta1.OpensearchS3Settings{
					Endpoint: "s3.example.com",
					Protocol: loggingv1beta1.OpensearchS3ProtocolHTTPS,
					CredentialSecret: corev1.LocalObjectReference{
						Name: "test-secret",
					},
					Repository: loggingv1beta1.S3PathSettings{
						Bucket: "test-bucket",
						Folder: "backups",
					},
				}
			})
		})
		It("should add the s3 settings to opensearch", func() {
			Eventually(func() bool {
				cluster := &opsterv1.OpenSearchCluster{}
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      object.Name,
					Namespace: testNs,
				}, cluster)
				if err != nil {
					return false
				}
				if !slices.ContainsFunc(cluster.Spec.General.Keystore, func(e opsterv1.KeystoreValue) bool {
					return reflect.DeepEqual(e, opsterv1.KeystoreValue{
						Secret: corev1.LocalObjectReference{
							Name: "test-secret",
						},
						KeyMappings: map[string]string{
							"accessKey": "s3.client.default.access_key",
							"secretKey": "s3.client.default.secret_key",
						},
					})
				}) {
					return false
				}
				endpoint, ok := cluster.Spec.General.AdditionalConfig["s3.client.default.endpoint"]
				if !ok || endpoint != "s3.example.com" {
					return false
				}
				protocol, ok := cluster.Spec.General.AdditionalConfig["s3.client.default.protocol"]
				if !ok || protocol != "https" {
					return false
				}
				return true
			}).Should(BeTrue())
		})
		It("should create the s3 repository", func() {
			Eventually(func() bool {
				repo := &loggingv1beta1.OpensearchRepository{}
				err := k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      object.Name,
					Namespace: testNs,
				}, repo)
				if err != nil {
					return false
				}

				if repo.Spec.Settings.S3 == nil {
					return false
				}
				if repo.Spec.Settings.S3.Bucket != "test-bucket" {
					return false
				}
				if repo.Spec.Settings.S3.Folder != "backups" {
					return false
				}
				return true
			}).Should(BeTrue())
		})
	})
})
