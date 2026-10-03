package identity

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/authn"
	privyadapter "github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/privy"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type Module struct {
	deps     module.Deps
	privy    app.PrivyUsers
	wallets  app.MemberWallets
	hints    app.Hints
	photos   app.PhotoStore
	stakes   app.Stakes
	balances app.Balances
	meters   metric.MeterProvider
}

type Option func(*Module)

func WithPrivy(users app.PrivyUsers, wallets app.MemberWallets) Option {
	return func(m *Module) { m.privy, m.wallets = users, wallets }
}

func WithHints(hints app.Hints) Option { return func(m *Module) { m.hints = hints } }

func WithMeters(meters metric.MeterProvider) Option {
	return func(m *Module) { m.meters = meters }
}

func WithPhotoStore(store app.PhotoStore) Option {
	return func(m *Module) { m.photos = store }
}

func WithHoldings(stakes app.Stakes, balances app.Balances) Option {
	return func(m *Module) { m.stakes, m.balances = stakes, balances }
}

func New(d module.Deps, opts ...Option) *Module {
	m := &Module{
		deps: d, meters: otel.GetMeterProvider(),
		stakes: treasury.New(d).Queries(), balances: funding.New(d).Balances(),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (*Module) Name() string { return "identity" }

func (m *Module) Routes(r *httpx.Routes) {
	hints := m.hints
	if hints == nil {
		hints = m.deps.Bus
	}
	store := m.photos
	if store == nil {
		store = m.deps.Photos
	}
	m.ensurePrivy()
	r.IdentityRoutes = adapters.HTTP{
		Open: m.openSession(), Reads: m.deps.Pool, Clock: m.deps.Clock,
		Onboard: app.NewOnboarding(app.OnboardingDeps{
			UoW: m.deps.UoW, Reads: m.deps.Pool, Users: adapters.Users{}, Privy: m.privy, Clock: m.deps.Clock,
			Hints: hints,
		}),
		SetHandle: app.NewSetHandle(app.SetHandleDeps{
			UoW: m.deps.UoW, Reads: m.deps.Pool, Hints: hints, Users: adapters.Users{},
			ClaimFacts: app.LoadLockedHandleClaimFacts,
		}),
		Update: app.UpdateProfileHandler{UoW: m.deps.UoW, Reads: m.deps.Pool, Clock: m.deps.Clock, Hints: hints},
		Photo: app.UploadProfilePhotoHandler{
			UoW: m.deps.UoW, Reads: m.deps.Pool, Clock: m.deps.Clock, IDs: m.deps.IDs, Hints: hints,
			Store: store,
		},
		Delete: app.NewDeleteAccount(app.DeleteAccountDeps{
			UoW: m.deps.UoW, Users: adapters.Users{}, Balances: m.balances, Stakes: m.stakes, Clock: m.deps.Clock,
			Hints: hints,
		}),
	}
}

func (m *Module) ensurePrivy() {
	if m.privy != nil {
		return
	}
	client, err := privy.New(m.deps.Config, m.deps.Clock)
	if err != nil {
		panic(err)
	}
	m.privy, m.wallets = privyadapter.Users{Client: client}, privyadapter.Wallets{Client: client}
}

func (m *Module) openSession() *app.OpenSessionHandler {
	rule, err := app.NewWalletRule(m.wallets, m.meters)
	if err != nil {
		panic(err)
	}
	return app.NewOpenSessionHandler(app.OpenSessionDeps{
		UoW:     m.deps.UoW,
		Reads:   m.deps.Pool,
		Users:   adapters.Users{},
		Links:   adapters.Users{},
		Privy:   m.privy,
		Wallets: rule,
		IDs:     m.deps.IDs,
		Clock:   m.deps.Clock,
		Hints:   m.deps.Bus,
	})
}

func (*Module) Consumers() []bus.Consumer {
	return []bus.Consumer{}
}

func (m *Module) Pollers() []poller.Poller {
	store := m.photos
	if store == nil {
		store = m.deps.Photos
	}
	if store == nil {
		return nil
	}
	return []poller.Poller{app.NewPhotoPurges(m.deps.Pool, store, m.deps.Clock)}
}

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
	client, err := privy.New(d.Config, d.Clock)
	if err != nil {
		return nil, err
	}
	return authn.New(d.Config, d.Clock, privyadapter.Users{Client: client}, d.Pool)
}
