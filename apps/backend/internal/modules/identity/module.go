package identity

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/authn"
	privyadapter "github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/privy"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct {
	deps module.Deps
}

func New(d module.Deps) *Module { return &Module{deps: d} }

func (*Module) Name() string { return "identity" }

func (*Module) Routes(*httpx.Routes) {}

func (*Module) Consumers() []bus.Consumer {
	return []bus.Consumer{}
}

func (*Module) Pollers() []poller.Poller { return nil }

func (m *Module) Queries() adapters.Queries { return adapters.NewQueries(m.deps.Pool) }

type (
	UserCard      = app.UserCard
	MemberWallet  = app.MemberWallet
	AuthState     = domain.AuthState
	AccountStatus = domain.AccountStatus
)

const (
	AuthCreated             = domain.AuthCreated
	AuthAwaitingPhone       = domain.AuthAwaitingPhone
	AuthAwaitingSocials     = domain.AuthAwaitingSocials
	AuthOnboardingCompleted = domain.AuthOnboardingCompleted

	AccountActive    = domain.AccountActive
	AccountSuspended = domain.AccountSuspended
	AccountBanned    = domain.AccountBanned
	AccountDeleted   = domain.AccountDeleted
)

const (
	MaxUsersByID   = app.MaxUsersByID
	MaxHandles     = app.MaxHandles
	MaxPhoneHashes = app.MaxPhoneHashes
	MaxXUserIDs    = app.MaxXUserIDs
	MaxWalletPage  = app.MaxWalletPage
)

type UserReader interface {
	UsersByID(ctx context.Context, userIDs []ids.UserID) (map[ids.UserID]UserCard, error)
	UserByHandle(ctx context.Context, handle string) (UserCard, error)
	UserIDsByHandles(ctx context.Context, handles []string) (map[string]ids.UserID, error)
}

type ContactMatcher interface {
	UsersByPhoneHashes(ctx context.Context, hashes [][]byte) (map[string]ids.UserID, error)
	UsersByXUserIDs(ctx context.Context, xUserIDs []string) (map[string]ids.UserID, error)
}

type WalletReader interface {
	MemberWallet(ctx context.Context, id ids.UserID) (MemberWallet, error)
	MemberWallets(ctx context.Context, after ids.UserID, limit int) ([]MemberWallet, error)
}

type Queries interface {
	UserReader
	ContactMatcher
	WalletReader
}

func NewVerifier(d module.Deps) (*authn.Verifier, error) {
	return authn.New(d.Config, d.Clock, privyadapter.Users{Client: privy.New(d.Config, d.Clock)}, d.Pool)
}
