package dateparser_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/supportagent/dateparser"
)

var _ = Describe("Day Month Dateparser", Ordered, Label("unit"), func() {
	var (
		parser dateparser.DateParser
		token  string
	)
	When("the log line is from rancher", func() {
		BeforeEach(func() {
			parser = &dateparser.MultipleParser{
				Dateformats: []dateparser.Dateformat{
					{
						DateRegex: dateparser.RancherRegex,
						Layout:    dateparser.RancherLayout,
					},
					{
						DateRegex:  dateparser.KlogRegex,
						Layout:     dateparser.KlogLayout,
						DateSuffix: fmt.Sprintf(" %s %s", "UTC", "2023"),
					},
				},
			}
		})
		When("the log line uses the rancher timestamp", func() {
			BeforeEach(func() {
				token = `2023/07/07 03:43:46 [ERROR] Failed to handle tunnel request from remote address 192.168.125.138:49186 (X-Forwarded-For: 192.168.202.248): response 401: failed authentication`
			})
			It("should parse the date", func() {
				timestamp, log, valid := parser.ParseTimestamp(token)
				Expect(valid).To(BeTrue())
				Expect(timestamp.String()).To(Equal("2023-07-07 03:43:46 +0000 UTC"))
				Expect(log).To(Equal(`2023/07/07 03:43:46 [ERROR] Failed to handle tunnel request from remote address 192.168.125.138:49186 (X-Forwarded-For: 192.168.202.248): response 401: failed authentication`))
			})
		})
		When("the log line uses the klog timestamp", func() {
			BeforeEach(func() {
				token = `W0707 03:43:48.432607      33 warnings.go:80] policy/v1beta1 PodSecurityPolicy is deprecated in v1.21+, unavailable in v1.25+`
			})
			It("should parse the date", func() {
				timestamp, log, valid := parser.ParseTimestamp(token)
				Expect(valid).To(BeTrue())
				Expect(timestamp.String()).To(Equal("2023-07-07 03:43:48.432607 +0000 UTC"))
				Expect(log).To(Equal(`W0707 03:43:48.432607      33 warnings.go:80] policy/v1beta1 PodSecurityPolicy is deprecated in v1.21+, unavailable in v1.25+`))
			})
		})
	})
})
