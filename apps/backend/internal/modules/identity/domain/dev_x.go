package domain

import "strings"

const (
	devEmailPrefix = "dev-"
	devEmailDomain = "@example.com"
)

func DevEmail(suffix string) string { return devEmailPrefix + suffix + devEmailDomain }

func DevSuffix(email string) (string, bool) {
	rest, ok := strings.CutPrefix(email, devEmailPrefix)
	if !ok {
		return "", false
	}
	suffix, ok := strings.CutSuffix(rest, devEmailDomain)
	if !ok || suffix == "" || strings.Contains(suffix, "@") {
		return "", false
	}
	return suffix, true
}

func DevXAccount(suffix, username string) XAccount {
	if username == "" {
		username = "dev_x_" + suffix
	}
	return XAccount{UserID: "dev:" + suffix, Username: username}
}
