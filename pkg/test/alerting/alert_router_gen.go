package alerting

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"path"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/client"
	"github.com/rancher/opni/pkg/alerting/drivers/config"
	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	"github.com/rancher/opni/pkg/alerting/interfaces"
	"github.com/rancher/opni/pkg/alerting/message"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/freeport"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gopkg.in/yaml.v2"
)

const (
	OpCreate = iota
	OpUpdate
	OpDelete
)

type MockIntegrationWebhookServer struct {
	EndpointId string
	Webhook    string
	Port       int
	Addr       string
}

func (m *MockIntegrationWebhookServer) GetWebhook() string {
	webhook := url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s:%d", m.Addr, m.Port),
	}
	return webhook.String()
}

func (m *MockIntegrationWebhookServer) Endpoint() *alertingv1.AlertEndpoint {
	return &alertingv1.AlertEndpoint{
		Name:        fmt.Sprintf("mock-integration-%s", m.EndpointId),
		Description: fmt.Sprintf("mock integration description %s", m.EndpointId),
		Endpoint: &alertingv1.AlertEndpoint_Webhook{
			Webhook: &alertingv1.WebhookEndpoint{
				Url: m.GetWebhook(),
			},
		},
		Id: m.EndpointId,
	}
}

func CreateWebhookServer(num int) []*MockIntegrationWebhookServer {
	var servers []*MockIntegrationWebhookServer
	for i := 0; i < num; i++ {
		servers = append(servers, NewWebhookMemoryServer("webhook"))
	}
	return servers
}

func NewWebhookMemoryServer(webHookRoute string) *MockIntegrationWebhookServer {
	return &MockIntegrationWebhookServer{
		EndpointId: shared.NewAlertingRefId("webhook"),
		Webhook:    webHookRoute,
		Port:       3000,
		Addr:       "127.0.0.1",
	}
}

type NamespaceSubTreeTestcase struct {
	Namespace   string
	ConditionId string
	Endpoints   *alertingv1.FullAttachedEndpoints
	Op          int
	*codes.Code
}

type DefaultNamespaceSubTreeTestcase struct {
	Endpoints []*alertingv1.AlertEndpoint
	*codes.Code
}

type IndividualEndpointTestcase struct {
	EndpointId     string
	UpdateEndpoint *alertingv1.AlertEndpoint
	Op             int
	*codes.Code
}

// the flakiest tests you could ever imagine !!!
func CreateRandomSetOfEndpoints() map[string]*alertingv1.FullAttachedEndpoint {
	endpoints := make(map[string]*alertingv1.FullAttachedEndpoint)
	for i := 0; i < 8; i++ {
		uuid := shared.NewAlertingRefId("endp")
		endpoints[uuid] = GenerateEndpoint(i, uuid)
	}
	return endpoints
}

