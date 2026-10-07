package verify

import (
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func flowLogs(u Unit) []logNeed {
	switch u.Flow.ID {
	case "19":
		return rankingLogs(u)
	case "25":
		return referralLogs(u)
	}
	return nil
}

func referralLogs(u Unit) []logNeed {
	attributed := logNeed{msg: observability.ReferralsAttributed}
	if u.Outcome == "ok" {
		return []logNeed{attributed, {msg: observability.ReferralsQualified}}
	}
	return []logNeed{attributed}
}

func rankingLogs(u Unit) []logNeed {
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
