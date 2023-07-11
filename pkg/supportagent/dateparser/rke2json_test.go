package dateparser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/supportagent/dateparser"
)

var _ = Describe("Day Month Dateparser", Ordered, Label("unit"), func() {
	var (
		parser dateparser.DateParser
		token  string
	)
	When("the log line is from rke2 etcd", func() {
		BeforeEach(func() {
			parser = &dateparser.RKE2EtcdParser{}
		})
		When("the log line is an etcd json log", func() {
			BeforeEach(func() {
				token = `{"level":"info","ts":"2023-06-12T01:20:00.276Z","caller":"embed/serve.go:100","msg":"ready to serve client requests"}`
			})
			It("should parse the date", func() {
				timestamp, log, valid := parser.ParseTimestamp(token)
				Expect(valid).To(BeTrue())
				Expect(timestamp.String()).To(Equal("2023-06-12 01:20:00.276 +0000 UTC"))
				Expect(log).To(Equal(`{"level":"info","ts":"2023-06-12T01:20:00.276Z","caller":"embed/serve.go:100","msg":"ready to serve client requests"}`))
			})
		})
	})
})
