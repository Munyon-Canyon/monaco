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
	pgInsufficientResource = "53"
	pgSystemError          = "58"
	pgLockNotAvailable     = "55P03"
	pgReadOnlyTransaction  = "25006"
	pgIdleInTransaction    = "25P03"
	pgUndefinedTable       = "42P01"
	pgUniqueViolation      = "23505"
	pgIntegrityClass       = "23"
)

func IsUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == pgUniqueViolation
}

func IsIntegrityViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && strings.HasPrefix(pg.Code, pgIntegrityClass)
}

func transient(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && (pg.Code == pgSerializationFailure || pg.Code == pgDeadlockDetected)
}

func classify(err error, op string) error {
	var coded *errs.Error
	if errors.As(err, &coded) {
		if coded.Code == errs.CodeInternal && codeFor(err) == errs.CodeDBUnavailable {
			return errs.Wrap(err, errs.CodeDBUnavailable, op)
		}
		return err
	}
	return errs.Wrap(err, codeFor(err), op)
}

func unavailable(sqlState string) bool {
	switch sqlState {
	case pgQueryCanceled, pgLockNotAvailable, pgReadOnlyTransaction, pgIdleInTransaction:
		return true
	}
	for _, class := range []string{pgConnectionClass, pgOperatorIntervention, pgInsufficientResource, pgSystemError} {
		if strings.HasPrefix(sqlState, class) {
			return true
		}
	}
	return false
}

func codeFor(err error) errs.Code {
	var pg *pgconn.PgError
	var netErr net.Error
	switch {
	case transient(err):
		return errs.CodeDBUnavailable
	case errors.As(err, &pg):
		if unavailable(pg.Code) {
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

func CodeOf(err error) errs.Code {
	code := errs.CodeOf(err)
	if code == errs.CodeInternal && codeFor(err) == errs.CodeDBUnavailable {
		return errs.CodeDBUnavailable
	}
	return code
}
