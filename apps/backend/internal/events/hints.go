package events

import "github.com/monaco/monaco/apps/backend/internal/platform/ids"

func UserBalanceChangedHint(user ids.UserID) string {
	return "user." + user.String() + ".balance_changed"
}
