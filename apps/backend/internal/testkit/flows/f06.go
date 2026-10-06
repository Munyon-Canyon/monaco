package flows

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	onrampSessions = "/v1/onramp/sessions"
	onrampSession  = onrampSessions + "/{session}"
	onrampExchange = onrampSessions + "/exchange"
)

func F06CreateOnrampSessionOK(s *scenario.Scenario) {
	s.Given(onrampBuyers("alice")).
		When(
			f06Create(),
			scenario.Replay(),
			f06Exchange(),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectRemembered("session_id", "session"),
			scenario.Remember("wallet_address", "exchanged_wallet"),
			f06ExpectWalletOf("alice"),
			scenario.ExpectJSON("suggested_amount_micros", "25000000"),
			scenario.AsUser("alice"),
			scenario.Patch(onrampSession, `{"status":"confirmed","provider":"moonpay"}`),
			scenario.ExpectStatus(http.StatusOK),
			scenario.Get(onrampSession),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "confirmed"),
		).
		Then(
			scenario.ExpectEvents(events.TypeOnrampStatusChanged, 3),
			scenario.EventuallyPublished(events.TypeOnrampStatusChanged, 3),
			scenario.EventuallyHint("onramp_changed"),
		)
	s.Then(scenario.EventuallyCapturedBy(
		events.TypeOnrampStatusChanged, "onramp_status_changed", "session_id", s.Recall("session")))
}

func F06ExchangeOnrampTokenOnrampLinkInvalid(s *scenario.Scenario) {
	s.Given(onrampBuyers("alice")).
		When(f06Create(), f06Exchange(), scenario.ExpectStatus(http.StatusOK), f06Exchange()).
		Then(
			scenario.ExpectProblem(errs.CodeOnrampLinkInvalid),
			scenario.ExpectEvents(events.TypeOnrampStatusChanged, 2),
		)
}

func F06ExchangeOnrampTokenOnrampLinkExpired(s *scenario.Scenario) {
	s.Given(onrampBuyers("alice")).
		When(f06Create(), f06ElevenMinutesLater(), f06Exchange()).
		Then(
			scenario.ExpectProblem(errs.CodeOnrampLinkExpired),
			scenario.ExpectEvents(events.TypeOnrampStatusChanged, 1),
		)
}

func F06ReportOnrampStatusOnrampInvalidTransition(s *scenario.Scenario) {
	s.Given(onrampBuyers("alice")).
		When(
			f06Create(), f06Exchange(), scenario.ExpectStatus(http.StatusOK), scenario.AsUser("alice"),
			scenario.Patch(onrampSession, `{"status":"confirmed"}`), scenario.ExpectStatus(http.StatusOK),
			scenario.Patch(onrampSession, `{"status":"cancelled"}`),
		).
		Then(
			scenario.ExpectProblem(errs.CodeOnrampInvalidTransition),
			scenario.ExpectEvents(events.TypeOnrampStatusChanged, 3),
		)
}

func F06ReportOnrampStatusForbidden(s *scenario.Scenario) {
	s.Given(onrampBuyers("alice", "bob")).
		When(
			f06Create(), f06Exchange(), scenario.ExpectStatus(http.StatusOK), scenario.AsUser("bob"),
			scenario.Patch(onrampSession, `{"status":"confirmed"}`),
		).
		Then(
			scenario.ExpectProblem(errs.CodeForbidden),
			scenario.ExpectEvents(events.TypeOnrampStatusChanged, 2),
		)
}

func onrampBuyers(names ...string) scenario.Step {
	return func(s *scenario.Scenario) {
		for _, name := range names {
			scenario.SeededUser(name, "active")(s)
			key := make([]byte, 32)
			_, _ = rand.Read(key)
			address := chain.AddressOf(key)
			if _, err := s.DB().Exec(s.Context(), `INSERT INTO user_wallets (user_id, privy_wallet_id, address,
				created_at) VALUES ($1, $2, $3, now())`, s.Recall(name), "wallet-"+s.Recall(name), string(address),
			); err != nil {
				s.Fatalf("flows: seed the wallet of %s: %v", name, err)
			}
		}
		scenario.AsUser(names[0])(s)
	}
}

func f06Create() scenario.Step {
	return func(s *scenario.Scenario) {
		for _, step := range []scenario.Step{
			scenario.AsUser("alice"),
			scenario.Post(onrampSessions, `{"suggested_amount_micros":"25000000"}`),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.Remember("session_id", "session"),
			scenario.Remember("url", "url"),
		} {
			step(s)
		}
	}
}

func f06Exchange() scenario.Step {
	return func(s *scenario.Scenario) {
		u, err := url.Parse(s.Recall("url"))
		if err != nil {
			s.Fatalf("flows: the fund page URL %q: %v", s.Recall("url"), err)
		}
		body, err := json.Marshal(map[string]string{"token": u.Query().Get("s")})
		if err != nil {
			s.Fatalf("flows: encode the exchange body: %v", err)
		}
		scenario.Anonymous()(s)
		scenario.Post(onrampExchange, string(body))(s)
	}
}

func f06ElevenMinutesLater() scenario.Step {
	return func(s *scenario.Scenario) {
		if _, err := s.DB().Exec(s.Context(), `UPDATE onramp_sessions
			SET created_at = created_at - interval '11 minutes', expires_at = expires_at - interval '11 minutes'
			WHERE id = $1`, s.Recall("session")); err != nil {
			s.Fatalf("flows: move the session 11 minutes back: %v", err)
		}
	}
}

func f06ExpectWalletOf(name string) scenario.Step {
	return func(s *scenario.Scenario) {
		var address string
		if err := s.DB().QueryRow(s.Context(), `SELECT address FROM user_wallets WHERE user_id = $1`,
			s.Recall(name)).Scan(&address); err != nil {
			s.Fatalf("flows: read the wallet of %s: %v", name, err)
		}
		if got := s.Recall("exchanged_wallet"); got != address {
			s.Fatalf("flows: the exchange answered wallet %s, want %s's %s", got, name, address)
		}
	}
}
