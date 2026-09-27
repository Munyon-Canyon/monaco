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

type beforeSendError struct{}

func (beforeSendError) Error() string { return "closed before send" }

func (beforeSendError) SafeToRetry() bool { return true }
