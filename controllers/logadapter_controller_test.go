package controllers

import (
	"context"
	"fmt"
	"strings"

	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/api/v1beta1"
	. "github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rancher/opni/apis/v1beta1"
)

var _ = XDescribe("LogAdapter Controller", func() {
	var (
		logadapter v1beta1.LogAdapter
		cluster    v1beta1.OpniCluster
		err        error
		testNs     string
	)
	Specify("setup", func() {
		testNs = makeTestNamespace()
		cluster = v1beta1.OpniCluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1beta1.GroupVersion.String(),
				Kind:       "OpniCluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: testNs,
			},
			Spec: v1beta1.OpniClusterSpec{
				Elastic: v1beta1.ElasticSpec{},
				Nats: v1beta1.NatsSpec{
					AuthMethod: v1beta1.NatsAuthUsername,
				},
			},
		}
		k8sClient.Create(context.Background(), &cluster)
	})
	When("creating a logadapter", func() {
		BeforeEach(func() {
			logadapter = v1beta1.LogAdapter{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: cluster.Namespace,
				},
				Spec: v1beta1.LogAdapterSpec{
					Provider: v1beta1.LogProviderEKS,
					OpniCluster: v1beta1.OpniClusterNameSpec{
						Name:      "test",
						Namespace: cluster.Namespace,
					},
				},
			}
		})
		JustBeforeEach(func() {
			logadapter.Default()
			err = k8sClient.Create(context.Background(), &logadapter)
		})
		It("should succeed", func() {
			Eventually(Object(&logadapter)).Should(Exist())
		})
		It("should create a logging", func() {
			Eventually(Object(&loggingv1beta1.Logging{
				ObjectMeta: metav1.ObjectMeta{
					Name:      logadapter.Name,
					Namespace: logadapter.Namespace,
				},
			})).Should(ExistAnd(HaveOwner(&logadapter)))
		})
		When("the OpniCluster does not exist", func() {
			BeforeEach(func() {
				logadapter.Spec.OpniCluster.Name = "doesnotexist"
			})
			It("should succeed", func() {
				Eventually(Object(&logadapter)).Should(Exist())
			})
			It("should not create a logging", func() {
				Consistently(Object(&loggingv1beta1.Logging{
					ObjectMeta: metav1.ObjectMeta{
						Name:      logadapter.Name,
						Namespace: logadapter.Namespace,
					},
				})).ShouldNot(Exist())
			})
		})
		Context("with the RKE provider", func() {
			BeforeEach(func() {
				logadapter.Spec.Provider = v1beta1.LogProviderRKE
			})
			AfterEach(func() {
				ds := appsv1.DaemonSet{}
				k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-rke-aggregator", logadapter.Name),
					Namespace: logadapter.Namespace,
				}, &ds)
				k8sClient.Delete(context.Background(), &ds)

				configmap := corev1.ConfigMap{}
				k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-rke", logadapter.Name),
					Namespace: logadapter.Namespace,
				}, &configmap)
				k8sClient.Delete(context.Background(), &configmap)
			})
			When("the spec is valid", func() {
				It("should create a daemonset", func() {
					Eventually(Object(&appsv1.DaemonSet{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("%s-rke-aggregator", logadapter.Name),
							Namespace: logadapter.Namespace,
						},
					})).Should(ExistAnd(HaveOwner(&logadapter)))
				})
			})
			When("a log level is not specified", func() {
				It("should create a configmap with default log level", func() {
					Eventually(Object(&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("%s-rke", logadapter.Name),
							Namespace: logadapter.Namespace,
						},
					})).Should(ExistAnd(
						HaveOwner(&logadapter),
						HaveData("fluent-bit.conf", fmt.Sprintf("Log_Level         %s", v1beta1.LogLevelInfo)),
					))
				})
			})
			When("warning log level is specified", func() {
				BeforeEach(func() {
					logadapter.Spec.RKE = &v1beta1.RKESpec{
						LogLevel: v1beta1.LogLevelWarn,
					}
				})
				It("should create a config map with warn log level", func() {
					Eventually(Object(&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("%s-rke", logadapter.Name),
							Namespace: logadapter.Namespace,
						},
					})).Should(ExistAnd(
						HaveOwner(&logadapter),
						HaveData("fluent-bit.conf", fmt.Sprintf("Log_Level         %s", v1beta1.LogLevelWarn)),
					))
				})
			})
			When("the OpniCluster does not exist", func() {
				BeforeEach(func() {
					logadapter.Spec.OpniCluster.Name = "doesnotexist"
				})
				XIt("should succeed", func() {
					Expect(err).NotTo(HaveOccurred())
					Eventually(func() error {
						return k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      logadapter.Name,
							Namespace: logadapter.Namespace,
						}, &logadapter)
					}).Should(BeNil())
				})
				XIt("should not create a daemonset", func() {
					Consistently(func() bool {
						ds := appsv1.DaemonSet{}
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-rke-aggregator", logadapter.Name),
							Namespace: logadapter.Namespace,
						}, &ds)
						return errors.IsNotFound(err)
					}).Should(BeTrue())
				})
				XIt("should not create a configmap", func() {
					Consistently(func() bool {
						configmap := corev1.ConfigMap{}
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-rke", logadapter.Name),
							Namespace: logadapter.Namespace,
						}, &configmap)
						return errors.IsNotFound(err)
					}).Should(BeTrue())
				})
			})
		})
		Context("with the RKE2 provider", func() {
			BeforeEach(func() {
				logadapter.Spec.Provider = v1beta1.LogProviderRKE2
			})
			AfterEach(func() {
				ds := appsv1.DaemonSet{}
				k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-rke2-journald-aggregator", logadapter.Name),
					Namespace: logadapter.Namespace,
				}, &ds)
				k8sClient.Delete(context.Background(), &ds)

				configmap := corev1.ConfigMap{}
				k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-rke2", logadapter.Name),
					Namespace: logadapter.Namespace,
				}, &configmap)
				k8sClient.Delete(context.Background(), &configmap)
			})
			When("a log path isn't specified", func() {
				It("should create a daemonset", func() {
					Eventually(Object(&appsv1.DaemonSet{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("%s-rke2-journald-aggregator", logadapter.Name),
							Namespace: logadapter.Namespace,
						},
					})).Should(ExistAnd(HaveOwner(&logadapter)))
				})
				It("should mount the default log path", func() {
					unset := corev1.HostPathUnset
					Eventually(Object(&appsv1.DaemonSet{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("%s-rke2-journald-aggregator", logadapter.Name),
							Namespace: logadapter.Namespace,
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
							Name:      fmt.Sprintf("%s-rke2", logadapter.Name),
							Namespace: logadapter.Namespace,
						},
					})).Should(ExistAnd(
						HaveOwner(&logadapter),
						HaveData("received_at.lua", func(data string) bool {
							return strings.Contains(data, "Path              /var/log/journal")
						}),
					))
				})
			})
			When("a log path is specified", func() {
				BeforeEach(func() {
					logadapter.Spec.RKE2 = &v1beta1.RKE2Spec{
						LogPath: systemdLogPath,
					}
				})
				It("should mount the specified log path", func() {
					unset := corev1.HostPathUnset
					Eventually(Object(&appsv1.DaemonSet{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("%s-rke2-journald-aggregator", logadapter.Name),
							Namespace: logadapter.Namespace,
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
							Name:      fmt.Sprintf("%s-rke2", logadapter.Name),
							Namespace: logadapter.Namespace,
						},
					})).Should(ExistAnd(
						HaveOwner(&logadapter),
						HaveData("received_at.lua", func(data string) bool {
							return strings.Contains(data, fmt.Sprintf("Path              %s", systemdLogPath))
						}),
					))
				})
			})
			When("the OpniCluster does not exist", func() {
				BeforeEach(func() {
					logadapter.Spec.OpniCluster.Name = "doesnotexist"
				})
				XIt("should succeed", func() {
					Expect(err).NotTo(HaveOccurred())
					Eventually(func() error {
						return k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      logadapter.Name,
							Namespace: logadapter.Namespace,
						}, &logadapter)
					}).Should(BeNil())
				})
				XIt("should not create a daemonset", func() {
					Consistently(func() bool {
						ds := appsv1.DaemonSet{}
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-rke2-journald-aggregator", logadapter.Name),
							Namespace: logadapter.Namespace,
						}, &ds)
						return errors.IsNotFound(err)
					}).Should(BeTrue())
				})
				XIt("should not create a configmap", func() {
					Eventually(func() bool {
						configmap := corev1.ConfigMap{}
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-rke2", logadapter.Name),
							Namespace: logadapter.Namespace,
						}, &configmap)
						return errors.IsNotFound(err)
					}).Should(BeTrue())
				})
			})
		})
		Context("with the K3s provider", func() {
			BeforeEach(func() {
				logadapter.Spec.Provider = v1beta1.LogProviderK3S
			})
			When("the container engine is systemd", func() {
				BeforeEach(func() {
					logadapter.Spec.K3S = &v1beta1.K3SSpec{
						ContainerEngine: v1beta1.ContainerEngineSystemd,
					}
				})
				AfterEach(func() {
					ds := appsv1.DaemonSet{}
					k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      fmt.Sprintf("%s-k3s-journald-aggregator", logadapter.Name),
						Namespace: logadapter.Namespace,
					}, &ds)
					k8sClient.Delete(context.Background(), &ds)

					configmap := corev1.ConfigMap{}
					k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      fmt.Sprintf("%s-k3s", logadapter.Name),
						Namespace: logadapter.Namespace,
					}, &configmap)
					k8sClient.Delete(context.Background(), &configmap)
				})
				It("should not create a logging", func() {
					Consistently(Object(&loggingv1beta1.Logging{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("%s-k3s", logadapter.Name),
							Namespace: logadapter.Namespace,
						},
					})).ShouldNot(Exist())
				})
				When("a log path isn't specified", func() {
					It("should create a daemonset", func() {
						Eventually(Object(&appsv1.DaemonSet{
							ObjectMeta: metav1.ObjectMeta{
								Name:      fmt.Sprintf("%s-k3s-journald-aggregator", logadapter.Name),
								Namespace: logadapter.Namespace,
							},
						})).Should(ExistAnd(HaveOwner(&logadapter)))
					})
					It("should mount the default log path", func() {
						unset := corev1.HostPathUnset
						Eventually(Object(&appsv1.DaemonSet{
							ObjectMeta: metav1.ObjectMeta{
								Name:      fmt.Sprintf("%s-k3s-journald-aggregator", logadapter.Name),
								Namespace: logadapter.Namespace,
							},
						})).Should(ExistAnd(
							HaveOwner(&logadapter),
							HaveVolumeSource(corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/var/log/journal",
									Type: &unset,
								},
							}),
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
								Name:      fmt.Sprintf("%s-k3s", logadapter.Name),
								Namespace: logadapter.Namespace,
							},
						})).Should(ExistAnd(
							HaveOwner(&logadapter),
							HaveData("received_at.lua", func(data string) bool {
								return strings.Contains(data, "Path              /var/log/journal")
							}),
						))
					})
				})
				When("a log path is specified", func() {
					BeforeEach(func() {
						logadapter.Spec.K3S.LogPath = systemdLogPath
					})
					It("should mount the specified log path", func() {
						unset := corev1.HostPathUnset
						Eventually(Object(&appsv1.DaemonSet{
							ObjectMeta: metav1.ObjectMeta{
								Name:      fmt.Sprintf("%s-k3s-journald-aggregator", logadapter.Name),
								Namespace: logadapter.Namespace,
							},
						})).Should(ExistAnd(
							HaveOwner(&logadapter),
							HaveVolumeSource(corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: systemdLogPath,
									Type: &unset,
								},
							}),
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
								Name:      fmt.Sprintf("%s-k3s", logadapter.Name),
								Namespace: logadapter.Namespace,
							},
						})).Should(ExistAnd(
							HaveOwner(&logadapter),
							HaveData("received_at.lua", func(data string) bool {
								return strings.Contains(data, fmt.Sprintf("Path              %s", systemdLogPath))
							}),
						))
					})
				})
			})
			When("the container engine is openrc", func() {
				BeforeEach(func() {
					logadapter.Spec.K3S = &v1beta1.K3SSpec{
						ContainerEngine: v1beta1.ContainerEngineOpenRC,
					}
				})
				AfterEach(func() {
					logging := loggingv1beta1.Logging{}
					k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      fmt.Sprintf("%s-k3s", logadapter.Name),
						Namespace: logadapter.Namespace,
					}, &logging)
					k8sClient.Delete(context.Background(), &logging, client.GracePeriodSeconds(0))
				})
				It("should not create a daemonset", func() {
					Consistently(Object(&appsv1.DaemonSet{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("%s-k3s-journald-aggregator", logadapter.Name),
							Namespace: logadapter.Namespace,
						},
					})).ShouldNot(Exist())
				})
				It("should not create a configmap", func() {
					Consistently(Object(&corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("%s-k3s", logadapter.Name),
							Namespace: logadapter.Namespace,
						},
					})).ShouldNot(Exist())
				})
				When("a log path isn't specified", func() {
					It("should create a logging with the default path", func() {
						logging := loggingv1beta1.Logging{}
						Eventually(func() error {
							return k8sClient.Get(context.Background(), types.NamespacedName{
								Name:      fmt.Sprintf("%s-k3s", logadapter.Name),
								Namespace: logadapter.Namespace,
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
					BeforeEach(func() {
						logadapter.Spec.K3S.LogPath = openrcLogPath
					})
					It("should create a logging with the specified path", func() {
						logging := loggingv1beta1.Logging{}
						Eventually(func() error {
							return k8sClient.Get(context.Background(), types.NamespacedName{
								Name:      fmt.Sprintf("%s-k3s", logadapter.Name),
								Namespace: logadapter.Namespace,
							}, &logging)
						}).Should(BeNil())
						Expect(logging.Spec.FluentbitSpec.ExtraVolumeMounts).To(ContainElement(&loggingv1beta1.VolumeMount{
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

func getOwnerReferenceUID(object client.Object, name types.NamespacedName) string {
	err := k8sClient.Get(context.Background(), name, object)
	if err != nil {
		return err.Error()
	}
	if len(object.GetOwnerReferences()) == 0 {
		return "no ownerreferences"
	}
	return string(object.GetOwnerReferences()[0].UID)
}
