package testkit

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

var (
	placeholderPattern = regexp.MustCompile(`00000000-0000-0000-0000-0000000000[0-9a-f]{2}`)
	seedUUIDPattern    = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	seedChainPattern   = regexp.MustCompile(`"(treasury_address|tx_signature)":"([1-9A-HJ-NP-Za-km-z]+)"`)
)

var errWantUsers = errors.New("want users")

func placeholders(raw []byte) []string {
	found := map[string]bool{}
	for _, p := range placeholderPattern.FindAll(raw, -1) {
		found[string(p)] = true
	}
	out := make([]string, 0, len(found))
	for p := range found {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

func WantUsers(raw []byte) int { return len(placeholders(raw)) }

func existingUsers(ctx context.Context, pool *pgxpool.Pool, raw []byte) ([]ids.UserID, error) {
	want := len(placeholders(raw))
	if want == 0 {
		return nil, nil
	}
	rows, _ := pool.Query(ctx, `SELECT id FROM users WHERE deleted_at IS NULL ORDER BY created_at, id LIMIT $1`, want)
	found, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, fmt.Errorf("read users: %w", err)
	}
	out := make([]ids.UserID, len(found))
	for i, id := range found {
		if out[i], err = ids.ParseUserID(id.String()); err != nil {
			return nil, fmt.Errorf("user %s: %w", id, err)
		}
	}
	return out, nil
}

func bindIDs(name string, raw []byte, users []ids.UserID) ([]byte, error) {
	holders := placeholders(raw)
	if len(holders) == 0 {
		return raw, nil
	}
	if len(users) != len(holders) {
		return nil, fmt.Errorf("%w: %d, have %d", errWantUsers, len(holders), len(users))
	}
	userIDs := make([]string, len(users))
	for i, u := range users {
		userIDs[i] = u.String()
	}
	scope := name + "|" + strings.Join(userIDs, ",")
	namespace := uuid.NewSHA1(uuid.NameSpaceURL, []byte("monaco/testkit/scenarios"))
	stamps := eventStamps(raw)
	bound := seedUUIDPattern.ReplaceAllFunc(raw, func(old []byte) []byte {
		if i := slices.Index(holders, string(old)); i >= 0 {
			return []byte(userIDs[i])
		}
		return []byte(derivedID(namespace, scope, string(old), stamps[string(old)]).String())
	})
	return seedChainPattern.ReplaceAllFunc(bound, func(m []byte) []byte {
		parts := seedChainPattern.FindSubmatch(m)
		sum := sha512.Sum512([]byte(scope + "|" + string(parts[2])))
		derived := string(chain.SignatureOf(sum[:]))
		if string(parts[1]) == "treasury_address" {
			derived = string(chain.AddressOf(sum[:32]))
		}
		return bytes.Join([][]byte{[]byte(`"` + string(parts[1]) + `":"`), []byte(derived + `"`)}, nil)
	}), nil
}

func eventStamps(raw []byte) map[string]time.Time {
	stamps := map[string]time.Time{}
	lines := bufio.NewScanner(bytes.NewReader(raw))
	for lines.Scan() {
		var head struct {
			ID        string    `json:"id"`
			CreatedAt time.Time `json:"created_at"`
		}
		if json.Unmarshal(lines.Bytes(), &head) == nil {
			stamps[head.ID] = head.CreatedAt
		}
	}
	return stamps
}

func derivedID(namespace uuid.UUID, scope, old string, stamp time.Time) uuid.UUID {
	id := uuid.NewSHA1(namespace, []byte(scope+"|"+old))
	id[6] = id[6]&0x0f | 0x70
	if !stamp.IsZero() {
		var ms [8]byte
		binary.BigEndian.PutUint64(ms[:], uint64(stamp.UnixMilli()))
		copy(id[:6], ms[2:])
	}
	return id
}

func rowsSeeded(name string) bool { return name == "two-cabals-ranked" }

func seedRows(ctx context.Context, t SeedT, pool *pgxpool.Pool, line seedLine, ev events.Event) {
	t.Helper()
	switch e := ev.(type) {
	case events.CabalCreated:
		seedCabalRow(ctx, t, pool, line.CreatedAt, e)
	case events.CabalMemberJoined:
		seedMemberRow(ctx, t, pool, line.CreatedAt, e)
	case events.Funded:
		seedFunding(ctx, t, pool, line, e)
	}
}

func seedCabalRow(ctx context.Context, t SeedT, pool *pgxpool.Pool, at time.Time, e events.CabalCreated) {
	t.Helper()
	sum := sha256.Sum256(e.CabalID[:])
	code, err := domain.NewInviteCode(bytes.NewReader(bytes.Repeat(sum[:], 4)))
	if err != nil {
		t.Fatalf("testkit.Seed: invite code: %v", err)
	}
	if _, err := pool.Exec(
		ctx,
		`INSERT INTO cabals
		(id, name, creator_id, join_mode, voter_mode, threshold, proposal_expiry_seconds, invite_code,
		 created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9) ON CONFLICT (id) DO NOTHING`,
		e.CabalID,
		e.Name,
		e.CreatorID,
		e.JoinMode,
		e.VoterMode,
		e.Threshold,
		e.ProposalExpirySeconds,
		code.String(),
		at,
	); err != nil {
		t.Fatalf("testkit.Seed: insert cabal %s: %v", e.CabalID, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO treasury_wallets (cabal_id, privy_wallet_id, address, created_at)
		VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
		e.CabalID, "treasury-"+e.CabalID.String(), string(e.TreasuryAddress), at,
	); err != nil {
		t.Fatalf("testkit.Seed: insert treasury wallet of %s: %v", e.CabalID, err)
	}
}

func seedMemberRow(ctx context.Context, t SeedT, pool *pgxpool.Pool, at time.Time, e events.CabalMemberJoined) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO cabal_members (cabal_id, user_id, role, can_vote, joined_at)
		SELECT $1, $2, $3, $3 = 'creator' OR voter_mode = 'all', $4 FROM cabals WHERE id = $1
		ON CONFLICT DO NOTHING`, e.CabalID, e.UserID, e.Role, at,
	); err != nil {
		t.Fatalf("testkit.Seed: insert member %s of %s: %v", e.UserID, e.CabalID, err)
	}
}

func seedFunding(ctx context.Context, t SeedT, pool *pgxpool.Pool, line seedLine, e events.Funded) {
	t.Helper()
	NewLedgerFor(
		t,
		pool,
		derivedIDs{seed: line.ID, n: new(int)},
		line.CreatedAt,
	).FundMember(ctx, ids.UserIDFrom(e.UserID), ids.CabalIDFrom(e.CabalID), e.AmountMicros)
}

type derivedIDs struct {
	seed uuid.UUID
	n    *int
}

func (d derivedIDs) NewV7() uuid.UUID {
	*d.n++
	return uuid.NewSHA1(d.seed, fmt.Appendf(nil, "%d", *d.n))
}
