package freeport

import (
	"syscall"

	"github.com/samber/lo"
)

func GetFreePort() int {
	sockfd := lo.Must1(syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0))
	defer func() { lo.Must0(syscall.Close(sockfd)) }()

	lo.Must0(syscall.SetsockoptInt(sockfd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1))
	lo.Must0(syscall.Bind(sockfd, &syscall.SockaddrInet4{Port: 0, Addr: [4]byte{127, 0, 0, 1}}))
	lo.Must0(syscall.Listen(sockfd, 1))
	s1addr := lo.Must1(syscall.Getsockname(sockfd))

	sockfd2 := lo.Must1(syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0))
	defer func() { lo.Must0(syscall.Close(sockfd2)) }()

	lo.Must0(syscall.Connect(sockfd2, s1addr))
	nfd, _ := lo.Must2(syscall.Accept(sockfd))
	defer func() { lo.Must0(syscall.Close(nfd)) }()
	return s1addr.(*syscall.SockaddrInet4).Port
}

func GetFreePorts(n int) []int {
	ports := make([]int, n)
	for i := range ports {
		ports[i] = GetFreePort()
	}
	return ports
}
