package testkit

import (
	"crypto/rand"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type cabalSpec struct {
	members   int
	joinMode  string
	voterMode string
	creator   *ids.UserID
	joiners   []ids.UserID
	name      string
}

type CabalOption func(*cabalSpec)

func WithMembers(n int) CabalOption { return func(s *cabalSpec) { s.members = n } }

func WithJoinMode(mode string) CabalOption { return func(s *cabalSpec) { s.joinMode = mode } }

func WithVoterMode(mode string) CabalOption { return func(s *cabalSpec) { s.voterMode = mode } }

func WithName(name string) CabalOption { return func(s *cabalSpec) { s.name = name } }

func WithCreator(user ids.UserID) CabalOption { return func(s *cabalSpec) { s.creator = &user } }

func WithJoiner(user ids.UserID) CabalOption {
	return func(s *cabalSpec) { s.joiners = append(s.joiners, user) }
}

type SeededCabal struct {
	ID              ids.CabalID
	Creator         SeededUser
	Members         []SeededUser
	InviteCode      string
	PrivyWalletID   string
	TreasuryAddress chain.SolanaAddress
}

func NewCabal(t SeedT, pool *pgxpool.Pool, opts ...CabalOption) SeededCabal {
	t.Helper()
	spec := cabalSpec{members: 1, joinMode: "request", voterMode: "all"}
	for _, opt := range opts {
		opt(&spec)
	}
	if spec.members < 1 {
		t.Fatalf("testkit.NewCabal: %d members, want at least the creator", spec.members)
	}
	id, err := ids.ParseCabalID(ids.Real{}.NewV7().String())
	if err != nil {
		t.Fatalf("testkit.NewCabal: %v", err)
	}
	if spec.name == "" {
		spec.name = "cabal " + id.String()[24:]
	}
	c := SeededCabal{ID: id, InviteCode: randomInviteCode(t), PrivyWalletID: "treasury-" + id.String()}
	c.Members = seatMembers(t, pool, spec)
	c.Creator = c.Members[0]
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	c.TreasuryAddress = chain.AddressOf(key)
	now := clock.Real{}.Now().UTC()
	if _, err := pool.Exec(t.Context(), `INSERT INTO cabals
		(id, name, creator_id, join_mode, voter_mode, threshold, proposal_expiry_seconds, invite_code,
		 created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'majority', 86400, $6, $7, $7)`,
		id.UUID(), spec.name, c.Creator.ID.UUID(), spec.joinMode, spec.voterMode, c.InviteCode, now,
	); err != nil {
		t.Fatalf("testkit.NewCabal: insert cabal: %v", err)
	}
	for i, m := range c.Members {
		role := "member"
		if i == 0 {
			role = "creator"
		}
		if _, err := pool.Exec(t.Context(), `INSERT INTO cabal_members (cabal_id, user_id, role, can_vote, joined_at)
			VALUES ($1, $2, $3, $4, $5)`,
			id.UUID(), m.ID.UUID(), role, i == 0 || spec.voterMode == "all", now.Add(time.Duration(i)*time.Millisecond),
		); err != nil {
			t.Fatalf("testkit.NewCabal: insert member %d: %v", i, err)
		}
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO treasury_wallets (cabal_id, privy_wallet_id, address, created_at)
		VALUES ($1, $2, $3, $4)`, id.UUID(), c.PrivyWalletID, string(c.TreasuryAddress), now,
	); err != nil {
		t.Fatalf("testkit.NewCabal: insert treasury wallet: %v", err)
	}
	return c
}

func seatMembers(t SeedT, pool *pgxpool.Pool, spec cabalSpec) []SeededUser {
	t.Helper()
	var members []SeededUser
	if spec.creator != nil {
		members = append(members, SeededUser{ID: *spec.creator})
	}
	for len(members) < spec.members {
		members = append(members, SeedUser(t, pool, UserOpts{}))
	}
	for _, user := range spec.joiners {
		members = append(members, SeededUser{ID: user})
	}
	return members
}

func randomInviteCode(t SeedT) string {
	t.Helper()
	code, err := domain.NewInviteCode(rand.Reader)
	if err != nil {
		t.Fatalf("testkit.NewCabal: %v", err)
	}
	return code.String()
}
