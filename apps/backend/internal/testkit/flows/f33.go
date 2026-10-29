package flows

import (
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const voteReminderPoller = "notify.vote_reminders"

func (defined) WorkerEnvF33() []string { return []string{"NOTIFY_VOTE_REMINDERS_INTERVAL=1s"} }

type voteReminderWorld struct {
	proposal ids.ProposalID
	voted    []ids.UserID
	waiting  []ids.UserID
}

func F33RemindVotersOK(s *scenario.Scenario) {
	var w voteReminderWorld
	s.Given(func(s *scenario.Scenario) { w = seedReminderWorld(s) }).
		When(scenario.EventuallyPolled(voteReminderPoller, "the reminders delivered", func(s *scenario.Scenario) bool {
			return remindersDelivered(s, &w) == len(w.waiting)
		})).
		Then(
			scenario.AwaitTick(voteReminderPoller),
			func(s *scenario.Scenario) { wantReminders(s, &w) },
		)
}

func F33RemindVotersCrashBeforeCommit(s *scenario.Scenario) {
	var w voteReminderWorld
	s.Given(func(s *scenario.Scenario) { w = seedReminderWorld(s) }).
		When(
			scenario.TickCrashingAt(voteReminderPoller, faultpoint.BeforeCommit),
			scenario.EventuallyPolled(voteReminderPoller, "the reminders delivered", func(s *scenario.Scenario) bool {
				return remindersDelivered(s, &w) == len(w.waiting)
			}),
		).
		Then(
			scenario.AwaitTick(voteReminderPoller),
			func(s *scenario.Scenario) { wantReminders(s, &w) },
		)
}

func seedReminderWorld(s *scenario.Scenario) voteReminderWorld {
	c := testkit.NewCabal(seedT{s}, s.DB(), testkit.WithMembers(6))
	id, now := ids.Real{}.NewV7(), time.Now().UTC()
	params := sqlc.InsertProposalParams{
		ID: id, CabalID: c.ID.UUID(), ProposerID: c.Creator.ID.UUID(), Kind: "buy", Symbol: "AAPLx",
		Mint: aaplxMint, UsdcMicros: pgtype.Int8{Int64: 5_000_000, Valid: true}, QuoteOutAmount: 21_000_000,
		CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(4 * time.Hour), Threshold: "majority",
	}
	w := voteReminderWorld{proposal: ids.ProposalIDFrom(id)}
	for i, m := range c.Members {
		params.VoterIds = append(params.VoterIds, m.ID.UUID())
		if i < 3 {
			w.voted = append(w.voted, m.ID)
		} else {
			w.waiting = append(w.waiting, m.ID)
		}
		registerDevice(s, m.ID)
	}
	if _, err := sqlc.New(s.DB()).InsertProposal(s.Context(), params); err != nil {
		s.Fatalf("flows: insert proposal: %v", err)
	}
	for i, voter := range w.voted {
		choice := "yes"
		if i == 2 {
			choice = "no"
		}
		if _, err := s.DB().Exec(s.Context(), `INSERT INTO votes (proposal_id, voter_id, choice, cast_at, first_cast_at)
			VALUES ($1, $2, $3, $4, $4)`, id, voter.UUID(), choice, now.Add(-time.Hour)); err != nil {
			s.Fatalf("flows: insert a vote: %v", err)
		}
	}
	return w
}

func registerDevice(s *scenario.Scenario, user ids.UserID) {
	if _, err := s.DB().Exec(s.Context(), `INSERT INTO device_tokens
		(id, user_id, token, environment, created_at, last_seen_at) VALUES ($1, $2, $3, $4, now(), now())`,
		ids.Real{}.NewV7(), user.UUID(), deviceToken(user, 0), string(apns.Sandbox)); err != nil {
		s.Fatalf("flows: register a device for %s: %v", user, err)
	}
}

func remindersDelivered(s *scenario.Scenario, w *voteReminderWorld) int {
	var n int
	if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM notifications
		WHERE kind = 'vote_reminder' AND state = 'delivered' AND data->>'proposal_id' = $1`,
		w.proposal.String()).Scan(&n); err != nil {
		s.Fatalf("flows: count the delivered reminders: %v", err)
	}
	return n
}

func wantReminders(s *scenario.Scenario, w *voteReminderWorld) {
	for _, user := range w.waiting {
		var rows int
		var title, body, category, level, collapse string
		if err := s.DB().QueryRow(s.Context(), `SELECT count(*), coalesce(min(title), ''), coalesce(min(body), ''),
			coalesce(min(category), ''), coalesce(min(interruption_level), ''), coalesce(min(collapse_id), '')
			FROM notifications WHERE user_id = $1 AND kind = 'vote_reminder'`,
			user.UUID()).Scan(&rows, &title, &body, &category, &level, &collapse); err != nil {
			s.Fatalf("flows: read the reminder of %s: %v", user, err)
		}
		if rows != 1 || title != "3 votes left · ends in 4h" || category != "VOTE" || level != "time-sensitive" ||
			collapse != "proposal-"+w.proposal.String() ||
			!strings.HasSuffix(body, ": 2 yes, 1 no so far. Yours counts.") {
			s.Fatalf("flows: reminder of %s: %d rows, %q / %q, category %q, level %q, collapse %q",
				user, rows, title, body, category, level, collapse)
		}
		wantNotificationsSent(s, user, 1)
	}
	var strangers int
	if err := s.DB().QueryRow(s.Context(), `SELECT count(*) FROM notifications
		WHERE kind = 'vote_reminder' AND data->>'proposal_id' = $1`, w.proposal.String()).Scan(&strangers); err != nil {
		s.Fatalf("flows: count the reminders: %v", err)
	}
	if strangers != len(w.waiting) {
		s.Fatalf(
			"flows: %d reminders, want one for each of the %d members who have not voted",
			strangers,
			len(w.waiting),
		)
	}
}
