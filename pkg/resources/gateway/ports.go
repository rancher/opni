package gateway

import (
	"fmt"
	"strconv"
	"strings"

	cfgv1beta1 "github.com/rancher/opni/pkg/config/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func (r *Reconciler) publicContainerPorts() ([]corev1.ContainerPort, error) {
	lg := r.lg
	ports := []corev1.ContainerPort{
		{
			Name:          "grpc",
			ContainerPort: 9090,
			Protocol:      corev1.ProtocolTCP,
		},
	}
	if r.gw.Spec.Auth.Provider == cfgv1beta1.AuthProviderNoAuth {
		if r.gw.Spec.Auth.Noauth == nil {
			return nil, field.Required(field.NewPath("spec", "auth", "noauth"),
				"must provide noauth config when it is used as the auth provider")
		}
		noauthPort := r.gw.Spec.Auth.Noauth.Port
		if noauthPort == 0 {
			lg.Warn("noauth port is not set, using default port 4000")
			noauthPort = 4000
		}
		ports = append(ports, corev1.ContainerPort{
			Name:          "noauth",
			ContainerPort: int32(noauthPort),
			Protocol:      corev1.ProtocolTCP,
		})
	}
	return ports, nil
}

func (r *Reconciler) internalContainerPorts() ([]corev1.ContainerPort, error) {
	ports := []corev1.ContainerPort{
		{
			Name:          "http",
			ContainerPort: 8080,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "metrics",
			ContainerPort: 8086,
			Protocol:      corev1.ProtocolTCP,
		},
	}
	if addr := r.gw.Spec.Management.GetGRPCListenAddress(); strings.HasPrefix(addr, "tcp://") {
		parts := strings.Split(addr, ":")
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid GRPC listen address %q", addr)
		}
		portNum, err := strconv.ParseInt(parts[2], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid GRPC listen address %q", addr)
		}
		ports = append(ports, corev1.ContainerPort{
			Name:          "management-grpc",
			ContainerPort: int32(portNum),
			Protocol:      corev1.ProtocolTCP,
		})
	}
	if addr := r.gw.Spec.Management.GetHTTPListenAddress(); addr != "" {
		parts := strings.Split(addr, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid HTTP listen address %q", addr)
		}
		portNum, err := strconv.ParseInt(parts[1], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid HTTP listen address %q", addr)
		}
		ports = append(ports, corev1.ContainerPort{
			Name:          "management-http",
			ContainerPort: int32(portNum),
			Protocol:      corev1.ProtocolTCP,
		})
	}
	return ports, nil
}

func (r *Reconciler) adminDashboardContainerPorts() ([]corev1.ContainerPort, error) {
	var ports []corev1.ContainerPort
	if addr := r.gw.Spec.Management.GetWebListenAddress(); addr != "" {
		parts := strings.Split(addr, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid Web listen address %q", addr)
		}
		portNum, err := strconv.ParseInt(parts[1], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid Web listen address %q", addr)
		}
		ports = append(ports, corev1.ContainerPort{
			Name:          "web",
			ContainerPort: int32(portNum),
			Protocol:      corev1.ProtocolTCP,
		})
	}
	return ports, nil
}

func servicePorts(containerPorts []corev1.ContainerPort) []corev1.ServicePort {
	svcPorts := make([]corev1.ServicePort, len(containerPorts))
	for i, port := range containerPorts {
		svcPorts[i] = corev1.ServicePort{
			Name:       port.Name,
			Protocol:   port.Protocol,
			Port:       port.ContainerPort,
			TargetPort: intstr.FromString(port.Name),
		}
	}
	return svcPorts
}
