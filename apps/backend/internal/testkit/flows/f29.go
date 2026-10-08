package flows

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	banRequestPath = "/v1/admin/cabals/{cabal}/ban-requests"
	approvePath    = "/v1/admin/approvals/{approval}/approve"
	windDownPoller = "treasury.winddown"
	expiryPoller   = "admin.approvals_expire"
	f29Creator     = 3_000_000
	f29Member      = 1_000_000
	f29Request     = `{"reason":"scam cabal"}`
	f29Approval    = `{"reason":"confirmed"}`
)

func f29Ready(script string) []scenario.Step {
	return []scenario.Step{
		scenario.SeededAdmin("opA", "operator"), scenario.SeededAdmin("opB", "operator"),
		scenario.SignIn("did:privy:qa-f29-" + script + "-creator"), scenario.ExpectStatus(http.StatusOK),
		scenario.Post(cabalsPath, cabalOf("open", "all")), scenario.ExpectStatus(http.StatusCreated),
		scenario.Remember("id", "cabal"), funded(f29Creator),
		scenario.SignIn("did:privy:qa-f29-" + script + "-member"), scenario.ExpectStatus(http.StatusOK),
		memberJoins, funded(f29Member), chainSays("finalized"),
	}
}

func memberJoins(s *scenario.Scenario) {
	if _, err := s.DB().Exec(s.Context(), `INSERT INTO cabal_members (cabal_id, user_id, role, can_vote, joined_at)
		VALUES ($1, $2, 'member', true, now())`, s.Recall("cabal"), s.ActorID().UUID()); err != nil {
		s.Fatalf("flows: seat the second member: %v", err)
	}
}

func f29Requested(script string) []scenario.Step {
	return append(f29Ready(script), asOperatorNamed("opA"), scenario.Post(banRequestPath, f29Request),
		scenario.ExpectStatus(http.StatusOK), scenario.ExpectJSON("status", "pending"),
		scenario.Remember("id", "approval"))
}

func asOperatorNamed(name string) scenario.Step { return scenario.AsUser(name) }

func cabalStatusIs(want string) scenario.Step {
	return func(s *scenario.Scenario) {
		var got string
		if err := s.DB().QueryRow(s.Context(), `SELECT status FROM cabals WHERE id = $1`, s.Recall("cabal")).
			Scan(&got); err != nil {
			s.Fatalf("flows: read the cabal status: %v", err)
		}
		if got != want {
			s.Fatalf("flows: cabal status %s, want %s", got, want)
		}
	}
}

func approvalStatusIs(want string) scenario.Step {
	return func(s *scenario.Scenario) {
		var got string
		if err := s.DB().QueryRow(s.Context(), `SELECT status FROM admin_approvals WHERE id = $1`,
			s.Recall("approval")).Scan(&got); err != nil {
			s.Fatalf("flows: read the approval status: %v", err)
		}
		if got != want {
			s.Fatalf("flows: approval status %s, want %s", got, want)
		}
	}
}

func windDownScalar(s *scenario.Scenario, query string) (n int64) {
	if err := s.DB().QueryRow(s.Context(), query, s.Recall("cabal")).Scan(&n); err != nil {
		s.Fatalf("flows: read the wind-down state: %v", err)
	}
	return n
}

func windDownCompletes() scenario.Step {
	return scenario.Eventually("the wind-down to complete", func(s *scenario.Scenario) bool {
		scenario.AwaitTick(windDownPoller)(s)
		return windDownScalar(
			s,
			`SELECT count(*) FROM events WHERE type = 'cabal.wound_down' AND payload->>'cabal_id' = $1`,
		) == 1
	})
}

