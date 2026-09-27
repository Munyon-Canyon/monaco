package pgx

import "context"

type Tx interface {
	Commit(ctx context.Context) error
}
