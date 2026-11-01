package chain

import "strings"

const standInPrefix = "unsent:"

func StandInSignature(treasury Signature) Signature {
	return Signature(standInPrefix + string(treasury))
}

func IsStandInSignature(sig Signature) bool { return strings.HasPrefix(string(sig), standInPrefix) }

func StandInTreasurySignature(sig Signature) (Signature, bool) {
	rest, ok := strings.CutPrefix(string(sig), standInPrefix)
	return Signature(rest), ok && rest != ""
}
