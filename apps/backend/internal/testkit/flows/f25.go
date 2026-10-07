package flows

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	attachPath    = "/v1/me/referral"
	attachDisplay = "Kai"

	qualifyFundBody = `{"amount_micros":"10000000"}`
)

type invite struct{ code, handle string }

func newInvite() invite {
	code, err := domain.NewRandomCode(rand.Reader)
	if err != nil {
		panic(err)
	}
	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		panic(err)
	}
	return invite{code: string(code), handle: "ref" + hex.EncodeToString(suffix)}
}

func (i invite) codeBody(source string) string { return attachBody(i.code, source) }

func (i invite) handleBody(source string) string { return attachBody(i.handle, source) }

func attachBody(code, source string) string {
	return fmt.Sprintf(`{"code":%q,"source":%q}`, code, source)
}

func seedInvite(name string, inv invite) scenario.Step {
	return func(s *scenario.Scenario) {
		scenario.SeededUser(name, "active")(s)
		_, err := s.DB().Exec(s.Context(), `UPDATE users
			SET display_name = $2, handle = $3, first_deposit_at = now()
			WHERE id = $1::uuid`, s.Recall(name), attachDisplay, inv.handle)
		if err != nil {
			s.Fatalf("flows: set invite profile %s: %v", name, err)
		}
		_, err = s.DB().Exec(s.Context(), `INSERT INTO referral_codes (code, user_id, created_at)
			VALUES ($1, $2::uuid, now())`, inv.code, s.Recall(name))
		if err != nil {
			s.Fatalf("flows: mint invite code for %s: %v", name, err)
		}
	}
}

func seedNewUser(name string) scenario.Step {
	return scenario.SeededUser(name, "active")
}

func ageCreated(name, interval string) scenario.Step {
	return func(s *scenario.Scenario) {
		_, err := s.DB().Exec(s.Context(), `UPDATE users
			SET created_at = now() - $2::interval, updated_at = now()
			WHERE id = $1::uuid`, s.Recall(name), interval)
		if err != nil {
			s.Fatalf("flows: age %s by %s: %v", name, interval, err)
		}
	}
}

func ageOnboarding(name, interval string) scenario.Step {
	return func(s *scenario.Scenario) {
		_, err := s.DB().Exec(s.Context(), `UPDATE users
			SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now() - $2::interval
			WHERE id = $1::uuid`, s.Recall(name), interval)
		if err != nil {
			s.Fatalf("flows: close the onboarding window for %s: %v", name, err)
		}
	}
}

func expectReferrer(name string, inv invite) scenario.Step {
	return scenario.ExpectField("referrer", func(s *scenario.Scenario, raw json.RawMessage) {
		var got struct {
			UserID      string `json:"user_id"`
			DisplayName string `json:"display_name"`
			Handle      string `json:"handle"`
		}
		if err := json.Unmarshal(raw, &got); err != nil {
			s.Fatalf("flows: referrer: %v", err)
		}
		if got.UserID != s.Recall(name) || got.DisplayName != attachDisplay || got.Handle != inv.handle {
			s.Fatalf("flows: referrer = %+v, want %s %s %s", got, s.Recall(name), attachDisplay, inv.handle)
		}
	})
}

func expectAttribution(referee, kind, source string) scenario.Step {
	return func(s *scenario.Scenario) {
		var referrerID, refereeID, codeKind, gotSource string
		var hasCode, hasHandle bool
		err := s.DB().QueryRow(s.Context(), `SELECT payload->>'referrer_id', payload->>'referee_id',
			payload->>'code_kind', payload->>'source', payload ? 'code', payload ? 'handle'
			FROM events WHERE type = 'referral.attributed' AND payload->>'referee_id' = $1`,
			s.Recall(referee)).Scan(&referrerID, &refereeID, &codeKind, &gotSource, &hasCode, &hasHandle)
		if err != nil || hasCode || hasHandle || referrerID != s.Recall("referrer") || refereeID != s.Recall(referee) ||
			codeKind != kind || gotSource != source {
			s.Fatalf("flows: attribution %s = %s %s %s %s code=%t handle=%t (%v)",
				referee, referrerID, refereeID, codeKind, gotSource, hasCode, hasHandle, err)
		}
		var stored string
		err = s.DB().QueryRow(s.Context(), `SELECT code_kind FROM referrals WHERE referee_id = $1::uuid`,
			s.Recall(referee)).Scan(&stored)
		if err != nil || stored != kind {
			s.Fatalf("flows: stored code_kind for %s = %s (%v), want %s", referee, stored, err, kind)
		}
	}
}

