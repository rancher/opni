/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers_test

import (
	"context"
	"crypto/sha1"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"path"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/controllers"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/resources/opnicluster"
	"github.com/rancher/opni/pkg/resources/opniopensearch"
	"github.com/rancher/opni/pkg/resources/preprocessor"
	"github.com/rancher/opni/pkg/test/freeport"
	_ "github.com/rancher/opni/pkg/test/setup"
	"github.com/rancher/opni/pkg/test/testk8s"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"

	aiv1beta1 "github.com/rancher/opni/apis/ai/v1beta1"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about

var (
	// all processes
	k8sClient client.Client
	certMgr   *testk8s.TestCertManager
	scheme    = apis.NewScheme()

	// process 1 only
	k8sManager ctrl.Manager
	testEnv    *envtest.Environment
	stopEnv    context.CancelFunc
)

const (
	systemdLogPath = "/var/log/testing"
	openrcLogPath  = "/var/test/alternate.log"
	openrcLogDir   = "/var/test"
)

func TestAPIs(t *testing.T) {
	SetDefaultEventuallyTimeout(30 * time.Second)
	// SetDefaultEventuallyTimeout(24 * time.Hour) // For debugging
	SetDefaultEventuallyPollingInterval(50 * time.Millisecond)
	SetDefaultConsistentlyDuration(1 * time.Second)
	SetDefaultConsistentlyPollingInterval(50 * time.Millisecond)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	logf.SetLogger(NewTestLogger())
	port := freeport.GetFreePort()
	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		Scheme: scheme,
		CRDs:   testk8s.GetCertManagerCRDs(scheme),
		CRDDirectoryPaths: []string{
			"../config/crd/bases",
			"../config/crd/logging",
			"../config/crd/grafana",
			"../config/crd/opensearch",
			"../test/resources",
		},
		BinaryAssetsDirectory: "../testbin/bin",
		ControlPlane: envtest.ControlPlane{
			APIServer: &envtest.APIServer{
				SecureServing: envtest.SecureServing{
					ListenAddr: envtest.ListenAddr{
						Address: "127.0.0.1",
						Port:    fmt.Sprint(port),
					},
				},
			},
		},
	}

	certMgr = &testk8s.TestCertManager{}

	stopEnv, k8sManager, k8sClient = RunTestEnvironment(testEnv, true, false,
		&controllers.CoreGatewayReconciler{},
		&controllers.CoreMonitoringReconciler{},
		&controllers.GrafanaReconciler{},
		&controllers.GrafanaDashboardReconciler{},
		&controllers.GrafanaDatasourceReconciler{},
		&controllers.LoggingDataPrepperReconciler{},
		&controllers.CoreAlertingReconciler{},
		&controllers.AIOpniClusterReconciler{
			Opts: []opnicluster.ReconcilerOption{
				opnicluster.WithContinueOnIndexError(),
				opnicluster.WithCertManager(certMgr),
			},
		},
		&controllers.LoggingOpniOpensearchReconciler{
			Opts: []opniopensearch.ReconcilerOption{
				opniopensearch.WithCertManager(certMgr),
			},
		},
		&controllers.AIPretrainedModelReconciler{},
		&controllers.NatsClusterReonciler{},
		&controllers.CoreCollectorReconciler{},
		&controllers.LoggingPreprocessorReconciler{
			Opts: []preprocessor.ReconcilerOption{
				preprocessor.WithCertManager(certMgr),
			},
		},
	)
	kmatch.SetDefaultObjectClient(k8sClient)

	restConfig := testEnv.Config
	config := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"default": {
				Server:                   restConfig.Host,
				CertificateAuthorityData: restConfig.CAData,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"default": {
				ClientCertificateData: restConfig.CertData,
				ClientKeyData:         restConfig.KeyData,
				Username:              restConfig.Username,
				Password:              restConfig.Password,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"default": {
				Cluster:  "default",
				AuthInfo: "default",
			},
		},
		CurrentContext: "default",
	}
	configBytes, err := clientcmd.Write(config)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() {
		By("tearing down the test environment")
		stopEnv()
		ExternalResources.Wait()
	})
	return configBytes
}, func(configBytes []byte) {
	By("connecting to the test environment")
	if k8sClient != nil {
		return
	}
	config, err := clientcmd.Load(configBytes)
	Expect(err).NotTo(HaveOccurred())
	restConfig, err := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{}).ClientConfig()
	restConfig.QPS = 1000.0
	restConfig.Burst = 2000.0
	Expect(err).NotTo(HaveOccurred())
	k8sClient, err = client.New(restConfig, client.Options{
		Scheme: scheme,
	})
	Expect(err).NotTo(HaveOccurred())
	kmatch.SetDefaultObjectClient(k8sClient)
})

func makeTestNamespace() string {
	for i := 0; i < 100; i++ {
		ns := fmt.Sprintf("test-%d", i)
		if err := k8sClient.Create(
			context.Background(),
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: ns,
					Annotations: map[string]string{
						"controller-test": "true",
					},
				},
			},
		); err != nil {
			continue
		}
		return ns
	}
	panic("could not create namespace")
}

