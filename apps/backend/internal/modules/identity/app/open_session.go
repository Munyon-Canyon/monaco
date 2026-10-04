package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type OpenSession struct {
	Token string
}

type SessionUsers interface {
	FindByPrivyUserID(ctx context.Context, q sqlc.DBTX, privyUserID string) (domain.User, error)
	Lock(ctx context.Context, q sqlc.DBTX, privyUserID string) (domain.User, error)
	Create(ctx context.Context, q sqlc.DBTX, u domain.NewUser, at time.Time) (bool, error)
	AttachWallet(ctx context.Context, q sqlc.DBTX, id ids.UserID, w domain.Wallet, at time.Time) (bool, error)
	RefreshEmail(ctx context.Context, q sqlc.DBTX, id ids.UserID, email string, at time.Time) error
}

type LinkUsers interface {
	HeldLinks(
		ctx context.Context, q sqlc.DBTX, id ids.UserID, claims domain.Claims, links domain.Links,
	) (domain.Claims, error)
	ApplyLinks(ctx context.Context, q sqlc.DBTX, id ids.UserID, sync domain.LinkSync, at time.Time) error
}

type Hints interface {
	PublishHint(ctx context.Context, key string, payload []byte)
}

type OpenSessionDeps struct {
	UoW     *db.UnitOfWork
	Reads   sqlc.DBTX
	Users   SessionUsers
	Links   LinkUsers
	Privy   PrivyUsers
	Wallets *WalletRule
	IDs     ids.Generator
	Clock   clock.Clock
	Hints   Hints
}

type OpenSessionHandler struct {
	d OpenSessionDeps
}

func NewOpenSessionHandler(d OpenSessionDeps) *OpenSessionHandler { return &OpenSessionHandler{d: d} }

type session struct {
	privyID  PrivyUserID
	user     PrivyUser
	provider domain.LoginProvider
	wallet   domain.Wallet
	known    bool
}

func (h *OpenSessionHandler) Handle(ctx context.Context, cmd OpenSession) (Me, error) {
	s, err := h.introduce(ctx, cmd.Token)
	if err != nil {
		return Me{}, err
	}
	var (
		id      ids.UserID
		created bool
	)
	err = h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		id, created, err = h.settle(ctx, tx, s)
		return err
	})
	if err != nil {
		return Me{}, err
	}
	observability.Info(ctx, observability.IdentitySessionOpened,
		slog.String("user_id", id.String()), slog.Bool("created", created))
	return GetMe(ctx, h.d.Reads, id)
}

func (h *OpenSessionHandler) introduce(ctx context.Context, token string) (session, error) {
	privyID, err := h.d.Privy.Verify(ctx, token)
	if err != nil {
		return session{}, err
	}
	user, err := h.d.Privy.User(ctx, privyID)
	if errs.CodeOf(err) == errs.CodeNotFound {
		return session{}, errs.Wrap(err, errs.CodeUnauthorized, "identity.OpenSession")
	}
	if err != nil {
		return session{}, err
	}
	provider, err := domain.PickLoginProvider(user.LoginMethods())
	if err != nil {
		return session{}, err
	}
	stored, known, err := h.stored(ctx, privyID)
	if err != nil {
		return session{}, err
	}
	wallet, err := h.d.Wallets.Resolve(ctx, privyID, stored)
	if err != nil {
		return session{}, err
	}
	return session{privyID: privyID, user: user, provider: provider, wallet: wallet, known: known}, nil
}

func (h *OpenSessionHandler) stored(ctx context.Context, privyID PrivyUserID) (*domain.Wallet, bool, error) {
	row, err := h.d.Users.FindByPrivyUserID(ctx, h.d.Reads, string(privyID))
	switch {
	case errs.CodeOf(err) == errs.CodeUserNotFound:
		return nil, false, nil
	case err != nil:
		return nil, false, err
	case row.AccountStatus == domain.AccountDeleted:
		return nil, true, errs.New(errs.CodeAccountDeleted, "identity.OpenSession")
	}
	return row.Wallet, true, nil
}

