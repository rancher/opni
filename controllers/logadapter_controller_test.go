package controllers

import (
	"context"
	"fmt"
	"strings"

	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/api/v1beta1"
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

var _ = Describe("LogAdapter Controller", func() {
	var (
		logadapter v1beta1.LogAdapter
		cluster    v1beta1.OpniCluster
		err        error
	)
	When("creating a logadapter", func() {
		JustAfterEach(func() {
			k8sClient.Delete(context.Background(), &logadapter, client.GracePeriodSeconds(0))

			logging := loggingv1beta1.Logging{}
			k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      laName,
				Namespace: laNamespace,
			}, &logging)
			k8sClient.Delete(context.Background(), &logging, client.GracePeriodSeconds(0))
		})
		BeforeEach(func() {
			logadapter = v1beta1.LogAdapter{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta1.GroupVersion.String(),
					Kind:       "LogAdapter",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      laName,
					Namespace: laNamespace,
				},
				Spec: v1beta1.LogAdapterSpec{
					Provider: v1beta1.LogProviderEKS,
					OpniCluster: v1beta1.OpniClusterNameSpec{
						Name:      crName,
						Namespace: laNamespace,
					},
				},
			}
			cluster = v1beta1.OpniCluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1beta1.GroupVersion.String(),
					Kind:       "OpniCluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName,
					Namespace: laNamespace,
				},
				Spec: v1beta1.OpniClusterSpec{
					Elastic: v1beta1.ElasticSpec{
						Credentials: v1beta1.CredentialsSpec{
							Keys: &v1beta1.KeysSpec{
								AccessKey: "testAccessKey",
								SecretKey: "testSecretKey",
							},
						},
					},
				},
			}
		})
		JustBeforeEach(func() {
			logadapter.Default()
			k8sClient.Create(context.Background(), &cluster)
			err = k8sClient.Create(context.Background(), &logadapter)
		})
		It("should succeed", func() {
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      laName,
					Namespace: laNamespace,
				}, &logadapter)
			}).Should(BeNil())
		})
		It("should create a logging", func() {
			Eventually(func() error {
				logging := loggingv1beta1.Logging{}
				return k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      laName,
					Namespace: laNamespace,
				}, &logging)
			}).Should(BeNil())
			Expect(getOwnerReferenceUID(&loggingv1beta1.Logging{}, types.NamespacedName{
				Name:      laName,
				Namespace: laNamespace,
			})).To(Equal(string(logadapter.ObjectMeta.UID)))
		})
		When("the OpniCluster does not exist", func() {
			BeforeEach(func() {
				logadapter.Spec.OpniCluster.Name = "doesnotexist"
			})
			It("should succeed", func() {
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() error {
					return k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      laName,
						Namespace: laNamespace,
					}, &logadapter)
				}).Should(BeNil())
			})
			It("should not create a logging", func() {
				Eventually(func() error {
					logging := loggingv1beta1.Logging{}
					return k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      laName,
						Namespace: laNamespace,
					}, &logging)
				}).ShouldNot(BeNil())
			})
		})
		Context("with the RKE provider", func() {
			BeforeEach(func() {
				logadapter.Spec.Provider = v1beta1.LogProviderRKE
			})
			AfterEach(func() {
				ds := appsv1.DaemonSet{}
				k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-rke-aggregator", laName),
					Namespace: laNamespace,
				}, &ds)
				k8sClient.Delete(context.Background(), &ds, client.GracePeriodSeconds(0))

				configmap := corev1.ConfigMap{}
				k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-rke", laName),
					Namespace: laNamespace,
				}, &configmap)
				k8sClient.Delete(context.Background(), &configmap, client.GracePeriodSeconds(0))
			})
			When("the spec is valid", func() {
				It("should create a daemonset", func() {
					Eventually(func() error {
						ds := appsv1.DaemonSet{}
						return k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-rke-aggregator", laName),
							Namespace: laNamespace,
						}, &ds)
					}).Should(BeNil())
					Expect(getOwnerReferenceUID(&appsv1.DaemonSet{}, types.NamespacedName{
						Name:      fmt.Sprintf("%s-rke-aggregator", laName),
						Namespace: laNamespace,
					})).To(Equal(string(logadapter.ObjectMeta.UID)))
				})
			})
			When("a log level is not specified", func() {
				It("should create a configmap with default log level", func() {
					Eventually(func() string {
						configmap := corev1.ConfigMap{}
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-rke", laName),
							Namespace: laNamespace,
						}, &configmap)
						if err != nil {
							return err.Error()
						}
						if strings.Contains(string(configmap.Data["fluent-bit.conf"]), fmt.Sprintf("Log_Level         %s", v1beta1.LogLevelInfo)) {
							return ""
						}
						return "config map has incorrect log level"
					}).Should(BeEmpty())
					Expect(getOwnerReferenceUID(&corev1.ConfigMap{}, types.NamespacedName{
						Name:      fmt.Sprintf("%s-rke", laName),
						Namespace: laNamespace,
					})).To(Equal(string(logadapter.ObjectMeta.UID)))
				})
			})
			When("warning log level is specified", func() {
				BeforeEach(func() {
					logadapter.Spec.RKE = &v1beta1.RKESpec{
						LogLevel: v1beta1.LogLevelWarn,
					}
				})
				It("should create a config map with warn log level", func() {
					Eventually(func() string {
						configmap := corev1.ConfigMap{}
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-rke", laName),
							Namespace: laNamespace,
						}, &configmap)
						if err != nil {
							return err.Error()
						}
						if strings.Contains(string(configmap.Data["fluent-bit.conf"]), fmt.Sprintf("Log_Level         %s", v1beta1.LogLevelWarn)) {
							return ""
						}
						return "config map has incorrect log level"
					}).Should(BeEmpty())
					Expect(getOwnerReferenceUID(&corev1.ConfigMap{}, types.NamespacedName{
						Name:      fmt.Sprintf("%s-rke", laName),
						Namespace: laNamespace,
					})).To(Equal(string(logadapter.ObjectMeta.UID)))
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
							Name:      laName,
							Namespace: laNamespace,
						}, &logadapter)
					}).Should(BeNil())
				})
				XIt("should not create a daemonset", func() {
					Consistently(func() bool {
						ds := appsv1.DaemonSet{}
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-rke-aggregator", laName),
							Namespace: laNamespace,
						}, &ds)
						return errors.IsNotFound(err)
					}).Should(BeTrue())
				})
				XIt("should not create a configmap", func() {
					Consistently(func() bool {
						configmap := corev1.ConfigMap{}
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-rke", laName),
							Namespace: laNamespace,
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
					Name:      fmt.Sprintf("%s-rke2-journald-aggregator", laName),
					Namespace: laNamespace,
				}, &ds)
				k8sClient.Delete(context.Background(), &ds, client.GracePeriodSeconds(0))

				configmap := corev1.ConfigMap{}
				k8sClient.Get(context.Background(), types.NamespacedName{
					Name:      fmt.Sprintf("%s-rke2", laName),
					Namespace: laNamespace,
				}, &configmap)
				k8sClient.Delete(context.Background(), &configmap, client.GracePeriodSeconds(0))
			})
			When("a log path isn't specified", func() {
				It("should create a daemonset", func() {
					Eventually(func() error {
						ds := appsv1.DaemonSet{}
						return k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-rke2-journald-aggregator", laName),
							Namespace: laNamespace,
						}, &ds)
					}).Should(BeNil())
					Expect(getOwnerReferenceUID(&appsv1.DaemonSet{}, types.NamespacedName{
						Name:      fmt.Sprintf("%s-rke2-journald-aggregator", laName),
						Namespace: laNamespace,
					})).To(Equal(string(logadapter.ObjectMeta.UID)))
				})
				It("should mount the default log path", func() {
					ds := appsv1.DaemonSet{}
					unset := corev1.HostPathUnset
					Eventually(func() error {
						return k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-rke2-journald-aggregator", laName),
							Namespace: laNamespace,
						}, &ds)
					}).Should(BeNil())
					Expect(ds.Spec.Template.Spec.Volumes).To(ContainElement(corev1.Volume{
						Name: "journal",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: "/var/log/journal",
								Type: &unset,
							},
						}}))
					Expect(ds.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(corev1.VolumeMount{
						Name:      "journal",
						MountPath: "/var/log/journal",
						ReadOnly:  true,
					}))
				})
				It("should create a configmap with the default path", func() {
					Eventually(func() string {
						configmap := corev1.ConfigMap{}
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-rke2", laName),
							Namespace: laNamespace,
						}, &configmap)
						if err != nil {
							return err.Error()
						}
						_, ok := configmap.Data["received_at.lua"]
						if ok && strings.Contains(string(configmap.Data["fluent-bit.conf"]), "Path              /var/log/journal") {
							return ""
						}
						return "configmap has incorrect log path"
					}).Should(BeEmpty())
				})
			})
			When("a log path is specified", func() {
				BeforeEach(func() {
					logadapter.Spec.RKE2 = &v1beta1.RKE2Spec{
						LogPath: systemdLogPath,
					}
				})
				It("should mount the specified log path", func() {
					ds := appsv1.DaemonSet{}
					unset := corev1.HostPathUnset
					k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      fmt.Sprintf("%s-rke2-journald-aggregator", laName),
						Namespace: laNamespace,
					}, &logadapter)
					Eventually(func() error {
						return k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-rke2-journald-aggregator", laName),
							Namespace: laNamespace,
						}, &ds)
					}).Should(BeNil())
					Expect(ds.Spec.Template.Spec.Volumes).To(ContainElement(corev1.Volume{
						Name: "journal",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: systemdLogPath,
								Type: &unset,
							},
						}}))
					Expect(ds.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(corev1.VolumeMount{
						Name:      "journal",
						MountPath: systemdLogPath,
						ReadOnly:  true,
					}))
				})
				It("should create a configmap with the specified path", func() {
					Eventually(func() string {
						configmap := corev1.ConfigMap{}
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-rke2", laName),
							Namespace: laNamespace,
						}, &configmap)
						if err != nil {
							return err.Error()
						}
						_, ok := configmap.Data["received_at.lua"]
						if ok && strings.Contains(string(configmap.Data["fluent-bit.conf"]), fmt.Sprintf("Path              %s", systemdLogPath)) {
							return ""
						}
						return "configmap has incorrect log path"
					}).Should(BeEmpty())
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
							Name:      laName,
							Namespace: laNamespace,
						}, &logadapter)
					}).Should(BeNil())
				})
				XIt("should not create a daemonset", func() {
					Consistently(func() bool {
						ds := appsv1.DaemonSet{}
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-rke2-journald-aggregator", laName),
							Namespace: laNamespace,
						}, &ds)
						return errors.IsNotFound(err)
					}).Should(BeTrue())
				})
				XIt("should not create a configmap", func() {
					Eventually(func() bool {
						configmap := corev1.ConfigMap{}
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-rke2", laName),
							Namespace: laNamespace,
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
						Name:      fmt.Sprintf("%s-k3s-journald-aggregator", laName),
						Namespace: laNamespace,
					}, &ds)
					k8sClient.Delete(context.Background(), &ds, client.GracePeriodSeconds(0))

					configmap := corev1.ConfigMap{}
					k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      fmt.Sprintf("%s-k3s", laName),
						Namespace: laNamespace,
					}, &configmap)
					k8sClient.Delete(context.Background(), &configmap, client.GracePeriodSeconds(0))
				})
				It("should not create a logging", func() {
					Consistently(func() bool {
						logging := loggingv1beta1.Logging{}
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-k3s", laName),
							Namespace: laNamespace,
						}, &logging)
						return errors.IsNotFound(err)
					}).Should(BeTrue())
				})
				When("a log path isn't specified", func() {
					It("should create a daemonset", func() {
						Eventually(func() error {
							ds := appsv1.DaemonSet{}
							return k8sClient.Get(context.Background(), types.NamespacedName{
								Name:      fmt.Sprintf("%s-k3s-journald-aggregator", laName),
								Namespace: laNamespace,
							}, &ds)
						}).Should(BeNil())
						Expect(getOwnerReferenceUID(&appsv1.DaemonSet{}, types.NamespacedName{
							Name:      fmt.Sprintf("%s-k3s-journald-aggregator", laName),
							Namespace: laNamespace,
						})).To(Equal(string(logadapter.ObjectMeta.UID)))
					})
					It("should mount the default log path", func() {
						ds := appsv1.DaemonSet{}
						unset := corev1.HostPathUnset
						Eventually(func() error {
							return k8sClient.Get(context.Background(), types.NamespacedName{
								Name:      fmt.Sprintf("%s-k3s-journald-aggregator", laName),
								Namespace: laNamespace,
							}, &ds)
						}).Should(BeNil())
						Expect(ds.Spec.Template.Spec.Volumes).To(ContainElement(corev1.Volume{
							Name: "journal",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/var/log/journal",
									Type: &unset,
								},
							}}))
						Expect(ds.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(corev1.VolumeMount{
							Name:      "journal",
							MountPath: "/var/log/journal",
							ReadOnly:  true,
						}))
					})
					It("should create a configmap with the default path", func() {
						Eventually(func() string {
							configmap := corev1.ConfigMap{}
							err := k8sClient.Get(context.Background(), types.NamespacedName{
								Name:      fmt.Sprintf("%s-k3s", laName),
								Namespace: laNamespace,
							}, &configmap)
							if err != nil {
								return err.Error()
							}
							_, ok := configmap.Data["received_at.lua"]
							if ok && strings.Contains(string(configmap.Data["fluent-bit.conf"]), "Path              /var/log/journal") {
								return ""
							}
							return "configmap has incorrect log path"
						}).Should(BeEmpty())
					})
				})
				When("a log path is specified", func() {
					BeforeEach(func() {
						logadapter.Spec.K3S.LogPath = systemdLogPath
					})
					It("should mount the specified log path", func() {
						ds := appsv1.DaemonSet{}
						unset := corev1.HostPathUnset
						k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-k3s-journald-aggregator", laName),
							Namespace: laNamespace,
						}, &logadapter)
						Eventually(func() error {
							return k8sClient.Get(context.Background(), types.NamespacedName{
								Name:      fmt.Sprintf("%s-k3s-journald-aggregator", laName),
								Namespace: laNamespace,
							}, &ds)
						}).Should(BeNil())
						Expect(ds.Spec.Template.Spec.Volumes).To(ContainElement(corev1.Volume{
							Name: "journal",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: systemdLogPath,
									Type: &unset,
								},
							}}))
						Expect(ds.Spec.Template.Spec.Containers[0].VolumeMounts).To(ContainElement(corev1.VolumeMount{
							Name:      "journal",
							MountPath: systemdLogPath,
							ReadOnly:  true,
						}))
					})
					It("should create a configmap with the specified path", func() {
						Eventually(func() string {
							configmap := corev1.ConfigMap{}
							err := k8sClient.Get(context.Background(), types.NamespacedName{
								Name:      fmt.Sprintf("%s-k3s", laName),
								Namespace: laNamespace,
							}, &configmap)
							if err != nil {
								return err.Error()
							}
							_, ok := configmap.Data["received_at.lua"]
							if ok && strings.Contains(string(configmap.Data["fluent-bit.conf"]), fmt.Sprintf("Path              %s", systemdLogPath)) {
								return ""
							}
							return "configmap has incorrect log path"
						}).Should(BeEmpty())
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
						Name:      fmt.Sprintf("%s-k3s", laName),
						Namespace: laNamespace,
					}, &logging)
					k8sClient.Delete(context.Background(), &logging, client.GracePeriodSeconds(0))
				})
				It("should not create a daemonset", func() {
					Consistently(func() bool {
						ds := appsv1.DaemonSet{}
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-k3s-journald-aggregator", laName),
							Namespace: laNamespace,
						}, &ds)
						return errors.IsNotFound(err)
					}).Should(BeTrue())
				})
				It("should not create a configmap", func() {
					Eventually(func() bool {
						configmap := corev1.ConfigMap{}
						err := k8sClient.Get(context.Background(), types.NamespacedName{
							Name:      fmt.Sprintf("%s-k3s", laName),
							Namespace: laNamespace,
						}, &configmap)
						return errors.IsNotFound(err)
					}).Should(BeTrue())
				})
				When("a log path isn't specified", func() {
					It("should create a logging with the default path", func() {
						logging := loggingv1beta1.Logging{}
						Eventually(func() error {
							return k8sClient.Get(context.Background(), types.NamespacedName{
								Name:      fmt.Sprintf("%s-k3s", laName),
								Namespace: laNamespace,
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
								Name:      fmt.Sprintf("%s-k3s", laName),
								Namespace: laNamespace,
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
