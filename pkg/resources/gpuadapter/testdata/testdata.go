package testdata

import (
	nvidiav1 "github.com/NVIDIA/gpu-operator/api/v1"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/providers"
	"github.com/rancher/opni/pkg/resources/gpuadapter"
)

var (
	matchingTests []*MatchingTest
)

type TestItem struct {
	Input    *v1beta1.GpuPolicyAdapter
	Provider providers.Provider
	Output   *nvidiav1.ClusterPolicy
}

type MatchFunc func(TestItem) bool
type AssertionFunc func(TestItem)

type MatchingTest struct {
	Matcher   MatchFunc
	Assertion AssertionFunc
}

type testBuilder struct {
	matcher         MatchFunc
	invertedMatcher MatchFunc
}

type iffTestBuilder struct {
	matcher         MatchFunc
	invertedMatcher MatchFunc
}

type TestBuilder interface {
	Then(assertion AssertionFunc) TestBuilder
	Else(assertion AssertionFunc)
}

func If(matcher MatchFunc) TestBuilder {
	return &testBuilder{
		matcher: matcher,
		invertedMatcher: func(ti TestItem) bool {
			return !matcher(ti)
		},
	}
}

func IfAndOnlyIf(matcher MatchFunc) TestBuilder {
	return &iffTestBuilder{
		matcher: matcher,
		invertedMatcher: func(ti TestItem) bool {
			return !matcher(ti)
		},
	}
}

func (b *testBuilder) Then(assertion AssertionFunc) TestBuilder {
	matchingTests = append(matchingTests, &MatchingTest{
		Matcher:   b.matcher,
		Assertion: assertion,
	})
	return b
}

func (b *testBuilder) Else(assertion AssertionFunc) {
	matchingTests = append(matchingTests, &MatchingTest{
		Matcher:   b.invertedMatcher,
		Assertion: assertion,
	})
}

func (b *iffTestBuilder) Then(assertion AssertionFunc) TestBuilder {
	invertedAssertion := func(ti TestItem) {
		Expect(InterceptGomegaFailure(func() {
			assertion(ti)
		})).NotTo(Succeed())
	}

	matchingTests = append(matchingTests, &MatchingTest{
		Matcher:   b.matcher,
		Assertion: assertion,
	})

	matchingTests = append(matchingTests, &MatchingTest{
		Matcher:   b.invertedMatcher,
		Assertion: invertedAssertion,
	})
	return b
}

func (b *iffTestBuilder) Else(assertion AssertionFunc) {
	panic("IfAndOnlyIf does not support Else")
}

func Check(input *v1beta1.GpuPolicyAdapter, provider providers.Provider) {
	Expect(input).NotTo(BeNil())
	output, err := gpuadapter.BuildClusterPolicy(input, provider)
	Expect(err).NotTo(HaveOccurred())
	Expect(output).NotTo(BeNil())
	item := TestItem{
		Input:    input,
		Provider: provider,
		Output:   output,
	}
	for _, t := range matchingTests {
		if t.Matcher(item) {
			t.Assertion(item)
		}
	}
}
