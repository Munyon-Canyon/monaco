package errs

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
