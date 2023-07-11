package stream_test

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "github.com/rancher/opni/pkg/test/setup"
)

func TestStream(t *testing.T) {
	RegisterFailHandler(func(message string, callerSkip ...int) {
		fmt.Println(message)
		Fail(message, callerSkip...)
	})
	RunSpecs(t, "Stream API Extensions Suite")
}
