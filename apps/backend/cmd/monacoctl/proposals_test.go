package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func seededProposals(t *testing.T) (tool, uuid.UUID, *pgxpool.Pool) {
	t.Helper()
	pool := testkit.DB(t)
	now := clock.Real{}.Now().UTC()
	id, voter := ids.Real{}.NewV7(), ids.Real{}.NewV7()
	if _, err := sqlc.New(pool).InsertProposal(t.Context(), sqlc.InsertProposalParams{
		ID: id, CabalID: ids.Real{}.NewV7(), ProposerID: voter, Kind: "buy", Symbol: "AAPLx",
		Mint: "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", UsdcMicros: pgtype.Int8{Int64: 5_000_000, Valid: true},
		QuoteOutAmount: 21_000_000, ExpiresAt: now.Add(time.Hour), CreatedAt: now, VoterIds: []uuid.UUID{voter},
	}); err != nil {
		t.Fatal(err)
	}
	environ := []string{"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=nats://unused"}
	return proposalsTool(environ, testkit.NewClock(now)), id, pool
}

func TestProposalsVoid_rejectsMalformedArgumentsWithTheUsage(t *testing.T) {
	t.Parallel()
	proposals, id, _ := seededProposals(t)
	for _, args := range [][]string{
		{"void", "--reason", "spam"},
		{"void", "--proposal", "not-a-uuid", "--reason", "spam"},
		{"void", "--proposal", id.String(), "--reason", "spam", "extra"},
	} {
		var stdout, stderr bytes.Buffer
		if code := proposals(args, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), proposalsUsage) {
			t.Errorf("proposals %v = %d, stderr %q, want 2 and the usage", args, code, stderr.String())
		}
	}
}

func TestProposalsVoid_voidsAsSystemOnceThenReportsTheProposalClosed(t *testing.T) {
	t.Parallel()
	proposals, id, pool := seededProposals(t)
	var stdout, stderr bytes.Buffer
	blank := []string{"void", "--proposal", id.String(), "--reason", " "}
	if code := proposals(blank, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "invalid_input") {
		t.Fatalf("void with a blank reason = %d, stderr %q, want 1 and invalid_input", code, stderr.String())
	}
	stderr.Reset()
	void := []string{"void", "--proposal", id.String(), "--reason", "spam"}
	if code := proposals(void, &stdout, &stderr); code != 0 || stdout.String() != "voided "+id.String()+"\n" {
		t.Fatalf("first void = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	var actor string
	if err := pool.QueryRow(t.Context(), `SELECT actor_type || ':' || actor_id FROM events
		WHERE type = 'proposal.voided' AND aggregate_id = $1`, id).Scan(&actor); err != nil || actor != "system:monacoctl" {
		t.Fatalf("proposal.voided actor = %q, %v, want system:monacoctl", actor, err)
	}
	stdout.Reset()
	if code := proposals(void, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "proposal_closed") {
		t.Fatalf("second void = %d, stderr %q, want 1 and proposal_closed", code, stderr.String())
	}
}

func TestProposalsVoid_reportsAnUnreachableDatabase(t *testing.T) {
	t.Parallel()
	environ := []string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://127.0.0.1:1/monaco?connect_timeout=1", "NATS_URL=nats://unused",
	}
	args := []string{"void", "--proposal", ids.Real{}.NewV7().String(), "--reason", "spam"}
	var stdout, stderr bytes.Buffer
	if code := proposalsTool(environ, clock.Real{})(args, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "monacoctl: db.Open: ") {
		t.Fatalf("void with no database = %d, stderr %q, want 1 and db.Open", code, stderr.String())
	}
}
