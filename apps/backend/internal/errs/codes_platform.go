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

func (codeFiles) Platform() map[Code]Row {
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
			Message: "That request was already sent with different details. Start again.",
		},
		CodeIdempotencyInFlight: {
			Name: "IdempotencyInFlight", Kind: KindConflict,
			Message: "Still sending your last request.",
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
			Message: "Monaco is busy right now. Try again in a minute.",
		},
		CodeUpstreamTimeout: {
			Name: "UpstreamTimeout", Kind: KindUnavailable, Retryable: true,
			Message: "That took too long. Try again in a minute.",
		},
		CodeJupiterUnavailable: {
			Name: "JupiterUnavailable", Kind: KindUnavailable, Retryable: true,
			Message: "Trading is busy. Try again in a minute.",
		},
		CodeJupiterRejected: {
			Name: "JupiterRejected", Kind: KindBlocked,
			Message: "That trade can't go through right now. Try again in a minute.",
		},
		CodePrivyUnavailable: {
			Name: "PrivyUnavailable", Kind: KindUnavailable, Retryable: true,
			Message: "Your wallet is busy. Try again in a minute.",
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
