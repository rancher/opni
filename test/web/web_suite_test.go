package web_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/onsi/biloba"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/dashboard"
	"github.com/rancher/opni/pkg/test"
	_ "github.com/rancher/opni/pkg/test/setup"
	"golang.org/x/net/context"

	_ "github.com/rancher/opni/plugins/metrics/test"
)

func TestWeb(t *testing.T) {
	SetDefaultEventuallyTimeout(5 * time.Second)
	gin.SetMode(gin.TestMode)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Web Suite")
}

var b *biloba.Biloba
var webUrl string
var env *test.Environment
var mgmtClient managementv1.ManagementClient

var _ = SynchronizedBeforeSuite(func() {
	biloba.SpinUpChrome(GinkgoT())
}, func(ctx context.Context) {
	env = &test.Environment{}
	Expect(env.Start()).To(Succeed())
	DeferCleanup(env.Stop)
	b = biloba.ConnectToChrome(GinkgoT())
	webUrl = "http://" + env.GatewayConfig().Spec.Management.WebListenAddress
	mgmtClient = env.NewManagementClient()

	dashboardSrv, err := dashboard.NewServer(&env.GatewayConfig().Spec.Management)
	Expect(err).NotTo(HaveOccurred())
	go dashboardSrv.ListenAndServe(env.Context())
})

var _ = BeforeEach(func() {
	b.Prepare()
}, OncePerOrdered)

type table string
type tableRow string
type tableHeader string
type TableCell = string

type TableInterface interface {
	fmt.Stringer
	Row(int) TableRowInterface
	Header() TableHeaderInterface
}

type TableRowInterface interface {
	fmt.Stringer
	Col(int) TableCell
}

type TableHeaderInterface interface {
	fmt.Stringer
	Col(int) TableCell
}

func Table() TableInterface {
	return table("table.sortable-table")
}

// 1-indexed
func (t table) Row(idx int) TableRowInterface {
	return tableRow(fmt.Sprintf("%s > tbody > tr.main-row:nth-child(%d) ", t, idx))
}

func (t table) Header() TableHeaderInterface {
	return tableHeader(fmt.Sprintf("%s> thead > tr ", t))
}

func (t table) String() string {
	return string(t)
}

// 1-indexed
func (tr tableRow) Col(idx int) TableCell {
	return TableCell(fmt.Sprintf("%s> td:nth-child(%d) ", tr, idx))
}

func (tr tableRow) String() string {
	return string(tr)
}

// 1-indexed
func (tr tableHeader) Col(idx int) TableCell {
	return TableCell(fmt.Sprintf("%s> th:nth-child(%d) ", tr, idx))
}

func (tr tableHeader) String() string {
	return string(tr)
}

func HaveBadge(class, text string) types.GomegaMatcher {
	return WithTransform(func(cell TableCell) string {
		return fmt.Sprintf("%s> div:nth-child(1) > span:nth-child(1)", cell)
	}, And(b.HaveClass(class), b.HaveInnerText(text)))
}

func HaveReadyBadge() types.GomegaMatcher {
	return HaveBadge("bg-success", "Ready")
}

func HaveNotInstalledBadge() types.GomegaMatcher {
	return HaveBadge("bg-info", "Not-installed")
}

func HaveInstalledBadge() types.GomegaMatcher {
	return HaveBadge("bg-success", "Installed")
}

func HaveDisconnectedBadge() types.GomegaMatcher {
	return HaveBadge("bg-error", "Disconnected")
}

func HaveConditionsBadge(conditions ...string) types.GomegaMatcher {
	return HaveBadge("bg-warning", strings.Join(conditions, ", "))
}

func HaveCheckmark() types.GomegaMatcher {
	return WithTransform(func(cell TableCell) string {
		return fmt.Sprintf("%s> span:nth-child(1) > i", cell)
	}, And(b.Exist(), b.HaveClass("icon-checkmark")))
}

func HaveNoRows() types.GomegaMatcher {
	return WithTransform(func(t TableInterface) string {
		return fmt.Sprintf("%s> tbody > tr.no-rows", t)
	}, b.BeVisible())
}

type CheckBoxState string

const (
	Checked   CheckBoxState = "true"
	Unchecked CheckBoxState = "false"
)

func CheckBox(state ...CheckBoxState) types.GomegaMatcher {
	if len(state) == 0 {
		return WithTransform(func(cell TableCell) string {
			return fmt.Sprintf("%s> div.checkbox-outer-container > label.checkbox-container", cell)
		}, b.Exist())
	}
	return WithTransform(func(cell TableCell) string {
		return fmt.Sprintf("%s> div.checkbox-outer-container > label.checkbox-container > span.checkbox-custom", cell)
	}, b.HaveProperty("aria-checked", string(state[0])))
}

func MatchCells(matchers ...types.GomegaMatcher) types.GomegaMatcher {
	items := make([]types.GomegaMatcher, len(matchers))
	for i, m := range matchers {
		i, m := i, m
		items[i] = WithTransform(func(row TableRowInterface) TableCell {
			return row.Col(i + 1)
		}, m)
	}
	return And(items...)
}

func ClickWithMouseover(args ...any) types.GomegaMatcher {
	return WithTransform(func(selector any) (any, error) {
		b.Run(fmt.Sprintf(`document.querySelector(%q).dispatchEvent(new MouseEvent("mouseover"))`, selector))
		return selector, nil
	}, b.Click(args...))
}
