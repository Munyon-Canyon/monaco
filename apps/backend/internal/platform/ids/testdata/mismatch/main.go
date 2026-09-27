package main

import "github.com/monaco/monaco/apps/backend/internal/platform/ids"

func takesUser(id ids.UserID) string { return id.String() }

func main() {
	var cabalID ids.CabalID
	_ = takesUser(cabalID)
}
