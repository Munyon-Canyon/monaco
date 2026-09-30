package identity

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/authn"
	privyadapter "github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/privy"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
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

func (m *Module) Queries() port.Postgres { return port.New(m.deps.Pool) }

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

type (
	UserReader     = port.UserReader
	ContactMatcher = port.ContactMatcher
	WalletReader   = port.WalletReader
	Queries        = port.Queries
)

func NewVerifier(d module.Deps) (*authn.Verifier, error) {
	return authn.New(d.Config, d.Clock, privyadapter.Users{Client: privy.New(d.Config, d.Clock)}, d.Pool)
}
