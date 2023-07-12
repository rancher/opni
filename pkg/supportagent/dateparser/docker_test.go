package dateparser_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/supportagent/dateparser"
)

var _ = Describe("Docker Dateparser", Ordered, Label("unit"), func() {
	var (
		parser dateparser.DateParser
		token  string
	)
	When("the log line is a valid docker etcd log", func() {
		BeforeEach(func() {
			parser = &dateparser.DockerParser{
				TimestampRegex: dateparser.EtcdRegex,
			}
			token = `2023-07-05T03:40:36.752853267Z {"level":"info","ts":"2023-07-05T03:40:36.752Z","caller":"mvcc/index.go:214","msg":"compact tree index","revision":1994}`
		})
		It("should parse the timestamp correctly", func() {
			timestamp, log, valid := parser.ParseTimestamp(token)
			Expect(valid).To(BeTrue())
			Expect(timestamp.String()).To(Equal("2023-07-05 03:40:36.752853267 +0000 UTC"))
			Expect(log).To(Equal(`{"level":"info","ts":"2023-07-05T03:40:36.752Z","caller":"mvcc/index.go:214","msg":"compact tree index","revision":1994}`))
		})
	})
	When("the log line is from a kubernetes component", func() {
		BeforeEach(func() {
			parser = &dateparser.DockerParser{
				TimestampRegex: dateparser.KlogRegex,
			}
		})
		When("the log line contains a valid klog timestamp", func() {
			BeforeEach(func() {
				token = `2023-07-05T03:32:47.785150492Z I0705 03:32:47.784673       1 trace.go:205] Trace[1710321054]: "List" url:/api/v1/limitranges,user-agent:rancher/v0.0.0 (linux/amd64) kubernetes/$Format cluster c-8z5tw,audit-id:18b80caf-19cb-489d-bc93-4ecfdf1f7419,client:54.191.144.79,accept:application/vnd.kubernetes.protobuf, application/json,protocol:HTTP/1.1 (05-Jul-2023 03:32:44.783) (total time: 3000ms):`
			})
			It("should parse the timestamp correctly", func() {
				timestamp, log, valid := parser.ParseTimestamp(token)
				Expect(valid).To(BeTrue())
				Expect(timestamp.String()).To(Equal("2023-07-05 03:32:47.785150492 +0000 UTC"))
				Expect(log).To(Equal(`I0705 03:32:47.784673       1 trace.go:205] Trace[1710321054]: "List" url:/api/v1/limitranges,user-agent:rancher/v0.0.0 (linux/amd64) kubernetes/$Format cluster c-8z5tw,audit-id:18b80caf-19cb-489d-bc93-4ecfdf1f7419,client:54.191.144.79,accept:application/vnd.kubernetes.protobuf, application/json,protocol:HTTP/1.1 (05-Jul-2023 03:32:44.783) (total time: 3000ms):`))
			})
		})
		When("the log line doesn't contain a valid klog timestamp", func() {
			BeforeEach(func() {
				token = `2023-07-05T03:32:47.785155828Z Trace[1710321054]: [3.000859539s] [3.000859539s] END`
			})
			It("should parse the timestamp correctly", func() {
				_, log, valid := parser.ParseTimestamp(token)
				Expect(valid).To(BeFalse())

				Expect(log).To(Equal(`Trace[1710321054]: [3.000859539s] [3.000859539s] END`))
			})
		})
	})
})
