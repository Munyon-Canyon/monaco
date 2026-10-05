package events

import "github.com/monaco/monaco/apps/backend/internal/platform/ids"

func UserBalanceChangedHint(user ids.UserID) string {
	return "user." + user.String() + ".balance_changed"
}

func CabalActivityChangedHint(cabalID ids.CabalID) string {
	return "cabal." + cabalID.String() + ".activity_changed"
}

func UserCashOutChangedHint(user ids.UserID) string {
	return "user." + user.String() + ".cashout_changed"
}
