package verify

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func TestFlowLogs_flow19NamesTheLineEachOutcomeMustEmit(t *testing.T) {
	t.Parallel()
	for outcome, want := range map[flows.Outcome]string{
		"ok":                  observability.RankingRunCompleted.Name,
		"crash:before-commit": observability.RankingRunCompleted.Name,
		"PricesStale":         observability.RankingCabalExcluded.Name,
		"ConservationBroken":  observability.RankingCabalExcluded.Name,
		"CabalPaused":         observability.RankingCabalExcluded.Name,
	} {
		got := flowLogs(Unit{Flow: flows.Flow{ID: "19"}, Outcome: outcome})
		if len(got) != 1 || got[0].msg.Name != want {
			t.Errorf("flow 19 %s requires %v, want %s", outcome, got, want)
		}
	}
	if got := flowLogs(Unit{Flow: flows.Flow{ID: "18"}, Outcome: "ok"}); got != nil {
		t.Errorf("flow 18 requires %v, want nothing extra", got)
	}
}

func TestFlowLogs_flow25NamesTheAttributionAndOnlyTheOkOutcomeNamesTheQualification(t *testing.T) {
	t.Parallel()
	names := func(outcome flows.Outcome) []string {
		needs := flowLogs(Unit{Flow: flows.Flow{ID: "25"}, Outcome: outcome})
		out := make([]string, 0, len(needs))
		for _, need := range needs {
			out = append(out, need.msg.Name)
		}
		return out
	}
	if got := names("ok"); !slices.Equal(got, []string{"referrals.attributed", "referrals.qualified"}) {
		t.Errorf("flow 25 ok requires %v, want the attribution and the qualification", got)
	}
	if got := names("crash:before-commit"); !slices.Equal(got, []string{"referrals.attributed"}) {
		t.Errorf("flow 25 crash requires %v, want the attribution", got)
	}
}

func TestPolledEvents_namesTheEventsOfAPollerFlowOnly(t *testing.T) {
	t.Parallel()
	poller := Unit{Flow: flows.Flow{Trigger: "poller:ranking.valuation", Events: []string{"ranking.snapshot_written"}}}
	if got := polledEvents(poller); len(got) != 1 || got[0] != "ranking.snapshot_written" {
		t.Errorf("poller flow events = %v, want the snapshot event", got)
	}
	route := Unit{Flow: flows.Flow{Trigger: "GET /healthz", Events: []string{"system.pinged"}}}
	if got := polledEvents(route); len(got) != 0 {
		t.Errorf("route flow events = %v, want none", got)
	}
}
