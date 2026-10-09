package domain

import (
	"regexp"
	"strings"
)

const (
	devEmailPrefix = "dev-"
	devEmailDomain = "@example.com"
)

const (
	DevHandlePrefix = "dev_"
	maxDevPoolName  = 16
)

var (
	devPoolName  = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	devThrowaway = regexp.MustCompile(`^[0-9a-f]{8}$`)
)

func ValidDevPool(name string) bool {
	return len(name) <= maxDevPoolName && devPoolName.MatchString(name) && !devThrowaway.MatchString(name)
}

func DevHandle(suffix string) string { return DevHandlePrefix + strings.ReplaceAll(suffix, "-", "_") }

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