func GenerateEndpoint(genId int, uuid string) *alertingv1.FullAttachedEndpoint {
	n := 4
	if genId%n == 0 {
		return &alertingv1.FullAttachedEndpoint{
			EndpointId: uuid,
			AlertEndpoint: &alertingv1.AlertEndpoint{
				Id:          uuid,
				Name:        fmt.Sprintf("test-%s", uuid),
				Description: fmt.Sprintf("description test-%s", uuid),
				Endpoint: &alertingv1.AlertEndpoint_Slack{
					Slack: &alertingv1.SlackEndpoint{
						WebhookUrl: fmt.Sprintf("https://slack.com/%s", uuid),
						Channel:    fmt.Sprintf("#test-%s", uuid),
					},
				},
			},
			Details: &alertingv1.EndpointImplementation{
				Title:        fmt.Sprintf("slack-%s", uuid),
				Body:         fmt.Sprintf("body-%s", uuid),
				SendResolved: lo.ToPtr(false),
			},
		}
	}
	if genId%n == 1 {
		return &alertingv1.FullAttachedEndpoint{
			EndpointId: uuid,
			AlertEndpoint: &alertingv1.AlertEndpoint{
				Id:          uuid,
				Name:        fmt.Sprintf("test-%s", uuid),
				Description: fmt.Sprintf("description test-%s", uuid),
				Endpoint: &alertingv1.AlertEndpoint_Email{
					Email: &alertingv1.EmailEndpoint{
						To:               fmt.Sprintf("%s-to@gmail.com", uuid),
						SmtpFrom:         lo.ToPtr(fmt.Sprintf("%s-from@gmail.com", uuid)),
						SmtpSmartHost:    lo.ToPtr("localhost:55567"),
						SmtpAuthUsername: lo.ToPtr("test"),
						SmtpAuthPassword: lo.ToPtr("password"),
						SmtpRequireTLS:   lo.ToPtr(false),
						SmtpAuthIdentity: lo.ToPtr("none"),
					},
				},
			},
			Details: &alertingv1.EndpointImplementation{
				Title:        fmt.Sprintf("email-%s", uuid),
				Body:         fmt.Sprintf("body-%s", uuid),
				SendResolved: lo.ToPtr(false),
			},
		}
	}
	if genId%n == 2 {
		return &alertingv1.FullAttachedEndpoint{
			EndpointId: uuid,
			AlertEndpoint: &alertingv1.AlertEndpoint{
				Id:          uuid,
				Name:        fmt.Sprintf("test-%s", uuid),
				Description: fmt.Sprintf("description test-%s", uuid),
				Endpoint: &alertingv1.AlertEndpoint_PagerDuty{
					PagerDuty: &alertingv1.PagerDutyEndpoint{
						IntegrationKey: fmt.Sprintf("https://pagerduty.com/%s", uuid),
					},
				},
			},
			Details: &alertingv1.EndpointImplementation{
				Title:        fmt.Sprintf("pagerduty-%s", uuid),
				Body:         fmt.Sprintf("body-%s", uuid),
				SendResolved: lo.ToPtr(false),
			},
		}
	}
	if genId%n == 3 {
		return &alertingv1.FullAttachedEndpoint{
			EndpointId: uuid,
			AlertEndpoint: &alertingv1.AlertEndpoint{
				Id:          uuid,
				Name:        fmt.Sprintf("test-%s", uuid),
				Description: fmt.Sprintf("description test-%s", uuid),
				Endpoint: &alertingv1.AlertEndpoint_Webhook{
					Webhook: &alertingv1.WebhookEndpoint{
						Url: fmt.Sprintf("https://webhook.com/%s", uuid),
					},
				},
			},
			Details: &alertingv1.EndpointImplementation{
				Title:        fmt.Sprintf("webhook-%s", uuid),
				Body:         fmt.Sprintf("body-%s", uuid),
				SendResolved: lo.ToPtr(false),
			},
		}
	}
	return nil
}

func testNamespaces() []string {
	return []string{"disconnect", "capability", "prometheusQuery", "cpu", "memory", "backend", "aiops"}
}

func CreateRandomNamespacedTestCases(n int, endpSet map[string]*alertingv1.FullAttachedEndpoint) []NamespaceSubTreeTestcase {
	testcases := []NamespaceSubTreeTestcase{}

	// create
	for i := 0; i < n; i++ {
		testcase := lo.Samples(lo.Keys(endpSet), rand.Intn(len(endpSet))+1)
		namespace := lo.Sample(testNamespaces())
		conditionId := shared.NewAlertingRefId()
		testcases = append(testcases, NamespaceSubTreeTestcase{
			ConditionId: conditionId,
			Namespace:   namespace,
			Endpoints: &alertingv1.FullAttachedEndpoints{
				Items: lo.Values(lo.PickByKeys(endpSet, testcase)),
				Details: &alertingv1.EndpointImplementation{
					Title: fmt.Sprintf("[ORIGINAL] testcase-%s", conditionId),
					Body:  fmt.Sprintf("[ORIGINAL] testcase-%s body", conditionId),
				},
			},
			Op:   OpCreate,
			Code: nil,
		})
	}

	// delete
	toDelete := lo.Samples(testcases, rand.Intn(len(testcases))/2+1)
	for _, tc := range toDelete {
		testcases = append(testcases, NamespaceSubTreeTestcase{
			ConditionId: tc.ConditionId,
			Endpoints:   nil,
			Namespace:   tc.Namespace,
			Op:          OpDelete,
			Code:        nil,
		})
	}

	//update
	for _, tc := range testcases {
		testcase := lo.Samples(lo.Keys(endpSet), rand.Intn(len(endpSet))+1)
		conditionId := tc.ConditionId
		namespace := tc.Namespace
		testcases = append(testcases, NamespaceSubTreeTestcase{
			ConditionId: conditionId,
			Namespace:   namespace,
			Endpoints: &alertingv1.FullAttachedEndpoints{
				Items: lo.Values(lo.PickByKeys(endpSet, testcase)),
				Details: &alertingv1.EndpointImplementation{
					Title: fmt.Sprintf("[UPDATED] testcase-%s to Endpoints %s", conditionId, testcase),
					Body:  fmt.Sprintf("[UPDATED] testcase-%s body", conditionId),
				},
			},
			Op:   OpUpdate,
			Code: nil,
		})
	}
	return testcases
}

