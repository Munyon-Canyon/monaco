package errs

import (
	"maps"
	"slices"
)

type Code string

const (
	CodeInvalidInput        Code = "invalid_input"
	CodeClientClosed        Code = "client_closed"
	CodeUnauthorized        Code = "unauthorized"
	CodeForbidden           Code = "forbidden"
	CodeNotFound            Code = "not_found"
	CodeIdempotencyMismatch Code = "idempotency_mismatch"
	CodeIdempotencyInFlight Code = "idempotency_in_flight"
	CodeVersionConflict     Code = "version_conflict"
	CodeUpstreamUnavailable Code = "upstream_unavailable"
	CodeUpstreamTimeout     Code = "upstream_timeout"
	CodeDBUnavailable       Code = "db_unavailable"
	CodeDecodeFailed        Code = "decode_failed"
	CodeInternal            Code = "internal"
	CodePanic               Code = "panic"
)

type Row struct {
	Name      string
	Kind      Kind
	Retryable bool
	Alert     bool
	Message   string
}

func table() map[Code]Row {
	return map[Code]Row{
		CodeInvalidInput: {
			Name: "InvalidInput", Kind: KindInvalid,
			Message: "The request is not valid.",
		},
		CodeClientClosed: {
			Name: "ClientClosed", Kind: KindInvalid,
			Message: "The connection closed before the response was sent.",
		},
		CodeUnauthorized: {
			Name: "Unauthorized", Kind: KindUnauthorized,
			Message: "Sign in to continue.",
		},
		CodeForbidden: {
			Name: "Forbidden", Kind: KindForbidden,
			Message: "You do not have access to this.",
		},
		CodeNotFound: {
			Name: "NotFound", Kind: KindNotFound,
			Message: "Not found.",
		},
		CodeIdempotencyMismatch: {
			Name: "IdempotencyMismatch", Kind: KindConflict,
			Message: "This idempotency key was already used for a different request.",
		},
		CodeIdempotencyInFlight: {
			Name: "IdempotencyInFlight", Kind: KindConflict,
			Message: "A request with this idempotency key is still in progress.",
		},
		CodeVersionConflict: {
			Name: "VersionConflict", Kind: KindConflict,
			Message: "This changed since you last loaded it. Refresh and try again.",
		},
		CodeUpstreamUnavailable: {
			Name: "UpstreamUnavailable", Kind: KindUnavailable, Retryable: true,
			Message: "A provider is unavailable. Try again shortly.",
		},
		CodeUpstreamTimeout: {
			Name: "UpstreamTimeout", Kind: KindUnavailable, Retryable: true,
			Message: "A provider timed out. Try again shortly.",
		},
		CodeDBUnavailable: {
			Name: "DBUnavailable", Kind: KindUnavailable, Retryable: true,
			Message: "The service is temporarily unavailable. Try again shortly.",
		},
		CodeDecodeFailed: {
			Name: "DecodeFailed", Kind: KindInternal, Alert: true,
			Message: "Something went wrong.",
		},
		CodeInternal: {
			Name: "Internal", Kind: KindInternal, Alert: true,
			Message: "Something went wrong.",
		},
		CodePanic: {
			Name: "Panic", Kind: KindInternal, Alert: true,
			Message: "Something went wrong.",
		},
	}
}

func row(code Code) Row {
	rows := table()
	if r, ok := rows[code]; ok {
		return r
	}
	return rows[CodeInternal]
}

func Name(code Code) string { return row(code).Name }

func KindOf(code Code) Kind { return row(code).Kind }

func Retryable(code Code) bool { return row(code).Retryable }

func Alert(code Code) bool { return row(code).Alert }

func Message(code Code) string { return row(code).Message }

func All() []Code { return slices.Sorted(maps.Keys(table())) }
