package funding_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
)

const depositSignature = "5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW"

func depositBlockTime() time.Time { return time.Unix(1_790_000_000, 0).UTC() }

func signaturesFromHistory(
	history []solana.SignatureInfo, before, until chain.Signature, limit int,
) []solana.SignatureInfo {
	start, end := 0, len(history)
	for i, sig := range history {
		if sig.Signature == before {
			start = i + 1
		}
	}
	for i, sig := range history {
		if sig.Signature == until && i >= start {
			end = i
		}
	}
	page := history[start:end]
	if len(page) > limit {
		return page[:limit]
	}
	return page
}

func assertCandidateSignatures(
	ctx context.Context, t *testing.T, pool *pgxpool.Pool, address chain.SolanaAddress, want ...string,
) {
	t.Helper()
	rows, err := pool.Query(
		ctx,
		`SELECT tx_signature FROM deposit_candidates WHERE wallet_address = $1 AND status = 'pending'
		ORDER BY tx_signature`, address,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var signature string
		if err := rows.Scan(&signature); err != nil {
			t.Fatal(err)
		}
		got = append(got, signature)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("pending candidates = %v, want %v", got, want)
	}
}

func unlimited() *rate.Limiter { return rate.NewLimiter(rate.Inf, 0) }

type rpcBudget struct {
	mu   sync.Mutex
	left int
}

func (b *rpcBudget) refill(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.left = n
}

func (b *rpcBudget) Wait(context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.left == 0 {
		return errs.New(errs.CodeInternal, "test.rpcBudget")
	}
	b.left--
	return nil
}