func CreateRandomDefaultNamespacedTestcases(endpSet map[string]*alertingv1.FullAttachedEndpoint) []DefaultNamespaceSubTreeTestcase {
	testcases := []DefaultNamespaceSubTreeTestcase{}

	for i := 0; i < rand.Intn(5)+1; i++ {
		testcase := lo.Samples(lo.Keys(endpSet), rand.Intn(len(endpSet))+1)
		endps := lo.Map(lo.Values(lo.PickByKeys(endpSet, testcase)), func(r *alertingv1.FullAttachedEndpoint, _ int) *alertingv1.AlertEndpoint {
			return r.AlertEndpoint
		})

		testcases = append(testcases, DefaultNamespaceSubTreeTestcase{
			Endpoints: endps,
		})
	}
	return testcases
}

func CreateRandomIndividualEndpointTestcases(endpSet map[string]*alertingv1.FullAttachedEndpoint) []IndividualEndpointTestcase {
	testcases := []IndividualEndpointTestcase{}

	// update
	for i := 0; i < 10+1; i++ {
		testcase := lo.Samples(lo.Keys(endpSet), 2)
		to, from := testcase[0], testcase[1]
		testcases = append(testcases, IndividualEndpointTestcase{
			EndpointId:     endpSet[from].EndpointId,
			UpdateEndpoint: endpSet[to].AlertEndpoint,
			Op:             OpUpdate,
			Code:           nil,
		})
	}

	// delete
	toDelete := lo.Samples(lo.Keys(endpSet), rand.Intn(len(endpSet))+1)
	for _, id := range toDelete {
		testcases = append(testcases, IndividualEndpointTestcase{
			EndpointId:     id,
			UpdateEndpoint: nil,
			Op:             OpDelete,
			Code:           nil,
		})
	}

	return testcases
}

func RunAlertManager(
	e *test.Environment,
	router routing.OpniRouting,
	tmpWritePath string,
	debugFile string,
) (int, context.CancelFunc) {
	ctx := e.Context()
	By("expecting to successfully build a config")
	cfg, err := router.BuildConfig()
	Expect(err).To(Succeed())
	By("checking that the config passes the marshalling validation")
	bytes, err := yaml.Marshal(cfg)
	Expect(err).To(Succeed())
	if _, ok := os.LookupEnv("OPNI_ROUTING_DEBUG"); ok {
		err = os.WriteFile(debugFile, bytes, 0644)
		Expect(err).To(Succeed())
	}
	By("checking that the config passes the unmarshalling validation")
	var c config.Config
	err = yaml.Unmarshal(bytes, &c)
	Expect(err).To(Succeed())
	By("writing the config file to a temporary directory")
	tmpFileName := shared.NewAlertingRefId("config")
	tmpPath := path.Join(tmpWritePath, tmpFileName)
	err = os.MkdirAll(path.Dir(tmpPath), 0755)
	Expect(err).To(Succeed())
	err = os.WriteFile(tmpPath, bytes, 0644)
	Expect(err).To(Succeed())
	if _, ok := os.LookupEnv("OPNI_ROUTING_DEBUG"); ok {
		err = os.WriteFile(debugFile, bytes, 0644)
		Expect(err).To(Succeed())
	}

	By("Verifying that the config can be loaded by alertmanager")
	freePort := freeport.GetFreePort()
	ctxCa, caF := context.WithCancel(ctx)
	ports := e.StartEmbeddedAlertManager(ctxCa, tmpPath, lo.ToPtr(freePort))
	alertingClient, err := client.NewClient(
		client.WithAlertManagerAddress(
			fmt.Sprintf("127.0.0.1:%d", ports.ApiPort),
		),
		client.WithQuerierAddress(
			fmt.Sprintf("127.0.0.1:%d", ports.EmbeddedPort),
		),
	)
	Expect(err).To(Succeed())
	Eventually(func() error {
		return alertingClient.Ready(ctxCa)
	}).Should(Succeed())
	return ports.ApiPort, caF
}

