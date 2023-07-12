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
	When("the log line is a journald log", func() {
		BeforeEach(func() {
			parser = dateparser.NewDayMonthParser(
				dateparser.JournaldRegex,
				dateparser.JournaldLayout,
				dateparser.WithYear("2023"),
			)
		})
		When("the log line is a valid journald log line", func() {
			BeforeEach(func() {
				token = `Jun 12 02:21:49 danb-k3s-ds1-pool1-9358e7c0-7jq9t k3s[1525]: I0612 02:21:49.026189    1525 server.go:408] "Kubelet version" kubeletVersion="v1.25.9+k3s1"`
			})
			It("should parse the date", func() {
				timestamp, log, valid := parser.ParseTimestamp(token)
				Expect(valid).To(BeTrue())
				Expect(timestamp.String()).To(Equal("2023-06-12 02:21:49 +0000 UTC"))
				Expect(log).To(Equal(`Jun 12 02:21:49 danb-k3s-ds1-pool1-9358e7c0-7jq9t k3s[1525]: I0612 02:21:49.026189    1525 server.go:408] "Kubelet version" kubeletVersion="v1.25.9+k3s1"`))
			})
		})
	})
	When("the log line is a kubernetes log", func() {
		BeforeEach(func() {
			parser = dateparser.NewDayMonthParser(
				dateparser.KlogRegex,
				dateparser.KlogLayout,
				dateparser.WithYear("2023"),
			)
		})
		When("the log line is a valid kubernetes log line", func() {
			BeforeEach(func() {
				token = `I0612 01:14:46.406489       1 shared_informer.go:259] Caches are synced for node_authorizer`
			})
			It("should parse the date", func() {
				timestamp, log, valid := parser.ParseTimestamp(token)
				Expect(valid).To(BeTrue())
				Expect(timestamp.String()).To(Equal("2023-06-12 01:14:46.406489 +0000 UTC"))
				Expect(log).To(Equal(`I0612 01:14:46.406489       1 shared_informer.go:259] Caches are synced for node_authorizer`))
			})
		})
		When("the log line doesn't have a data", func() {
			BeforeEach(func() {
				token = `Trace[775348432]: ---"About to write a response" 675ms (01:15:16.097)`
			})
			It("should not parse the date", func() {
				_, log, valid := parser.ParseTimestamp(token)
				Expect(valid).To(BeFalse())
				Expect(log).To(Equal(`Trace[775348432]: ---"About to write a response" 675ms (01:15:16.097)`))
			})
		})
	})
})
