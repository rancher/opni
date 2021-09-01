package test

import (
	"context"
	"fmt"

	helmv1 "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"github.com/phayes/freeport"
	demov1alpha1 "github.com/rancher/opni/apis/demo/v1alpha1"
	opniloggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	opninvidiav1 "github.com/rancher/opni/apis/nvidia/v1"
	"github.com/rancher/opni/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type Reconciler interface {
	SetupWithManager(ctrl.Manager) error
}

func RunTestEnvironment(
	testEnv *envtest.Environment,
	reconcilers ...Reconciler,
) (stop context.CancelFunc, k8sManager ctrl.Manager, k8sClient client.Client) {
	if len(reconcilers) == 0 {
		panic("no reconcilers")
	}
	var ctx context.Context
	ctx, stop = context.WithCancel(ctrl.SetupSignalHandler())

	cfg, err := testEnv.Start()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(cfg).NotTo(gomega.BeNil())

	go func() {
		defer ginkgo.GinkgoRecover()
		<-ctx.Done()
		err := testEnv.Stop()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}()

	StartControllerManager(ctx, testEnv)

	err = v1beta1.AddToScheme(scheme.Scheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = demov1alpha1.AddToScheme(scheme.Scheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = helmv1.AddToScheme(scheme.Scheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = apiextv1beta1.AddToScheme(scheme.Scheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = opniloggingv1beta1.AddToScheme(scheme.Scheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = opninvidiav1.AddToScheme(scheme.Scheme)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// +kubebuilder:scaffold:scheme

	ports, err := freeport.GetFreePorts(2)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// add the opnicluster manager
	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme.Scheme,
		MetricsBindAddress:     fmt.Sprintf(":%d", ports[0]),
		HealthProbeBindAddress: fmt.Sprintf(":%d", ports[1]),
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	k8sClient = k8sManager.GetClient()
	gomega.Expect(k8sClient).NotTo(gomega.BeNil())

	for _, rec := range reconcilers {
		gomega.Expect(rec.SetupWithManager(k8sManager)).NotTo(gomega.HaveOccurred())
	}

	go func() {
		defer ginkgo.GinkgoRecover()
		err = k8sManager.Start(ctx)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	}()

	err = k8sClient.Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "opnicluster-test",
		},
	})
	gomega.Expect(err).Should(gomega.Or(gomega.BeNil(), gomega.WithTransform(errors.IsAlreadyExists, gomega.BeTrue())))

	err = k8sClient.Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "opnidemo-test",
		},
	})
	gomega.Expect(err).Should(gomega.Or(gomega.BeNil(), gomega.WithTransform(errors.IsAlreadyExists, gomega.BeTrue())))

	err = k8sClient.Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "logadapter-test",
		},
	})
	gomega.Expect(err).Should(gomega.Or(gomega.BeNil(), gomega.WithTransform(errors.IsAlreadyExists, gomega.BeTrue())))
	return
}