func ExpectAlertManagerConfigToBeValid(
	ctx context.Context,
	env *test.Environment,
	tmpWritePath string,
	writeFile string,
	cfg *config.Config,
	port int,
) {
	By("checking that the config passes the marshalling validation")
	bytes, err := yaml.Marshal(cfg)
	Expect(err).To(Succeed())
	if _, ok := os.LookupEnv("OPNI_ROUTING_DEBUG"); ok {
		err = os.WriteFile("./test.yaml", bytes, 0644)
		Expect(err).To(Succeed())
	}

	By("checking that the config passes the unmarshalling validation")
	var c config.Config
	err = yaml.Unmarshal(bytes, &c)
	Expect(err).To(Succeed())
	By("writing the config file to a temporary directory")
	tmpFileName := shared.NewAlertingRefId("config")
	tmpPath := path.Join(tmpWritePath, tmpFileName)
	err = os.MkdirAll(path.Dir(tmpPath), 0755)
	Expect(err).To(Succeed())
	err = os.WriteFile(tmpPath, bytes, 0644)
	Expect(err).To(Succeed())
	if _, ok := os.LookupEnv("OPNI_ROUTING_DEBUG"); ok {
		err = os.WriteFile(writeFile, bytes, 0644)
		Expect(err).To(Succeed())
	}

	By("Verifying that the config can be loaded by alertmanager")
	ports := env.StartEmbeddedAlertManager(ctx, tmpPath, lo.ToPtr(port))
	alertingClient, err := client.NewClient(
		client.WithAlertManagerAddress(
			fmt.Sprintf("127.0.0.1:%d", ports.ApiPort),
		),
		client.WithQuerierAddress(
			fmt.Sprintf("127.0.0.1:%d", ports.EmbeddedPort),
		),
	)
	Expect(err).To(Succeed())
	Eventually(func() error {
		return alertingClient.Ready(ctx)
	}).Should(Succeed())

}

func ExpectToRecoverConfig(router routing.OpniRouting, name ...string) {
	initialConstruct, err := router.BuildConfig()
	Expect(err).To(Succeed())
	By("expecting it to be marshallable/unmarshallable")
	raw, err := yaml.Marshal(initialConstruct)
	Expect(err).To(Succeed())

	err = yaml.Unmarshal(raw, &config.Config{})
	Expect(err).To(Succeed())
	By("expecting both router constructs to be valid AlertManager configs")
	recoveredConstruct, err := router.BuildConfig()
	Expect(err).To(Succeed())
	ic, rc := &config.Config{}, &config.Config{}

	// a little messy but we need to unmarshal to yaml to pass the configs through validation
	err = yaml.Unmarshal(util.Must(yaml.Marshal(initialConstruct)), ic)
	Expect(err).To(Succeed())
	err = yaml.Unmarshal(util.Must(yaml.Marshal(recoveredConstruct)), rc)
	Expect(err).To(Succeed())
	By("expecting the recovered router to match the saved router")
	// the unmarshalled configs need to be checked, since some invalid configs can be patched during unmarshalling
	ExpectConfigEqual(ic, rc, name...)
}

func ExpectRouterEqual(r1, r2 routing.OpniRouting, name ...string) {
	c1, c2 := util.Must(r1.BuildConfig()), util.Must(r2.BuildConfig())
	ExpectConfigEqual(c1, c2, name...)

}

func ExpectRouterNotEqual(r1, r2 routing.OpniRouting, name ...string) {
	c1, c2 := util.Must(r1.BuildConfig()), util.Must(r2.BuildConfig())
	ExpectConfigNotEqual(c1, c2, name...)
}

func ExpectConfigEqual(c1, c2 *config.Config, name ...string) {
	if len(name) > 0 {
		if _, ok := os.LookupEnv("OPNI_ROUTING_DEBUG"); ok {
			err := os.WriteFile(fmt.Sprintf("%s1.yaml", name[0]), util.Must(yaml.Marshal(c1)), 0644)
			Expect(err).To(Succeed())
			err = os.WriteFile(fmt.Sprintf("%s2.yaml", name[0]), util.Must(yaml.Marshal(c2)), 0644)
			Expect(err).To(Succeed())
		}
	}
	Expect(util.Must(yaml.Marshal(c1))).To(MatchYAML(util.Must(yaml.Marshal(c2))))
}

