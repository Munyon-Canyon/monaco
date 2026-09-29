package errs

import (
	"errors"
	"log/slog"
	"net/http"
)

type Kind uint8

const (
	KindInvalid Kind = iota + 1
	KindUnauthorized
	KindForbidden
	KindNotFound
	KindConflict
	KindBlocked
	KindRateLimited
	KindUnavailable
	KindInternal
)

type Error struct {
	Code  Code
	Op    string
	Attrs []slog.Attr
	Err   error
}

func New(code Code, op string, attrs ...slog.Attr) error {
	return &Error{Code: code, Op: op, Attrs: attrs}
}

func Wrap(err error, code Code, op string, attrs ...slog.Attr) error {
	return &Error{Code: code, Op: op, Attrs: attrs, Err: err}
}

func (e *Error) Error() string {
	msg := e.Op + ": " + string(e.Code)
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

func CodeOf(err error) Code {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return CodeInternal
}

func HTTPStatus(kind Kind) int {
	switch kind {
	case KindInvalid:
		return http.StatusBadRequest
	case KindUnauthorized:
		return http.StatusUnauthorized
	case KindForbidden:
		return http.StatusForbidden
	case KindNotFound:
		return http.StatusNotFound
	case KindConflict:
		return http.StatusConflict
	case KindBlocked:
		return http.StatusUnprocessableEntity
	case KindRateLimited:
		return http.StatusTooManyRequests
	case KindUnavailable:
		return http.StatusServiceUnavailable
	case KindInternal:
		return http.StatusInternalServerError
	}
	return http.StatusInternalServerError
}

type Verdict uint8

const (
	VerdictAck Verdict = iota + 1
	VerdictNak
	VerdictTerm
)

func VerdictFor(code Code) Verdict {
	if Retryable(code) {
		return VerdictNak
	}
	return VerdictTerm
}

func Detail(err error) []slog.Attr {
	var detail []slog.Attr
	var e *Error
	for cur := err; errors.As(cur, &e); cur = e.Err {
		detail = append(detail, e.Attrs...)
	}
	return detail
}
