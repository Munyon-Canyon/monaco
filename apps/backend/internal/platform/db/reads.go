package db

import "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"

func (u *UnitOfWork) Reads() sqlc.DBTX { return u.pool }
