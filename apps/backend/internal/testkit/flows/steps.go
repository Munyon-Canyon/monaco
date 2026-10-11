package flows

import (
	"slices"
	"strings"
)

type StepStatus string

const (
	StepPending StepStatus = "pending"
	StepReady   StepStatus = "ready"
)

type FlowStep struct {
	Flow   string
	Order  int
	Name   string
	Owner  string
	Status StepStatus
	Script Script
}

func Steps() []FlowStep {
	var steps []FlowStep
	each("Steps", func(_ string, out any) { steps = append(steps, out.(FlowStep)) })
	slices.SortFunc(steps, func(a, b FlowStep) int {
		if c := strings.Compare(a.Flow, b.Flow); c != 0 {
			return c
		}
		return a.Order - b.Order
	})
	return steps
}
