package domain

import (
	_ "embed"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const ReferralAlphabet = "23456789abcdefghjkmnpqrstuvwxyz"

//go:embed profanity.txt
var profanityFile string

type HandleFacts struct {
	OwnXUsername            string
	OtherUserXUsernameMatch bool
	CurrentHandle           string
	HandleChangedAt         time.Time
	Now                     time.Time
}

type HandlePolicy struct{}

func (HandlePolicy) Check(h Handle, facts HandleFacts) error {
	const op = "identity.HandlePolicy.Check"
	if h.String() != strings.ToLower(facts.OwnXUsername) {
		switch {
		case reservedHandle(h.String()):
			return errs.New(errs.CodeHandleReserved, op)
		case profane(h.String()):
			return errs.New(errs.CodeHandleReserved, op)
		case referralCodeShape(h.String()):
			return errs.New(errs.CodeHandleReserved, op)
		case facts.OtherUserXUsernameMatch:
			return errs.New(errs.CodeHandleTaken, op)
		}
	}
	if facts.CurrentHandle != "" && h.String() != facts.CurrentHandle &&
		facts.Now.Sub(facts.HandleChangedAt) < HandleChangeInterval {
		return errs.New(errs.CodeHandleTooSoon, op)
	}
	return nil
}

func reservedHandle(name string) bool {
	_, ok := reservedHandles()[name]
	return ok
}

func profane(name string) bool {
	folded := foldLeet(name)
	for _, term := range loadProfanity(profanityFile) {
		if strings.Contains(folded, term) {
			return true
		}
	}
	return false
}

func foldLeet(name string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '0':
			return 'o'
		case '1':
			return 'i'
		case '3':
			return 'e'
		case '4':
			return 'a'
		case '5':
			return 's'
		case '7':
			return 't'
		default:
			return r
		}
	}, name)
}

const referralCodeLen = 8

func referralCodeShape(name string) bool {
	if len(name) != referralCodeLen {
		return false
	}
	for _, r := range name {
		if !strings.ContainsRune(ReferralAlphabet, r) {
			return false
		}
	}
	return true
}

func loadProfanity(raw string) []string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		term := strings.TrimSpace(strings.ToLower(line))
		if term == "" {
			continue
		}
		out = append(out, term)
	}
	return out
}