func (h *OpenSessionHandler) settle(ctx context.Context, tx db.Tx, s session) (ids.UserID, bool, error) {
	now := h.d.Clock.Now()
	q := tx.Queries()
	row, created, err := h.lock(ctx, q, s, now)
	if err != nil {
		return ids.UserID{}, false, err
	}
	ctx = observability.WithActor(ctx, auth.Actor{Kind: auth.ActorUser, ID: row.ID.String()}.Key())
	if err := h.refresh(ctx, q, row, s, now); err != nil {
		return ids.UserID{}, false, err
	}
	changed, err := h.syncLinks(ctx, tx, row, s.user.Links(), now)
	if err != nil {
		return ids.UserID{}, false, err
	}
	if created {
		err := tx.Events.Append(ctx, events.UserCreated{
			V: 1, UserID: row.ID.UUID(), LoginProvider: string(s.provider), CreatedAt: now,
		})
		if err != nil {
			return ids.UserID{}, false, err
		}
	}
	if created || changed {
		h.announce(tx, row.ID)
	}
	return row.ID, created, nil
}

func (h *OpenSessionHandler) refresh(
	ctx context.Context, q sqlc.DBTX, row domain.User, s session, now time.Time,
) error {
	if err := h.attach(ctx, q, row, s.wallet, now); err != nil {
		return err
	}
	return h.d.Users.RefreshEmail(ctx, q, row.ID, s.user.ContactEmail(), now)
}

func (h *OpenSessionHandler) syncLinks(
	ctx context.Context, tx db.Tx, row domain.User, privy domain.Links, now time.Time,
) (bool, error) {
	q := tx.Queries()
	if claims := privy.Claims(row.Links, row.AuthState); claims.Any() {
		held, err := h.d.Links.HeldLinks(ctx, q, row.ID, claims, privy)
		if err != nil {
			return false, err
		}
		privy = privy.Without(held)
	}
	sync, err := domain.SyncLinks(row.AuthState, row.Links, privy)
	if err != nil || sync.Empty() {
		return false, err
	}
	if err := h.d.Links.ApplyLinks(ctx, q, row.ID, sync, now); err != nil {
		return false, err
	}
	return true, appendAuthSteps(ctx, tx, row.ID, sync.Steps, now)
}

func (h *OpenSessionHandler) lock(
	ctx context.Context, q sqlc.DBTX, s session, now time.Time,
) (domain.User, bool, error) {
	created := false
	if !s.known {
		var err error
		created, err = h.d.Users.Create(ctx, q, domain.NewUser{
			ID: ids.NewUserID(h.d.IDs), PrivyUserID: string(s.privyID), LoginProvider: s.provider,
		}, now)
		if err != nil {
			return domain.User{}, false, err
		}
	}
	row, err := h.d.Users.Lock(ctx, q, string(s.privyID))
	if err != nil {
		return domain.User{}, false, err
	}
	if row.AccountStatus == domain.AccountDeleted {
		return domain.User{}, false, errs.New(errs.CodeAccountDeleted, "identity.OpenSession")
	}
	return row, created, nil
}

func (h *OpenSessionHandler) attach(
	ctx context.Context, q sqlc.DBTX, row domain.User, wallet domain.Wallet, now time.Time,
) error {
	const op = "identity.OpenSession"
	if row.Wallet != nil {
		if row.Wallet.Address == wallet.Address {
			return nil
		}
		return errs.New(errs.CodeWalletMismatch, op, slog.String("stored_wallet_id", row.Wallet.PrivyWalletID),
			slog.String("privy_wallet_id", wallet.PrivyWalletID))
	}
	attached, err := h.d.Users.AttachWallet(ctx, q, row.ID, wallet, now)
	if err != nil {
		return err
	}
	if !attached {
		return errs.New(errs.CodeWalletMismatch, op, slog.String("privy_wallet_id", wallet.PrivyWalletID))
	}
	return nil
}

func (h *OpenSessionHandler) announce(tx db.Tx, id ids.UserID) {
	tx.AfterCommit(func(ctx context.Context) {
		h.d.Hints.PublishHint(ctx, "user."+id.String()+".me_changed", nil)
	})
}