func attachPair(inv invite) scenario.Step {
	return func(s *scenario.Scenario) {
		seedInvite("referrer", inv)(s)
		seedNewUser("caller")(s)
	}
}

type payer struct {
	cabal testkit.SeededCabal
	user  testkit.SeededUser
}

func (p payer) fundPath() string { return "/v1/cabals/" + p.cabal.ID.String() + "/fund" }

func seedPayer(s *scenario.Scenario, referrer string) payer {
	user := testkit.SeedUser(seedT{s}, s.DB(), testkit.UserOpts{WithWallet: true})
	user.Address = chain.AddressOf(fakes.PrivyWalletKey(user.PrivyWalletID).Public().(ed25519.PublicKey))
	if _, err := s.DB().Exec(s.Context(), `UPDATE user_wallets SET address = $1 WHERE user_id = $2`,
		string(user.Address), user.ID.UUID()); err != nil {
		s.Fatalf("flows: give the payer the fake Privy wallet's address: %v", err)
	}
	if _, err := s.DB().Exec(s.Context(), `UPDATE users SET phone_verified_at = now() WHERE id = $1`,
		user.ID.UUID()); err != nil {
		s.Fatalf("flows: verify the payer's phone: %v", err)
	}
	friend, err := ids.ParseUserID(s.Recall(referrer))
	if err != nil {
		s.Fatalf("flows: referrer id: %v", err)
	}
	return payer{
		user:  user,
		cabal: testkit.NewCabal(seedT{s}, s.DB(), testkit.WithCreator(user.ID), testkit.WithJoiner(friend)),
	}
}

func expectQualified(referee string) scenario.Step {
	return scenario.Eventually("the referral of "+referee+" qualified", func(s *scenario.Scenario) bool {
		var qualified bool
		err := s.DB().QueryRow(s.Context(), `SELECT EXISTS (SELECT 1 FROM referrals r
			JOIN events e ON e.type = 'referral.qualified' AND e.aggregate_id = r.id
			WHERE r.referee_id = $1::uuid AND r.status = 'qualified' AND r.qualified_at IS NOT NULL)`,
			referee).Scan(&qualified)
		return err == nil && qualified
	})
}

