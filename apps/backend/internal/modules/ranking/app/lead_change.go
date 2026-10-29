package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
)

type leadCall struct {
	ProposalID uuid.UUID `json:"proposal_id"`
	Kind       string    `json:"kind"`
	Symbol     string    `json:"symbol"`
}

func recordLeadChange(ctx context.Context, q *sqlc.Queries, in rankedInput, run boardRun) error {
	old, err := q.CallBoardRanksOf(ctx, sqlc.CallBoardRanksOfParams{
		Board: string(in.board), Period: string(in.period.Kind),
	})
	if err != nil {
		return err
	}
	if len(old) == 0 || len(in.entries) == 0 || !old[0].PeriodStart.Time.Equal(in.period.Start) {
		return nil
	}
	leader := in.entries[0]
	if leader.Rank == nil {
		return nil
	}
	held := slices.IndexFunc(old, func(r sqlc.CallBoardRanksOfRow) bool { return r.SubjectID == leader.ID })
	if held >= 0 && old[held].Rank.Int32 == 1 {
		return nil
	}
	prior, places := 0, 0
	if held >= 0 {
		prior = int(old[held].Rank.Int32)
		places = prior - 1
	}
	call, err := causalCall(ctx, q, in, leader.ID)
	if err != nil {
		return err
	}
	return q.InsertLeadChange(ctx, sqlc.InsertLeadChangeParams{
		ID:    uuid.NewSHA1(run.id, []byte(string(in.board)+"/"+string(in.period.Kind))),
		Board: string(in.board), Period: string(in.period.Kind),
		PeriodStart:     in.period.Start,
		LeaderID:        leader.ID,
		PassedID:        old[0].SubjectID,
		LeaderRateBps:   rankArg(*leader.RateBps),
		LeaderPriorRank: rankArg(prior),
		MovementPlaces:  rankArg(places),
		Call:            call,
		ChangedAt:       run.finishedAt,
	})
}

func causalCall(ctx context.Context, q *sqlc.Queries, in rankedInput, leader uuid.UUID) ([]byte, error) {
	row, err := q.LeaderCauseSince(ctx, sqlc.LeaderCauseSinceParams{
		SubjectID: leader, WindowStart: in.period.Start, WindowEnd: in.period.End,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !row.IsRight.Bool {
		return nil, nil
	}
	encoded, _ := json.Marshal(leadCall{ProposalID: row.ProposalID, Kind: row.Kind.String, Symbol: row.Symbol.String})
	return encoded, nil
}

type CallLeadChange struct {
	ID             uuid.UUID
	Leader, Passed CallSubjectView
	LeaderRatePct  int
	Call           *LeadCall
	ChangedAt      time.Time
	MovementPlaces int
}

type LeadCall struct {
	ProposalID uuid.UUID
	Kind       string
	Symbol     string
}

func (c *callBoardRead) leadChange(ctx context.Context, view *CallBoardView) error {
	if c.Leads == nil || c.head.PeriodStart.IsZero() {
		return nil
	}
	row, err := c.Leads.LatestLeadChange(ctx, sqlc.LatestLeadChangeParams{
		Board: string(c.q.Board), Period: string(c.q.Period), PeriodStart: c.head.PeriodStart,
		Since: c.Clock.Now().Add(-leadChangeTTL),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if c.q.Friends && !c.friendsLead(row) {
		return nil
	}
	out := &CallLeadChange{
		ID: row.ID, LeaderRatePct: domain.RatePct(int(row.LeaderRateBps)), ChangedAt: row.ChangedAt,
		MovementPlaces: int(row.MovementPlaces),
		Leader: CallSubjectView{
			ID: row.LeaderID, Cabal: row.LeaderKind == subjectCabal, Name: row.LeaderName,
			Handle: textOrNil(row.LeaderHandle.String), PictureURL: textOrNil(row.LeaderPictureUrl.String),
		},
		Passed: CallSubjectView{
			ID: row.PassedID, Cabal: row.PassedKind == subjectCabal, Name: row.PassedName,
			Handle: textOrNil(row.PassedHandle.String), PictureURL: textOrNil(row.PassedPictureUrl.String),
		},
	}
	var call leadCall
	if json.Unmarshal(row.Call, &call) == nil && call.ProposalID != uuid.Nil {
		out.Call = &LeadCall{ProposalID: call.ProposalID, Kind: call.Kind, Symbol: call.Symbol}
	}
	view.LeadChange = out
	return nil
}

func (c *callBoardRead) friendsLead(row sqlc.LatestLeadChangeRow) bool {
	return len(c.ranked) > 0 && c.ranked[0].SubjectID == row.LeaderID && slices.Contains(c.subjects, row.PassedID)
}
