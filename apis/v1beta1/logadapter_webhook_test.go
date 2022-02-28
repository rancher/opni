package v1beta1

import (
	"context"

	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/logging/api/v1beta1"
	"github.com/go-test/deep"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	allProviders = []LogProvider{
		LogProviderAKS,
		LogProviderEKS,
		LogProviderGKE,
		LogProviderK3S,
		LogProviderRKE,
		LogProviderRKE2,
	}
)

func init() {
	deep.NilMapsAreEmpty = true
	deep.NilSlicesAreEmpty = true
}

var _ = Describe("LogadapterWebhook", Label("controller"), func() {
	When("creating a LogAdapter", func() {
		It("should apply defaults", func() {
			for _, provider := range allProviders {
				expected := &LogAdapter{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test" + string(provider),
					},
					Spec: LogAdapterSpec{
						Provider: provider,
					},
				}
				expected.Default()

				actual := &LogAdapter{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test" + string(provider),
					},
					Spec: LogAdapterSpec{
						Provider: provider,
					},
				}
				Expect(k8sClient.Create(context.Background(), actual)).To(Succeed())
				Expect(deep.Equal(expected.Spec, actual.Spec)).To(BeEmpty())
			}
		})
	})
	When("editing a LogAdapter", func() {
		It("should succeed", func() {
			for _, provider := range allProviders {
				existing := &LogAdapter{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test" + string(provider),
					},
					Spec: LogAdapterSpec{
						Provider: provider,
					},
				}
				err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(existing), existing)
				Expect(err).NotTo(HaveOccurred())

				existing.Spec.SELinuxEnabled = true
				err = k8sClient.Update(context.Background(), existing)
				Expect(err).NotTo(HaveOccurred())
			}
		})
		When("attempting to change the Provider field", func() {
			It("should cause an error", func() {
				for i, provider := range allProviders {
					existing := &LogAdapter{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test" + string(provider),
						},
						Spec: LogAdapterSpec{
							Provider: provider,
						},
					}
					err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(existing), existing)
					Expect(err).NotTo(HaveOccurred())

					existing.Spec.Provider = allProviders[(i+1)%len(allProviders)]
					err = k8sClient.Update(context.Background(), existing)
					Expect(err).To(Equal(&errors.StatusError{
						ErrStatus: metav1.Status{
							Status:  "Failure",
							Message: `admission webhook "vlogadapter.kb.io" denied the request: spec.provider: Forbidden: spec.provider cannot be modified once set.`,
							Reason:  "spec.provider: Forbidden: spec.provider cannot be modified once set.",
							Code:    403,
						},
					}))
				}
			})
		})
	})
	When("deleting a LogAdapter", func() {
		It("should succeed", func() {
			for _, provider := range allProviders {
				existing := &LogAdapter{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test" + string(provider),
					},
				}
				err := k8sClient.Delete(context.Background(), existing)
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})
	When("configuring conflicting providers", func() {
		It("should cause an error", func() {
			adapter := &LogAdapter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: LogAdapterSpec{
					Provider: LogProviderK3S,
					RKE: &RKESpec{
						LogLevel: "debug",
					},
				},
			}
			err := k8sClient.Create(context.Background(), adapter)
			Expect(err).To(HaveOccurred())
		})
	})
	When("specifying an alternative container log directory", func() {
		It("should add extra volume mounts to fluentbit", func() {
			adapter := &LogAdapter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: LogAdapterSpec{
					Provider:        LogProviderK3S,
					ContainerLogDir: "/path/to/logdir",
				},
			}
			err := k8sClient.Create(context.Background(), adapter)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(adapter), adapter)
			Expect(err).NotTo(HaveOccurred())

			Expect(adapter.Spec.RootFluentConfig.Fluentbit.ExtraVolumeMounts).To(HaveLen(1))
			Expect(adapter.Spec.RootFluentConfig.Fluentbit.ExtraVolumeMounts[0]).To(Equal(
				&loggingv1beta1.VolumeMount{
					Source:      "/path/to/logdir",
					Destination: "/path/to/logdir",
					ReadOnly:    pointer.Bool(true),
				},
			))
		})
	})
	When("k3s is configured to use systemd", func() {
		It("should set the log path to /var/log/journal", func() {
			adapter := &LogAdapter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test2",
				},
				Spec: LogAdapterSpec{
					Provider: LogProviderK3S,
					K3S: &K3SSpec{
						ContainerEngine: ContainerEngineSystemd,
					},
				},
			}
			err := k8sClient.Create(context.Background(), adapter)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(adapter), adapter)
			Expect(err).NotTo(HaveOccurred())

			Expect(adapter.Spec.K3S.LogPath).To(Equal("/var/log/journal"))
		})
	})
	When("k3s is configured to use openrc", func() {
		It("should set the log path to /var/log/k3s.log", func() {
			adapter := &LogAdapter{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test3",
				},
				Spec: LogAdapterSpec{
					Provider: LogProviderK3S,
					K3S: &K3SSpec{
						ContainerEngine: ContainerEngineOpenRC,
					},
				},
			}
			err := k8sClient.Create(context.Background(), adapter)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(context.Background(), client.ObjectKeyFromObject(adapter), adapter)
			Expect(err).NotTo(HaveOccurred())

			Expect(adapter.Spec.K3S.LogPath).To(Equal("/var/log/k3s.log"))
		})
	})
})
