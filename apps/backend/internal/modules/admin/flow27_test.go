package admin_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/admin"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func flow27Scenario(t *testing.T) *scenario.Scenario {
	t.Helper()
	upstreams := fakes.New()
	srv := httptest.NewServer(upstreams)
	t.Cleanup(srv.Close)
	cfg, err := config.Load([]string{
		"MONACO_ENV=test", "DATABASE_URL=unused", "NATS_URL=unused", "POSTHOG_API_KEY=flow27-posthog",
		"POSTHOG_HOST=" + srv.URL + "/posthog",
	})
	if err != nil {
		t.Fatal(err)
	}
	return scenario.New(t, scenario.WithPrivy(upstreams, "app"), scenario.WithModules(
		func(d module.Deps) module.Module {
			d.Config.Admin = config.Admin{DeadLettersInterval: time.Second}
			return admin.New(d)
		},
		func(d module.Deps) module.Module { return social.New(d) },
		func(d module.Deps) module.Module {
			d.Config.PostHog, d.Config.Timeouts.PostHog = cfg.PostHog, cfg.Timeouts.PostHog
			d.HTTPClient = httpclient.New
			return analytics.New(d)
		},
	))
}

func TestFlow27_RecordDeadLetter_OK(t *testing.T) {
	t.Parallel()
	flows.F27RecordDeadLetterOK(flow27Scenario(t))
}

func TestFlow27_RedriveDeadLetter_OK(t *testing.T) {
	t.Parallel()
	flows.F27RedriveDeadLetterOK(flow27Scenario(t))
}

func TestFlow27_RedriveDeadLetter_DeadLetterNotOpen(t *testing.T) {
	t.Parallel()
	flows.F27RedriveDeadLetterDeadLetterNotOpen(flow27Scenario(t))
}

func TestFlow27_RedriveDeadLetter_NotFound(t *testing.T) {
	t.Parallel()
	flows.F27RedriveDeadLetterNotFound(flow27Scenario(t))
}

func TestFlow27_RedriveDeadLetter_AdminForbidden(t *testing.T) {
	t.Parallel()
	flows.F27RedriveDeadLetterAdminForbidden(flow27Scenario(t))
}

func TestFlow27_DiscardDeadLetter_OK(t *testing.T) {
	t.Parallel()
	flows.F27DiscardDeadLetterOK(flow27Scenario(t))
}
