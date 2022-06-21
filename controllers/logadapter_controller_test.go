package controllers

import (
	"context"
	"fmt"
	"strings"

	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/logging/api/v1beta1"
	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	"github.com/rancher/opni/apis/v1beta2"
)

var _ = Describe("LogAdapter Controller", Ordered, Label("controller"), func() {
	var (
		logadapter v1beta2.LogAdapter
		cluster    v1beta2.OpniCluster
		err        error
		testNs     string
	)
	Specify("setup", func() {
		testNs = makeTestNamespace()
		cluster = v1beta2.OpniCluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1beta2.GroupVersion.String(),
				Kind:       "OpniCluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: testNs,
			},
			Spec: v1beta2.OpniClusterSpec{
				Opensearch: v1beta2.OpensearchClusterSpec{},
				Nats: v1beta2.NatsSpec{
					AuthMethod: v1beta2.NatsAuthUsername,
				},
			},
		}
		k8sClient.Create(context.Background(), &cluster)
		logadapter = v1beta2.LogAdapter{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: testNs,
			},
			Spec: v1beta2.LogAdapterSpec{
				Provider: v1beta2.LogProviderEKS,
				OpniCluster: &v1beta2.OpniClusterNameSpec{
					Name:      "doesnotexist",
					Namespace: testNs,
				},
			},
		}
		logadapter.Default()
		err = k8sClient.Create(context.Background(), &logadapter)
	})
	When("creating a logadapter", func() {
		When("the OpniCluster does not exist", func() {
			It("should succeed", func() {
				Expect(err).To(BeNil())
				Eventually(Object(&logadapter)).Should(Exist())
			})
			XIt("should not create a logging", func() {
				Consistently(Object(&loggingv1beta1.Logging{
					ObjectMeta: metav1.ObjectMeta{
						Name: fmt.Sprintf("opni-%s", logadapter.Name),
					},
				})).ShouldNot(Exist())
			})
		})
		When("the OpniCluster exists", func() {
			Specify("update opni cluster", func() {
				updateObject(&logadapter, func(l *v1beta2.LogAdapter) {
					l.Spec.OpniCluster.Name = "test"
				})
			})
			It("should succeed", func() {
				Eventually(Object(&logadapter)).Should(Exist())
			})
			It("should create a logging", func() {
				Eventually(Object(&loggingv1beta1.Logging{
					ObjectMeta: metav1.ObjectMeta{
						Name: fmt.Sprintf("opni-%s", logadapter.Name),
					},
				})).Should(ExistAnd(HaveOwner(&logadapter)))
			})
		})
		Context("with the RKE provider", func() {
			Specify("update opni cluster", func() {
				updateObject(&logadapter, func(l *v1beta2.LogAdapter) {
					l.Spec.Provider = v1beta2.LogProviderRKE
					l.Default()
				})
			})
			When("the spec is valid", func() {
				It("should create a daemonset", func() {
					Eventually(Object(&appsv1.DaemonSet{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("opni-%s-rke-aggregator", logadapter.Name),
							Namespace: logadapter.Spec.OpniCluster.Namespace,
						},
					})).Should(ExistAnd(HaveOwner(&logadapter)))
				})
			})
			When("a log level is not specified", func() {
				It("should create a configmap with default log level", func() {
					Eventually(Object(&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("opni-%s-rke", logadapter.Name),
							Namespace: logadapter.Spec.OpniCluster.Namespace,
						},
					})).Should(ExistAnd(
						HaveOwner(&logadapter),
						HaveData("fluent-bit.conf", func(d string) bool {
							return strings.Contains(d, fmt.Sprintf("Log_Level         %s", opnimeta.LogLevelInfo))
						}),
					))
				})
			})
			When("warning log level is specified", func() {
				Specify("update opni cluster", func() {
					updateObject(&logadapter, func(l *v1beta2.LogAdapter) {
						l.Spec.RKE = &v1beta2.RKESpec{
							LogLevel: opnimeta.LogLevelWarn,
						}
					})
				})
				It("should create a config map with warn log level", func() {
					Eventually(Object(&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("opni-%s-rke", logadapter.Name),
							Namespace: logadapter.Spec.OpniCluster.Namespace,
						},
					})).Should(ExistAnd(
						HaveOwner(&logadapter),
						HaveData("fluent-bit.conf", func(d string) bool {
							return strings.Contains(d, fmt.Sprintf("Log_Level         %s", opnimeta.LogLevelWarn))
						}),
					))
				})
			})
		})
		Context("with the RKE2 provider", func() {
			Specify("update opni cluster", func() {
				updateObject(&logadapter, func(l *v1beta2.LogAdapter) {
					l.Spec.Provider = v1beta2.LogProviderRKE2
					l.Default()
				})
			})
			When("a log path isn't specified", func() {
				It("should create a daemonset", func() {
					Eventually(Object(&appsv1.DaemonSet{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("opni-%s-rke2-journald-aggregator", logadapter.Name),
							Namespace: logadapter.Spec.OpniCluster.Namespace,
						},
					})).Should(ExistAnd(HaveOwner(&logadapter)))
				})
				It("should mount the default log path", func() {
					unset := corev1.HostPathUnset
					Eventually(Object(&appsv1.DaemonSet{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("opni-%s-rke2-journald-aggregator", logadapter.Name),
							Namespace: logadapter.Spec.OpniCluster.Namespace,
						},
					})).Should(ExistAnd(
						HaveOwner(&logadapter),
						HaveMatchingVolume(And(
							HaveName("journal"),
							HaveVolumeSource(corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/var/log/journal",
									Type: &unset,
								},
							}),
						)),
						HaveMatchingContainer(And(
							HaveVolumeMounts(corev1.VolumeMount{
								Name:      "journal",
								MountPath: "/var/log/journal",
								ReadOnly:  true,
							}),
						)),
					))
				})
				It("should create a configmap with the default path", func() {
					Eventually(Object(&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("opni-%s-rke2", logadapter.Name),
							Namespace: logadapter.Spec.OpniCluster.Namespace,
						},
					})).Should(ExistAnd(
						HaveOwner(&logadapter),
						HaveData("fluent-bit.conf", func(d string) bool {
							return strings.Contains(d, "Path              /var/log/journal")
						}),
					))
				})
			})
			When("a log path is specified", func() {
				BeforeEach(func() {
					logadapter.Spec.RKE2 = &v1beta2.RKE2Spec{
						LogPath: systemdLogPath,
					}
				})
				Specify("update opni cluster", func() {
					updateObject(&logadapter, func(l *v1beta2.LogAdapter) {
						l.Spec.RKE2 = &v1beta2.RKE2Spec{
							LogPath: systemdLogPath,
						}
					})
				})
				It("should mount the specified log path", func() {
					unset := corev1.HostPathUnset
					Eventually(Object(&appsv1.DaemonSet{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("opni-%s-rke2-journald-aggregator", logadapter.Name),
							Namespace: logadapter.Spec.OpniCluster.Namespace,
						},
					})).Should(ExistAnd(
						HaveOwner(&logadapter),
						HaveMatchingVolume(And(
							HaveName("journal"),
							HaveVolumeSource(corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: systemdLogPath,
									Type: &unset,
								},
							}),
						)),
						HaveMatchingContainer(And(
							HaveVolumeMounts(corev1.VolumeMount{
								Name:      "journal",
								MountPath: systemdLogPath,
								ReadOnly:  true,
							}),
						)),
					))
				})
				It("should create a configmap with the specified path", func() {
					Eventually(Object(&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("opni-%s-rke2", logadapter.Name),
							Namespace: logadapter.Spec.OpniCluster.Namespace,
						},
					})).Should(ExistAnd(
						HaveOwner(&logadapter),
						HaveData("fluent-bit.conf", func(d string) bool {
							return strings.Contains(d, fmt.Sprintf("Path              %s", systemdLogPath))
						}),
					))
				})
			})
		})
		Context("with the K3s provider", func() {
			Specify("update opni cluster", func() {
				updateObject(&logadapter, func(l *v1beta2.LogAdapter) {
					l.Spec.Provider = v1beta2.LogProviderK3S
					l.Default()
				})
			})
			When("the container engine is systemd", func() {
				Specify("update opni cluster", func() {
					updateObject(&logadapter, func(l *v1beta2.LogAdapter) {
						l.Spec.K3S = &v1beta2.K3SSpec{
							ContainerEngine: v1beta2.ContainerEngineSystemd,
						}
						l.Default()
					})
				})
				It("should not create a logging", func() {
					Consistently(Object(&loggingv1beta1.Logging{
						ObjectMeta: metav1.ObjectMeta{
							Name: fmt.Sprintf("opni-%s-k3s", logadapter.Name),
						},
					})).ShouldNot(Exist())
				})
				When("a log path isn't specified", func() {
					It("should create a daemonset", func() {
						Eventually(Object(&appsv1.DaemonSet{
							ObjectMeta: metav1.ObjectMeta{
								Name:      fmt.Sprintf("opni-%s-k3s-journald-aggregator", logadapter.Name),
								Namespace: logadapter.Spec.OpniCluster.Namespace,
							},
						})).Should(ExistAnd(HaveOwner(&logadapter)))
					})
					It("should mount the default log path", func() {
						unset := corev1.HostPathUnset
						Eventually(Object(&appsv1.DaemonSet{
							ObjectMeta: metav1.ObjectMeta{
								Name:      fmt.Sprintf("opni-%s-k3s-journald-aggregator", logadapter.Name),
								Namespace: logadapter.Spec.OpniCluster.Namespace,
							},
						})).Should(ExistAnd(
							HaveOwner(&logadapter),
							HaveMatchingVolume(And(
								HaveName("journal"),
								HaveVolumeSource(corev1.VolumeSource{
									HostPath: &corev1.HostPathVolumeSource{
										Path: "/var/log/journal",
										Type: &unset,
									},
								}),
							)),
							HaveMatchingContainer(HaveVolumeMounts(
								corev1.VolumeMount{
									Name:      "journal",
									MountPath: "/var/log/journal",
									ReadOnly:  true,
								},
							)),
						))
					})
					It("should create a configmap with the default path", func() {
						Eventually(Object(&corev1.ConfigMap{
							ObjectMeta: metav1.ObjectMeta{
								Name:      fmt.Sprintf("opni-%s-k3s", logadapter.Name),
								Namespace: logadapter.Spec.OpniCluster.Namespace,
							},
						})).Should(ExistAnd(
							HaveOwner(&logadapter),
							HaveData("fluent-bit.conf", func(d string) bool {
								return strings.Contains(d, "Path              /var/log/journal")
							}),
						))
					})
				})
				When("a log path is specified", func() {
					Specify("update opni cluster", func() {
						updateObject(&logadapter, func(l *v1beta2.LogAdapter) {
							l.Spec.K3S.LogPath = systemdLogPath
						})
					})
					It("should mount the specified log path", func() {
						unset := corev1.HostPathUnset
						Eventually(Object(&appsv1.DaemonSet{
							ObjectMeta: metav1.ObjectMeta{
								Name:      fmt.Sprintf("opni-%s-k3s-journald-aggregator", logadapter.Name),
								Namespace: logadapter.Spec.OpniCluster.Namespace,
							},
						})).Should(ExistAnd(
							HaveOwner(&logadapter),
							HaveMatchingVolume(And(
								HaveName("journal"),
								HaveVolumeSource(corev1.VolumeSource{
									HostPath: &corev1.HostPathVolumeSource{
										Path: systemdLogPath,
										Type: &unset,
									},
								}),
							)),
							HaveMatchingContainer(HaveVolumeMounts(
								corev1.VolumeMount{
									Name:      "journal",
									MountPath: systemdLogPath,
									ReadOnly:  true,
								},
							)),
						))
					})
					It("should create a configmap with the specified path", func() {
						Eventually(Object(&corev1.ConfigMap{
							ObjectMeta: metav1.ObjectMeta{
								Name:      fmt.Sprintf("opni-%s-k3s", logadapter.Name),
								Namespace: logadapter.Spec.OpniCluster.Namespace,
							},
						})).Should(ExistAnd(
							HaveOwner(&logadapter),
							HaveData("fluent-bit.conf", func(d string) bool {
								return strings.Contains(d, fmt.Sprintf("Path              %s", systemdLogPath))
							}),
						))
					})
				})
			})
			When("the container engine is openrc", func() {
				Specify("update opni cluster", func() {
					updateObject(&logadapter, func(l *v1beta2.LogAdapter) {
						l.Spec.K3S = &v1beta2.K3SSpec{
							ContainerEngine: v1beta2.ContainerEngineOpenRC,
						}
						l.Spec.K3S.LogPath = ""
						l.Default()
					})
				})
				It("should not create a daemonset", func() {
					Eventually(Object(&appsv1.DaemonSet{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("opni-%s-k3s-journald-aggregator", logadapter.Name),
							Namespace: logadapter.Spec.OpniCluster.Namespace,
						},
					})).ShouldNot(Exist())
				})
				It("should not create a configmap", func() {
					Eventually(Object(&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("opni-%s-k3s", logadapter.Name),
							Namespace: logadapter.Spec.OpniCluster.Namespace,
						},
					})).ShouldNot(Exist())
				})
				When("a log path isn't specified", func() {
					It("should create a logging with the default path", func() {
						logging := loggingv1beta1.Logging{}
						Eventually(func() error {
							return k8sClient.Get(context.Background(), types.NamespacedName{
								Name: fmt.Sprintf("opni-%s-k3s", logadapter.Name),
							}, &logging)
						}).Should(BeNil())
						Expect(logging.Spec.FluentbitSpec.ExtraVolumeMounts).To(ContainElement(&loggingv1beta1.VolumeMount{
							Source:      "/var/log",
							Destination: "/var/log",
							ReadOnly:    pointer.Bool(true),
						}))
						Expect(logging.Spec.FluentbitSpec.InputTail.Path).To(Equal("/var/log/k3s.log"))
					})
				})
				When("a log path is specified", func() {
					Specify("update opni cluster", func() {
						updateObject(&logadapter, func(l *v1beta2.LogAdapter) {
							l.Spec.K3S.LogPath = openrcLogPath
							l.Default()
						})
					})
					It("should create a logging with the specified path", func() {
						logging := loggingv1beta1.Logging{}
						Eventually(func() error {
							return k8sClient.Get(context.Background(), types.NamespacedName{
								Name: fmt.Sprintf("opni-%s-k3s", logadapter.Name),
							}, &logging)
						}).Should(BeNil())
						Eventually(func() []*loggingv1beta1.VolumeMount {
							k8sClient.Get(context.Background(), types.NamespacedName{
								Name: fmt.Sprintf("opni-%s-k3s", logadapter.Name),
							}, &logging)
							return logging.Spec.FluentbitSpec.ExtraVolumeMounts
						}).Should(ContainElement(&loggingv1beta1.VolumeMount{
							Source:      openrcLogDir,
							Destination: openrcLogDir,
							ReadOnly:    pointer.Bool(true),
						}))
						Expect(logging.Spec.FluentbitSpec.InputTail.Path).To(Equal(openrcLogPath))
					})
				})
			})
		})
	})
})
