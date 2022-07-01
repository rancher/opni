package rules_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/rancher/opni/pkg/rules"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/notifier"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Prometheus Rule Group Discovery", Ordered, Label("unit", "slow"), func() {
	testGroups1 := []monitoringv1.RuleGroup{
		{
			Name: "test",
			Rules: []monitoringv1.Rule{
				{
					Alert:  "foo",
					Expr:   intstr.FromString("foo"),
					For:    "1m",
					Labels: map[string]string{"foo": "bar"},
				},
			},
			Interval: "1m",
		},
		{
			Name: "test2",
			Rules: []monitoringv1.Rule{
				{
					Alert:  "bar",
					Expr:   intstr.FromString("bar"),
					For:    "2m",
					Labels: map[string]string{"bar": "baz"},
				},
			},
			Interval: "2m",
		},
	}
	testGroups2 := make([]monitoringv1.RuleGroup, len(testGroups1))
	for i, group := range testGroups1 {
		testGroups2[i] = *group.DeepCopy()
	}
	testGroups2[0].Name = "test3"
	testGroups2[1].Name = "test4"
	testGroups3 := []monitoringv1.RuleGroup{
		{
			Name: "test5",
			Rules: []monitoringv1.Rule{
				{
					Alert: "foo",
					Expr:  intstr.FromString("foo"),
					For:   "invalid",
				},
				{
					Record: "bar",
					Expr:   intstr.FromString("bar"),
					For:    "1m", // not allowed in recording rule
				},
				{
					Alert: "baz",
					Expr:  intstr.FromString("baz"),
					For:   "1m",
				},
			},
			Interval: "1m",
		},
		{
			Name: "test6",
			Rules: []monitoringv1.Rule{
				{
					Alert: "baz",
					Expr:  intstr.FromString("baz"),
					For:   "2m",
				},
			},
			Interval: "invalid",
		},
	}

	var k8sClient client.Client
	var finder notifier.Finder[rules.RuleGroup]
	BeforeAll(func() {
		env := test.Environment{
			TestBin: "../../testbin/bin",
			CRDDirectoryPaths: []string{
				"testdata/crds",
			},
		}
		restConfig, err := env.StartK8s()
		Expect(err).NotTo(HaveOccurred())

		scheme := runtime.NewScheme()
		util.Must(clientgoscheme.AddToScheme(scheme))
		util.Must(monitoringv1.AddToScheme(scheme))
		k8sClient, err = client.New(restConfig, client.Options{
			Scheme: scheme,
		})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(env.Stop)

		Expect(k8sClient.Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test1",
			},
		})).To(Succeed())
		Expect(k8sClient.Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test2",
			},
		})).To(Succeed())

		finder = rules.NewPrometheusRuleFinder(k8sClient, rules.WithLogger(test.Log))
	})

	It("should initially find no groups", func() {
		groups, err := finder.Find(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(groups).To(BeEmpty())
	})

	When("creating a PrometheusRule", func() {
		It("should find the groups in the PrometheusRule", func() {
			Expect(k8sClient.Create(context.Background(), &monitoringv1.PrometheusRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test1",
				},
				Spec: monitoringv1.PrometheusRuleSpec{
					Groups: testGroups1,
				},
			})).To(Succeed())

			By("finding the groups in the new PrometheusRule")
			groups, err := finder.Find(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(groups).To(HaveLen(2))
			Expect([]string{
				groups[0].Name,
				groups[1].Name,
			}).To(ContainElements("test", "test2"))
		})
	})
	When("creating a PrometheusRule in a different namespace", func() {
		It("should find PrometheusRules in both namespaces", func() {
			Expect(k8sClient.Create(context.Background(), &monitoringv1.PrometheusRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "test2",
				},
				Spec: monitoringv1.PrometheusRuleSpec{
					Groups: testGroups2,
				},
			})).To(Succeed())
			groups, err := finder.Find(context.Background())

			Expect(err).NotTo(HaveOccurred())
			Expect(groups).To(HaveLen(4))

			Expect([]string{
				groups[0].Name,
				groups[1].Name,
				groups[2].Name,
				groups[3].Name,
			}).To(ContainElements("test", "test2", "test3", "test4"))
		})
	})
	When("creating a PrometheusRule with invalid contents", func() {
		It("should skip invalid rules or groups", func() {
			Expect(k8sClient.Create(context.Background(), &monitoringv1.PrometheusRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-invalid",
					Namespace: "test2",
				},
				Spec: monitoringv1.PrometheusRuleSpec{
					Groups: testGroups3,
				},
			})).To(Succeed())

			groups, err := finder.Find(context.Background())
			Expect(err).NotTo(HaveOccurred())

			// It should skip test6 and 2 of the rules in test5
			Expect(groups).To(HaveLen(5))
			Expect([]string{
				groups[0].Name,
				groups[1].Name,
				groups[2].Name,
				groups[3].Name,
				groups[4].Name,
			}).To(ContainElements("test", "test2", "test3", "test4", "test5"))
			for _, group := range groups {
				if group.Name == "test5" {
					Expect(group.Rules).To(HaveLen(1))
					Expect(group.Rules[0].Alert.Value).To(Equal("baz"))
				}
			}
		})
	})
	It("should allow specifying namespaces to search in", func() {
		finder1 := rules.NewPrometheusRuleFinder(k8sClient, rules.WithNamespaces("test1"))
		groups, err := finder1.Find(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(groups).To(HaveLen(2))
		Expect([]string{
			groups[0].Name,
			groups[1].Name,
		}).To(ContainElements("test", "test2"))

		finder2 := rules.NewPrometheusRuleFinder(k8sClient, rules.WithNamespaces("test2"))
		groups, err = finder2.Find(context.Background())
		Expect(err).NotTo(HaveOccurred())
		Expect(groups).To(HaveLen(3))
		Expect([]string{
			groups[0].Name,
			groups[1].Name,
			groups[2].Name,
		}).To(ContainElements("test3", "test4", "test5"))

		// these should all match both namespaces
		for _, namespaces := range [][]string{
			{""},
			{"test1", "test2"},
			{"test2", "test1", ""},
			{"test1", "test2", "test3"},
		} {
			finder := rules.NewPrometheusRuleFinder(k8sClient,
				rules.WithLogger(test.Log), rules.WithNamespaces(namespaces...))
			groups, err = finder.Find(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(groups).To(HaveLen(5))
		}
	})
})
