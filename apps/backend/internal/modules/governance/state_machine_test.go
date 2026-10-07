package governance_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
)

func wantNext() map[domain.Status]map[domain.Event]domain.Status {
	return map[domain.Status]map[domain.Event]domain.Status{
		domain.StatusOpen: {
			domain.EventPass:     domain.StatusPassed,
			domain.EventFail:     domain.StatusFailed,
			domain.EventExpire:   domain.StatusExpired,
			domain.EventWithdraw: domain.StatusWithdrawn,
			domain.EventVoid:     domain.StatusVoided,
		},
		domain.StatusPassed: {
			domain.EventExecute: domain.StatusExecuted,
			domain.EventBlock:   domain.StatusExecutionBlocked,
			domain.EventVoid:    domain.StatusVoided,
		},
		domain.StatusExecutionBlocked: {
			domain.EventReopen: domain.StatusPassed,
		},
	}
}

func TestNext_everyStatusAndEventPair(t *testing.T) {
	t.Parallel()
	want := wantNext()
	pairs := 0
	for _, from := range domain.Statuses() {
		for _, ev := range domain.Events() {
			pairs++
			to, err := domain.Next(from, ev)
			allowed, ok := want[from][ev]
			switch {
			case ok && (err != nil || to != allowed):
				t.Errorf("Next(%s, %s) = %s, %v, want %s", from, ev, to, err, allowed)
			case !ok && (errs.CodeOf(err) != errs.CodeVersionConflict || to != from):
				t.Errorf("Next(%s, %s) = %s, %v, want %s and version_conflict", from, ev, to, err, from)
			}
		}
	}
	if pairs != 8*8 {
		t.Fatalf("checked %d pairs, want every one of 8 statuses by 8 events", pairs)
	}
}

func TestNext_voidIsLegalFromOpenAndPassedOnly(t *testing.T) {
	t.Parallel()
	for _, from := range domain.Statuses() {
		_, err := domain.Next(from, domain.EventVoid)
		legal := from == domain.StatusOpen || from == domain.StatusPassed
		if (err == nil) != legal {
			t.Errorf("Next(%s, void) err = %v, want legal = %t", from, err, legal)
		}
	}
}

func checkParse[T ~string](t *testing.T, name string, parse func(string) (T, error), known []T) {
	t.Helper()
	for _, v := range known {
		if got, err := parse(string(v)); err != nil || got != v {
			t.Errorf("%s(%s) = %s, %v", name, v, got, err)
		}
	}
	for _, raw := range []string{"", "Open", "agent_add", "supermajority"} {
		if got, err := parse(raw); errs.CodeOf(err) != errs.CodeDecodeFailed || got != "" {
			t.Errorf("%s(%q) = %q, %v, want decode_failed", name, raw, got, err)
		}
	}
}

func TestParse_acceptsEachKnownValueAndRejectsTheRest(t *testing.T) {
	t.Parallel()
	checkParse(t, "ParseStatus", domain.ParseStatus, domain.Statuses())
	checkParse(t, "ParseKind", domain.ParseKind, domain.Kinds())
	checkParse(t, "ParseThresholdRule", domain.ParseThresholdRule, domain.ThresholdRules())
}
