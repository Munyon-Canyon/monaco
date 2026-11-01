package governance_test

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type cabalRoster struct {
	pool      *pgxpool.Pool
	mu        sync.Mutex
	left      map[[2]uuid.UUID]bool
	err       error
	cabalsErr error
}

func (d proposalDB) roster() *cabalRoster {
	return &cabalRoster{pool: d.pool, left: map[[2]uuid.UUID]bool{}}
}

func (r *cabalRoster) leave(cabal, user uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.left[[2]uuid.UUID{cabal, user}] = true
}

func (r *cabalRoster) IsMember(_ context.Context, id ids.CabalID, user ids.UserID) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err == nil && !r.left[[2]uuid.UUID{id.UUID(), user.UUID()}], r.err
}

func (r *cabalRoster) CabalsOf(ctx context.Context, user ids.UserID) ([]ids.CabalID, error) {
	if r.cabalsErr != nil {
		return nil, r.cabalsErr
	}
	rows, err := r.pool.Query(context.WithoutCancel(ctx), `SELECT DISTINCT p.cabal_id FROM proposals AS p
		JOIN proposal_voters AS pv ON pv.proposal_id = p.id WHERE pv.voter_id = $1`, user.UUID())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []ids.CabalID
	for rows.Next() {
		var cabal uuid.UUID
		if err := rows.Scan(&cabal); err != nil {
			return nil, err
		}
		if !r.left[[2]uuid.UUID{cabal, user.UUID()}] {
			out = append(out, ids.CabalIDFrom(cabal))
		}
	}
	return out, rows.Err()
}
