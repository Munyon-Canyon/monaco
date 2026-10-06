package agents_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	checkViolation      = "23514"
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
)

func newID() uuid.UUID { return ids.Real{}.NewV7() }

func run(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) error {
	t.Helper()
	_, err := pool.Exec(t.Context(), sql, args...)
	return err
}

func wantViolation(t *testing.T, what string, err error, code, constraint string) {
	t.Helper()
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != code || pg.ConstraintName != constraint {
		t.Errorf("%s: err = %v, want SQLSTATE %s on %s", what, err, code, constraint)
	}
}

func insertAgent(t *testing.T, pool *pgxpool.Pool, id, cabal uuid.UUID, status string) error {
	t.Helper()
	return run(t, pool, `INSERT INTO agents (id, cabal_id, name, budget_usdc_micros, status, added_by_proposal_id,
		created_at, updated_at) VALUES ($1, $2, 'Scout', 100000000, $3, $4, now(), now())`, id, cabal, status, newID())
}

func insertIntent(t *testing.T, pool *pgxpool.Pool, agent uuid.UUID, side string, usdc, token *int64) error {
	t.Helper()
	return run(t, pool, `INSERT INTO agent_intents (id, agent_id, cabal_id, side, mint, symbol, usdc_micros,
		token_amount, quote_out_amount, status, created_at) VALUES ($1, $2, $3, $4,
		'XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp', 'AAPLx', $5, $6, 105000000, 'accepted', now())`,
		newID(), agent, newID(), side, usdc, token)
}

func ptr(v int64) *int64 { return &v }

func TestAgentsSchema_oneLiveAgentPerCabal(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	cabal, first := newID(), newID()
	if err := insertAgent(t, pool, first, cabal, "active"); err != nil {
		t.Fatal(err)
	}
	wantViolation(t, "a second active agent", insertAgent(t, pool, newID(), cabal, "active"),
		uniqueViolation, "agents_live_cabal_idx")
	if err := run(t, pool, `UPDATE agents SET status = 'paused' WHERE id = $1`, first); err != nil {
		t.Fatal(err)
	}
	wantViolation(t, "a second agent beside a paused one", insertAgent(t, pool, newID(), cabal, "paused"),
		uniqueViolation, "agents_live_cabal_idx")
	if err := insertAgent(t, pool, newID(), newID(), "active"); err != nil {
		t.Errorf("an agent for another cabal: %v", err)
	}

	const removeFirst = `UPDATE agents SET status = 'removed', removed_at = now() WHERE id = $1`
	if err := run(t, pool, removeFirst, first); err != nil {
		t.Fatal(err)
	}
	if err := insertAgent(t, pool, newID(), cabal, "active"); err != nil {
		t.Errorf("a second agent once the first is removed: %v", err)
	}
	if err := insertAgent(t, pool, newID(), cabal, "removed"); err != nil {
		t.Errorf("another removed agent: %v", err)
	}
	wantViolation(t, "a third agent beside the live second", insertAgent(t, pool, newID(), cabal, "active"),
		uniqueViolation, "agents_live_cabal_idx")
}

func TestAgentsSchema_refusesAValueOutsideItsSet(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	agent, other := newID(), newID()
	for _, id := range []uuid.UUID{agent, other} {
		if err := insertAgent(t, pool, id, newID(), "active"); err != nil {
			t.Fatal(err)
		}
	}
	var proposal uuid.UUID
	row := pool.QueryRow(t.Context(), `SELECT added_by_proposal_id FROM agents WHERE id = $1`, other)
	if err := row.Scan(&proposal); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		column     string
		value      any
		code       string
		constraint string
	}{
		{"name", "", checkViolation, "agents_name_check"},
		{"name", strings.Repeat("x", 41), checkViolation, "agents_name_check"},
		{"budget_usdc_micros", 0, checkViolation, "agents_budget_usdc_micros_check"},
		{"budget_usdc_micros", -1, checkViolation, "agents_budget_usdc_micros_check"},
		{"status", "banned", checkViolation, "agents_status_check"},
		{"added_by_proposal_id", proposal, uniqueViolation, "agents_added_by_proposal_id_key"},
	} {
		err := run(t, pool, `UPDATE agents SET `+tt.column+` = $1 WHERE id = $2`, tt.value, agent)
		wantViolation(t, fmt.Sprintf("%s = %v", tt.column, tt.value), err, tt.code, tt.constraint)
	}
	for _, name := range []string{"x", strings.Repeat("x", 40)} {
		if err := run(t, pool, `UPDATE agents SET name = $1 WHERE id = $2`, name, agent); err != nil {
			t.Errorf("name of %d characters: %v", len(name), err)
		}
	}
}

