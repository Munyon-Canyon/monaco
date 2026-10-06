package verify

import (
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
