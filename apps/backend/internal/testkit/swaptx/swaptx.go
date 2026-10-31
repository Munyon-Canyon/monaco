package swaptx

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const (
	routeDiscriminator  = "e517cb977ae3ad2a"
	sharedDiscriminator = "c1209b3341d69c81"
	budgetUnitLimit     = 2
	budgetUnitPrice     = 3
	honestUnits         = 400_000
	honestMicroLamports = 1_000
)

type Ix struct {
	Program  chain.SolanaAddress
	Accounts []chain.SolanaAddress
	Data     []byte
}

type Spec struct {
	Payer       chain.SolanaAddress
	Wallet      chain.SolanaAddress
	In          chain.Mint
	Out         chain.Mint
	InAmount    uint64
	QuotedOut   uint64
	SlippageBps uint16
	Nonce       uint64
}

type Tx struct {
	Payer  chain.SolanaAddress
	Wallet chain.SolanaAddress
	Nonce  uint64
	Ixs    []Ix
	Table  chain.SolanaAddress
	Tabled []chain.SolanaAddress
}

func Address(label string) chain.SolanaAddress {
	sum := sha256.Sum256([]byte("swaptx:" + label))
	return chain.AddressOf(sum[:ed25519.PublicKeySize])
}

func ATA(owner, mint chain.SolanaAddress) chain.SolanaAddress {
	a, err := chain.AssociatedTokenAccount(owner, mint, chain.SPLProgram)
	if err != nil {
		return Address("ata:" + string(owner) + ":" + string(mint))
	}
	return a
}

func Honest(s Spec) []byte { return HonestTx(s).Bytes() }

func HonestTx(s Spec) Tx {
	return Tx{Payer: s.Payer, Wallet: s.Wallet, Nonce: s.Nonce, Ixs: HonestIxs(s)}
}

func HonestIxs(s Spec) []Ix {
	return []Ix{
		Budget(budgetUnitLimit, binary.LittleEndian.AppendUint32(nil, honestUnits)),
		Budget(budgetUnitPrice, binary.LittleEndian.AppendUint64(nil, honestMicroLamports)),
		{
			Program: chain.ATAProgram, Data: []byte{1},
			Accounts: []chain.SolanaAddress{
				s.Payer, ATA(s.Wallet, s.Out.Address), s.Wallet, s.Out.Address, chain.SystemProgram, chain.SPLProgram,
			},
		},
		SharedRoute(s),
	}
}

func Budget(tag byte, args []byte) Ix {
	return Ix{Program: chain.ComputeBudgetProgram, Data: append([]byte{tag}, args...)}
}

func SharedRoute(s Spec) Ix {
	return Ix{
		Program: chain.JupiterV6Program,
		Accounts: []chain.SolanaAddress{
			chain.SPLProgram, Address("program-authority"), s.Wallet, ATA(s.Wallet, s.In.Address),
			Address("program-source"), Address("program-destination"), ATA(s.Wallet, s.Out.Address),
			s.In.Address, s.Out.Address, chain.JupiterV6Program, chain.SPL2022Program,
			Address("event-authority"), chain.JupiterV6Program,
		},
		Data: RouteData(sharedDiscriminator, s),
	}
}

func Route(s Spec) Ix {
	return Ix{
		Program: chain.JupiterV6Program,
		Accounts: []chain.SolanaAddress{
			chain.SPLProgram, s.Wallet, ATA(s.Wallet, s.In.Address), ATA(s.Wallet, s.Out.Address),
			chain.JupiterV6Program, s.Out.Address, chain.JupiterV6Program, Address("event-authority"),
			chain.JupiterV6Program,
		},
		Data: RouteData(routeDiscriminator, s),
	}
}

func RouteData(discriminator string, s Spec) []byte {
	disc, _ := hex.DecodeString(discriminator)
	data := append(slices.Clone(disc), 0, 0, 0, 0, 0)
	data = binary.LittleEndian.AppendUint64(data, s.InAmount)
	data = binary.LittleEndian.AppendUint64(data, s.QuotedOut)
	data = binary.LittleEndian.AppendUint16(data, s.SlippageBps)
	return append(data, 0)
}

func (t Tx) Tables() chain.LookupTables {
	if t.Table == "" {
		return nil
	}
	return chain.LookupTables{t.Table: t.Tabled}
}

func (t Tx) tabled(a chain.SolanaAddress) int {
	if t.Table == "" || a == t.Payer || a == t.Wallet {
		return -1
	}
	for _, ix := range t.Ixs {
		if ix.Program == a {
			return -1
		}
	}
	return slices.Index(t.Tabled, a)
}

func (t Tx) static() []chain.SolanaAddress {
	static := []chain.SolanaAddress{t.Payer}
	if t.Wallet != t.Payer {
		static = append(static, t.Wallet)
	}
	add := func(a chain.SolanaAddress) {
		if !slices.Contains(static, a) {
			static = append(static, a)
		}
	}
	for _, ix := range t.Ixs {
		add(ix.Program)
	}
	for _, ix := range t.Ixs {
		for _, a := range ix.Accounts {
			if t.tabled(a) < 0 {
				add(a)
			}
		}
	}
	return static
}

func Bps[T ~int | ~int64 | ~uint64](n T) uint16 {
	return uint16(min(max(int64(min(n, math.MaxInt32)), 0), math.MaxUint16))
}

func u8(n int) byte { return byte(min(max(n, 0), math.MaxUint8)) }

func (t Tx) body(static []chain.SolanaAddress) (body []byte, used []int) {
	for _, ix := range t.Ixs {
		body = append(body, u8(slices.Index(static, ix.Program)))
		body = append(body, chain.CompactU16(len(ix.Accounts))...)
		for _, a := range ix.Accounts {
			at := t.tabled(a)
			if at < 0 {
				body = append(body, u8(slices.Index(static, a)))
				continue
			}
			if !slices.Contains(used, at) {
				used = append(used, at)
			}
			body = append(body, u8(len(static)+slices.Index(used, at)))
		}
		body = append(body, chain.CompactU16(len(ix.Data))...)
		body = append(body, ix.Data...)
	}
	return body, used
}

func (t Tx) lookups(used []int) []byte {
	table, _ := t.Table.Bytes()
	out := append([]byte{1}, table...)
	out = append(out, 0)
	out = append(out, chain.CompactU16(len(used))...)
	for _, u := range used {
		out = append(out, u8(u))
	}
	return out
}

func (t Tx) Bytes() []byte {
	static := t.static()
	body, used := t.body(static)
	signers := 2
	if t.Payer == t.Wallet {
		signers = 1
	}
	var msg []byte
	if t.Table != "" {
		msg = append(msg, 0x80)
	}
	msg = append(msg, u8(signers), 0, 0)
	msg = append(msg, chain.CompactU16(len(static))...)
	for _, k := range static {
		b, _ := k.Bytes()
		msg = append(msg, b...)
	}
	blockhash := make([]byte, ed25519.PublicKeySize)
	binary.BigEndian.PutUint64(blockhash[len(blockhash)-8:], t.Nonce)
	msg = append(msg, blockhash...)
	msg = append(msg, chain.CompactU16(len(t.Ixs))...)
	msg = append(msg, body...)
	if t.Table != "" {
		msg = append(msg, t.lookups(used)...)
	}
	out := chain.CompactU16(signers)
	out = append(out, make([]byte, signers*ed25519.SignatureSize)...)
	return append(out, msg...)
}
