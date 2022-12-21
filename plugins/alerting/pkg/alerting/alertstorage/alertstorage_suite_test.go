package alertstorage_test

import (
	"testing"

	"github.com/nats-io/nats.go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test"
)

func TestAlertstorage(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Alertstorage Suite")
}

var embeddedJetstream nats.JetStreamContext
var testKv nats.KeyValue
var testKv2 nats.KeyValue
var testKv3 nats.KeyValue
var env *test.Environment

var _ = BeforeSuite(func() {
	env = &test.Environment{
		TestBin: "../../../../../testbin/bin",
	}
	nc, err := env.StartEmbeddedJetstream()
	Expect(err).NotTo(HaveOccurred())
	embeddedJetstream, err = nc.JetStream()
	Expect(err).NotTo(HaveOccurred())
	Expect(embeddedJetstream).NotTo(BeNil())
	kv, err := embeddedJetstream.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      "test-kv",
		Description: "bucket for testing",
	})
	Expect(err).To(Succeed())
	Expect(kv).NotTo(BeNil())
	kv2, err := embeddedJetstream.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      "test-kv2",
		Description: "bucket for testing",
	})
	Expect(err).To(Succeed())
	Expect(kv2).NotTo(BeNil())
	kv3, err := embeddedJetstream.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      "test-kv3",
		Description: "bucket for testing",
	})
	Expect(err).To(Succeed())
	Expect(kv3).NotTo(BeNil())

	testKv = kv
	testKv2 = kv2
	testKv3 = kv3
})