func membersWoundDown(s *scenario.Scenario) {
	shares := windDownScalar(s, `SELECT coalesce(sum(share_units), 0)::bigint FROM user_positions WHERE cabal_id = $1`)
	settled := windDownScalar(s, `SELECT count(*) FROM user_txns WHERE cabal_id = $1 AND kind = 'cash_out'
		AND status = 'settled'`)
	jobs := windDownScalar(s, `SELECT count(*) FROM cash_out_jobs WHERE cabal_id = $1 AND cause = 'wind_down'
		AND status = 'completed'`)
	returned := windDownScalar(s, `SELECT coalesce(sum(payout_micros), 0)::bigint FROM cash_out_jobs WHERE cabal_id = $1
		AND cause = 'wind_down'`)
	if shares != 0 || settled != 2 || jobs != 2 || returned != f29Creator+f29Member {
		s.Fatalf("flows: wind-down left %d shares, %d settled cash outs, %d completed jobs and %d returned, "+
			"want none, 2, 2 and %d", shares, settled, jobs, returned, f29Creator+f29Member)
	}
	var payload []byte
	if err := s.DB().QueryRow(s.Context(), `SELECT payload FROM events WHERE type = 'cabal.wound_down'
		AND payload->>'cabal_id' = $1`, s.Recall("cabal")).Scan(&payload); err != nil {
		s.Fatalf("flows: read cabal.wound_down: %v", err)
	}
	var wound struct {
		MembersPaid int64  `json:"members_paid"`
		Returned    string `json:"usdc_returned_micros"`
	}
	if err := json.Unmarshal(payload, &wound); err != nil ||
		wound.MembersPaid != 2 || wound.Returned != strconv.Itoa(f29Creator+f29Member) {
		s.Fatalf("flows: cabal.wound_down %s, want 2 members and %d micros", payload, f29Creator+f29Member)
	}
}

func hiddenFromTheMember(script string) []scenario.Step {
	empty := func(s *scenario.Scenario, raw json.RawMessage) {
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil || len(items) != 0 {
			s.Fatalf("flows: banned cabal still listed in %s", raw)
		}
	}
	return []scenario.Step{
		scenario.AsUser("did:privy:qa-f29-" + script + "-member"),
		scenario.Get(cabalsPath + "?q=Friends"), scenario.ExpectStatus(http.StatusOK),
		scenario.ExpectField("items", empty),
	}
}

func F29RequestCabalBanOK(s *scenario.Scenario) {
	s.Given(f29Ready("request")...).
		When(asOperatorNamed("opA"), scenario.Post(banRequestPath, f29Request), scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "pending"), scenario.ExpectJSON("action", "cabal_ban")).
		Then(scenario.ExpectAllEvents(events.TypeAdminApprovalRequested, 1),
			scenario.ExpectAllEvents(events.TypeAdminCabalBanApproved, 0), cabalStatusIs("active"), cabalHolds())
}

func F29RequestCabalBanCabalNotActive(s *scenario.Scenario) {
	s.Given(append(f29Ready("notactive"), func(s *scenario.Scenario) {
		if _, err := s.DB().
			Exec(s.Context(), `UPDATE cabals SET status = 'banned' WHERE id = $1`, s.Recall("cabal")); err != nil {
			s.Fatalf("flows: ban the cabal: %v", err)
		}
	})...).
		When(asOperatorNamed("opA"), scenario.Post(banRequestPath, f29Request)).
		Then(scenario.ExpectProblem(errs.CodeCabalNotActive), scenario.ExpectAllEvents(events.TypeAdminApprovalRequested, 0))
}

func F29ApproveCabalBanOK(s *scenario.Scenario) {
	s.Given(f29Requested("ok")...).
		When(asOperatorNamed("opB"), scenario.Post(approvePath, f29Approval), scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "approved")).
		Then(bannedAndWoundDown("ok")...)
}

func F29ApproveCabalBanCrashBeforeCommit(s *scenario.Scenario) {
	s.Given(f29Requested("commitcrash")...).
		When(asOperatorNamed("opB"), scenario.Post(approvePath, f29Approval), scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("status", "approved"),
			scenario.Eventually("every payout to land", func(s *scenario.Scenario) bool {
				return windDownScalar(s, `SELECT count(*) FROM cash_out_jobs WHERE cabal_id = $1 AND cause = 'wind_down'
					AND status = 'completed'`) == 2
			}), scenario.TickCrashingAt(windDownPoller, faultpoint.BeforeCommit)).
		Then(bannedAndWoundDown("commitcrash")...)
}

