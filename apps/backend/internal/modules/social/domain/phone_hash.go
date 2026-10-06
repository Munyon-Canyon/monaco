package domain

import "github.com/monaco/monaco/apps/backend/internal/errs"

const MaxContactHashes = 2000

func errContactHashes(op string) error {
	return errs.New(errs.CodeContactHashesInvalid, op)
}

func errTooManyContactHashes(op string) error {
	return errs.New(errs.CodeTooManyContactHashes, op)
}

func ParsePhoneHashes(raw []string) ([][]byte, error) {
	const op = "social.ParsePhoneHashes"
	switch {
	case len(raw) == 0:
		return nil, errContactHashes(op)
	case len(raw) > MaxContactHashes:
		return nil, errTooManyContactHashes(op)
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([][]byte, 0, len(raw))
	for _, h := range raw {
		if !phoneHashHex(h) {
			return nil, errContactHashes(op)
		}
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, decodePhoneHash(h))
	}
	return out, nil
}

func phoneHashHex(h string) bool {
	if len(h) != 64 {
		return false
	}
	for i := range len(h) {
		if hexValue(h[i]) > 15 {
			return false
		}
	}
	return true
}

func decodePhoneHash(h string) []byte {
	out := make([]byte, len(h)/2)
	for i := 0; i < len(h); i += 2 {
		out[i/2] = hexValue(h[i])<<4 | hexValue(h[i+1])
	}
	return out
}

func hexValue(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return 255
	}
}
