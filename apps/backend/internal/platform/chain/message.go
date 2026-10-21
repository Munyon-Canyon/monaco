package chain

import (
	"crypto/ed25519"
	"log/slog"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	versionPrefix  = 0x80
	blockhashSize  = 32
	messageVersion = 0
)

type CompiledInstruction struct {
	Program  int
	Accounts []int
	Data     []byte
}

type AddressLookup struct {
	Table    SolanaAddress
	Writable []int
	Readonly []int
}

type Message struct {
	Versioned        bool
	Signers          int
	ReadonlySigned   int
	ReadonlyUnsigned int
	Keys             []SolanaAddress
	Instructions     []CompiledInstruction
	Lookups          []AddressLookup
}

type Instruction struct {
	Program  SolanaAddress
	Accounts []SolanaAddress
	Data     []byte
}

type LookupTables map[SolanaAddress][]SolanaAddress

func ParseMessage(raw []byte) (Message, error) {
	const op = "chain.ParseMessage"
	r := reader{op: op, raw: raw}
	var m Message
	first := r.byte()
	if first&versionPrefix != 0 {
		if first&^versionPrefix != messageVersion {
			return Message{}, malformed(op, "version")
		}
		m.Versioned = true
		first = r.byte()
	}
	m.Signers, m.ReadonlySigned, m.ReadonlyUnsigned = int(first), int(r.byte()), int(r.byte())
	m.Keys = r.keys()
	r.take(blockhashSize)
	m.Instructions = r.instructions()
	if m.Versioned {
		m.Lookups = r.lookups()
	}
	if r.err != nil {
		return Message{}, r.err
	}
	if len(r.raw) != 0 || m.Signers == 0 || m.Signers > len(m.Keys) || m.ReadonlySigned >= m.Signers {
		return Message{}, malformed(op, "layout")
	}
	return m, nil
}

func (r *reader) keys() []SolanaAddress {
	n := r.count("account keys")
	out := make([]SolanaAddress, 0, n)
	for range n {
		out = append(out, AddressOf(r.take(ed25519.PublicKeySize)))
	}
	return out
}

func (r *reader) indexes(what string) []int {
	n := r.count(what)
	out := make([]int, 0, n)
	for range n {
		out = append(out, int(r.byte()))
	}
	return out
}

func (r *reader) instructions() []CompiledInstruction {
	n := r.count("instructions")
	out := make([]CompiledInstruction, 0, n)
	for range n {
		ix := CompiledInstruction{Program: int(r.byte())}
		ix.Accounts = r.indexes("instruction accounts")
		ix.Data = r.take(r.count("instruction data"))
		out = append(out, ix)
	}
	return out
}

func (r *reader) lookups() []AddressLookup {
	n := r.count("lookups")
	out := make([]AddressLookup, 0, n)
	for range n {
		l := AddressLookup{Table: AddressOf(r.take(ed25519.PublicKeySize))}
		l.Writable = r.indexes("writable lookups")
		l.Readonly = r.indexes("readonly lookups")
		out = append(out, l)
	}
	return out
}

func (m Message) Tables() []SolanaAddress {
	out := make([]SolanaAddress, 0, len(m.Lookups))
	for _, l := range m.Lookups {
		out = append(out, l.Table)
	}
	return out
}

func (m Message) Signing() []SolanaAddress { return m.Keys[:m.Signers] }

func (m Message) loaded(tables LookupTables) (writable, readonly []SolanaAddress, err error) {
	const op = "chain.Message.Resolve"
	for _, l := range m.Lookups {
		entries, ok := tables[l.Table]
		if !ok {
			return nil, nil, malformed(op, "lookup table not loaded", slog.String("table", string(l.Table)))
		}
		for _, group := range []struct {
			indexes []int
			into    *[]SolanaAddress
		}{{l.Writable, &writable}, {l.Readonly, &readonly}} {
			for _, i := range group.indexes {
				if i >= len(entries) {
					return nil, nil, malformed(op, "lookup index", slog.String("table", string(l.Table)))
				}
				*group.into = append(*group.into, entries[i])
			}
		}
	}
	return writable, readonly, nil
}

func (m Message) Resolve(tables LookupTables) ([]Instruction, error) {
	const op = "chain.Message.Resolve"
	writable, readonly, err := m.loaded(tables)
	if err != nil {
		return nil, err
	}
	accounts := slices.Concat(m.Keys, writable, readonly)
	out := make([]Instruction, 0, len(m.Instructions))
	for _, c := range m.Instructions {
		if c.Program >= len(m.Keys) {
			return nil, malformed(op, "program index")
		}
		ix := Instruction{Program: accounts[c.Program], Data: c.Data}
		for _, i := range c.Accounts {
			if i >= len(accounts) {
				return nil, malformed(op, "account index")
			}
			ix.Accounts = append(ix.Accounts, accounts[i])
		}
		out = append(out, ix)
	}
	return out, nil
}

func malformed(op, reason string, attrs ...slog.Attr) error {
	return errs.New(errs.CodeInvalidInput, op, append(attrs, slog.String("reason", reason))...)
}

type reader struct {
	op  string
	raw []byte
	err error
}

func (r *reader) take(n int) []byte {
	if r.err != nil || n < 0 || n > len(r.raw) {
		r.err = malformed(r.op, "truncated")
		return make([]byte, max(n, 0))
	}
	out := r.raw[:n]
	r.raw = r.raw[n:]
	return out
}

func (r *reader) byte() byte { return r.take(1)[0] }

func (r *reader) count(what string) int {
	if r.err != nil {
		return 0
	}
	n, rest, ok := compactU16(r.raw)
	if !ok {
		r.err = malformed(r.op, what)
		return 0
	}
	if r.raw = rest; n > len(rest) {
		r.err = malformed(r.op, "truncated")
		return 0
	}
	return n
}
