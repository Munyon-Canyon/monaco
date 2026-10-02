package domain

import "crypto/sha256"

type AuthCause string

const (
	CauseOnboarding AuthCause = "onboarding"
	CauseLink       AuthCause = "link"
	CauseUnlink     AuthCause = "unlink"
)

type XAccount struct {
	UserID   string
	Username string
}

type Links struct {
	Phone string
	X     *XAccount
}

type Claims struct {
	Phone bool
	X     bool
}

func (c Claims) Any() bool { return c.Phone || c.X }

func (s AuthState) resyncsNewLinks() bool { return s != AuthCreated }

func (l Links) Claims(stored Links, from AuthState) Claims {
	if !from.resyncsNewLinks() {
		return Claims{}
	}
	return Claims{
		Phone: l.Phone != "" && l.Phone != stored.Phone,
		X:     l.X != nil && (stored.X == nil || stored.X.UserID != l.X.UserID),
	}
}

func (l Links) Without(held Claims) Links {
	if held.Phone {
		l.Phone = ""
	}
	if held.X {
		l.X = nil
	}
	return l
}

func PhoneHash(e164 string) []byte {
	sum := sha256.Sum256([]byte(e164))
	return sum[:]
}

type Write[T comparable] struct {
	Changed bool
	Value   T
}

type AuthStep struct {
	From  AuthState
	To    AuthState
	Cause AuthCause
}

type LinkSync struct {
	Phone Write[string]
	X     Write[*XAccount]
	Steps []AuthStep
}

func (s LinkSync) Empty() bool { return !s.Phone.Changed && !s.X.Changed && len(s.Steps) == 0 }

type linkFold struct {
	state   AuthState
	linking bool
	phone   bool
	x       bool
	sync    LinkSync
	err     error
}

func SyncLinks(from AuthState, stored, privy Links) (LinkSync, error) {
	f := linkFold{state: from, linking: from.resyncsNewLinks(), phone: stored.Phone != "", x: stored.X != nil}
	f.unlinkPhone(stored.Phone, privy.Phone)
	f.unlinkX(stored.X, privy.X)
	f.linkPhone(privy.Phone)
	f.linkX(privy.X)
	return f.sync, f.err
}

func (f *linkFold) unlinkPhone(stored, privy string) {
	if stored != "" && privy != stored {
		f.sync.Phone = Write[string]{Changed: true}
		f.phone = false
		f.move(PhoneUnlinked, CauseUnlink)
	}
}

func (f *linkFold) linkPhone(privy string) {
	if privy != "" && !f.phone && f.linking {
		f.sync.Phone = Write[string]{Changed: true, Value: privy}
		f.phone = true
		f.move(PhoneVerified, CauseLink)
	}
}

func (f *linkFold) unlinkX(stored, privy *XAccount) {
	if stored != nil && (privy == nil || privy.UserID != stored.UserID) {
		f.sync.X = Write[*XAccount]{Changed: true}
		f.x = false
		f.move(XUnlinked, CauseUnlink)
	}
}

func (f *linkFold) linkX(privy *XAccount) {
	if privy != nil && !f.x && f.linking {
		f.sync.X = Write[*XAccount]{Changed: true, Value: privy}
		f.x = true
		f.move(XLinked, CauseLink)
	}
}

func (f *linkFold) move(kind AuthEventKind, cause AuthCause) {
	if f.err != nil {
		return
	}
	next, err := NextAuthState(f.state, AuthEvent{Kind: kind, HasPhone: f.phone, HasX: f.x})
	if err != nil {
		f.err = err
		return
	}
	if next != f.state {
		f.sync.Steps = append(f.sync.Steps, AuthStep{From: f.state, To: next, Cause: cause})
		f.state = next
	}
}
