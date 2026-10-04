package db

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	pgSerializationFailure = "40001"
	pgDeadlockDetected     = "40P01"
	pgConnectionClass      = "08"
	pgOperatorIntervention = "57P"
	pgUndefinedTable       = "42P01"
)

func transient(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && (pg.Code == pgSerializationFailure || pg.Code == pgDeadlockDetected)
}

func classify(err error, op string) error {
	var coded *errs.Error
	if errors.As(err, &coded) {
		return err
	}
	return errs.Wrap(err, codeFor(err), op)
}

func codeFor(err error) errs.Code {
	var pg *pgconn.PgError
	var netErr net.Error
	switch {
	case transient(err):
		return errs.CodeDBUnavailable
	case errors.As(err, &pg):
		if strings.HasPrefix(pg.Code, pgConnectionClass) || strings.HasPrefix(pg.Code, pgOperatorIntervention) {
			return errs.CodeDBUnavailable
		}
		return errs.CodeInternal
	case errors.As(err, &netErr), pgconn.SafeToRetry(err):
		return errs.CodeDBUnavailable
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return errs.CodeDBUnavailable
	}
	return errs.CodeInternal
}

func CodeFor(err error) errs.Code { return codeFor(err) }
