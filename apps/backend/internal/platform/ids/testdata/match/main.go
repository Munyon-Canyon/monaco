package main

import "github.com/monaco/monaco/apps/backend/internal/platform/ids"

func takesUser(id ids.UserID) string { return id.String() }

func main() {
	var userID ids.UserID
	_ = takesUser(userID)
}
