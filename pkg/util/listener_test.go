package util_test

import (
	"net"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/util"
)

var _ = Describe("Listener Utils", Ordered, Label("unit"), func() {
	When("the given address uses the tcp or tcp4 scheme", func() {
		It("should return a tcp listener", func() {
			listener, err := util.NewProtocolListener("tcp://:0")
			Expect(err).NotTo(HaveOccurred())
			Expect(listener).To(BeAssignableToTypeOf(&net.TCPListener{}))
			listener.Close()

			listener, err = util.NewProtocolListener("tcp4://:0")
			Expect(err).NotTo(HaveOccurred())
			Expect(listener).To(BeAssignableToTypeOf(&net.TCPListener{}))
			listener.Close()
		})
	})
	When("the given address uses the unix scheme", func() {
		It("should return a socket listener", func() {
			listener, err := util.NewProtocolListener("unix:///tmp/opni-monitoring-test-util.sock")
			Expect(err).NotTo(HaveOccurred())
			Expect(listener).To(BeAssignableToTypeOf(&net.UnixListener{}))
			listener.Close()
		})
		It("should create the socket's parent directory if needed", func() {
			os.RemoveAll("/tmp/opni-monitoring-test-util-dir")
			listener, err := util.NewProtocolListener(
				"unix:///tmp/opni-monitoring-test-util-dir/opni-monitoring-test-util.sock")
			Expect(err).NotTo(HaveOccurred())
			Expect(listener).To(BeAssignableToTypeOf(&net.UnixListener{}))
			_, err = os.Stat("/tmp/opni-monitoring-test-util-dir")
			Expect(err).NotTo(HaveOccurred())
			listener.Close()
			Expect(os.Remove("/tmp/opni-monitoring-test-util-dir")).To(Succeed())
		})
		It("should clean up existing sockets before creating a new one", func() {
			By("creating a socket")
			listener, err := util.NewProtocolListener(
				"unix:///tmp/opni-monitoring-test-util-dir/opni-monitoring-test-util.sock")
			Expect(err).NotTo(HaveOccurred())
			Expect(listener).To(BeAssignableToTypeOf(&net.UnixListener{}))

			By("ensuring the socket exists")
			_, err = os.Stat("/tmp/opni-monitoring-test-util-dir/opni-monitoring-test-util.sock")
			Expect(err).NotTo(HaveOccurred())

			By("creating a new socket in its place")
			listener, err = util.NewProtocolListener(
				"unix:///tmp/opni-monitoring-test-util-dir/opni-monitoring-test-util.sock")
			Expect(err).NotTo(HaveOccurred())
			Expect(listener).To(BeAssignableToTypeOf(&net.UnixListener{}))
			listener.Close()

			By("creating a non-empty directory where the socket should be")
			Expect(os.Mkdir("/tmp/opni-monitoring-test-util-dir/opni-monitoring-test-util.sock", 0700)).To(Succeed())
			os.Create("/tmp/opni-monitoring-test-util-dir/opni-monitoring-test-util.sock/foo")

			By("creating a new socket where the non-empty directory is")
			listener, err = util.NewProtocolListener(
				"unix:///tmp/opni-monitoring-test-util-dir/opni-monitoring-test-util.sock")
			Expect(err).To(HaveOccurred())

			os.RemoveAll("/tmp/opni-monitoring-test-util-dir")
		})
	})
	When("the given address uses an unsupported scheme", func() {
		It("should return an error", func() {
			_, err := util.NewProtocolListener("foo://:0")
			Expect(err).To(HaveOccurred())
		})
	})
	When("an invalid address is given", func() {
		It("should return an error", func() {
			_, err := util.NewProtocolListener("")
			Expect(err).To(HaveOccurred())

			_, err = util.NewProtocolListener("tcp://")
			Expect(err).To(HaveOccurred())

			_, err = util.NewProtocolListener(string([]byte{0x7f}))
			Expect(err).To(HaveOccurred())
		})
	})
})
