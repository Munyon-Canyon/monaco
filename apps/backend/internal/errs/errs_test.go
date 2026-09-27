package errs

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"
)

func TestNewCarriesCodeOpAndAttrs(t *testing.T) {
	t.Parallel()
	err := New(CodeNotFound, "cabal.Get", slog.String("cabal_id", "c1"))
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("errors.As(%v) = false", err)
	}
	if e.Code != CodeNotFound || e.Op != "cabal.Get" || e.Err != nil {
		t.Errorf("got %+v", e)
	}
	if len(e.Attrs) != 1 || e.Attrs[0].Key != "cabal_id" {
		t.Errorf("Attrs = %v", e.Attrs)
	}
	if got, want := err.Error(), "cabal.Get: not_found"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestWrapKeepsTheCauseReachable(t *testing.T) {
	t.Parallel()
	inner := New(CodeDBUnavailable, "db.Begin")
	err := Wrap(fmt.Errorf("pool: %w", io.ErrUnexpectedEOF), CodeUpstreamTimeout, "market.Poll")
	outer := Wrap(inner, CodeInternal, "treasury.Fund")

	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Error("errors.Is did not find the wrapped cause")
	}
	if got, want := err.Error(), "market.Poll: upstream_timeout: pool: unexpected EOF"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(outer, inner) {
		t.Error("errors.Is did not find the inner *Error")
	}
	var e *Error
	if !errors.As(errors.Unwrap(outer), &e) || e.Code != CodeDBUnavailable {
		t.Errorf("errors.As on the unwrapped chain = %+v, want the inner db_unavailable", e)
	}
	if got := CodeOf(outer); got != CodeInternal {
		t.Errorf("CodeOf(outer) = %q, want the outermost code", got)
	}
}

func TestCodeOfFindsTheCodeThroughForeignWrapping(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf("handler: %w", New(CodeForbidden, "cabal.Join"))
	if got := CodeOf(err); got != CodeForbidden {
		t.Errorf("CodeOf = %q, want forbidden", got)
	}
}

func TestCodeOfAnythingWithoutACodeIsInternal(t *testing.T) {
	t.Parallel()
	for _, err := range []error{nil, io.EOF, fmt.Errorf("plain %d", 1)} {
		if got := CodeOf(err); got != CodeInternal {
			t.Errorf("CodeOf(%v) = %q, want internal", err, got)
		}
	}
}

func TestHTTPStatusCoversEveryKind(t *testing.T) {
	t.Parallel()
	want := map[Kind]int{
		KindInvalid:      http.StatusBadRequest,
		KindUnauthorized: http.StatusUnauthorized,
		KindForbidden:    http.StatusForbidden,
		KindNotFound:     http.StatusNotFound,
		KindConflict:     http.StatusConflict,
		KindBlocked:      http.StatusUnprocessableEntity,
		KindUnavailable:  http.StatusServiceUnavailable,
		KindInternal:     http.StatusInternalServerError,
		Kind(0):          http.StatusInternalServerError,
	}
	for kind, status := range want {
		if got := HTTPStatus(kind); got != status {
			t.Errorf("HTTPStatus(%d) = %d, want %d", kind, got, status)
		}
	}
}

func TestVerdictIsNakExactlyForRetryableCodes(t *testing.T) {
	t.Parallel()
	for _, code := range All() {
		want := VerdictTerm
		if Retryable(code) {
			want = VerdictNak
		}
		if got := VerdictFor(code); got != want {
			t.Errorf("VerdictFor(%q) = %d, want %d", code, got, want)
		}
	}
}

func TestDetailCollectsAttrsFromEveryCodedErrorInTheChain(t *testing.T) {
	t.Parallel()
	inner := New(CodeDBSchemaBehind, "db.Open", slog.String("have", "0001"), slog.String("want", "0002"))
	err := fmt.Errorf(
		"boot: %w",
		Wrap(fmt.Errorf("open: %w", inner), CodeInternal, "api.run", slog.String("step", "db")),
	)

	got := Detail(err)
	want := []string{"step=db", "have=0001", "want=0002"}
	if len(got) != len(want) {
		t.Fatalf("Detail = %v, want %v", got, want)
	}
	for i, a := range got {
		if a.String() != want[i] {
			t.Errorf("Detail[%d] = %s, want %s", i, a, want[i])
		}
	}
	if d := Detail(errors.New("plain")); len(d) != 0 {
		t.Errorf("Detail of an uncoded error = %v, want none", d)
	}
}
