package test

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/drivers/backend"
	"github.com/rancher/opni/pkg/alerting/drivers/config"
	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	"github.com/rancher/opni/pkg/alerting/interfaces"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/test/freeport"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
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
	*sync.RWMutex
	Buffer []*config.WebhookMessage
}

func (m *MockIntegrationWebhookServer) WriteBuffer(msg *config.WebhookMessage) {
	m.Lock()
	defer m.Unlock()
	m.Buffer = append(m.Buffer, msg)
}

func (m *MockIntegrationWebhookServer) ClearBuffer() {
	m.Lock()
	defer m.Unlock()
	m.Buffer = m.Buffer[:0]
}

func (m *MockIntegrationWebhookServer) GetBuffer() []*config.WebhookMessage {
	m.RLock()
	defer m.RUnlock()
	return lo.Map(m.Buffer, func(msg *config.WebhookMessage, _ int) *config.WebhookMessage {
		return msg
	})
}

func (m *MockIntegrationWebhookServer) GetWebhook() string {
	return "http://" + path.Join(m.Addr, m.Webhook)
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

func (e *Environment) CreateWebhookServer(parentCtx context.Context, num int) []*MockIntegrationWebhookServer {
	var servers []*MockIntegrationWebhookServer
	for i := 0; i < num; i++ {
		servers = append(servers, e.NewWebhookMemoryServer(parentCtx, "webhook"))
	}
	return servers
}

func (e *Environment) NewWebhookMemoryServer(parentCtx context.Context, webHookRoute string) *MockIntegrationWebhookServer {
	port := freeport.GetFreePort()
	buf := []*config.WebhookMessage{}
	mu := &sync.RWMutex{}
	mux := http.NewServeMux()
	res := &MockIntegrationWebhookServer{
		Webhook:    webHookRoute,
		Port:       port,
		Buffer:     buf,
		RWMutex:    mu,
		EndpointId: uuid.New().String(),
	}
	if !strings.HasPrefix(webHookRoute, "/") {
		webHookRoute = "/" + webHookRoute
	}
	mux.HandleFunc(webHookRoute, func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		var msg config.WebhookMessage
		err = yaml.Unmarshal(data, &msg)
		if err != nil {
			panic(err)
		}
		res.WriteBuffer(&msg)
	})
	webhookServer := &http.Server{
		Addr:           fmt.Sprintf("127.0.0.1:%d", port),
		Handler:        mux,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	res.Addr = webhookServer.Addr

	waitctx.Permissive.Go(e.ctx, func() {
		go func() {
			err := webhookServer.ListenAndServe()
			if err != http.ErrServerClosed {
				panic(err)
			}
		}()
		defer webhookServer.Shutdown(context.Background())
		select {
		case <-e.ctx.Done():
		}
	})
	return res
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

func (e *Environment) RunAlertManager(
	ctx context.Context,
	router routing.OpniRouting,
	tmpWritePath string,
	debugFile string,
) (int, context.CancelFunc) {
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
	apiPort, ctxCa := e.StartEmbeddedAlertManager(ctx, tmpPath, lo.ToPtr(freePort))
	defer ctxCa()
	apiNode := backend.NewAlertManagerReadyClient(
		ctx,
		fmt.Sprintf("http://localhost:%d", apiPort),
		backend.WithExpectClosure(backend.NewExpectStatusOk()),
		backend.WithDefaultRetrier(),
	)
	err = apiNode.DoRequest()
	Expect(err).To(Succeed())
	return apiPort, ctxCa
}

func ExpectAlertManagerConfigToBeValid(
	env *Environment,
	tmpWritePath string,
	writeFile string,
	ctx context.Context,
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
	apiPort, ctxCa := env.StartEmbeddedAlertManager(ctx, tmpPath, lo.ToPtr(port))
	defer ctxCa()
	apiNode := backend.NewAlertManagerReadyClient(
		ctx,
		fmt.Sprintf("http://localhost:%d", apiPort),
		backend.WithExpectClosure(backend.NewExpectStatusOk()),
		backend.WithDefaultRetrier(),
	)
	err = apiNode.DoRequest()
	Expect(err).To(Succeed(), fmt.Sprintf("failed to load the config into alertmanager on step : %s", strings.TrimSuffix(writeFile, filepath.Ext(writeFile))))
	// in the future we can look at enhancing by providing partial matches through the /api/v2/status api
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
type RoutingDatasetKV = lo.Tuple2[*alertingv1.ListMessageRequest, int]

type RoutableDataset struct {
	Routables     []interfaces.Routable
	ExpectedPairs []RoutingDatasetKV
}

func NewRoutableDataset() *RoutableDataset {
	r := []interfaces.Routable{}
	i := 0
	for _, name := range alertingv1.OpniSeverity_name {
		notif := &alertingv1.Notification{
			Title: fmt.Sprintf("test %d", i),
			Body:  "test",
			Properties: map[string]string{
				alertingv1.NotificationPropertySeverity: name,
				alertingv1.NotificationPropertyOpniUuid: uuid.New().String(),
			},
		}
		err := notif.Validate()
		if err != nil {
			panic(err)
		}
		r = append(r, notif)
		i++
	}
	return &RoutableDataset{
		Routables: r,
		ExpectedPairs: []RoutingDatasetKV{
			{A: &alertingv1.ListMessageRequest{}, B: len(r)},
		},
	}
}
