/// Reference : https://github.dev/slok/sloth/tree/main/internal/alert/window.go
package slo

import (
	"fmt"
	"time"
)

type Windows struct {
	SLOPeriod   time.Duration
	PageQuick   Window
	PageSlow    Window
	TicketQuick Window
	TicketSlow  Window
}

//https://sre.google/workbook/alerting-on-slos/
func WindowDefaults(period time.Duration) *Windows {
	return &Windows{
		SLOPeriod: period,
		PageQuick: Window{
			LongWindow:         time.Minute * 60,
			ShortWindow:        time.Minute * 5,
			ErrorBudgetPercent: 2,
		},
		PageSlow: Window{
			LongWindow:         time.Hour * 6,
			ShortWindow:        time.Minute * 30,
			ErrorBudgetPercent: 5,
		},
		TicketQuick: Window{
			LongWindow:         (time.Hour * 24) * 3,
			ShortWindow:        time.Hour * 6,
			ErrorBudgetPercent: 10,
		},
		TicketSlow: Window{
			LongWindow:         (time.Hour * 24) * 3,
			ShortWindow:        time.Hour * 6,
			ErrorBudgetPercent: 10,
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
	ShortWindow time.Duration
	// LongWindow is the long window used to alert based on the errors happened on that
	// long window.
	LongWindow time.Duration
}

func (w Window) Validate() error {
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

func (w Windows) Validate() error {
	if w.SLOPeriod == 0 {
		return fmt.Errorf("slo period is required")
	}

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

// Error budget speeds based on a full time window, however once we have the factor (speed)
// the value can be used with any time window.
func (w Windows) GetSpeedPageQuick() float64 {
	return w.GetBurnRateFactor(w.SLOPeriod, float64(w.PageQuick.ErrorBudgetPercent), w.PageQuick.LongWindow)
}
func (w Windows) GetSpeedPageSlow() float64 {
	return w.GetBurnRateFactor(w.SLOPeriod, float64(w.PageSlow.ErrorBudgetPercent), w.PageSlow.LongWindow)
}
func (w Windows) GetSpeedTicketQuick() float64 {
	return w.GetBurnRateFactor(w.SLOPeriod, float64(w.TicketQuick.ErrorBudgetPercent), w.TicketQuick.LongWindow)
}
func (w Windows) GetSpeedTicketSlow() float64 {
	return w.GetBurnRateFactor(w.SLOPeriod, float64(w.TicketSlow.ErrorBudgetPercent), w.TicketSlow.LongWindow)
}

// getBurnRateFactor calculates the burnRateFactor (speed) needed to consume all the error budget available percent
// in a specific time window taking into account the total time window.
func (w Windows) GetBurnRateFactor(totalWindow time.Duration, errorBudgetPercent float64, consumptionWindow time.Duration) float64 {
	// First get the total hours required to consume the % of the error budget in the total window.
	hoursRequiredConsumption := errorBudgetPercent * totalWindow.Hours() / 100

	// Now calculate how much is the factor required for the hours consumption, in case we would need to use
	// a different time window (e.g: hours required: 36h, if we want to do it in 6h: would be `x6`).
	speed := hoursRequiredConsumption / consumptionWindow.Hours()

	return speed
}
