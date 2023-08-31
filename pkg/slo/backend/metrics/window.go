package metrics

import (
	"fmt"
	"time"

	"slices"

	"github.com/prometheus/common/model"
	"github.com/samber/lo"
)

func NormalizePeriodToBudgetingInterval(period time.Duration) (budgetingInterval time.Duration) {
	return time.Duration(
		int((float64(time.Duration(period).Nanoseconds()) * (float64(5) / float64(43200)))),
	)
}

type MWMBWindows struct {
	PageQuick   Window
	PageSlow    Window
	TicketQuick Window
	TicketSlow  Window
}

func (w MWMBWindows) windows() []Window {
	return []Window{
		w.PageQuick,
		w.PageSlow,
		w.TicketQuick,
		w.TicketSlow,
	}
}

func (w MWMBWindows) WindowRange() []string {
	windows := w.windowRangeDur()
	return lo.Map(windows, func(w model.Duration, _ int) string {
		return w.String()
	})
}

func (w MWMBWindows) windowRangeDur() []model.Duration {
	windows := []model.Duration{}
	for _, w := range w.windows() {
		windows = append(windows, w.ShortWindow)
		windows = append(windows, w.LongWindow)
	}
	slices.SortFunc(windows, func(i, j model.Duration) int {
		return int(int64(i) - int64(j))
	})
	return windows
}

// https://sre.google/workbook/alerting-on-slos/
//
// budgeting interval is the shortest interval to monitor in a window
//
// budgeting interval should roughly be set to 5/432 of the SLO period
func GenerateMWMBWindows(budgetingInterval model.Duration) *MWMBWindows {
	pageQuickShort := budgetingInterval
	pageQuickLong := budgetingInterval * 12

	pageSlowShort := budgetingInterval * 6
	pageSlowLong := pageSlowShort * 12

	ticketQuickShort := pageSlowShort * 4
	ticketQuickLong := ticketQuickShort * 12

	ticketSlowShort := ticketQuickShort * 3
	ticketSlowLong := ticketQuickLong * 3

	return &MWMBWindows{
		PageQuick: Window{
			LongWindow:         pageQuickLong,
			ShortWindow:        pageQuickShort,
			ErrorBudgetPercent: 2,
		},
		PageSlow: Window{
			LongWindow:         pageSlowLong,
			ShortWindow:        pageSlowShort,
			ErrorBudgetPercent: 5,
		},
		TicketQuick: Window{
			LongWindow:         ticketQuickLong,
			ShortWindow:        ticketQuickShort,
			ErrorBudgetPercent: 10,
		},
		TicketSlow: Window{
			LongWindow:         ticketSlowLong,
			ShortWindow:        ticketSlowShort,
			ErrorBudgetPercent: 10,
		},
	}
}

// https://sre.google/workbook/alerting-on-slos/
func WindowDefaults(period model.Duration) *MWMBWindows {
	return &MWMBWindows{
		PageQuick: Window{
			ShortWindow:        model.Duration(time.Minute) * 5,
			LongWindow:         model.Duration(time.Minute) * 60,
			ErrorBudgetPercent: 2,
			Period:             period,
		},
		PageSlow: Window{
			ShortWindow:        model.Duration(time.Minute) * 30,
			LongWindow:         model.Duration(time.Hour) * 6,
			ErrorBudgetPercent: 5,
			Period:             period,
		},
		TicketQuick: Window{
			LongWindow:         (model.Duration(time.Hour) * 24),
			ShortWindow:        model.Duration(time.Hour) * 2,
			ErrorBudgetPercent: 10,
			Period:             period,
		},
		TicketSlow: Window{
			LongWindow:         (model.Duration(time.Hour) * 24) * 3,
			ShortWindow:        model.Duration(time.Hour) * 6,
			ErrorBudgetPercent: 10,
			Period:             period,
		},
	}
}

type Window struct {
	// ErrorBudgetPercent is the error budget % consumed for a full time window.
	// Google gives us some defaults in its SRE workbook that work correctly most of the times:
	// - Page quick:   2%
	// - Page slow:    5%
	// - Ticket quick: 10%
	// - Ticket slow:  10%
	ErrorBudgetPercent float64
	// ShortWindow is the small window used on the alerting part to stop alerting
	// during a long window because we consumed a lot of error budget but the problem
	// is already gone.
	ShortWindow model.Duration
	// LongWindow is the long window used to alert based on the errors happened on that
	// long window.
	LongWindow model.Duration
	Period     model.Duration
}

func (w Window) Validate() error {
	if w.Period == 0 {
		return fmt.Errorf("period is required")
	}
	if w.LongWindow == 0 {
		return fmt.Errorf("long window is required")
	}

	if w.ShortWindow == 0 {
		return fmt.Errorf("short window is required")
	}

	if w.ErrorBudgetPercent == 0 {
		return fmt.Errorf("error budget is required")
	}

	return nil
}

func (w MWMBWindows) Validate() error {

	err := w.PageQuick.Validate()
	if err != nil {
		return fmt.Errorf("invalid page quick: %w", err)
	}

	err = w.PageSlow.Validate()
	if err != nil {
		return fmt.Errorf("invalid page slow: %w", err)
	}

	err = w.TicketQuick.Validate()
	if err != nil {
		return fmt.Errorf("invalid ticket quick: %w", err)
	}

	err = w.TicketSlow.Validate()
	if err != nil {
		return fmt.Errorf("invalid ticket slow: %w", err)
	}

	return nil
}

func (w MWMBWindows) PageWindows() (quickWindow, slowWindow Window) {
	return w.PageQuick, w.PageSlow
}

func (w MWMBWindows) TicketWindows() (quickWindow, slowWindow Window) {
	return w.TicketQuick, w.TicketSlow
}

// Error budget speeds based on a full time window, however once we have the factor (speed)
// the value can be used with any time window.
func (w Window) GetLongBurnRateFactor() float64 {
	return GetBurnRateFactor(w.Period, w.ErrorBudgetPercent, w.LongWindow)
}

func (w Window) GetShortBurnRateFactor() float64 {
	return GetBurnRateFactor(w.Period, w.ErrorBudgetPercent, w.ShortWindow)
}

// getBurnRateFactor calculates the burnRateFactor (speed) needed to consume all the error budget available percent
// in a specific time window taking into account the total time window.
// returns a value between 0 and 100
func GetBurnRateFactor(totalWindow model.Duration, errorBudgetPercent float64, consumptionWindow model.Duration) float64 {
	// First get the total seconds required to consume the % of the error budget in the total window.
	secondsRequiredConsumption := (errorBudgetPercent * time.Duration(totalWindow).Seconds()) / 100
	// Now calculate how much is the factor required for the hours consumption, in case we would need to use
	// a different time window (e.g: hours required: 36h, if we want to do it in 6h: would be `x6`).
	speed := secondsRequiredConsumption / time.Duration(consumptionWindow).Seconds()
	return speed
}
