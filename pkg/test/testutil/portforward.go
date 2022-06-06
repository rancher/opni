package testutil

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func PortForward(
	ctx context.Context,
	service types.NamespacedName,
	servicePorts []string,
	restConfig *rest.Config,
	scheme *runtime.Scheme,
) ([]portforward.ForwardedPort, error) {
	cl, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}
	svc := &corev1.Service{}
	cl.Get(ctx, service, svc)

	pods := &corev1.PodList{}
	cl.List(ctx, pods, &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set(svc.Spec.Selector)),
	})

	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	for _, pod := range pods.Items {
		transport, upgrader, err := spdy.RoundTripperFor(restConfig)
		if err != nil {
			return nil, err
		}
		u, err := url.Parse(restConfig.Host)
		u.Path = path.Join(u.Path, fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", pod.Namespace, pod.Name))
		dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, u)
		forwarder, err := portforward.New(dialer, servicePorts, stopCh, readyCh, os.Stdout, os.Stderr)
		if err != nil {
			return nil, err
		}
		startErr := make(chan error)
		go func() {
			if err := forwarder.ForwardPorts(); err != nil {
				<-startErr
			}
		}()
		select {
		case <-readyCh:
			go func() {
				<-ctx.Done()
				close(stopCh)
			}()
			return forwarder.GetPorts()
		case err := <-startErr:
			return nil, fmt.Errorf("error starting port forward: %v", err)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, fmt.Errorf("no pods matching the service were found")
}
