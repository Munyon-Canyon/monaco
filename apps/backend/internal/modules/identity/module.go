package identity

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
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
	deps    module.Deps
	privy   app.PrivyUsers
	wallets app.MemberWallets
	photos  app.PhotoStore
	meters  metric.MeterProvider
}

type Option func(*Module)

func WithPrivy(users app.PrivyUsers, wallets app.MemberWallets) Option {
	return func(m *Module) { m.privy, m.wallets = users, wallets }
}

func WithMeters(meters metric.MeterProvider) Option {
	return func(m *Module) { m.meters = meters }
}

func WithPhotoStore(store app.PhotoStore) Option {
	return func(m *Module) { m.photos = store }
}

func New(d module.Deps, opts ...Option) *Module {
	m := &Module{deps: d, meters: otel.GetMeterProvider()}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (*Module) Name() string { return "identity" }

func (m *Module) Routes(r *httpx.Routes) {
	store := m.photos
	if store == nil {
		store = m.deps.Photos
	}
	r.IdentityRoutes = adapters.HTTP{
		Open: m.openSession(), Reads: m.deps.Pool, Clock: m.deps.Clock,
		Update: app.UpdateProfileHandler{UoW: m.deps.UoW, Reads: m.deps.Pool, Clock: m.deps.Clock, Hints: m.deps.Bus},
		Photo: app.UploadProfilePhotoHandler{
			UoW: m.deps.UoW, Reads: m.deps.Pool, Clock: m.deps.Clock, IDs: m.deps.IDs, Hints: m.deps.Bus,
			Store: store,
		},
	}
}

func (m *Module) openSession() *app.OpenSessionHandler {
	users, wallets := m.privy, m.wallets
	if users == nil {
		client := privy.New(m.deps.Config, m.deps.Clock)
		users, wallets = privyadapter.Users{Client: client}, privyadapter.Wallets{Client: client}
	}
	rule, err := app.NewWalletRule(wallets, m.meters)
	if err != nil {
		panic(err)
	}
	return app.NewOpenSessionHandler(app.OpenSessionDeps{
		UoW:     m.deps.UoW,
		Reads:   m.deps.Pool,
		Users:   adapters.Users{},
		Links:   adapters.Users{},
		Privy:   users,
		Wallets: rule,
		IDs:     m.deps.IDs,
		Clock:   m.deps.Clock,
		Hints:   m.deps.Bus,
	})
}

func (*Module) Consumers() []bus.Consumer {
	return []bus.Consumer{}
}

func (*Module) Pollers() []poller.Poller { return nil }

func (m *Module) Queries() port.Queries { return adapters.NewQueries(m.deps.Pool) }

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