func TestAgentIntentsSchema_aBuyNamesUSDCAndASellNamesTokens(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	agent := newID()
	if err := insertAgent(t, pool, agent, newID(), "active"); err != nil {
		t.Fatal(err)
	}
	const sideAmount = "agent_intents_side_amount_check"
	for name, tt := range map[string]struct {
		side        string
		usdc, token *int64
		constraint  string
	}{
		"a buy with usdc":       {"buy", ptr(25_000_000), nil, ""},
		"a sell with tokens":    {"sell", nil, ptr(10_000_000), ""},
		"a buy with both":       {"buy", ptr(1), ptr(1), sideAmount},
		"a sell with both":      {"sell", ptr(1), ptr(1), sideAmount},
		"a buy with neither":    {"buy", nil, nil, sideAmount},
		"a sell with neither":   {"sell", nil, nil, sideAmount},
		"a buy naming tokens":   {"buy", nil, ptr(1), sideAmount},
		"a sell naming usdc":    {"sell", ptr(1), nil, sideAmount},
		"a buy of zero usdc":    {"buy", ptr(0), nil, "agent_intents_usdc_micros_check"},
		"a sell of zero tokens": {"sell", nil, ptr(0), "agent_intents_token_amount_check"},
	} {
		err := insertIntent(t, pool, agent, tt.side, tt.usdc, tt.token)
		if tt.constraint == "" && err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if tt.constraint != "" {
			wantViolation(t, name, err, checkViolation, tt.constraint)
		}
	}
	if err := run(t, pool, `UPDATE agent_intents SET reason = $1`, strings.Repeat("r", 280)); err != nil {
		t.Errorf("a reason of 280 characters: %v", err)
	}
	wantViolation(t, "a reason of 281 characters", run(t, pool, `UPDATE agent_intents SET reason = $1`,
		strings.Repeat("r", 281)), checkViolation, "agent_intents_reason_check")
	wantViolation(t, "an unknown status", run(t, pool, `UPDATE agent_intents SET status = 'pending'`),
		checkViolation, "agent_intents_status_check")
}

func TestAgentsSchema_keysAreUniqueByHashAndEveryChildNamesAnAgent(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	first, second, unknown := newID(), newID(), newID()
	for _, id := range []uuid.UUID{first, second} {
		if err := insertAgent(t, pool, id, newID(), "active"); err != nil {
			t.Fatal(err)
		}
	}
	const insertKey = `INSERT INTO agent_keys (agent_id, key_hash, created_at) VALUES ($1, $2, now())`
	hash := make([]byte, 32)
	if err := run(t, pool, insertKey, first, hash); err != nil {
		t.Fatal(err)
	}
	wantViolation(t, "the same key hash for another agent", run(t, pool, insertKey, second, hash),
		uniqueViolation, "agent_keys_key_hash_key")
	wantViolation(t, "a key for an unknown agent", run(t, pool, insertKey, unknown, []byte{1}),
		foreignKeyViolation, "agent_keys_agent_id_fkey")
	wantViolation(t, "a reveal for an unknown agent", run(t, pool, `INSERT INTO agent_key_reveals
		(id, agent_id, user_id, revealed_at) VALUES ($1, $2, $3, now())`, newID(), unknown, newID()),
		foreignKeyViolation, "agent_key_reveals_agent_id_fkey")
	wantViolation(t, "an intent for an unknown agent", insertIntent(t, pool, unknown, "buy", ptr(1), nil),
		foreignKeyViolation, "agent_intents_agent_id_fkey")
}
