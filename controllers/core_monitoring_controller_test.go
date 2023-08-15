package controllers_test

import (
	"context"
	"fmt"
	"os"
	"time"

	grafanav1alpha1 "github.com/grafana-operator/grafana-operator/v4/api/integreatly/v1alpha1"
	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/internal/cortex/config/storage"
	"github.com/rancher/opni/internal/cortex/config/validation"
	cfgv1beta1 "github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/noauth"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util/flagutil"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/durationpb"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Monitoring Controller", Ordered, Label("controller", "slow"), func() {
	gateway := &types.NamespacedName{}
	testImage := "monitoring-controller-test:latest"
	volumeMounts := []any{"data", "config", "runtime-config", "client-certs", "server-certs", "etcd-client-certs", "etcd-server-cacert"}
	BeforeAll(func() {
		os.Setenv("OPNI_DEBUG_MANAGER_IMAGE", testImage)
		DeferCleanup(os.Unsetenv, "OPNI_DEBUG_MANAGER_IMAGE")
	})

	BeforeEach(func() {
		ns := makeTestNamespace()
		nc := &corev1beta1.NatsCluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      "test",
			},
			Spec: corev1beta1.NatsSpec{
				AuthMethod: corev1beta1.NatsAuthNkey,
				JetStream: corev1beta1.JetStreamSpec{
					Enabled: lo.ToPtr(true),
				},
			},
		}
		gw := &corev1beta1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: ns,
			},
			Spec: corev1beta1.GatewaySpec{
				Auth: corev1beta1.AuthSpec{
					Provider: cfgv1beta1.AuthProviderNoAuth,
					Noauth:   &noauth.ServerConfig{},
				},
				StorageType: cfgv1beta1.StorageTypeEtcd,
				NatsRef: corev1.LocalObjectReference{
					Name: "test",
				},
			},
		}
		Expect(k8sClient.Create(context.Background(), nc)).To(Succeed())
		Expect(k8sClient.Create(context.Background(), gw)).To(Succeed())

		Eventually(Object(gw)).Should(Exist())
		*gateway = types.NamespacedName{
			Namespace: gw.Namespace,
			Name:      gw.Name,
		}
	})

	Context("API Upgrades", func() {
		newmcv0 := func() *unstructured.Unstructured {
			return &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "core.opni.io/v1beta1",
					"kind":       "MonitoringCluster",
					"metadata": map[string]any{
						"name":      "samplev0",
						"namespace": gateway.Namespace,
					},
					"spec": map[string]any{
						"cortex": map[string]any{
							"deploymentMode": "HighlyAvailable",
							"enabled":        false,
							"storage": map[string]any{
								"backend": "s3",
								"retentionPeriod": map[string]any{
									"seconds": 2592000,
								},
								"s3": map[string]any{
									"accessKeyID": "test-id",
									"bucketName":  "test-bucket",
									"endpoint":    "test-endpoint",
									"http": map[string]any{
										"expectContinueTimeout": map[string]any{
											"seconds": 10,
										},
										"idleConnTimeout": map[string]any{
											"seconds": 90,
										},
										"maxConnsPerHost": 100,
										"maxIdleConns":    100,
										"responseHeaderTimeout": map[string]any{
											"seconds": 120,
										},
										"tlsHandshakeTimeout": map[string]any{
											"seconds": 10,
										},
									},
									"region":           "test-region",
									"secretAccessKey":  "test-secret",
									"signatureVersion": "v4",
									"sse": map[string]any{
										"type": "SSE-S3",
									},
									"insecure":         false,
									"bucketLookupType": "auto",
								},
								"gcs": map[string]any{
									"bucketName": "test-bucket",
								},
								"azure": map[string]any{
									"containerName": "test-container",
								},
								"swift": map[string]any{
									"containerName": "test-container",
								},
								"filesystem": map[string]any{
									"dir": "/dev/null",
								},
							},
							"workloads": map[string]any{
								"distributor": map[string]any{
									"replicas":            6,
									"extraArgs":           []string{"--foo", "--bar"},
									"no-equivalent-in-v1": "foo",
								},
							},
						},
						"gateway": map[string]any{
							"name": gateway.Name,
						},
						"grafana": map[string]any{
							"config": map[string]any{
								"auth.generic_oauth": map[string]any{
									"enabled": true,
								},
							},
							"enabled":  true,
							"hostname": "x",
						},
					},
				},
			}
		}
		newmcv1 := func() (*cortexops.CortexApplicationConfig, *cortexops.CortexWorkloadsConfig) {
			return &cortexops.CortexApplicationConfig{
					Limits: &validation.Limits{
						CompactorBlocksRetentionPeriod: durationpb.New(2592000 * time.Second),
					},
					Storage: &storage.Config{
						Backend: lo.ToPtr("s3"),
						S3: &storage.S3Config{
							AccessKeyId:      lo.ToPtr("test-id"),
							BucketName:       lo.ToPtr("test-bucket"),
							Endpoint:         lo.ToPtr("test-endpoint"),
							Region:           lo.ToPtr("test-region"),
							SecretAccessKey:  lo.ToPtr("test-secret"),
							SignatureVersion: lo.ToPtr("v4"),
							Http: &storage.HttpConfig{
								ExpectContinueTimeout: durationpb.New(10 * time.Second),
								IdleConnTimeout:       durationpb.New(90 * time.Second),
								MaxConnectionsPerHost: lo.ToPtr(int32(100)),
								MaxIdleConnections:    lo.ToPtr(int32(100)),
								ResponseHeaderTimeout: durationpb.New(120 * time.Second),
								TlsHandshakeTimeout:   durationpb.New(10 * time.Second),
							},
							Sse: &storage.S3SSEConfig{
								Type: lo.ToPtr("SSE-S3"),
							},
							Insecure:         lo.ToPtr(false),
							BucketLookupType: lo.ToPtr("auto"),
						},
						Gcs: &storage.GcsConfig{
							BucketName: lo.ToPtr("test-bucket"),
						},
						Azure: &storage.AzureConfig{
							ContainerName: lo.ToPtr("test-container"),
						},
						Swift: &storage.SwiftConfig{
							ContainerName: lo.ToPtr("test-container"),
						},
						Filesystem: &storage.FilesystemConfig{
							Dir: lo.ToPtr("/dev/null"),
						},
					},
				}, &cortexops.CortexWorkloadsConfig{
					Targets: map[string]*cortexops.CortexWorkloadSpec{
						"distributor": {
							Replicas:  lo.ToPtr[int32](6),
							ExtraArgs: []string{"--foo", "--bar"},
						},
						"query-frontend": {Replicas: lo.ToPtr[int32](1)},
						"purger":         {Replicas: lo.ToPtr[int32](1)},
						"ruler":          {Replicas: lo.ToPtr[int32](3)},
						"compactor":      {Replicas: lo.ToPtr[int32](3)},
						"store-gateway":  {Replicas: lo.ToPtr[int32](3)},
						"ingester":       {Replicas: lo.ToPtr[int32](3)},
						"alertmanager":   {Replicas: lo.ToPtr[int32](3)},
						"querier":        {Replicas: lo.ToPtr[int32](3)},
					},
				}
		}

		It("should upgrade the monitoring cluster from revision 0 to 1", func() {
			mcv0 := newmcv0()
			mcv1config, mcv1workloads := newmcv1()
			Expect(k8sClient.Create(context.Background(), mcv0)).To(Succeed())
			target := &corev1beta1.MonitoringCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "samplev0",
					Namespace: gateway.Namespace,
				},
			}
			Eventually(Object(target)).Should(WithTransform(corev1beta1.GetMonitoringClusterRevision, BeEquivalentTo(1)))
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(target), target)).To(Succeed())

			Expect(*target.Spec.Cortex.Enabled).To(BeFalse())
			Expect(target.Spec.Cortex.CortexConfig).To(testutil.ProtoEqual(mcv1config))
			Expect(target.Spec.Cortex.CortexWorkloads).To(testutil.ProtoEqual(mcv1workloads))
			Expect(target.Spec.Grafana.GrafanaConfig).To(testutil.ProtoEqual(&cortexops.GrafanaConfig{
				Enabled:  lo.ToPtr(true),
				Hostname: lo.ToPtr("x"),
			}))
			Expect(target.Spec.Grafana.GrafanaSpec).To(Equal(grafanav1alpha1.GrafanaSpec{
				Config: grafanav1alpha1.GrafanaConfig{
					AuthGenericOauth: &grafanav1alpha1.GrafanaConfigAuthGenericOauth{
						Enabled: lo.ToPtr(true),
					},
				},
			}))
			Expect(k8sClient.Delete(context.Background(), target)).To(Succeed())
			Eventually(Object(target)).ShouldNot(Exist())
		})
		When("CRDs are not updated", func() {
			It("should return an error when attempting to upgrade", func() {
				mcv0 := newmcv0()
				crd := &apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "monitoringclusters.core.opni.io",
					},
				}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(crd), crd)).To(Succeed())
				Expect(crd.Annotations).To(HaveKey(corev1beta1.InternalSchemalessAnnotation))

				By("patching the CRD to be out of date")
				delete(crd.Annotations, corev1beta1.InternalSchemalessAnnotation)
				Expect(k8sClient.Update(context.Background(), crd)).To(Succeed())

				Expect(k8sClient.Create(context.Background(), mcv0)).To(Succeed())

				target := &corev1beta1.MonitoringCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "samplev0",
						Namespace: gateway.Namespace,
					},
				}
				By("ensuring the object is not upgraded")
				Consistently(Object(target)).Should(WithTransform(corev1beta1.GetMonitoringClusterRevision, BeEquivalentTo(0)))

				By("reverting the CRD to be up to date")
				crd.Annotations[corev1beta1.InternalSchemalessAnnotation] = "true"
				Expect(k8sClient.Update(context.Background(), crd)).To(Succeed())

				By("ensuring the object is upgraded")
				Eventually(Object(target)).Should(WithTransform(corev1beta1.GetMonitoringClusterRevision, BeEquivalentTo(1)))
			})
		})
		It("should apply a missing revision annotation to an existing v1 object", func() {
			mc := &corev1beta1.MonitoringCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "samplev1",
					Namespace: gateway.Namespace,
				},
				Spec: corev1beta1.MonitoringClusterSpec{
					Gateway: corev1.LocalObjectReference{
						Name: gateway.Name,
					},
					Cortex: corev1beta1.CortexSpec{
						Enabled:      lo.ToPtr(false),
						CortexConfig: &cortexops.CortexApplicationConfig{},
						CortexWorkloads: &cortexops.CortexWorkloadsConfig{
							Targets: map[string]*cortexops.CortexWorkloadSpec{
								"all": {},
							},
						},
					},
				},
			}
			flagutil.LoadDefaults(mc.Spec.Cortex.CortexConfig)
			originalMc := mc.DeepCopy()

			Expect(k8sClient.Create(context.Background(), mc)).To(Succeed())
			Eventually(Object(mc)).Should(WithTransform(corev1beta1.GetMonitoringClusterRevision, BeEquivalentTo(1)))
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(mc), mc)).To(Succeed())

			Expect(mc.Spec.Cortex.CortexConfig).To(testutil.ProtoEqual(originalMc.Spec.Cortex.CortexConfig))
			Expect(mc.Spec.Cortex.CortexWorkloads).To(testutil.ProtoEqual(originalMc.Spec.Cortex.CortexWorkloads))
			Expect(mc.Spec.Cortex.Enabled).To(Equal(originalMc.Spec.Cortex.Enabled))
			Expect(k8sClient.Delete(context.Background(), mc)).To(Succeed())
			Eventually(Object(mc)).ShouldNot(Exist())
		})
	})

	Context("cortex configuration", func() {
		When("using the AllInOne mode", func() {
			It("should create a single workload with all cortex components", func() {
				aio := &corev1beta1.MonitoringCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cortex",
						Namespace: gateway.Namespace,
					},
					Spec: corev1beta1.MonitoringClusterSpec{
						Gateway: corev1.LocalObjectReference{
							Name: gateway.Name,
						},
						Cortex: corev1beta1.CortexSpec{
							Enabled: lo.ToPtr(true),
							CortexWorkloads: &cortexops.CortexWorkloadsConfig{
								Targets: map[string]*cortexops.CortexWorkloadSpec{
									"all": {},
								},
							},
						},
						Grafana: corev1beta1.GrafanaSpec{
							GrafanaConfig: &cortexops.GrafanaConfig{
								Enabled: lo.ToPtr(true),
								Version: lo.ToPtr("10.0.0"),
							},
						},
					},
				}
				corev1beta1.SetMonitoringClusterRevision(aio, corev1beta1.MonitoringClusterTargetRevision)
				Expect(k8sClient.Create(context.Background(), aio)).To(Succeed())
				Eventually(Object(aio)).Should(Exist())
				Eventually(Object(&appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cortex-all",
						Namespace: aio.Namespace,
					},
				})).Should(ExistAnd(
					HaveOwner(aio),
					HaveReplicaCount(1),
					HaveMatchingContainer(And(
						HaveName("all"),
						HavePorts("http-metrics", "gossip", "grpc"),
						HaveVolumeMounts(volumeMounts...),
					)),
				))

				// only 1 statefulset should be created
				Expect(List(&appsv1.StatefulSetList{}, &client.ListOptions{
					LabelSelector: labels.SelectorFromSet(labels.Set{
						"app.kubernetes.io/name": "cortex",
					}),
				})()).To(HaveLen(1))

				// no deployments should be created
				Expect(List(&appsv1.DeploymentList{}, &client.ListOptions{
					LabelSelector: labels.SelectorFromSet(labels.Set{
						"app.kubernetes.io/name": "cortex",
					}),
				})()).To(HaveLen(0))

				// grafana should be deployed
				Eventually(Object(&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "grafana",
						Namespace: aio.Namespace,
					},
				})).Should(ExistAnd(
					HaveReplicaCount(1),
					HaveMatchingContainer(And(
						HaveName("grafana"),
						HaveImage("grafana/grafana:10.0.0"),
					)),
				))
			})
		})
		When("using the HighlyAvailable mode", func() {
			It("should deploy separate workloads for cortex components", func() {
				aio := &corev1beta1.MonitoringCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cortex",
						Namespace: gateway.Namespace,
					},
					Spec: corev1beta1.MonitoringClusterSpec{
						Gateway: corev1.LocalObjectReference{
							Name: gateway.Name,
						},
						Cortex: corev1beta1.CortexSpec{
							Enabled: lo.ToPtr(true),
							CortexWorkloads: &cortexops.CortexWorkloadsConfig{
								Targets: map[string]*cortexops.CortexWorkloadSpec{
									"distributor":    {Replicas: lo.ToPtr[int32](1)},
									"query-frontend": {Replicas: lo.ToPtr[int32](1)},
									"purger":         {Replicas: lo.ToPtr[int32](1)},
									"ruler":          {Replicas: lo.ToPtr[int32](1)},
									"compactor":      {Replicas: lo.ToPtr[int32](1)},
									"store-gateway":  {Replicas: lo.ToPtr[int32](1)},
									"ingester":       {Replicas: lo.ToPtr[int32](1)},
									"alertmanager":   {Replicas: lo.ToPtr[int32](1)},
									"querier":        {Replicas: lo.ToPtr[int32](1)},
								},
							},
						},
						Grafana: corev1beta1.GrafanaSpec{
							GrafanaConfig: &cortexops.GrafanaConfig{
								Enabled: lo.ToPtr(true),
							},
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), aio)).To(Succeed())
				Eventually(Object(aio)).Should(Exist())

				statefulsets := []string{"compactor", "store-gateway", "ingester", "querier", "alertmanager"}
				deployments := []string{"distributor", "query-frontend", "purger", "ruler"}

				for _, target := range statefulsets {
					Eventually(Object(&appsv1.StatefulSet{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("cortex-%s", target),
							Namespace: aio.Namespace,
						},
					})).Should(ExistAnd(
						HaveOwner(aio),
						HaveMatchingContainer(And(
							HaveName(target),
							HaveImage(testImage),
							HaveVolumeMounts(volumeMounts...),
						)),
					))
				}
				for _, target := range deployments {
					Eventually(Object(&appsv1.Deployment{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("cortex-%s", target),
							Namespace: aio.Namespace,
						},
					})).Should(ExistAnd(
						HaveOwner(aio),
						HaveMatchingContainer(And(
							HaveName(target),
							HaveImage(testImage),
							HaveVolumeMounts(volumeMounts...),
						)),
					))
				}

				// ensure cortex-all is not created
				Consistently(Object(&appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cortex-all",
						Namespace: aio.Namespace,
					},
				})).ShouldNot(Exist())

				// grafana should be deployed
				Eventually(Object(&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "grafana",
						Namespace: aio.Namespace,
					},
				})).Should(ExistAnd(
					HaveReplicaCount(1),
					HaveMatchingContainer(And(
						HaveName("grafana"),
						HaveImage("grafana/grafana:latest"),
					)),
				))
			})
		})
	})
})
