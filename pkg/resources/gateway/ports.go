package gateway

import (
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (r *Reconciler) publicContainerPorts() ([]corev1.ContainerPort, error) {
	ports := []corev1.ContainerPort{
		{
			Name:          "grpc",
			ContainerPort: 9090,
			Protocol:      corev1.ProtocolTCP,
		},
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
	if addr := r.gw.Spec.Config.GetManagement().GetGrpcListenAddress(); addr != "" {
		parts := strings.Split(addr, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid HTTP listen address %q", addr)
		}
		portNum, err := strconv.ParseInt(parts[1], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid HTTP listen address %q", addr)
		}
		ports = append(ports, corev1.ContainerPort{
			Name:          "management-grpc",
			ContainerPort: int32(portNum),
			Protocol:      corev1.ProtocolTCP,
		})
	}
	if addr := r.gw.Spec.Config.GetManagement().GetHttpListenAddress(); addr != "" {
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
	if addr := r.gw.Spec.Config.GetDashboard().GetHttpListenAddress(); addr != "" {
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
