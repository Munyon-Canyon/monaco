package pgxpool

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type Pool struct{}

func (p *Pool) Begin(_ context.Context) (pgx.Tx, error) {
	return nil, nil
}
