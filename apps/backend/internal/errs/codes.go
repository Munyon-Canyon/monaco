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
	CodeDecodeFailed        Code = "decode_failed"
	CodeInvalidConfig       Code = "invalid_config"
	CodeInternal            Code = "internal"
	CodePanic               Code = "panic"
)

const (
	CodeUserNotFound            Code = "user_not_found"
	CodeSessionRequired         Code = "session_required"
	CodeAccountSuspended        Code = "account_suspended"
	CodeAccountBanned           Code = "account_banned"
	CodeAccountDeleted          Code = "account_deleted"
	CodeLoginMethodNotAllowed   Code = "login_method_not_allowed"
	CodeHandleRequired          Code = "handle_required"
	CodeHandleTaken             Code = "handle_taken"
	CodeHandleReserved          Code = "handle_reserved"
	CodeHandleTooSoon           Code = "handle_too_soon"
	CodePhoneNotLinked          Code = "phone_not_linked"
	CodeXNotLinked              Code = "x_not_linked"
	CodeAccountHasBalance       Code = "account_has_balance"
	CodeAccountHasPositions     Code = "account_has_positions"
	CodeAccountStatusTransition Code = "account_status_transition"
	CodeHandleInvalid           Code = "handle_invalid"
	CodeDisplayNameInvalid      Code = "display_name_invalid"
	CodePhotoInvalid            Code = "photo_invalid"
	CodeAuthStateTransition     Code = "auth_state_transition"
	CodeWalletMismatch          Code = "wallet_mismatch"
)

const (
	CodePotValueZero     Code = "pot_value_zero"
	CodeLedgerUnbalanced Code = "ledger_unbalanced"
)

const (
	CodeAssetNotFound Code = "asset_not_found"
)

const (
	CodeSwapNotFound     Code = "swap_not_found"
	CodeSwapNotRetryable Code = "swap_not_retryable"
	CodeSlippageExceeded Code = "slippage_exceeded"
	CodeNoRoute          Code = "no_route"
	CodeSwapFailed       Code = "swap_failed"
)

type Row struct {
	Name      string
	Kind      Kind
	Retryable bool
	Alert     bool
	Message   string
}

func rowGroups() [5]func() map[Code]Row {
	return [...]func() map[Code]Row{platformRows, identityRows, treasuryRows, marketRows, tradingRows}
}

func table() map[Code]Row {
	rows := map[Code]Row{}
	for _, group := range rowGroups() {
		maps.Copy(rows, group())
	}
	return rows
}

func platformRows() map[Code]Row {
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
		CodeInvalidConfig: {Name: "InvalidConfig", Kind: KindInternal, Alert: true, Message: "Something went wrong."},
		CodeDecodeFailed:  {Name: "DecodeFailed", Kind: KindInternal, Alert: true, Message: "Something went wrong."},
		CodeInternal:      {Name: "Internal", Kind: KindInternal, Alert: true, Message: "Something went wrong."},
		CodePanic:         {Name: "Panic", Kind: KindInternal, Alert: true, Message: "Something went wrong."},
	}
}

