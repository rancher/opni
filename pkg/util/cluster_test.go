package util_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

var _ = Describe("Cluster Utils", Ordered, Label("unit", "slow"), func() {
	var restConfig *rest.Config
	var kubeconfigPath string
	BeforeAll(func() {
		env := test.Environment{
			TestBin: "../../testbin/bin",
		}
		var err error
		restConfig, err = env.StartK8s()
		Expect(err).NotTo(HaveOccurred())
		tempFile, err := os.CreateTemp("", "test-kubeconfig")
		Expect(err).NotTo(HaveOccurred())
		kubeconfigPath = tempFile.Name()
		apiConfig := api.NewConfig()
		apiConfig.CurrentContext = "test"
		apiConfig.Clusters[apiConfig.CurrentContext] = &api.Cluster{
			Server:                   restConfig.Host,
			CertificateAuthorityData: restConfig.CAData,
		}
		apiConfig.AuthInfos[apiConfig.CurrentContext] = &api.AuthInfo{
			ClientCertificateData: restConfig.CertData,
			ClientKeyData:         restConfig.KeyData,
		}
		apiConfig.Contexts[apiConfig.CurrentContext] = &api.Context{
			Cluster:  apiConfig.CurrentContext,
			AuthInfo: apiConfig.CurrentContext,
		}
		err = clientcmd.WriteToFile(*apiConfig, kubeconfigPath)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(env.Stop)
	})
	Describe("NewK8sClient", func() {
		When("a kubeconfig is given", func() {
			It("should create the client from the kubeconfig", func() {
				_, err := util.NewK8sClient(util.ClientOptions{
					Kubeconfig: &kubeconfigPath,
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})
		When("a REST config is given", func() {
			It("should create the client from the REST config", func() {
				_, err := util.NewK8sClient(util.ClientOptions{
					RestConfig: restConfig,
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})
		When("neither a kubeconfig nor a REST config is given", func() {
			It("should create the client from the in-cluster config", func() {
				_, err := util.NewK8sClient(util.ClientOptions{})
				Expect(err).To(MatchError(rest.ErrNotInCluster))
			})
		})
		It("should handle errors", func() {
			_, err := util.NewK8sClient(util.ClientOptions{
				Kubeconfig: util.Pointer("/dev/null"),
			})
			Expect(err).To(HaveOccurred())
			_, err = util.NewK8sClient(util.ClientOptions{
				Kubeconfig: util.Pointer("/does/not/exist"),
			})
			Expect(err).To(HaveOccurred())
		})
	})
})