func updateObject(existing client.Object, patchFn interface{}) {
	patchFnValue := reflect.ValueOf(patchFn)
	if patchFnValue.Kind() != reflect.Func {
		panic("patchFn must be a function")
	}
	var lastErr error
	waitErr := wait.ExponentialBackoff(wait.Backoff{
		Duration: 10 * time.Millisecond,
		Factor:   2,
		Steps:    10,
	}, func() (bool, error) {
		// Make a copy of the existing object
		existingCopy := existing.DeepCopyObject().(client.Object)
		// Get the latest version of the object
		lastErr = k8sClient.Get(context.Background(),
			client.ObjectKeyFromObject(existingCopy), existingCopy)
		if lastErr != nil {
			return false, nil
		}
		// Call the patchFn to make changes to the object
		patchFnValue.Call([]reflect.Value{reflect.ValueOf(existingCopy)})
		// Apply the patch
		lastErr = k8sClient.Update(context.Background(), existingCopy, &client.UpdateOptions{})
		if lastErr != nil {
			return false, nil
		}
		// Replace the existing object with the new one
		existing = existingCopy
		return true, nil // exit backoff loop
	})
	if waitErr != nil {
		Fail("failed to update object: " + lastErr.Error())
	}
}

type opniClusterOpts struct {
	Name                string
	Namespace           string
	Models              []string
	DisableOpniServices bool
	PrometheusEndpoint  string
	UsePrometheusRef    bool
}

func buildAICluster(opts opniClusterOpts) *aiv1beta1.OpniCluster {
	imageSpec := opnimeta.ImageSpec{
		ImagePullPolicy: (*corev1.PullPolicy)(lo.ToPtr(string(corev1.PullNever))),
		ImagePullSecrets: []corev1.LocalObjectReference{
			{
				Name: "lorem-ipsum",
			},
		},
	}
	return &aiv1beta1.OpniCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: aiv1beta1.GroupVersion.String(),
			Kind:       "OpniCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: opts.Name,
			Namespace: func() string {
				if opts.Namespace == "" {
					return makeTestNamespace()
				}
				return opts.Namespace
			}(),
		},
		Spec: aiv1beta1.OpniClusterSpec{
			Version:     "test",
			DefaultRepo: lo.ToPtr("docker.biz/rancher"), // nonexistent repo
			GlobalNodeSelector: map[string]string{
				"foo": "bar",
			},
			GlobalTolerations: []corev1.Toleration{
				{
					Key:      "foo",
					Operator: corev1.TolerationOpExists,
				},
			},
			Services: aiv1beta1.ServicesSpec{
				Inference: aiv1beta1.InferenceServiceSpec{
					Enabled:   lo.ToPtr(!opts.DisableOpniServices),
					ImageSpec: imageSpec,
					PretrainedModels: func() []corev1.LocalObjectReference {
						var ret []corev1.LocalObjectReference
						for _, model := range opts.Models {
							ret = append(ret, corev1.LocalObjectReference{
								Name: model,
							})
						}
						return ret
					}(),
				},
				Drain: aiv1beta1.DrainServiceSpec{
					Enabled:   lo.ToPtr(!opts.DisableOpniServices),
					ImageSpec: imageSpec,
				},
				Preprocessing: aiv1beta1.PreprocessingServiceSpec{
					Enabled:   lo.ToPtr(!opts.DisableOpniServices),
					ImageSpec: imageSpec,
				},
				PayloadReceiver: aiv1beta1.PayloadReceiverServiceSpec{
					Enabled:   lo.ToPtr(!opts.DisableOpniServices),
					ImageSpec: imageSpec,
				},
				GPUController: aiv1beta1.GPUControllerServiceSpec{
					Enabled:   lo.ToPtr(!opts.DisableOpniServices),
					ImageSpec: imageSpec,
				},
				TrainingController: aiv1beta1.TrainingControllerServiceSpec{
					Enabled:   lo.ToPtr(!opts.DisableOpniServices),
					ImageSpec: imageSpec,
				},
				Metrics: aiv1beta1.MetricsServiceSpec{
					Enabled:   lo.ToPtr(!opts.DisableOpniServices),
					ImageSpec: imageSpec,
					PrometheusEndpoint: func() string {
						if opts.PrometheusEndpoint != "" {
							return opts.PrometheusEndpoint
						}
						if opts.UsePrometheusRef {
							return ""
						}
						return "http://dummy-endpoint"
					}(),
					PrometheusReference: func() *opnimeta.PrometheusReference {
						if opts.UsePrometheusRef {
							return &opnimeta.PrometheusReference{
								Name:      "test-prometheus",
								Namespace: "prometheus-new",
							}
						}
						return nil
					}(),
					ExtraVolumeMounts: []opnimeta.ExtraVolumeMount{
						{
							Name:      "test-volume",
							MountPath: "/var/test-volume",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}
}

func generateSHAID(name string, namespace string) string {
	hash := sha1.New()
	hash.Write([]byte(name + namespace))
	sum := hash.Sum(nil)
	return fmt.Sprintf("%x", sum[:3])
}

func marshal(hp map[string]intstr.IntOrString) string {
	b, err := json.MarshalIndent(hp, "", "  ")
	Expect(err).NotTo(HaveOccurred())
	return string(b)
}

type Reconciler interface {
	SetupWithManager(ctrl.Manager) error
}

var ExternalResources sync.WaitGroup

func RunTestEnvironment(
	testEnv *envtest.Environment,
	runControllerManager bool,
	externalEnv bool,
	reconcilers ...Reconciler,
) (stop context.CancelFunc, k8sManager ctrl.Manager, k8sClient client.Client) {
	if !externalEnv && len(reconcilers) == 0 {
		panic("no reconcilers")
	}
	var ctx context.Context
	ctx, stop = context.WithCancel(ctrl.SetupSignalHandler())

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())
	ExternalResources.Add(1)

	go func() {
		defer GinkgoRecover()
		defer ExternalResources.Done()
		<-ctx.Done()
		err := testEnv.Stop()
		Expect(err).NotTo(HaveOccurred())
	}()

	if runControllerManager {
		StartControllerManager(ctx, testEnv)
	}

	scheme := apis.NewScheme()

	ports := freeport.GetFreePorts(2)

	// add the opnicluster manager
	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     fmt.Sprintf(":%d", ports[0]),
		HealthProbeBindAddress: fmt.Sprintf(":%d", ports[1]),
	})
	Expect(err).NotTo(HaveOccurred())

	k8sClient = k8sManager.GetClient()
	Expect(k8sClient).NotTo(BeNil())

	for _, rec := range reconcilers {
		Expect(rec.SetupWithManager(k8sManager)).NotTo(HaveOccurred())
	}

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()

	err = k8sClient.Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "opnicluster-test",
		},
	})
	Expect(err).Should(Or(BeNil(), WithTransform(errors.IsAlreadyExists, BeTrue())))

	err = k8sClient.Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "opnidemo-test",
		},
	})
	Expect(err).Should(Or(BeNil(), WithTransform(errors.IsAlreadyExists, BeTrue())))

	err = k8sClient.Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "logadapter-test",
		},
	})
	Expect(err).Should(Or(BeNil(), WithTransform(errors.IsAlreadyExists, BeTrue())))
	return
}

