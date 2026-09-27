package domain

import "github.com/jackc/pgx/v5"

type Store struct {
	Tx pgx.Tx
}
