package db

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func TestClassify_mapsEachFailureToOneCode(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		err  error
		want errs.Code
	}{
		"serialization failure": {&pgconn.PgError{Code: "40001"}, errs.CodeDBUnavailable},
		"deadlock":              {&pgconn.PgError{Code: "40P01"}, errs.CodeDBUnavailable},
		"connection exception":  {&pgconn.PgError{Code: "08006"}, errs.CodeDBUnavailable},
		"admin shutdown":        {&pgconn.PgError{Code: "57P01"}, errs.CodeDBUnavailable},
		"constraint violation":  {&pgconn.PgError{Code: "23514"}, errs.CodeInternal},
		"network":               {&net.OpError{Op: "dial", Err: io.EOF}, errs.CodeDBUnavailable},
		"safe to retry":         {beforeSendError{}, errs.CodeDBUnavailable},
		"plain":                 {io.ErrUnexpectedEOF, errs.CodeInternal},
		"caller gone":           {context.Canceled, errs.CodeDBUnavailable},
		"deadline":              {context.DeadlineExceeded, errs.CodeDBUnavailable},
		"already coded":         {errs.New(errs.CodeNotFound, "x"), errs.CodeNotFound},

		"internal wrapping a connection exception": {internal(&pgconn.PgError{Code: "08006"}), errs.CodeDBUnavailable},
		"internal wrapping a serialization failure": {
			internal(&pgconn.PgError{Code: "40001"}), errs.CodeDBUnavailable,
		},
		"internal wrapping a network error": {
			internal(&net.OpError{Op: "read", Err: io.EOF}), errs.CodeDBUnavailable,
		},
		"internal wrapping safe to retry":    {internal(beforeSendError{}), errs.CodeDBUnavailable},
		"internal wrapping a deadline":       {internal(context.DeadlineExceeded), errs.CodeDBUnavailable},
		"internal wrapping a cancelled tick": {internal(context.Canceled), errs.CodeDBUnavailable},
		"internal wrapping a unique violation": {
			internal(&pgconn.PgError{Code: "23505"}), errs.CodeInternal,
		},
		"internal wrapping a missing table": {internal(&pgconn.PgError{Code: "42P01"}), errs.CodeInternal},
		"internal wrapping a plain error":   {internal(io.ErrUnexpectedEOF), errs.CodeInternal},
		"not found wrapping a deadline": {
			errs.Wrap(context.DeadlineExceeded, errs.CodeNotFound, "x"), errs.CodeNotFound,
		},
		"unavailable wrapping a unique violation": {
			errs.Wrap(&pgconn.PgError{Code: "23505"}, errs.CodeDBUnavailable, "x"), errs.CodeDBUnavailable,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := classify(tc.err, "db.test")
			if errs.CodeOf(got) != tc.want || !errors.Is(got, tc.err) {
				t.Fatalf("classify(%v) = %v, want code %s wrapping the cause", tc.err, got, tc.want)
			}
		})
	}
}

func internal(cause error) error { return errs.Wrap(cause, errs.CodeInternal, "app.op") }

type beforeSendError struct{}

func (beforeSendError) Error() string { return "closed before send" }

func (beforeSendError) SafeToRetry() bool { return true }
