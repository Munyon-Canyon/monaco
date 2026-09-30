package governance_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
)

func carries(rule domain.ThresholdRule, voters, yes int) bool {
	if rule == domain.RuleUnanimous {
		return yes == voters
	}
	return 2*yes > voters
}

func wantTally(rule domain.ThresholdRule, voters, yes, no int) domain.Outcome {
	switch {
	case carries(rule, voters, yes):
		return domain.Passed
	case !carries(rule, voters, voters-no):
		return domain.Failed
	}
	return domain.Undecided
}

type split struct {
	rule            domain.ThresholdRule
	voters, yes, no int
}

func (s split) tally() domain.Outcome { return domain.Tally(s.rule, s.voters, s.yes, s.no) }

func everySplitUpTo50Voters() []split {
	var out []split
	for _, rule := range domain.ThresholdRules() {
		for voters := 1; voters <= 50; voters++ {
			for yes := range voters + 1 {
				for no := range voters - yes + 1 {
					out = append(out, split{rule, voters, yes, no})
				}
			}
		}
	}
	return out
}

func TestTally_everyBallotSplitUpTo50Voters(t *testing.T) {
	t.Parallel()
	for _, s := range everySplitUpTo50Voters() {
		if got, want := s.tally(), wantTally(s.rule, s.voters, s.yes, s.no); got != want {
			t.Fatalf("Tally(%+v) = %s, want %s", s, got, want)
		}
	}
}

func TestTally_aDecidedOutcomeNeverFlips(t *testing.T) {
	t.Parallel()
	for _, s := range everySplitUpTo50Voters() {
		now := s.tally()
		if now == domain.Undecided || s.yes+s.no == s.voters {
			continue
		}
		moreYes, moreNo := s, s
		moreYes.yes++
		moreNo.no++
		if moreYes.tally() != now || moreNo.tally() != now {
			t.Fatalf("Tally(%+v) = %s, one more ballot gives %s or %s", s, now, moreYes.tally(), moreNo.tally())
		}
	}
}

func TestTally_unanimousFailsAtTheFirstNo(t *testing.T) {
	t.Parallel()
	for voters := 1; voters <= 50; voters++ {
		for yes := range voters {
			if got := domain.Tally(domain.RuleUnanimous, voters, yes, 1); got != domain.Failed {
				t.Fatalf("Tally(unanimous, %d voters, %d yes, 1 no) = %s, want failed", voters, yes, got)
			}
		}
	}
}

func TestTally_majorityOfThree(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		yes, no int
		want    domain.Outcome
	}{
		{1, 0, domain.Undecided},
		{1, 1, domain.Undecided},
		{2, 0, domain.Passed},
		{0, 2, domain.Failed},
		{2, 1, domain.Passed},
	} {
		if got := domain.Tally(domain.RuleMajority, 3, tc.yes, tc.no); got != tc.want {
			t.Errorf("Tally(majority, 3 voters, %d yes, %d no) = %s, want %s", tc.yes, tc.no, got, tc.want)
		}
	}
}

func TestTally_decidesNothingWithoutVotersOrAKnownRule(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		rule            domain.ThresholdRule
		voters, yes, no int
	}{
		{domain.RuleMajority, 0, 0, 0},
		{domain.RuleUnanimous, 0, 0, 0},
		{"supermajority", 3, 3, 0},
		{"supermajority", 3, 0, 3},
	} {
		if got := domain.Tally(tc.rule, tc.voters, tc.yes, tc.no); got != domain.Undecided {
			t.Errorf("Tally(%s, %d voters, %d yes, %d no) = %s, want undecided", tc.rule, tc.voters, tc.yes, tc.no, got)
		}
	}
}