func ExpectConfigNotEqual(c1, c2 *config.Config, name ...string) {
	if len(name) > 0 {
		if len(name) > 0 {
			if _, ok := os.LookupEnv("OPNI_ROUTING_DEBUG"); ok {
				err := os.WriteFile(fmt.Sprintf("%s1.yaml", name[0]), util.Must(yaml.Marshal(c1)), 0644)
				Expect(err).To(Succeed())
				err = os.WriteFile(fmt.Sprintf("%s2.yaml", name[0]), util.Must(yaml.Marshal(c2)), 0644)
				Expect(err).To(Succeed())
			}
		}
	}
	Expect(util.Must(yaml.Marshal(c1))).NotTo(MatchYAML(util.Must(yaml.Marshal(c2))))
}

// list request to expect length of notifications
type NotificationPair = lo.Tuple2[*alertingv1.ListNotificationRequest, int]

type AlarmPair = lo.Tuple2[*alertingv1.ListAlarmMessageRequest, int]

type RoutableDataset struct {
	Routables             []interfaces.Routable
	ExpectedNotifications []NotificationPair
	ExpectedAlarms        []AlarmPair
}

func NewRoutableDataset() *RoutableDataset {
	r := []interfaces.Routable{}
	i := 0
	for _, name := range alertingv1.OpniSeverity_name {
		notif := &alertingv1.Notification{
			Title: fmt.Sprintf("test %d", i),
			Body:  "test",
			Properties: map[string]string{
				message.NotificationPropertySeverity: name,
				message.NotificationPropertyOpniUuid: uuid.New().String(),
			},
		}
		notif.Sanitize()
		err := notif.Validate()
		if err != nil {
			panic(err)
		}
		r = append(r, notif)
		i++
	}
	condId1 := uuid.New().String()
	condId2 := uuid.New().String()
	for i := 0; i < 5; i++ {
		r = append(r, &alertingv1.AlertCondition{
			Name:        "test header 1",
			Description: "test header 2",
			Id:          condId1,
			Severity:    alertingv1.OpniSeverity_Error,
			AlertType: &alertingv1.AlertTypeDetails{
				Type: &alertingv1.AlertTypeDetails_System{
					System: &alertingv1.AlertConditionSystem{
						ClusterId: &corev1.Reference{Id: uuid.New().String()},
						Timeout:   durationpb.New(10 * time.Minute),
					},
				},
			},
		})
	}
	r = append(r, &alertingv1.AlertCondition{
		Name:        "test header 2",
		Description: "body 2",
		Id:          condId2,
		Severity:    alertingv1.OpniSeverity_Error,
		AlertType: &alertingv1.AlertTypeDetails{
			Type: &alertingv1.AlertTypeDetails_System{
				System: &alertingv1.AlertConditionSystem{
					ClusterId: &corev1.Reference{Id: uuid.New().String()},
					Timeout:   durationpb.New(10 * time.Minute),
				},
			},
		},
	})
	return &RoutableDataset{
		Routables: r,
		ExpectedNotifications: []NotificationPair{
			{A: &alertingv1.ListNotificationRequest{}, B: 4},
		},
		ExpectedAlarms: []AlarmPair{
			{A: &alertingv1.ListAlarmMessageRequest{
				ConditionId:  &alertingv1.ConditionReference{Id: condId1},
				Start:        timestamppb.New(time.Now().Add(-10000 * time.Hour)),
				End:          timestamppb.New(time.Now().Add(10000 * time.Hour)),
				Fingerprints: []string{"fingerprint"},
			}, B: 1},
			{A: &alertingv1.ListAlarmMessageRequest{
				ConditionId:  &alertingv1.ConditionReference{Id: condId2},
				Start:        timestamppb.New(time.Now().Add(-10000 * time.Hour)),
				End:          timestamppb.New(time.Now().Add(10000 * time.Hour)),
				Fingerprints: []string{"fingerprint"},
			}, B: 1},
		},
	}
}
