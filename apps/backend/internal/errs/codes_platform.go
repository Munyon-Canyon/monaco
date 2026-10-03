package errs

const (
	CodeInvalidInput        Code = "invalid_input"
	CodeClientClosed        Code = "client_closed"
	CodeUnauthorized        Code = "unauthorized"
	CodeForbidden           Code = "forbidden"
	CodeNotFound            Code = "not_found"
	CodeIdempotencyMismatch Code = "idempotency_mismatch"
	CodeIdempotencyInFlight Code = "idempotency_in_flight"
	CodeVersionConflict     Code = "version_conflict"
	CodeRateLimited         Code = "rate_limited"
	CodeUpstreamUnavailable Code = "upstream_unavailable"
	CodeUpstreamTimeout     Code = "upstream_timeout"
	CodeJupiterUnavailable  Code = "jupiter_unavailable"
	CodeJupiterRejected     Code = "jupiter_rejected"
	CodePrivyUnavailable    Code = "privy_unavailable"
	CodeRPCUnavailable      Code = "rpc_unavailable"
	CodeRelayerUnderfunded  Code = "relayer_underfunded"
	CodeInvalidAddress      Code = "invalid_address"
	CodeDBUnavailable       Code = "db_unavailable"
	CodeDBSchemaBehind      Code = "db_schema_behind"
	CodeStorageUnavailable  Code = "storage_unavailable"
	CodeDecodeFailed        Code = "decode_failed"
	CodeInvalidConfig       Code = "invalid_config"
	CodeInternal            Code = "internal"
	CodePanic               Code = "panic"
	CodeFaultpoint          Code = "faultpoint"
)

func platformRows() map[Code]Row {
	storageUnavailable := CodeStorageUnavailable
	return map[Code]Row{
		CodeInvalidInput: {Name: "InvalidInput", Kind: KindInvalid, Message: "The request is not valid."},
		CodeClientClosed: {
			Name: "ClientClosed", Kind: KindInvalid,
			Message: "The connection closed before the response was sent.",
		},
		CodeUnauthorized: {Name: "Unauthorized", Kind: KindUnauthorized, Message: "Sign in to continue."},
		CodeForbidden:    {Name: "Forbidden", Kind: KindForbidden, Message: "You do not have access to this."},
		CodeNotFound:     {Name: "NotFound", Kind: KindNotFound, Message: "Not found."},
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
		CodeRateLimited: {
			Name: "RateLimited", Kind: KindRateLimited, Retryable: true,
			Message: "Too many requests. Try again in a moment.",
		},
		CodeUpstreamUnavailable: {
			Name: "UpstreamUnavailable", Kind: KindUnavailable, Retryable: true,
			Message: "A provider is unavailable. Try again shortly.",
		},
		CodeUpstreamTimeout: {
			Name: "UpstreamTimeout", Kind: KindUnavailable, Retryable: true,
			Message: "A provider timed out. Try again shortly.",
		},
		CodeJupiterUnavailable: {
			Name: "JupiterUnavailable", Kind: KindUnavailable, Retryable: true,
			Message: "The swap venue is unavailable. Try again shortly.",
		},
		CodeJupiterRejected: {
			Name: "JupiterRejected", Kind: KindBlocked,
			Message: "The swap venue refused this trade.",
		},
		CodePrivyUnavailable: {
			Name: "PrivyUnavailable", Kind: KindUnavailable, Retryable: true,
			Message: "The wallet provider is unavailable. Try again shortly.",
		},
		CodeRPCUnavailable: {
			Name: "RPCUnavailable", Kind: KindUnavailable, Retryable: true,
			Message: "The Solana network is unavailable. Try again shortly.",
		},
		CodeRelayerUnderfunded: {
			Name: "RelayerUnderfunded", Kind: KindUnavailable, Alert: true,
			Message: "The service is temporarily unavailable. Try again shortly.",
		},
		CodeInvalidAddress: {Name: "InvalidAddress", Kind: KindInvalid, Message: "That is not a valid Solana address."},
		CodeDBUnavailable: {
			Name: "DBUnavailable", Kind: KindUnavailable, Retryable: true,
			Message: "The service is temporarily unavailable. Try again shortly.",
		},
		CodeDBSchemaBehind: {
			Name: "DBSchemaBehind", Kind: KindUnavailable, Alert: true,
			Message: "The service is temporarily unavailable. Try again shortly.",
		},
		storageUnavailable: storageUnavailableRow(),
		CodeInvalidConfig:  {Name: "InvalidConfig", Kind: KindInternal, Alert: true, Message: "Something went wrong."},
		CodeDecodeFailed:   {Name: "DecodeFailed", Kind: KindInternal, Alert: true, Message: "Something went wrong."},
		CodeInternal:       {Name: "Internal", Kind: KindInternal, Alert: true, Message: "Something went wrong."},
		CodePanic:          {Name: "Panic", Kind: KindInternal, Alert: true, Message: "Something went wrong."},
		CodeFaultpoint:     faultpointRow(),
	}
}

func faultpointRow() Row {
	return Row{
		Name: "Faultpoint", Kind: KindUnavailable, Retryable: true,
		Message: "The service is temporarily unavailable. Try again shortly.",
	}
}

func storageUnavailableRow() Row {
	return Row{
		Name: "StorageUnavailable", Kind: KindUnavailable, Retryable: true,
		Message: "Photo storage is temporarily unavailable. Try again shortly.",
	}
}
