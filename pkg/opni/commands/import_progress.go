//go:build !noplugins

package commands

import (
	"context"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
	"google.golang.org/protobuf/types/known/timestamppb"
	"math"
	"strings"
	"time"
)

const (
	padding  = 2
	maxWidth = 80
)

var (
	progressStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("4")).String()
)

func getNextStatus(ctx context.Context, request *remoteread.TargetStatusRequest) tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(_ time.Time) tea.Msg {
		status, err := remoteReadClient.GetTargetStatus(ctx, request)

		if err != nil {
			return err
		}

		return status
	})
}

func getProgressAsPercent(progress *remoteread.TargetProgress) float64 {
	if progress == nil || progress.LastReadTimestamp == nil {
		return 0
	}

	percent := float64(progress.LastReadTimestamp.Seconds-progress.StartTimestamp.Seconds) /
		float64(progress.EndTimestamp.Seconds-progress.StartTimestamp.Seconds)

	return math.Min(1, percent)
}

type ProgressModel struct {
	ctx     context.Context
	request *remoteread.TargetStatusRequest

	percent  float64
	progress progress.Model

	message  string
	lastRead *timestamppb.Timestamp
	state    string
}

func NewProgressModel(ctx context.Context, statusRequest *remoteread.TargetStatusRequest) ProgressModel {
	return ProgressModel{
		ctx:     ctx,
		request: statusRequest,

		progress: progress.New(progress.WithColorProfile(termenv.TrueColor), progress.WithSolidFill(progressStyle)),

		state: "running",
	}
}

func (model ProgressModel) Init() tea.Cmd {
	return getNextStatus(model.ctx, model.request)
}

func (model ProgressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return model, tea.Quit
		}

		return model, nil

	case tea.WindowSizeMsg:
		model.progress.Width = msg.Width - padding*2 - 4

		if model.progress.Width > maxWidth {
			model.progress.Width = maxWidth
		}

		return model, nil

	case string:
		model.message = msg
		return model, tea.Quit

	case *remoteread.TargetStatus:
		if msg.Message != "" {
			model.message = msg.Message
		}

		importDone := false

		switch msg.State {
		case remoteread.TargetState_Running:
			model.state = "running"
		case remoteread.TargetState_Failed:
			model.state = "failed"
			importDone = true

		case remoteread.TargetState_Canceled:
			model.state = "canceled"
			importDone = true

		case remoteread.TargetState_Completed:
			model.state = "complete"
			importDone = true

		case remoteread.TargetState_NotRunning:
			model.state = "not running"
		default:
			model.state = "unknown"
		}

		model.percent = getProgressAsPercent(msg.Progress)
		model.lastRead = msg.Progress.LastReadTimestamp

		if model.percent >= 1 {
			importDone = true
		}

		if importDone {
			return model, tea.Quit
		}

		return model, tea.Batch(getNextStatus(model.ctx, model.request))
	default:
		return model, nil
	}
}

func (model ProgressModel) View() string {
	builder := strings.Builder{}
	paddingStr := strings.Repeat(" ", padding)

	builder.WriteString("\n")

	builder.WriteString(paddingStr + model.progress.ViewAs(model.percent) + "\n\n")

	builder.WriteString(paddingStr + "State: " + model.state + "\n")

	if model.lastRead == nil {
		builder.WriteString(paddingStr + "Last Read Timestamp: nil \n")
	} else {
		builder.WriteString(paddingStr + "Last Read Timestamp: " + model.lastRead.AsTime().String() + "\n")
	}

	if model.message != "" {
		builder.WriteString(paddingStr + "Message: " + model.message + "\n\n")
	}

	return builder.String()
}
