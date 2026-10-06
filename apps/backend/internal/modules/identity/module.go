package identity

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/authn"
	privyadapter "github.com/monaco/monaco/apps/backend/internal/modules/identity/adapters/privy"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/identityapi"
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
	follows  app.FollowCounts
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

func WithFollowCounts(follows app.FollowCounts) Option {
	return func(m *Module) { m.follows = follows }
}

func New(d module.Deps, opts ...Option) *Module {
	m := &Module{
		deps: d, meters: otel.GetMeterProvider(),
		stakes: app.UnwiredHoldings{}, balances: app.UnwiredHoldings{}, follows: app.UnwiredFollowCounts{},
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (*Module) Name() string { return "identity" }

func (m *Module) Wire(set module.Set) {
	for _, mod := range set {
		_, balancesUnwired := m.balances.(app.UnwiredHoldings)
		_, stakesUnwired := m.stakes.(app.UnwiredHoldings)
		_, followsUnwired := m.follows.(app.UnwiredFollowCounts)
		switch provider := mod.(type) {
		case interface{ Balances() fundingport.Balances }:
			if balancesUnwired {
				m.balances = provider.Balances()
			}
		case interface{ FollowCounts() app.FollowCounts }:
			if followsUnwired {
				m.follows = provider.FollowCounts()
			}
		case interface{ Queries() treasuryport.Queries }:
			if stakesUnwired {
				m.stakes = provider.Queries()
			}
		}
	}
}

func (m *Module) Mount(r api.Mount) {
	hints := m.hints
	if hints == nil {
		hints = m.deps.Bus
	}
	store := m.photos
	if store == nil {
		store = m.deps.Photos
	}
	m.ensurePrivy()
	identityapi.Mount(adapters.HTTP{
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
		DevX: m.devXLink(),
	}, r)
}

func (m *Module) devXLink() *app.DevXLink {
	if m.deps.Config.Env == config.EnvProduction {
		return nil
	}
	return app.NewDevXLink(app.DevXLinkDeps{
		UoW: m.deps.UoW, Reads: m.deps.Pool, Users: adapters.Users{}, Privy: m.privy, Clock: m.deps.Clock,
	})
}

func (m *Module) ensurePrivy() {
	if m.privy == nil {
		client, err := privy.New(m.deps.Config, m.deps.Clock)
		if err != nil {
			panic(err)
		}
		m.privy, m.wallets = privyadapter.Users{Client: client}, privyadapter.Wallets{Client: client}
	}
	if _, wrapped := m.privy.(adapters.DevX); !wrapped && m.deps.Config.Env != config.EnvProduction {
		m.privy = adapters.DevX{PrivyUsers: m.privy, Reads: m.deps.Pool}
	}
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
	return []bus.Consumer{
		{
			Durable: "identity_first_deposit",
			Handlers: []bus.HandlerSpec{
				bus.Handle("identity.first_deposit", adapters.FirstDeposit{}.Handle),
			},
		},
	}
}

func (m *Module) Pollers() []poller.Poller {
	nudges := app.NewEmitNudges(m.deps.UoW, m.deps.Pool, m.deps.Clock, m.deps.Config.Identity.NudgesInterval)
	store := m.photos
	if store == nil {
		store = m.deps.Photos
	}
	if store == nil {
		return []poller.Poller{nudges}
	}
	return []poller.Poller{
		nudges,
		app.NewPhotoPurges(m.deps.Pool, store, m.deps.Clock, m.deps.Config.Identity.PhotoPurgesInterval),
	}
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
