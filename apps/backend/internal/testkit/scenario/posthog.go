package scenario

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit/posthogfake"
)

const (
	postHogKey     = "scenario-posthog-key"
	postHogTimeout = 5 * time.Second
)

func WithPostHog(t *testing.T) Option {
	t.Helper()
	fake := posthogfake.New(t)
	return WithModules(func(d module.Deps) module.Module {
		d.Config.PostHog = config.PostHog{APIKey: postHogKey, Host: fake.Host()}
		d.Config.Timeouts.PostHog = postHogTimeout
		d.HTTPClient = httpclient.New
		return analytics.New(d)
	})
}

func EventuallyCapturedBy(typ events.Type, event, field, value string) Step {
	return func(s *Scenario) {
		s.t.Helper()
		rows, err := s.DB().Query(s.t.Context(),
			`SELECT id::text FROM events WHERE type = $1 AND payload->>$2::text = $3 ORDER BY id`,
			string(typ), field, value)
		appended := scanIDs(s.t, typ, rows, err)
		if len(appended) == 0 {
			s.t.Fatalf("scenario: no %s event has %s %s to export", typ, field, value)
		}
		for _, id := range appended {
			EventuallyLog(observability.AnalyticsCaptureSent, map[string]string{"event": event, "uuid": id})(s)
		}
	}
}

func EventuallyCaptured(typ events.Type, event string, proposal ids.ProposalID) Step {
	return func(s *Scenario) {
		s.t.Helper()
		var id string
		err := s.DB().QueryRow(s.t.Context(), `SELECT id::text FROM events
			WHERE type = $1 AND (payload->>'proposal_id' = $2 OR payload->'source'->>'id' = $2)
			ORDER BY id LIMIT 1`, string(typ), proposal.String()).Scan(&id)
		if err != nil {
			s.t.Fatalf("scenario: find the %s event of proposal %s: %v", typ, proposal, err)
		}
		EventuallyLog(observability.AnalyticsCaptureSent, map[string]string{"event": event, "uuid": id})(s)
	}
}