func F25AttachReferralOK(s *scenario.Scenario) {
	inv := newInvite()
	var pay payer
	s.Given(
		seedInvite("referrer", inv),
		func(s *scenario.Scenario) { pay = seedPayer(s, "referrer") },
		seedNewUser("caller"),
		seedNewUser("week"),
		ageCreated("week", "6 days 23 hours"),
		seedNewUser("day"),
		ageOnboarding("day", "23 hours"),
		seedNewUser("handle"),
		scenario.AsUser("caller"),
	).When(
		scenario.Post(attachPath, inv.codeBody("manual")),
		scenario.ExpectStatus(http.StatusCreated),
		expectReferrer("referrer", inv),
		scenario.Replay(),
		scenario.AsUser("week"),
		scenario.Post(attachPath, inv.codeBody("manual")),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.AsUser("day"),
		scenario.Post(attachPath, inv.codeBody("manual")),
		scenario.ExpectStatus(http.StatusCreated),
		scenario.AsUser("handle"),
		scenario.Post(attachPath, inv.handleBody("universal_link")),
		scenario.ExpectStatus(http.StatusCreated),
		expectReferrer("referrer", inv),
		func(s *scenario.Scenario) { scenario.AsSeededUser("payer", pay.user.ID)(s) },
		scenario.Post(attachPath, inv.codeBody("clipboard")),
		scenario.ExpectStatus(http.StatusCreated),
		func(s *scenario.Scenario) { scenario.Post(pay.fundPath(), qualifyFundBody)(s) },
		scenario.ExpectStatus(http.StatusAccepted),
		scenario.AwaitTick("treasury.fund-transfers"),
		scenario.AwaitTick("treasury.fund-transfers"),
		func(s *scenario.Scenario) { expectQualified(pay.user.ID.String())(s) },
	).Then(
		scenario.ExpectEvents(events.TypeReferralAttributed, 5),
		scenario.EventuallyPublished(events.TypeReferralAttributed, 5),
		func(s *scenario.Scenario) {
			var referral uuid.UUID
			if err := s.DB().QueryRow(s.Context(), `SELECT id FROM referrals WHERE referee_id = $1::uuid`,
				pay.user.ID.String()).Scan(&referral); err != nil {
				s.Fatalf("flows: read the payer's referral: %v", err)
			}
			scenario.EventuallyAggregateEvent(referral, "referral "+referral.String(), events.TypeReferralQualified)(s)
			scenario.EventuallyLog(observability.ReferralsQualified,
				map[string]string{"referral_id": referral.String()})(s)
		},
		scenario.EventuallyEvent(events.TypeReferralAttributed),
		expectAttribution("caller", "random", "manual"),
		expectAttribution("week", "random", "manual"),
		expectAttribution("day", "random", "manual"),
		expectAttribution("handle", "handle", "universal_link"),
		scenario.EventuallyLog(observability.ReferralsAttributed, map[string]string{
			"code_kind": "handle", "source": "universal_link",
		}),
	)
	s.Then(scenario.EventuallyCapturedBy(
		events.TypeReferralAttributed, "referral_attributed", "referrer_id", s.Recall("referrer")))
}

func F25AttachReferralReferralCodeUnknown(s *scenario.Scenario) {
	s.Given(seedNewUser("caller"), scenario.AsUser("caller")).
		When(scenario.Post(attachPath, newInvite().codeBody("manual"))).
		Then(
			scenario.ExpectProblem(errs.CodeReferralCodeUnknown),
			scenario.ExpectEvents(events.TypeReferralAttributed, 0),
		)
}

func F25AttachReferralReferralSelf(s *scenario.Scenario) {
	inv := newInvite()
	s.Given(seedInvite("caller", inv), scenario.AsUser("caller")).
		When(scenario.Post(attachPath, inv.codeBody("manual"))).
		Then(
			scenario.ExpectProblem(errs.CodeReferralSelf),
			scenario.ExpectEvents(events.TypeReferralAttributed, 0),
		)
}

func F25AttachReferralReferralAlreadyAttached(s *scenario.Scenario) {
	inv := newInvite()
	s.Given(attachPair(inv), scenario.AsUser("caller")).
		When(
			scenario.Post(attachPath, inv.codeBody("manual")),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.Post(attachPath, inv.codeBody("clipboard")),
		).
		Then(
			scenario.ExpectProblem(errs.CodeReferralAlreadyAttached),
			scenario.ExpectEvents(events.TypeReferralAttributed, 1),
		)
}

func F25AttachReferralReferralWindowClosed(s *scenario.Scenario) {
	inv := newInvite()
	s.Given(
		seedInvite("referrer", inv),
		seedNewUser("old"),
		ageCreated("old", "7 days"),
		seedNewUser("settled"),
		ageOnboarding("settled", "25 hours"),
		scenario.AsUser("old"),
	).When(
		scenario.Post(attachPath, inv.codeBody("manual")),
		scenario.ExpectProblem(errs.CodeReferralWindowClosed),
		scenario.AsUser("settled"),
		scenario.Post(attachPath, inv.codeBody("manual")),
	).Then(
		scenario.ExpectProblem(errs.CodeReferralWindowClosed),
		scenario.ExpectEvents(events.TypeReferralAttributed, 0),
	)
}

