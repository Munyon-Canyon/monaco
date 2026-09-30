package domain

import "slices"

type ThresholdRule string

const (
	RuleMajority  ThresholdRule = "majority"
	RuleUnanimous ThresholdRule = "unanimous"
)

func ThresholdRules() []ThresholdRule { return []ThresholdRule{RuleMajority, RuleUnanimous} }

func ParseThresholdRule(raw string) (ThresholdRule, error) {
	if !slices.Contains(ThresholdRules(), ThresholdRule(raw)) {
		return "", unknown("governance.ParseThresholdRule", raw)
	}
	return ThresholdRule(raw), nil
}

type Outcome string

const (
	Undecided Outcome = "undecided"
	Passed    Outcome = "passed"
	Failed    Outcome = "failed"
)

func Tally(rule ThresholdRule, voters, yes, no int) Outcome {
	need, ok := rule.need(voters)
	switch {
	case !ok || voters < 1:
		return Undecided
	case yes >= need:
		return Passed
	case no > voters-need:
		return Failed
	}
	return Undecided
}

func (r ThresholdRule) need(voters int) (int, bool) {
	switch r {
	case RuleMajority:
		return voters/2 + 1, true
	case RuleUnanimous:
		return voters, true
	}
	return 0, false
}
