//go:build !e2e
// +build !e2e

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

package controllers

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/kralicky/kmatch"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	k8sClient  client.Client
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
	SetDefaultEventuallyPollingInterval(100 * time.Millisecond)
	SetDefaultConsistentlyDuration(2 * time.Second)
	SetDefaultConsistentlyPollingInterval(100 * time.Millisecond)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(util.NewTestLogger())
	port, err := freeport.GetFreePort()
	Expect(err).NotTo(HaveOccurred())
	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			"../config/crd/bases",
			"../config/crd/logging",
			"../config/crd/nvidia",
			"../config/crd/nfd",
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
	stopEnv, k8sManager, k8sClient = test.RunTestEnvironment(testEnv, true,
		&OpniClusterReconciler{},
		&LogAdapterReconciler{},
		&PretrainedModelReconciler{},
		&LoggingReconciler{},
		&ClusterPolicyReconciler{},
	)
	kmatch.SetDefaultObjectClient(k8sClient)
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	stopEnv()
})

func makeTestNamespace() string {
	for i := 0; i < 100; i++ {
		ns := fmt.Sprintf("test-%d", i)
		if err := k8sClient.Create(
			context.Background(),
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: ns,
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
}

func buildCluster(opts opniClusterOpts) *v1beta1.OpniCluster {
	imageSpec := v1beta1.ImageSpec{
		ImagePullPolicy: (*corev1.PullPolicy)(pointer.String(string(corev1.PullNever))),
		ImagePullSecrets: []corev1.LocalObjectReference{
			{
				Name: "lorem-ipsum",
			},
		},
	}
	return &v1beta1.OpniCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1beta1.GroupVersion.String(),
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
		Spec: v1beta1.OpniClusterSpec{
			Version:     "test",
			DefaultRepo: pointer.String("docker.biz/rancher"), // nonexistent repo
			GlobalNodeSelector: map[string]string{
				"foo": "bar",
			},
			GlobalTolerations: []corev1.Toleration{
				{
					Key:      "foo",
					Operator: corev1.TolerationOpExists,
				},
			},
			Nats: v1beta1.NatsSpec{
				AuthMethod: v1beta1.NatsAuthUsername,
			},
			Services: v1beta1.ServicesSpec{
				Inference: v1beta1.InferenceServiceSpec{
					Enabled:   pointer.Bool(!opts.DisableOpniServices),
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
				Drain: v1beta1.DrainServiceSpec{
					Enabled:   pointer.Bool(!opts.DisableOpniServices),
					ImageSpec: imageSpec,
				},
				Preprocessing: v1beta1.PreprocessingServiceSpec{
					Enabled:   pointer.Bool(!opts.DisableOpniServices),
					ImageSpec: imageSpec,
				},
				PayloadReceiver: v1beta1.PayloadReceiverServiceSpec{
					Enabled:   pointer.Bool(!opts.DisableOpniServices),
					ImageSpec: imageSpec,
				},
				GPUController: v1beta1.GPUControllerServiceSpec{
					Enabled:   pointer.Bool(!opts.DisableOpniServices),
					ImageSpec: imageSpec,
				},
			},
		},
	}
}
