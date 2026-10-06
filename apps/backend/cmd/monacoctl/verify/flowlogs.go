package verify

import (
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func flowLogs(u Unit) []logNeed {
	if u.Flow.ID != "19" {
		return nil
	}
	if _, crash := u.Outcome.CrashPoint(); crash || u.Outcome == "ok" {
		return []logNeed{{msg: observability.RankingRunCompleted}}
	}
	return []logNeed{{msg: observability.RankingCabalExcluded}}
}

func polledEvents(u Unit) []string {
	if kind, _ := u.Flow.TriggerKind(u.Command); kind != flows.TriggerPoller {
		return []string{}
	}
	return u.Flow.Events
}
