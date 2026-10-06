package notify_test

import (
	"net/http/httptest"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow24_Notify_OK(t *testing.T) {
	t.Parallel()
	flows.F24NotifyOK(flow24Scenario(t))
}

func TestFlow24_Notify_APNSUnavailable(t *testing.T) {
	t.Parallel()
	flows.F24NotifyAPNSUnavailable(flow24Scenario(t))
}

func TestFlow24_Notify_APNSAuthFailed(t *testing.T) {
	t.Parallel()
	flows.F24NotifyAPNSAuthFailed(flow24Scenario(t))
}

func flow24Scenario(t *testing.T) *scenario.Scenario {
	t.Helper()
	upstreams := fakes.New()
	srv := httptest.NewServer(upstreams)
	t.Cleanup(srv.Close)
	cfg, err := config.Load(append([]string{
		"MONACO_ENV=test", "DATABASE_URL=unused", "NATS_URL=unused", "APNS_BASE_URL=" + srv.URL + "/apns",
	}, testkit.APNsEnv()...))
	if err != nil {
		t.Fatal(err)
	}
	client, err := apns.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return scenario.New(t, scenario.WithPrivy(upstreams, "app"),
		scenario.WithModules(func(d module.Deps) module.Module { return notify.New(d, notify.WithSender(client)) }))
}
