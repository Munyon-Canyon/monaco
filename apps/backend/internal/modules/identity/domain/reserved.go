package domain

import "strings"

func brandPrefixed(name string) bool {
	for _, prefix := range []string{"monaco", "support", "admin", "official", "team", "help"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func reservedHandles() map[string]struct{} {
	names := []string{
		"admin", "monaco", "monacolabs", "support", "help", "api", "app", "r", "official", "team",
		"root", "system", "staff", "mod", "moderator", "security", "billing", "cabal", "cabals",
		"null", "undefined", "me", "settings",
	}
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[name] = struct{}{}
	}
	return out
}
