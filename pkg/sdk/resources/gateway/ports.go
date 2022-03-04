package gateway

import (
	"fmt"
	"strconv"
	"strings"

	cfgv1beta1 "github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (r *Reconciler) optionalContainerPorts() ([]corev1.ContainerPort, error) {
	var ports []corev1.ContainerPort
	if addr := r.gateway.Spec.Management.GRPCListenAddress; strings.HasPrefix(addr, "tcp://") {
		parts := strings.Split(addr, ":")
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid GRPC listen address %q", addr)
		}
		portNum, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid GRPC listen address %q", addr)
		}
		ports = append(ports, corev1.ContainerPort{
			Name:          "management-grpc",
			ContainerPort: int32(portNum),
			Protocol:      corev1.ProtocolTCP,
		})
	}
	if addr := r.gateway.Spec.Management.HTTPListenAddress; addr != "" {
		parts := strings.Split(addr, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid HTTP listen address %q", addr)
		}
		portNum, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid HTTP listen address %q", addr)
		}
		ports = append(ports, corev1.ContainerPort{
			Name:          "management-http",
			ContainerPort: int32(portNum),
			Protocol:      corev1.ProtocolTCP,
		})
	}
	if addr := r.gateway.Spec.Management.WebListenAddress; addr != "" {
		parts := strings.Split(addr, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid Web listen address %q", addr)
		}
		portNum, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid Web listen address %q", addr)
		}
		ports = append(ports, corev1.ContainerPort{
			Name:          "management-web",
			ContainerPort: int32(portNum),
			Protocol:      corev1.ProtocolTCP,
		})
	}
	if r.gateway.Spec.Auth.Provider == string(cfgv1beta1.AuthProviderNoAuth) {
		ports = append(ports, corev1.ContainerPort{
			Name:          "noauth",
			ContainerPort: int32(r.gateway.Spec.Auth.Noauth.Port),
			Protocol:      corev1.ProtocolTCP,
		})
	}
	return ports, nil
}

func (r *Reconciler) optionalServicePorts() ([]corev1.ServicePort, error) {
	ports, err := r.optionalContainerPorts()
	if err != nil {
		return nil, err
	}
	var svcPorts []corev1.ServicePort
	for _, port := range ports {
		svcPorts = append(svcPorts, corev1.ServicePort{
			Name:       port.Name,
			Port:       port.ContainerPort,
			TargetPort: intstr.FromInt(int(port.ContainerPort)),
			Protocol:   port.Protocol,
		})
	}
	return svcPorts, nil
}
