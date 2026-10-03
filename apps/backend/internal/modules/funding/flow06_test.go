package funding_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func withFunding() scenario.Option {
	return scenario.WithModules(func(d module.Deps) module.Module {
		d.Config.Solana.RPCURL = "http://127.0.0.1:1/rpc/"
		d.Config.Timeouts.RPC = time.Second
		return funding.New(d)
	})
}

func TestFlow06_CreateOnrampSession_OK(t *testing.T) {
	t.Parallel()
	flows.F06CreateOnrampSessionOK(scenario.New(t, withFunding()))
}

func TestFlow06_ExchangeOnrampToken_OnrampLinkInvalid(t *testing.T) {
	t.Parallel()
	flows.F06ExchangeOnrampTokenOnrampLinkInvalid(scenario.New(t, withFunding()))
}

func TestFlow06_ExchangeOnrampToken_OnrampLinkExpired(t *testing.T) {
	t.Parallel()
	flows.F06ExchangeOnrampTokenOnrampLinkExpired(scenario.New(t, withFunding()))
}

func TestFlow06_ReportOnrampStatus_OnrampInvalidTransition(t *testing.T) {
	t.Parallel()
	flows.F06ReportOnrampStatusOnrampInvalidTransition(scenario.New(t, withFunding()))
}

func TestFlow06_ReportOnrampStatus_Forbidden(t *testing.T) {
	t.Parallel()
	flows.F06ReportOnrampStatusForbidden(scenario.New(t, withFunding()))
}