func identityRows() map[Code]Row {
	return map[Code]Row{
		CodeUserNotFound:    {Name: "UserNotFound", Kind: KindNotFound, Message: "We could not find that account."},
		CodeSessionRequired: {Name: "SessionRequired", Kind: KindUnauthorized, Message: "Sign in to continue."},
		CodeAccountSuspended: {
			Name: "AccountSuspended", Kind: KindForbidden,
			Message: "Your account is suspended. You can still withdraw and cash out.",
		},
		CodeAccountBanned: {
			Name: "AccountBanned", Kind: KindForbidden,
			Message: "Your account is banned. You can still withdraw and cash out.",
		},
		CodeAccountDeleted: {Name: "AccountDeleted", Kind: KindForbidden, Message: "This account was deleted."},
		CodeLoginMethodNotAllowed: {
			Name: "LoginMethodNotAllowed", Kind: KindForbidden,
			Message: "Sign in with your phone number or email.",
		},
		CodeHandleRequired: {Name: "HandleRequired", Kind: KindBlocked, Message: "Pick a handle to continue."},
		CodeHandleTaken:    {Name: "HandleTaken", Kind: KindBlocked, Message: "That handle is taken."},
		CodeHandleReserved: {Name: "HandleReserved", Kind: KindBlocked, Message: "That handle is not available."},
		CodeHandleTooSoon: {
			Name: "HandleTooSoon", Kind: KindBlocked,
			Message: "You can change your handle once every 30 days.",
		},
		CodePhoneNotLinked: {Name: "PhoneNotLinked", Kind: KindBlocked, Message: "Add your phone number first."},
		CodeXNotLinked:     {Name: "XNotLinked", Kind: KindBlocked, Message: "Connect your X account first."},
		CodeAccountHasBalance: {
			Name: "AccountHasBalance", Kind: KindBlocked,
			Message: "Withdraw your balance before you delete your account.",
		},
		CodeAccountHasPositions: {
			Name: "AccountHasPositions", Kind: KindBlocked,
			Message: "Cash out of every cabal before you delete your account.",
		},
		CodeAccountStatusTransition: {
			Name: "AccountStatusTransition", Kind: KindBlocked,
			Message: "This account cannot move to that status.",
		},
		CodeHandleInvalid: {
			Name: "HandleInvalid", Kind: KindInvalid,
			Message: "A handle is 3 to 20 letters, numbers or underscores.",
		},
		CodeDisplayNameInvalid: {
			Name: "DisplayNameInvalid", Kind: KindInvalid, Message: "That display name is not valid.",
		},
		CodePhotoInvalid: {Name: "PhotoInvalid", Kind: KindInvalid, Message: "That photo is not valid."},
		CodeAuthStateTransition: {
			Name: "AuthStateTransition", Kind: KindInternal, Retryable: true,
			Message: "Something went wrong. Try again.",
		},
		CodeWalletMismatch: {Name: "WalletMismatch", Kind: KindInternal, Alert: true, Message: "Something went wrong."},
	}
}

func treasuryRows() map[Code]Row {
	return map[Code]Row{
		CodePotValueZero: {Name: "PotValueZero", Kind: KindBlocked, Message: "This cabal's pot has no value."},
		CodeLedgerUnbalanced: {
			Name: "LedgerUnbalanced", Kind: KindInternal, Alert: true, Message: "Something went wrong.",
		},
	}
}

func marketRows() map[Code]Row {
	return map[Code]Row{
		CodeAssetNotFound: {
			Name: "AssetNotFound", Kind: KindNotFound, Message: "That asset is not in the catalog.",
		},
	}
}

func tradingRows() map[Code]Row {
	return map[Code]Row{
		CodeSwapNotFound: {Name: "SwapNotFound", Kind: KindNotFound, Message: "That trade was not found."},
		CodeSwapNotRetryable: {
			Name: "SwapNotRetryable", Kind: KindBlocked,
			Message: "This trade can't be retried. Only the latest failed trade can be.",
		},
		CodeSlippageExceeded: {
			Name: "SlippageExceeded", Kind: KindBlocked,
			Message: "The price moved past the cabal's slippage limit.",
		},
		CodeNoRoute: {
			Name: "NoRoute", Kind: KindBlocked,
			Message: "No route for this trade right now. Try a smaller amount.",
		},
		CodeSwapFailed: {Name: "SwapFailed", Kind: KindBlocked, Message: "The trade did not go through."},
	}
}

func row(code Code) Row {
	for _, group := range rowGroups() {
		if r, ok := group()[code]; ok {
			return r
		}
	}
	return platformRows()[CodeInternal]
}

func Name(code Code) string { return row(code).Name }

func KindOf(code Code) Kind { return row(code).Kind }

func Retryable(code Code) bool { return row(code).Retryable }

func Alert(code Code) bool { return row(code).Alert }

func Message(code Code) string { return row(code).Message }

func All() []Code { return slices.Sorted(maps.Keys(table())) }