func bannedAndWoundDown(script string) []scenario.Step {
	return slices.Concat([]scenario.Step{
		scenario.Eventually("the cabal to be banned", func(s *scenario.Scenario) bool {
			return windDownScalar(s, `SELECT count(*) FROM cabals WHERE id = $1 AND status = 'banned'`) == 1
		}),
		banAudited,
		scenario.ExpectAllEvents(events.TypeCabalBanned, 1), windDownCompletes(), membersWoundDown,
		cabalHolds(),
	}, hiddenFromTheMember(script))
}

func F29ApproveCabalBanSameApprover(s *scenario.Scenario) {
	s.Given(f29Requested("same")...).
		When(asOperatorNamed("opA"), scenario.Post(approvePath, f29Approval)).
		Then(scenario.ExpectProblem(errs.CodeSameApprover), approvalStatusIs("pending"), cabalStatusIs("active"),
			scenario.ExpectAllEvents(events.TypeAdminCabalBanApproved, 0), sharesIntact, cabalHolds())
}

func F29ApproveCabalBanApprovalExpired(s *scenario.Scenario) {
	s.Given(append(f29Requested("expired"), func(s *scenario.Scenario) {
		if _, err := s.DB().Exec(s.Context(),
			`UPDATE admin_approvals SET expires_at = now() - interval '1 minute' WHERE id = $1`, s.Recall("approval")); err != nil {
			s.Fatalf("flows: age the approval: %v", err)
		}
	})...).
		When(asOperatorNamed("opB"), scenario.Post(approvePath, f29Approval)).
		Then(scenario.ExpectProblem(errs.CodeApprovalExpired), cabalStatusIs("active"),
			scenario.AwaitTick(expiryPoller), approvalStatusIs("expired"),
			scenario.ExpectAllEvents(events.TypeAdminCabalBanApproved, 0), sharesIntact, cabalHolds())
}

func sharesIntact(s *scenario.Scenario) {
	if got := windDownScalar(
		s,
		`SELECT coalesce(sum(share_units), 0)::bigint FROM user_positions WHERE cabal_id = $1`,
	); got == 0 {
		s.Fatalf("flows: no member holds shares, want the stake untouched")
	}
}

func banAudited(s *scenario.Scenario) {
	var n int
	err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM events WHERE type = 'admin.action'
		AND payload->>'action' = 'cabal_ban' AND payload->>'target_id' = $1 AND payload->>'admin_id' = $2
		AND payload->>'approved_by' = $3 AND payload->'after'->>'status' = 'banned'`,
		s.Recall("cabal"), s.Recall("opA"), s.Recall("opB")).Scan(&n)
	if err != nil || n != 1 {
		s.Fatalf(
			"flows: %d cabal_ban audits naming operator A as the requester and B as the approver (%v), want 1",
			n,
			err,
		)
	}
}

func BannedCabalRefusesProposeFundAndJoin(s *scenario.Scenario) {
	s.Given(append(f29Ready("banned"), func(s *scenario.Scenario) {
		if _, err := s.DB().
			Exec(s.Context(), `UPDATE cabals SET status = 'banned' WHERE id = $1`, s.Recall("cabal")); err != nil {
			s.Fatalf("flows: ban the cabal: %v", err)
		}
	})...).
		When(
			scenario.Post(cabalsPath+"/{cabal}/fund", `{"amount_micros":"1000000"}`),
			scenario.ExpectProblem(errs.CodeCabalBanned),
			scenario.Post(cabalsPath+"/{cabal}/proposals", `{"kind":"buy","symbol":"AAPLx","usdc_micros":1000000}`),
			scenario.ExpectProblem(errs.CodeCabalBanned),
			scenario.SignIn("did:privy:qa-f29-banned-outsider"), scenario.ExpectStatus(http.StatusOK),
			scenario.Post(requestsPath, ""), scenario.ExpectProblem(errs.CodeCabalBanned),
			scenario.AsUser("did:privy:qa-f29-banned-member"),
			scenario.Post(cashOutsPath, `{"all":true}`), scenario.ExpectStatus(http.StatusAccepted),
			scenario.Remember("id", "job"),
		).
		Then(jobEnds("completed", ""), cabalHolds())
}
