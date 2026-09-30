package app

import "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"

const (
	MaxUsersByID   = port.MaxUsersByID
	MaxHandles     = port.MaxHandles
	MaxPhoneHashes = port.MaxPhoneHashes
	MaxXUserIDs    = port.MaxXUserIDs
	MaxWalletPage  = port.MaxWalletPage
)

type (
	UserCard     = port.UserCard
	MemberWallet = port.MemberWallet
)
