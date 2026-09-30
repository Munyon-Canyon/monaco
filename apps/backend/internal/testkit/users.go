package testkit

import (
	"crypto/rand"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type UserOpts struct {
	Handle        string
	AuthState     string
	AccountStatus string
	WithWallet    bool
}

type SeededUser struct {
	ID            ids.UserID
	PrivyUserID   string
	PrivyWalletID string
	Address       chain.SolanaAddress
}

func SeedUser(t SeedT, pool *pgxpool.Pool, opts UserOpts) SeededUser {
	t.Helper()
	id, err := ids.ParseUserID(ids.Real{}.NewV7().String())
	if err != nil {
		t.Fatalf("testkit.SeedUser: %v", err)
	}
	u := SeededUser{ID: id, PrivyUserID: "did:privy:" + id.String()}
	now := clock.Real{}.Now().UTC()
	if _, err := pool.Exec(t.Context(), `INSERT INTO users
		(id, privy_user_id, handle, login_provider, auth_state, auth_state_changed_at, account_status,
		 created_at, updated_at, deleted_at)
		VALUES ($1, $2, NULLIF($3::text, ''), 'sms', COALESCE(NULLIF($4::text, ''), 'CREATED'), $5::timestamptz,
		 COALESCE(NULLIF($6::text, ''), 'active'), $5, $5, CASE WHEN $6 = 'deleted' THEN $5 END)`,
		id.UUID(), u.PrivyUserID, opts.Handle, opts.AuthState, now, opts.AccountStatus,
	); err != nil {
		t.Fatalf("testkit.SeedUser: insert user: %v", err)
	}
	if !opts.WithWallet {
		return u
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	u.PrivyWalletID, u.Address = "wallet-"+id.String(), chain.AddressOf(key)
	if _, err := pool.Exec(t.Context(), `INSERT INTO user_wallets (user_id, privy_wallet_id, address, created_at)
		VALUES ($1, $2, $3, $4)`, id.UUID(), u.PrivyWalletID, string(u.Address), now); err != nil {
		t.Fatalf("testkit.SeedUser: insert wallet: %v", err)
	}
	return u
}
