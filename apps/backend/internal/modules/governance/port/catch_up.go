package port

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type CatchUpPerson struct {
	ID       ids.UserID
	Name     string
	PhotoURL string
}

type CatchUpProposal struct {
	ID          ids.ProposalID
	CabalID     ids.CabalID
	CabalName   string
	Kind        string
	Symbol      string
	Mint        string
	AssetName   string
	AssetLogo   string
	USDCMicros  *uint64
	TokenAmount *uint64
	Proposer    CatchUpPerson
	CreatedAt   time.Time
	ExpiresAt   time.Time
	Status      Status
	Yes, No     int
	Voted       int
	Eligible    int
	ReplyCount  int
	YourChoice  string
	DecidedBy   *ids.UserID
}

type CatchUp interface {
	PendingVotes(ctx context.Context, viewer ids.UserID) ([]CatchUpProposal, error)
	ProposalCards(
		ctx context.Context,
		viewer ids.UserID,
		proposals []ids.ProposalID,
	) (map[ids.ProposalID]CatchUpProposal, error)
}
