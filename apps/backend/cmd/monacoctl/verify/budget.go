package verify

import (
	"fmt"
	"time"
)

type Phase string

const (
	PhaseBuild    Phase = "build"
	PhaseStack    Phase = "stack-up"
	PhaseSeed     Phase = "seed"
	PhaseFlow     Phase = "flow"
	PhaseConverge Phase = "converge"
	PhaseTeardown Phase = "teardown"
	PhaseTotal    Phase = "total"
)

type Budget struct {
	Total    time.Duration
	Stack    time.Duration
	Seed     time.Duration
	Flow     time.Duration
	Converge time.Duration
	Teardown time.Duration
}

func DefaultBudget() Budget {
	return Budget{
		Total:    90 * time.Second,
		Stack:    10 * time.Second,
		Seed:     2 * time.Second,
		Flow:     15 * time.Second,
		Converge: 30 * time.Second,
		Teardown: 5 * time.Second,
	}
}

type OverBudgetError struct {
	Phase  Phase
	Flow   string
	Budget time.Duration
}

func (e *OverBudgetError) Error() string {
	if e.Flow == "" {
		return fmt.Sprintf("over budget: %s took longer than %s", e.Phase, e.Budget)
	}
	return fmt.Sprintf("over budget: flow %s %s took longer than %s", e.Flow, e.Phase, e.Budget)
}