func F25AttachReferralUnauthorized(s *scenario.Scenario) {
	inv := newInvite()
	s.Given(scenario.Anonymous()).
		When(scenario.Post(attachPath, inv.codeBody("manual"))).
		Then(
			scenario.ExpectProblem(errs.CodeUnauthorized),
			scenario.ExpectEvents(events.TypeReferralAttributed, 0),
		)
}

func F25AttachReferralRateLimited(s *scenario.Scenario) {
	inv := newInvite()
	s.Given(attachPair(inv), scenario.AsUser("caller")).
		When(
			scenario.Post(attachPath, inv.codeBody("manual")),
			scenario.ExpectStatus(http.StatusCreated),
			scenario.Post(attachPath, inv.codeBody("clipboard")),
			scenario.ExpectProblem(errs.CodeReferralAlreadyAttached),
			scenario.Post(attachPath, inv.codeBody("manual")),
			scenario.ExpectProblem(errs.CodeReferralAlreadyAttached),
			scenario.Post(attachPath, inv.handleBody("manual")),
			scenario.ExpectProblem(errs.CodeReferralAlreadyAttached),
			scenario.Post(attachPath, inv.codeBody("universal_link")),
			scenario.ExpectProblem(errs.CodeReferralAlreadyAttached),
		).
		Then(
			scenario.Post(attachPath, inv.codeBody("manual")),
			scenario.ExpectProblem(errs.CodeRateLimited),
		)
}

func F25AttachReferralCrashBeforeCommit(s *scenario.Scenario) {
	inv := newInvite()
	s.Given(attachPair(inv), scenario.AsUser("caller")).
		When(
			scenario.Post(attachPath, inv.codeBody("manual")),
			scenario.Retry(),
			scenario.ExpectStatus(http.StatusCreated),
			expectReferrer("referrer", inv),
		).
		Then(
			scenario.ExpectEvents(events.TypeReferralAttributed, 1),
			scenario.EventuallyPublished(events.TypeReferralAttributed, 1),
			expectAttribution("caller", "random", "manual"),
		)
}

func expectReferralFollows(referrer, referee string) scenario.Step {
	return func(s *scenario.Scenario) {
		pairs := map[string]string{s.Recall(referrer): s.Recall(referee), s.Recall(referee): s.Recall(referrer)}
		for follower, followee := range pairs {
			var created int
			err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM follows f
				JOIN events e ON e.aggregate_id = f.id AND e.type = 'follow.created'
				WHERE f.follower_id = $1::text::uuid AND f.followee_id = $2::text::uuid AND f.source = 'referral'
				  AND f.deleted_at IS NULL AND e.payload->>'source' = 'referral'
				  AND e.payload->>'follower_id' = $1::text AND e.payload->>'followee_id' = $2::text`,
				follower, followee).Scan(&created)
			if err != nil || created != 1 {
				s.Fatalf("flows: referral follow %s -> %s with one follow.created = %d (%v), want 1",
					follower, followee, created, err)
			}
		}
		var acked int
		err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM event_deliveries d
			JOIN events e ON e.id = d.event_id
			WHERE d.handler = 'social.referral_follows' AND d.code = 'ok' AND e.type = 'referral.attributed'`).Scan(&acked)
		if err != nil || acked != 1 {
			s.Fatalf("flows: social.referral_follows acked %d referral.attributed events (%v), want 1", acked, err)
		}
	}
}

func F25AttachReferralSocialFollowsBothWays(s *scenario.Scenario) {
	inv := newInvite()
	s.Given(attachPair(inv), scenario.AsUser("caller")).
		When(
			scenario.Post(attachPath, inv.codeBody("manual")),
			scenario.ExpectStatus(http.StatusCreated),
		).
		Then(
			scenario.EventuallyPublished(events.TypeReferralAttributed, 1),
			scenario.Eventually("both referral follows land", func(s *scenario.Scenario) bool {
				var n int
				err := s.DB().QueryRow(s.Context(),
					`SELECT count(*) FROM events WHERE type = 'follow.created'`).Scan(&n)
				return err == nil && n == 2
			}),
			scenario.ExpectAllEvents(events.TypeFollowCreated, 2),
			expectReferralFollows("referrer", "caller"),
		)
}