var defaultControllers = []string{
	"cronjob",
	"daemonset",
	"deployment",
	"garbagecollector",
	"namespace",
	"replicaset",
	"service",
	"serviceaccount",
	"statefulset",
}

func StartControllerManager(ctx context.Context, testEnv *envtest.Environment) {
	cfg := testEnv.Config
	controllerMgrBin := path.Join(testEnv.BinaryAssetsDirectory, "kube-controller-manager")

	apiCfg := api.Config{
		Clusters: map[string]*api.Cluster{
			"default": {
				Server:                   cfg.Host,
				CertificateAuthorityData: cfg.CAData,
			},
		},
		Contexts: map[string]*api.Context{
			"default": {
				Cluster:  "default",
				AuthInfo: "default",
			},
		},
		AuthInfos: map[string]*api.AuthInfo{
			"default": {
				ClientCertificateData: cfg.CertData,
				ClientKeyData:         cfg.KeyData,
				Token:                 cfg.BearerToken,
				Username:              cfg.Username,
				Password:              cfg.Password,
			},
		},
		CurrentContext: "default",
	}
	port := freeport.GetFreePort()
	clientcmd.WriteToFile(apiCfg, path.Join(testEnv.BinaryAssetsDirectory, "kubeconfig.yaml"))
	cmd := exec.CommandContext(ctx, controllerMgrBin,
		"--kubeconfig", path.Join(testEnv.BinaryAssetsDirectory, "kubeconfig.yaml"),
		"--controllers", strings.Join(defaultControllers, ","),
		"--leader-elect=false",
		"--bind-address=127.0.0.1",
		fmt.Sprintf("--secure-port=%d", port),
		"--enable-garbage-collector",
		"--concurrent-gc-syncs=40",
	)
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	go func() {
		if err := cmd.Start(); err != nil {
			panic(err)
		}
		ExternalResources.Add(1)
		defer ExternalResources.Done()
		if err := cmd.Wait(); err != nil {
			fmt.Fprintln(GinkgoWriter, err)
		}
	}()
	// wait for controller manager to be ready
	Eventually(func() error {
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, //nolint:gosec
				},
			},
		}
		resp, err := client.Get(fmt.Sprintf("https://127.0.0.1:%d/healthz", port))
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("controller manager is not ready yet: %v", resp.Status)
		}
		return nil
	}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
}

func NewTestLogger() logr.Logger {
	return logger.NewLogr(logger.WithWriter(GinkgoWriter), logger.WithLogLevel(slog.LevelInfo))
}
