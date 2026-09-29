package chain

import (
	"crypto/ed25519"
	"crypto/sha256"
	"log/slog"
	"math/big"
	"slices"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type SolanaAddress string

type Signature string

type Mint struct {
	Address  SolanaAddress
	Decimals uint8
}

type Wallet struct {
	ID           string
	Address      SolanaAddress
	HasAppSigner bool
}

const (
	SystemProgram  SolanaAddress = "11111111111111111111111111111111"
	SPLProgram     SolanaAddress = "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"
	SPL2022Program SolanaAddress = "TokenzQdBNbLqP5VEhdkAS6EPFLC1PHnBqCXEpPxuEb"
	ATAProgram     SolanaAddress = "ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL"
)

func ParseAddress(raw string) (SolanaAddress, error) {
	if b, ok := DecodeBase58(raw); ok && len(b) == ed25519.PublicKeySize {
		return SolanaAddress(raw), nil
	}
	return "", errs.New(errs.CodeInvalidAddress, "chain.ParseAddress", slog.String("address", raw))
}

func AddressOf(key []byte) SolanaAddress { return SolanaAddress(EncodeBase58(key)) }

func (a SolanaAddress) Bytes() ([]byte, error) {
	if _, err := ParseAddress(string(a)); err != nil {
		return nil, err
	}
	b, _ := DecodeBase58(string(a))
	return b, nil
}

func SignatureOf(sig []byte) Signature { return Signature(EncodeBase58(sig)) }

func AssociatedTokenAccount(owner, mint, tokenProgram SolanaAddress) (SolanaAddress, error) {
	seeds := make([][]byte, 0, 3)
	for _, a := range []SolanaAddress{owner, tokenProgram, mint} {
		b, err := a.Bytes()
		if err != nil {
			return "", err
		}
		seeds = append(seeds, b)
	}
	program, _ := ATAProgram.Bytes()
	for bump := byte(255); ; bump-- {
		h := sha256.New()
		for _, s := range seeds {
			h.Write(s)
		}
		h.Write([]byte{bump})
		h.Write(program)
		h.Write([]byte("ProgramDerivedAddress"))
		if key := h.Sum(nil); !onCurve(key) {
			return AddressOf(key), nil
		}
	}
}

func onCurve(key []byte) bool {
	p := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(19))
	le := make([]byte, len(key))
	for i, b := range key {
		le[len(key)-1-i] = b
	}
	le[0] &= 0x7f
	y := new(big.Int).SetBytes(le)
	d := new(big.Int).Mul(big.NewInt(-121665), new(big.Int).ModInverse(big.NewInt(121666), p))
	y2 := new(big.Int).Mul(y, y)
	u := new(big.Int).Sub(y2, big.NewInt(1))
	v := new(big.Int).Add(new(big.Int).Mul(d, y2), big.NewInt(1))
	x2 := new(big.Int).Mul(u, new(big.Int).ModInverse(v.Mod(v, p), p))
	x2.Mod(x2, p)
	if x2.Sign() == 0 {
		return true
	}
	return new(big.Int).Exp(x2, new(big.Int).Rsh(new(big.Int).Sub(p, big.NewInt(1)), 1), p).Cmp(big.NewInt(1)) == 0
}

const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

func EncodeBase58(b []byte) string {
	n := new(big.Int).SetBytes(b)
	base, mod := big.NewInt(58), new(big.Int)
	var rev []byte
	for n.Sign() > 0 {
		n.DivMod(n, base, mod)
		rev = append(rev, alphabet[mod.Int64()])
	}
	for i := 0; i < len(b) && b[i] == 0; i++ {
		rev = append(rev, '1')
	}
	slices.Reverse(rev)
	return string(rev)
}

func DecodeBase58(s string) ([]byte, bool) {
	n, base := new(big.Int), big.NewInt(58)
	zeros := 0
	for zeros < len(s) && s[zeros] == '1' {
		zeros++
	}
	for _, c := range s {
		i := strings.IndexRune(alphabet, c)
		if i < 0 {
			return nil, false
		}
		n.Mul(n, base).Add(n, big.NewInt(int64(i)))
	}
	return append(make([]byte, zeros), n.Bytes()...), true
}
